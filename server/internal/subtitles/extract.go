package subtitles

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/mmcdole/kivo/internal/cachedir"
	"github.com/mmcdole/kivo/internal/catalog"
	"github.com/mmcdole/kivo/internal/fault"
	"github.com/mmcdole/kivo/internal/id"
	"github.com/mmcdole/kivo/internal/media"
	"github.com/mmcdole/kivo/internal/media/container"
	"github.com/mmcdole/kivo/internal/media/subtitle"
	"github.com/mmcdole/kivo/internal/users"
)

const (
	// fontStore is the folder in cache/ that keeps one copy of every font,
	// as cache/fonts/<xx>/<SHA-256><ext>, where xx is the hash's first two
	// hex digits.
	fontStore = "fonts"
	// fontsDir is the folder, beside a file's extracted tracks, that its
	// attached fonts are copied to before they move to the font store.
	fontsDir = "fonts"
	// fontsList is the file in a subtitle result that lists the fonts the
	// result uses, by their names in the store.
	fontsList = "fonts.json"
)

// trackFormats are the formats a track is served in. Every text track is
// served as WebVTT. ASS is also served as ASS, and PGS as PGS.
var trackFormats = []media.SubtitleFormat{media.VTT, media.ASS, media.PGS}

// extractAs is the format ffmpeg extracts an embedded subtitle track to,
// chosen by its codec. Text is extracted as SubRip and then converted to
// WebVTT here. ASS and PGS are extracted unchanged. Image formats other
// than PGS are drawn from the source and never extracted.
func extractAs(codec string) (media.SubtitleFormat, bool) {
	f, ok := media.Codec(codec).SubtitleFormat()
	switch {
	case !ok:
		return "", false
	case f == media.VTT:
		return media.SRT, true
	}
	return f, true
}

// Subtitle returns the path of an embedded subtitle track extracted in
// format (VTT, ASS or PGS), if it's extracted. If a file was extracted
// without the track, because the track couldn't be converted, the track is
// never available.
func (s *Service) Subtitle(ctx context.Context, file id.ID, track int, format media.SubtitleFormat) (string, bool, error) {
	if !slices.Contains(trackFormats, format) {
		return "", false, fmt.Errorf("unsupported subtitle format %q", format)
	}
	cur, err := s.current(ctx, file)
	if err != nil {
		return "", false, err
	}
	p := filepath.Join(s.result(file, cur), strconv.Itoa(track)+"."+format.Ext())
	if _, err := os.Stat(p); err != nil {
		return "", false, nil
	}
	return p, true, nil
}

// Styled reports whether a file's ASS tracks are prepared for styled
// drawing, and returns the paths of the fonts they use. They're prepared
// when the file's subtitles are extracted at its current generation and
// every font the result lists is in the store. Each path's last element is
// the font's SHA-256 and extension, which is the same for every file that
// has the font. Calling Styled never extracts a file.
func (s *Service) Styled(ctx context.Context, file id.ID) ([]string, bool, error) {
	cur, err := s.current(ctx, file)
	if err != nil {
		return nil, false, err
	}
	dir := s.result(file, cur)
	if _, err := os.Stat(dir); err != nil {
		return nil, false, nil
	}
	names, err := readFontsList(filepath.Join(dir, fontsList))
	if errors.Is(err, os.ErrNotExist) {
		return nil, true, nil // the file needs no fonts
	}
	if err != nil {
		return nil, false, err
	}
	if !s.allFontsStored(names) {
		return nil, false, nil
	}
	paths := make([]string, len(names))
	for i, n := range names {
		paths[i] = s.fontPath(n)
	}
	return paths, true, nil
}

// Font is a stored font to serve: its bytes, and the content type detected
// from them.
type Font struct {
	Data []byte
	Type string
}

