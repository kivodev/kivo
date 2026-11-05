package scan

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"strings"
	"sync"
	"unicode"

	"github.com/mmcdole/kivo/internal/store"

	"github.com/mmcdole/kivo/internal/catalog"
	"github.com/mmcdole/kivo/internal/id"
	"github.com/mmcdole/kivo/internal/media"
	"github.com/mmcdole/kivo/internal/media/container"
	"github.com/mmcdole/kivo/internal/scan/dbq"
	"github.com/mmcdole/kivo/internal/tasks"
	"golang.org/x/sync/errgroup"
)

// probeAtOnce is how many files a library's scan probes at once.
const probeAtOnce = 4

// probeAll probes a library's present files that aren't probed at their
// generation, probeAtOnce at a time. A file that fails for a reason that
// may pass is left for the next scan. Only an error that ends the scan is
// returned.
func (s *Scanner) probeAll(ctx context.Context, lib catalog.Library) error {
	files, err := s.q.Unprobed(ctx, lib.ID)
	if err != nil {
		return err
	}
	var mu sync.Mutex
	done := 0
	tasks.Progress(ctx, StepProbe, 0, len(files))
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(probeAtOnce)
	for _, f := range files {
		g.Go(func() error {
			err := s.probeFile(gctx, lib, f)
			if gctx.Err() != nil {
				return gctx.Err()
			}
			if err != nil {
				slog.WarnContext(gctx, "scan: file left for the next scan", "file", f, "err", err)
			}
			mu.Lock()
			done++
			tasks.Progress(ctx, StepProbe, done, len(files))
			mu.Unlock()
			return nil
		})
	}
	return g.Wait()
}

// probeFile probes a file, detects a move, and stores the result if the
// file is still at the generation probed. In the same transaction it asks
// previews and subtitles for the work that follows. A file the media
// package can't read is recorded as failed, so it isn't probed again until
// it changes or is retried.
func (s *Scanner) probeFile(ctx context.Context, lib catalog.Library, file id.ID) error {
	f, err := s.deps.Catalog.File(ctx, file)
	if errors.Is(err, catalog.ErrNotFound) {
		return nil // the file was merged into a gone file by a move, or deleted
	}
	if err != nil {
		return err
	}
	if !f.Present {
		return nil
	}
	fsys, closeFolder, err := s.opt.OpenFolder(f.FolderPath)
	if err != nil {
		return fmt.Errorf("%w: %s", catalog.ErrUnreachable, f.FolderPath)
	}
	defer closeFolder()
	edge, osHash, err := hashes(fsys, f.RelPath, f.Size)
	if err != nil {
		return err
	}
	res, err := s.deps.Prober.Probe(ctx, f.Path())
	if errors.Is(err, media.ErrSourceCorrupt) || errors.Is(err, media.ErrUnsupported) {
		slog.WarnContext(ctx, "scan: can't probe it", "file", f.RelPath, "err", err)
		return s.db.Write(ctx, func(ctx context.Context, tx *store.Tx) error {
			return dbq.New(tx).SetProbeFailed(ctx, dbq.SetProbeFailedParams{ID: f.ID, Generation: f.Generation, Reason: sql.NullString{String: err.Error(), Valid: true}})
		})
	}
	if err != nil {
		return err
	}
	attached, err := attachments(fsys, f.RelPath, f.Size)
	if errors.Is(err, media.ErrSourceCorrupt) {
		// Bad attachments are left out. The file is still playable.
		slog.WarnContext(ctx, "scan: attachments left out", "file", f.RelPath, "err", err)
	} else if err != nil {
		return err
	}
	moved, err := s.moveSource(ctx, f, edge)
	if err != nil {
		return err
	}

	summary := catalog.ProbeSummary{Container: res.Container, Duration: res.Duration, Start: res.Start, Bitrate: res.Bitrate, EdgeHash: edge, OSHash: osHash, Tags: res.Tags}
	tracks, chapters := convertTracks(res)
	return s.db.Write(ctx, func(ctx context.Context, tx *store.Tx) error {
		return s.storeProbe(ctx, tx, lib, f, moved, catalog.Probe{Summary: summary, Tracks: tracks, Chapters: chapters, Attachments: attached})
	})
}

