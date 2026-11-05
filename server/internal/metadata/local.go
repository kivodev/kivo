package metadata

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/mmcdole/kivo/internal/catalog"
)

// Local artwork names, as media servers conventionally name them.
var (
	localArtworkNames = map[string][]string{
		artworkPoster:   {"poster", "folder", "cover"},
		artworkBackdrop: {"fanart", "backdrop", "background"},
		artworkThumb:    {"landscape", "thumb"},
		artworkLogo:     {"logo", "clearlogo"},
		artworkBanner:   {"banner"},
	}
	localArtworkExts = []string{".jpg", ".jpeg", ".png", ".webp"}
)

// local is what the files next to a work's media say about it.
type local struct {
	nfo     *nfo                   // the most recently changed NFO, or nil
	artwork map[string]localFile   // the artwork to use, by kind
	choices map[string][]localFile // every artwork file by kind, newest first, for "Choose artwork"
	cover   *embeddedCover         // a book's embedded cover, used as its poster when no image file is found
}

// localFile is a file next to the media, inside a library folder.
type localFile struct {
	folder, rel string // the library folder, and the slash-separated path under it
	mtime       time.Time
}

func (f localFile) origin() string { return filepath.Join(f.folder, filepath.FromSlash(f.rel)) }

// read reads the file through os.Root, so no path can escape the library
// folder. It refuses a file over the artwork size cap. Its errors name the
// file, and a refresh's stored error keeps that text.
func (f localFile) read() ([]byte, error) {
	r, err := os.OpenRoot(f.folder)
	if err != nil {
		return nil, err
	}
	defer r.Close()
	file, err := r.Open(filepath.FromSlash(f.rel))
	if err != nil {
		return nil, err
	}
	defer file.Close()
	b, err := io.ReadAll(io.LimitReader(file, maxImageBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", f.origin(), err)
	}
	if len(b) > maxImageBytes {
		return nil, fmt.Errorf("%w: %s is over %d bytes", ErrBadImage, f.rel, maxImageBytes)
	}
	return b, nil
}

// fromLocal finds a work's NFO and artwork next to its files, in a library
// that uses local metadata. Several files may describe one work, such as an
// NFO beside each version, or the work's folder appearing under more than
// one of the library's folders. The most recently changed file wins.
func (s *Service) fromLocal(ctx context.Context, w catalog.Work) (local, error) {
	out := local{artwork: map[string]localFile{}, choices: map[string][]localFile{}}
	rows, err := s.q.LocalFiles(ctx, w.ID)
	if err != nil {
		return out, err
	}
	byFolder := map[string][]string{}
	for _, r := range rows {
		byFolder[r.FolderPath] = append(byFolder[r.FolderPath], r.RelPath)
	}
	for folder, files := range byFolder {
		if err := s.scanLocal(w, folder, files, &out); err != nil {
			// Reporting an unreachable folder is scan's job. Its files just
			// don't count this time.
			slog.WarnContext(ctx, "metadata: local metadata skipped", "folder", folder, "err", err)
		}
	}
	for _, files := range out.choices {
		slices.SortStableFunc(files, func(a, b localFile) int { return b.mtime.Compare(a.mtime) })
	}
	return out, nil
}

func (s *Service) scanLocal(w catalog.Work, folderPath string, files []string, out *local) error {
	r, err := os.OpenRoot(folderPath)
	if err != nil {
		return err
	}
	defer r.Close()
	d := dirs{r: r, folder: folderPath, list: map[string]map[string]string{}}

	switch w.Kind {
	case catalog.KindMovie, catalog.KindVideo:
		for _, f := range files {
			dir, base := splitMedia(f)
			own := dir != "." // folder-wide names only count in the work's own folder
			d.nfo(out, dir, base+".nfo")
			if own {
				d.nfo(out, dir, "movie.nfo")
			}
			for kind, names := range localArtworkNames {
				for _, n := range names {
					d.artwork(out, kind, dir, base+"-"+n)
					if own {
						d.artwork(out, kind, dir, n)
					}
				}
			}
		}
	case catalog.KindSeries, catalog.KindSeason:
		for _, folder := range seriesFolders(files) {
			if w.Kind == catalog.KindSeason {
				d.artwork(out, artworkPoster, folder, fmt.Sprintf("season%02d-poster", w.Number))
				if w.Number == 0 {
					d.artwork(out, artworkPoster, folder, "season-specials-poster")
				}
				continue
			}
			d.nfo(out, folder, "tvshow.nfo")
			for kind, names := range localArtworkNames {
				for _, n := range names {
					d.artwork(out, kind, folder, n)
				}
			}
		}
	case catalog.KindEpisode:
		for _, f := range files {
			dir, base := splitMedia(f)
			d.nfo(out, dir, base+".nfo")
			d.artwork(out, artworkStill, dir, base+"-thumb")
		}
	case catalog.KindBook:
		d.bookCovers(out, files)
	}
	return nil
}

// splitMedia splits a media file's path into its folder and its name
// without the extension.
func splitMedia(rel string) (dir, base string) {
	name := path.Base(rel)
	return path.Dir(rel), strings.TrimSuffix(name, path.Ext(name))
}

// seriesFolders returns the top-level folders under the library folder
// that hold a series' files. A series is its top-level folder.
func seriesFolders(files []string) []string {
	var out []string
	for _, f := range files {
		top, _, nested := strings.Cut(f, "/")
		if nested && !slices.Contains(out, top) {
			out = append(out, top)
		}
	}
	return out
}

// dirs looks up files case-insensitively, listing each folder only once.
type dirs struct {
	r      *os.Root
	folder string
	list   map[string]map[string]string // folder → lowercase name → actual name
}

// find returns the file named name in dir, ignoring case.
func (d *dirs) find(dir, name string) (localFile, bool) {
	names, ok := d.list[dir]
	if !ok {
		names = map[string]string{}
		if f, err := d.r.Open(filepath.FromSlash(dir)); err == nil {
			entries, _ := f.ReadDir(-1)
			f.Close()
			for _, e := range entries {
				if e.Type().IsRegular() {
					names[strings.ToLower(e.Name())] = e.Name()
				}
			}
		}
		d.list[dir] = names
	}
	actual, ok := names[strings.ToLower(name)]
	if !ok {
		return localFile{}, false
	}
	rel := path.Join(dir, actual)
	info, err := d.r.Stat(filepath.FromSlash(rel))
	if err != nil || !info.Mode().IsRegular() {
		return localFile{}, false
	}
	return localFile{folder: d.folder, rel: rel, mtime: info.ModTime()}, true
}

// nfo reads the NFO at dir/name, and keeps it if it is the newest so far.
func (d *dirs) nfo(out *local, dir, name string) {
	f, ok := d.find(dir, name)
	if !ok || (out.nfo != nil && !f.mtime.After(out.nfo.mtime)) {
		return
	}
	file, err := d.r.Open(filepath.FromSlash(f.rel))
	if err != nil {
		return
	}
	defer file.Close()
	n, err := parseNFO(file)
	if err != nil {
		slog.Warn("metadata: unreadable NFO", "file", f.origin(), "err", err)
		return
	}
	n.mtime = f.mtime
	out.nfo = n
}

// artwork looks for dir/name with each artwork extension. It adds each file
// found to the kind's choices, and makes it the kind's artwork if it is the
// newest so far. Newest wins in every conflict, whether between two names in
// one folder or between two library folders.
func (d *dirs) artwork(out *local, kind, dir, name string) {
	for _, ext := range localArtworkExts {
		f, ok := d.find(dir, name+ext)
		if !ok {
			continue
		}
		if !slices.ContainsFunc(out.choices[kind], func(c localFile) bool { return c.origin() == f.origin() }) {
			out.choices[kind] = append(out.choices[kind], f)
		}
		if cur, have := out.artwork[kind]; !have || f.mtime.After(cur.mtime) {
			out.artwork[kind] = f
		}
	}
}