// Font returns the stored font with this SHA-256. The viewer must be able
// to see a library holding a file that carries the font as an attachment.
// found is false when the font isn't stored, when the viewer may not see
// it, or when its bytes aren't a font. The caller can't tell these cases
// apart.
func (s *Service) Font(ctx context.Context, v users.Viewer, hash string) (f Font, found bool, err error) {
	if !validHash(hash) {
		return Font{}, false, nil
	}
	libs, err := s.deps.Catalog.AttachmentLibraries(ctx, hash)
	if err != nil {
		return Font{}, false, err
	}
	path, ok := s.fontFile(hash)
	if !slices.ContainsFunc(libs, v.CanSee) || !ok {
		return Font{}, false, nil
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return Font{}, false, err
	}
	f = Font{Data: b, Type: container.FontType(b)}
	return f, f.Type != "", nil
}

// fontFile returns where the store keeps the font with this SHA-256, if
// the store has it.
func (s *Service) fontFile(hash string) (string, bool) {
	if !validHash(hash) {
		return "", false
	}
	found, err := filepath.Glob(filepath.Join(s.deps.CacheDir, fontStore, hash[:2], hash+".*"))
	if err != nil || len(found) == 0 {
		return "", false
	}
	return found[0], true
}

// fontPath is where the store keeps a font, given its name in the store.
func (s *Service) fontPath(name string) string {
	return filepath.Join(s.deps.CacheDir, fontStore, name[:2], name)
}

// allFontsStored reports whether the store has every font a result lists.
// It won't if cache/fonts/ was deleted.
func (s *Service) allFontsStored(names []string) bool {
	for _, n := range names {
		if _, err := os.Stat(s.fontPath(n)); err != nil {
			return false
		}
	}
	return true
}

// readFontsList reads a subtitle result's list of fonts. A name the store
// wouldn't give is an error, so a name can never point outside the store.
func readFontsList(path string) ([]string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var names []string
	if err := json.Unmarshal(b, &names); err != nil {
		return nil, err
	}
	for _, n := range names {
		if !fontName(n) {
			return nil, fmt.Errorf("%s: font %q", path, n)
		}
	}
	return names, nil
}

