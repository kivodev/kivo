package subtitles

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/mmcdole/kivo/internal/store"

	"github.com/mmcdole/kivo/internal/atomicfile"
	"github.com/mmcdole/kivo/internal/catalog"
	"github.com/mmcdole/kivo/internal/id"
	"github.com/mmcdole/kivo/internal/users"
)

// SubtitleSearch is what a subtitle search asks for.
type SubtitleSearch struct {
	// Languages are BCP 47 tags. Empty means the user's subtitle language,
	// or else the file's metadata language.
	Languages []string
}

// downloadFormats are the subtitle file formats kept as downloaded tracks.
var downloadFormats = []string{"srt", "ass", "ssa", "vtt"}

// SearchSubtitles searches OpenSubtitles for subtitles for a file the
// viewer can see. Results are ordered best first, as rank orders them,
// with SDH first when the viewer's last subtitles for the file's series
// were SDH.
func (s *Service) SearchSubtitles(ctx context.Context, v users.Viewer, file id.ID, q SubtitleSearch) ([]SubtitleResult, error) {
	u, err := s.deps.Users.User(ctx, v.User())
	if err != nil {
		return nil, err
	}
	f, err := s.visibleFile(ctx, v, file)
	if err != nil {
		return nil, err
	}
	langs := q.Languages
	if len(langs) == 0 && u.Playback.SubtitleLanguage != "" {
		langs = []string{u.Playback.SubtitleLanguage}
	}
	results, err := s.search(ctx, f, langs)
	if err != nil {
		return nil, err
	}
	rank(results, s.prefersSDH(ctx, u, file))
	return results, nil
}

// prefersSDH reports whether the user's last subtitle choice for the series
// a file is an episode of was one for the deaf and hard of hearing. Only a
// user who remembers track choices has one. Anything it can't read is no.
func (s *Service) prefersSDH(ctx context.Context, u users.User, file id.ID) bool {
	if !u.Playback.RememberTracks {
		return false
	}
	ver, err := s.deps.Catalog.VersionOfFile(ctx, file)
	if err != nil || len(ver.Works) == 0 {
		return false
	}
	w, err := s.deps.Catalog.Work(ctx, ver.Works[0])
	if err != nil || w.Kind != catalog.KindEpisode || w.Root.IsZero() {
		return false
	}
	c, ok, err := s.deps.Userstate.TrackChoice(ctx, u.ID, w.Root)
	return err == nil && ok && c.SubtitleKind == "sdh"
}

// visibleFile returns a file the viewer may see, which is a part of a
// version of a work the viewer sees. Any other file returns
// catalog.ErrNotFound.
func (s *Service) visibleFile(ctx context.Context, v users.Viewer, file id.ID) (catalog.File, error) {
	f, err := s.deps.Catalog.File(ctx, file)
	if err != nil {
		return catalog.File{}, err
	}
	ver, err := s.deps.Catalog.VersionOfFile(ctx, file)
	if err != nil {
		return catalog.File{}, err
	}
	for _, w := range ver.Works {
		if s.deps.Catalog.Visible(ctx, v, w) == nil {
			return f, nil
		}
	}
	return catalog.File{}, fmt.Errorf("%w: file %s", catalog.ErrNotFound, file)
}

// rank orders results best first. Results made for this file come first.
// Then full subtitles come before forced ones. Then subtitles that aren't
// for the deaf and hard of hearing come before SDH ones, or the other way
// round with preferSDH. Last, the most downloaded come first.
func rank(results []SubtitleResult, preferSDH bool) {
	sdh := func(r SubtitleResult) int {
		if r.HearingImpaired == preferSDH {
			return 0
		}
		return 1
	}
	slices.SortStableFunc(results, func(a, b SubtitleResult) int {
		return cmp.Or(-cmp.Compare(boolRank(a.HashMatch), boolRank(b.HashMatch)),
			cmp.Compare(boolRank(a.Forced), boolRank(b.Forced)), cmp.Compare(sdh(a), sdh(b)),
			-cmp.Compare(a.Downloads, b.Downloads))
	})
}

func boolRank(b bool) int {
	if b {
		return 1
	}
	return 0
}

// DownloadedSubtitle is a subtitle downloaded for a file, and the
// downloaded subtitles it replaced.
type DownloadedSubtitle struct {
	Track    catalog.SubtitleFile
	Replaced []id.ID
}

// DownloadSubtitle downloads a subtitle result from SearchSubtitles into
// data/subtitles/. It attaches the subtitle to a file the viewer sees, as a
// downloaded subtitle titled with the result's release. From then on,
// every user who sees the file has it. It replaces the file's existing
// downloaded subtitle with the same language and forced flag, if there is
// one.
func (s *Service) DownloadSubtitle(ctx context.Context, v users.Viewer, file id.ID, result string) (DownloadedSubtitle, error) {
	if _, err := s.visibleFile(ctx, v, file); err != nil {
		return DownloadedSubtitle{}, err
	}
	got, err := s.fetch(ctx, result)
	if err != nil {
		return DownloadedSubtitle{}, err
	}
	r := got.result
	t := catalog.SubtitleFile{
		RelPath: path.Join(file.String(), got.name), Format: strings.TrimPrefix(path.Ext(got.name), "."),
		Language: r.Language, Title: r.Release, Forced: r.Forced, HearingImpaired: r.HearingImpaired,
	}
	d, err := s.storeSubtitle(ctx, file, t, got.data)
	if err != nil {
		return DownloadedSubtitle{}, err
	}
	slog.InfoContext(ctx, "subtitles: subtitle downloaded", "file", file, "language", r.Language, "release", r.Release, "replaced", len(d.Replaced))
	return d, nil
}

// storeSubtitle writes a subtitle's data at t.RelPath under
// data/subtitles/. It attaches the subtitle to the file as a downloaded
// subtitle, or updates the one already at that path. It removes the
// downloaded subtitles it replaces, along with their files.
func (s *Service) storeSubtitle(ctx context.Context, file id.ID, t catalog.SubtitleFile, data []byte) (DownloadedSubtitle, error) {
	if err := atomicfile.Write(s.DownloadedPath(t), data); err != nil {
		return DownloadedSubtitle{}, err
	}
	var replaced []catalog.SubtitleFile
	err := s.db.Write(ctx, func(ctx context.Context, tx *store.Tx) error {
		var err error
		if t.ID, err = s.deps.Catalog.AddDownloadedSubtitle(ctx, tx, file, t); err != nil {
			return err
		}
		if replaced, err = s.deps.Catalog.DropReplacedSubtitles(ctx, tx, file, t.ID); err != nil || len(replaced) == 0 {
			return err
		}
		ids := make([]id.ID, len(replaced))
		for i, x := range replaced {
			ids[i] = x.ID
		}
		return s.replaced(ctx, tx, ids, t.ID)
	})
	if err != nil {
		return DownloadedSubtitle{}, err
	}
	t.Downloaded = true
	d := DownloadedSubtitle{Track: t}
	for _, x := range replaced {
		d.Replaced = append(d.Replaced, x.ID)
		if err := os.Remove(s.DownloadedPath(x)); err != nil && !errors.Is(err, fs.ErrNotExist) {
			slog.WarnContext(ctx, "subtitles: replaced subtitle's file", "file", file, "path", x.RelPath, "err", err)
		}
	}
	return d, nil
}

// DownloadedPath returns where a downloaded subtitle's file is kept, under
// data/subtitles/.
func (s *Service) DownloadedPath(t catalog.SubtitleFile) string {
	return filepath.Join(s.deps.SubtitleDir, filepath.FromSlash(t.RelPath))
}