// storeProbe stores file f's probe and records it as probed, unless the
// file changed while it was probed. When f was moved from a gone file, the
// probe is stored on the gone file instead.
func (s *Scanner) storeProbe(ctx context.Context, tx *store.Tx, lib catalog.Library, f, moved catalog.File, p catalog.Probe) error {
	target, gen := f.ID, f.Generation
	if !moved.ID.IsZero() {
		// This is a move. The new row merges into the old one before
		// identify runs, which keeps the old ID with everything attached to
		// it. The file is then identified again where it now is.
		if err := s.deps.Catalog.Relink(ctx, tx, moved.ID, f.ID); err != nil {
			return err
		}
		if err := s.deps.Identifier.Reidentify(ctx, tx, []id.ID{moved.ID}); err != nil {
			return err
		}
		target, gen = moved.ID, moved.Generation
	}
	err := s.deps.Catalog.SetProbe(ctx, tx, target, gen, p)
	if errors.Is(err, catalog.ErrStale) {
		return nil // the file changed while it was probed, so the next scan probes it as it is now
	}
	if err != nil {
		return err
	}
	if err := dbq.New(tx).SetProbed(ctx, dbq.SetProbedParams{ID: target, Generation: gen}); err != nil {
		return err
	}
	s.deps.Previews.Probed(tx, lib, target)
	s.deps.Subtitles.Probed(tx, lib, target, p.Tracks)
	return nil
}

// edgeSize is how much of each end of a file the hashes read.
const edgeSize = 64 << 10

// hashes reads the first and last 64 KiB of a file once for both hashes.
// The edge hash is for move detection. The OpenSubtitles hash is for
// subtitle search: the size plus the 64-bit little-endian words of both
// ends.
func hashes(fsys fs.FS, rel string, size int64) (edge []byte, osHash uint64, err error) {
	f, err := fsys.Open(rel)
	if err != nil {
		return nil, 0, err
	}
	defer f.Close()
	ra, ok := f.(io.ReaderAt)
	if !ok {
		return nil, 0, errors.New("file can't be read at an offset")
	}
	n := min(int64(edgeSize), size)
	head := make([]byte, n)
	tail := make([]byte, n)
	if _, err := ra.ReadAt(head, 0); err != nil && !errors.Is(err, io.EOF) {
		return nil, 0, err
	}
	if _, err := ra.ReadAt(tail, size-n); err != nil && !errors.Is(err, io.EOF) {
		return nil, 0, err
	}
	h := sha256.New()
	h.Write(head)
	h.Write(tail)
	osHash = uint64(size)
	for _, b := range [][]byte{head, tail} {
		for i := 0; i+8 <= len(b); i += 8 {
			osHash += binary.LittleEndian.Uint64(b[i:])
		}
	}
	return h.Sum(nil)[:16], osHash, nil
}

// attachments reads the files a container carries besides its tracks,
// which ffprobe lists as streams without their contents.
func attachments(fsys fs.FS, rel string, size int64) ([]catalog.Attachment, error) {
	f, err := fsys.Open(rel)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	ra, ok := f.(io.ReaderAt)
	if !ok {
		return nil, errors.New("file can't be read at an offset")
	}
	found, err := container.Attachments(ra, size) // on ErrSourceCorrupt, found still holds what was read
	out := make([]catalog.Attachment, len(found))
	for i, a := range found {
		out[i] = catalog.Attachment{Kind: catalog.AttachmentKind(a.Kind), Filename: a.Filename, MimeType: a.MimeType, Size: a.Size, SHA256: a.SHA256}
	}
	return out, err
}

func convertTracks(res media.ProbeResult) ([]catalog.Track, []catalog.Chapter) {
	tracks := make([]catalog.Track, 0, len(res.Tracks))
	for _, t := range res.Tracks {
		if t.Kind != media.Video && t.Kind != media.Audio && t.Kind != media.Subtitle {
			continue // skip attachments, which are read from the container, and data streams
		}
		info, _ := json.Marshal(t)
		forced, sdh := t.Forced, t.HearingImpaired
		if t.Kind == media.Subtitle {
			// Releases often name a track "Forced" or "English SDH" without
			// setting the flag. The title says it the way a file name would.
			titled := titleFlags(t.Title)
			forced, sdh = forced || titled.forced, sdh || titled.sdh
		}
		tracks = append(tracks, catalog.Track{
			Index: t.Index, Type: string(t.Kind), Codec: string(t.Codec), Language: t.Language,
			Title: t.Title, Default: t.Default, Forced: forced, HearingImpaired: sdh, Info: info,
		})
	}
	chapters := make([]catalog.Chapter, 0, len(res.Chapters))
	for i, c := range res.Chapters {
		chapters = append(chapters, catalog.Chapter{Index: i, Start: c.Start, End: c.End, Title: c.Title})
	}
	return tracks, chapters
}

type flags struct{ forced, sdh bool }

// titleFlags reads a subtitle title's words for the flags a file can set.
// The words are the ones subtitle file names use too: "Forced" and
// "Foreign" mean forced, and "SDH", "CC" and "Hearing Impaired" mean
// hearing impaired.
func titleFlags(title string) flags {
	words := strings.FieldsFunc(strings.ToLower(title), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsNumber(r) })
	var f flags
	for i, w := range words {
		switch w {
		case "forced", "foreign":
			f.forced = true
		case "sdh", "cc":
			f.sdh = true
		case "hearing":
			if i+1 < len(words) && words[i+1] == "impaired" {
				f.sdh = true
			}
		}
	}
	return f
}