// fontName reports whether name has the form the font store gives: a
// SHA-256 in hex and an extension.
func fontName(name string) bool {
	hash, ext, _ := strings.Cut(name, ".")
	return validHash(hash) && ext != "" && !strings.ContainsAny(ext, `./\`)
}

// validHash reports whether hash is a SHA-256 in hex.
func validHash(hash string) bool {
	_, err := hex.DecodeString(hash)
	return len(hash) == 2*sha256.Size && err == nil
}

// ErrUnextractable means a file's subtitles can't be extracted at its
// generation, because the media package can't read the file or doesn't
// support its tracks. That stays true until the file changes or its
// failure is retried.
var ErrUnextractable = fault.New(fault.Internal, "subtitles can't be extracted")

// Prepare makes sure a file's embedded subtitles are extracted at its
// current generation, regardless of its library's schedule. Only one
// caller extracts a given file at a time. Another caller waits, then finds
// the outcome. An already prepared file returns at once. A file that
// failed for good at its generation returns ErrUnextractable at once, with
// the reason. A file that was deleted or is missing from disk has nothing
// to prepare.
func (s *Service) Prepare(ctx context.Context, file id.ID) error {
	unlock, err := s.files.Lock(ctx, file)
	if err != nil {
		return err
	}
	defer unlock()
	return s.extract(ctx, file)
}

// do is the task's Do. It calls Prepare, and treats a permanent failure as
// done, because it's recorded for the task's Failed to list.
func (s *Service) do(ctx context.Context, file id.ID) error {
	if err := s.Prepare(ctx, file); !errors.Is(err, ErrUnextractable) {
		return err
	}
	return nil
}

// extract prepares a file's embedded subtitle tracks, extracting them all
// in one read of the file. Text tracks become WebVTT, using the same
// conversion as subtitle files. ASS tracks are also kept as ASS, along
// with the fonts the probe recorded among the file's attachments, for
// clients that draw ASS. PGS tracks are kept as PGS.
//
// A file that can't be read is recorded as failed, so it isn't read again
// until it changes or is retried. A file already extracted, with every
// font it lists in the store, isn't read again. A file whose fonts are
// missing from the store is extracted again.
func (s *Service) extract(ctx context.Context, file id.ID) error {
	f, err := s.deps.Catalog.File(ctx, file)
	switch {
	case errors.Is(err, catalog.ErrNotFound):
		return nil // the file was deleted, or a move merged it into another file
	case err != nil:
		return err
	case !f.Present:
		return nil
	}
	settled, err := s.q.Settled(ctx, f.ID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	switch settled.State {
	case "failed":
		return fmt.Errorf("%w: %s", ErrUnextractable, settled.Reason.String)
	case "done":
		if ok, err := s.complete(f.ID, f.Generation); ok || err != nil {
			return err
		}
	}
	req := media.SubtitlesRequest{Path: f.Path()}
	for _, t := range f.Tracks {
		if format, ok := extractAs(t.Codec); ok && media.TrackKind(t.Type) == media.Subtitle {
			req.Tracks = append(req.Tracks, media.SubtitleOut{Track: t.Index, Format: format})
		}
	}
	if len(req.Tracks) == 0 {
		return s.settle(ctx, f, nil) // nothing embedded to extract
	}
	ok, err := s.complete(f.ID, f.Generation)
	switch {
	case err != nil:
		return err
	case ok:
		return s.settle(ctx, f, nil)
	case s.has(f.ID, f.Generation):
		// The store lost some of its fonts, so extract the file again.
		slog.DebugContext(ctx, "subtitles: subtitle fonts missing, extracting again", "file", f.ID)
		if err := os.RemoveAll(s.result(f.ID, f.Generation)); err != nil {
			return s.room.Err(err)
		}
	}
	if err := s.room.Check(); err != nil {
		return err
	}
	tmp, err := cachedir.MakeTemp(s.resultDir())
	if err != nil {
		return s.room.Err(err)
	}
	defer os.RemoveAll(tmp)
	for i, t := range req.Tracks {
		req.Tracks[i].Path = filepath.Join(tmp, strconv.Itoa(t.Track)+"."+t.Format.Ext())
	}
	start := time.Now()
	mctx, cancel := context.WithTimeout(ctx, extractTimeout)
	defer cancel()
	err = s.deps.Media.Subtitles(mctx, req)
	if errors.Is(err, media.ErrSourceCorrupt) || errors.Is(err, media.ErrUnsupported) {
		if serr := s.settle(ctx, f, err); serr != nil {
			return serr
		}
		return fmt.Errorf("%w: %w", ErrUnextractable, err)
	}
	if err != nil {
		return s.room.Err(err)
	}
	if err := toWebVTT(req); err != nil {
		return s.room.Err(err)
	}
	var fonts map[string]string
	if slices.ContainsFunc(req.Tracks, func(t media.SubtitleOut) bool { return t.Format == media.ASS }) {
		if fonts, err = copyFonts(f, filepath.Join(tmp, fontsDir)); err != nil {
			return s.room.Err(err)
		}
	}
	if made, err := s.commitFonts(ctx, f, tmp, fonts); !made || err != nil {
		return err
	}
	slog.DebugContext(ctx, "subtitles: subtitles extracted", "file", f.ID, "tracks", len(req.Tracks), "duration", time.Since(start).Round(time.Millisecond))
	return s.settle(ctx, f, nil)
}

// commitFonts moves the fonts that copyFonts copied into tmp to the font
// store, keeping one copy of each. fonts maps each font's store name to
// its copied path. It then writes their list into tmp in their place, and
// commits the result. The store stays locked until the result is in place,
// so a prune never deletes a font the result is about to list. It reports
// whether the result was put in place.
func (s *Service) commitFonts(ctx context.Context, f catalog.File, tmp string, fonts map[string]string) (bool, error) {
	if len(fonts) == 0 {
		return s.commit(ctx, f, tmp)
	}
	s.fonts.Lock()
	defer s.fonts.Unlock()
	for name, src := range fonts {
		dst := s.fontPath(name)
		if _, err := os.Stat(dst); err == nil {
			continue // already stored for another file
		}
		err := os.MkdirAll(filepath.Dir(dst), 0o755)
		if err == nil {
			err = os.Rename(src, dst)
		}
		if err != nil {
			return false, s.room.Err(err)
		}
	}
	err := os.RemoveAll(filepath.Join(tmp, fontsDir))
	if err == nil {
		b, _ := json.Marshal(slices.Sorted(maps.Keys(fonts)))
		err = os.WriteFile(filepath.Join(tmp, fontsList), b, 0o644)
	}
	if err != nil {
		return false, s.room.Err(err)
	}
	return s.commit(ctx, f, tmp)
}

// copyFonts copies a file's font attachments, as the probe recorded them,
// to dir. It returns a map from each font's store name to where it was
// copied. The store name is the recorded SHA-256 plus an extension chosen
// from the font's bytes. A font whose bytes don't match the recorded hash,
// for example because the file changed since the probe, is left out.
func copyFonts(f catalog.File, dir string) (map[string]string, error) {
	if !slices.ContainsFunc(f.Attachments, func(at catalog.Attachment) bool { return at.Kind == catalog.AttachmentFont }) {
		return nil, nil
	}
	src, err := os.Open(f.Path())
	if err != nil {
		return nil, err
	}
	defer src.Close()
	st, err := src.Stat()
	if err != nil {
		return nil, err
	}
	if err := os.Mkdir(dir, 0o755); err != nil {
		return nil, err
	}
	out := map[string]string{}
	for i, at := range f.Attachments {
		if at.Kind != catalog.AttachmentFont || !validHash(at.SHA256) {
			continue
		}
		p := filepath.Join(dir, strconv.Itoa(i))
		w, err := os.Create(p)
		if err != nil {
			return nil, err
		}
		h := sha256.New()
		err = container.CopyAttachment(src, st.Size(), i, io.MultiWriter(w, h))
		head := make([]byte, 4)
		if err == nil {
			if _, err = w.ReadAt(head, 0); errors.Is(err, io.EOF) {
				err = nil // the attachment is shorter than any font's magic number
			}
		}
		if cerr := w.Close(); err == nil {
			err = cerr
		}
		if err != nil {
			return nil, err
		}
		if got := hex.EncodeToString(h.Sum(nil)); got != at.SHA256 {
			slog.Warn("subtitles: font attachment changed since the file was probed", "file", f.ID, "attachment", i)
			continue
		}
		out[at.SHA256+fontExt(head)] = p
	}
	return out, nil
}

// fontExt is the extension a font is stored with, chosen from its first
// bytes.
func fontExt(head []byte) string {
	switch container.FontType(head) {
	case "font/otf":
		return ".otf"
	case "font/collection":
		return ".ttc"
	case "font/woff":
		return ".woff"
	case "font/woff2":
		return ".woff2"
	}
	return ".ttf"
}

// toWebVTT converts each extracted text track to a WebVTT file beside it.
// It keeps ASS files and deletes the SubRip files. A track that fails to
// convert is logged and left out, and the rest are still served.
func toWebVTT(req media.SubtitlesRequest) error {
	for _, t := range req.Tracks {
		if t.Format == media.PGS {
			continue
		}
		b, err := os.ReadFile(t.Path)
		if err != nil {
			return err
		}
		vtt, err := subtitle.WebVTT(b, t.Format)
		if err != nil {
			slog.Debug("subtitles: subtitle track not converted", "path", req.Path, "track", t.Track, "err", err)
		} else if err := os.WriteFile(strings.TrimSuffix(t.Path, string(t.Format))+"vtt", vtt, 0o644); err != nil {
			return err
		}
		if t.Format == media.SRT {
			if err := os.Remove(t.Path); err != nil {
				return err
			}
		}
	}
	return nil
}
