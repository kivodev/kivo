// Package cachedir lays out results in a cache folder: where each file's
// result goes, where results are built before they're moved into place,
// and which stale results are removed.
package cachedir

import (
	"context"
	"errors"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"

	"github.com/mmcdole/kivo/internal/atomicfile"
	"github.com/mmcdole/kivo/internal/health"
	"github.com/mmcdole/kivo/internal/id"
)

// Temp is the folder inside a cache folder where results are built
// before they're renamed into place. So a result that is in place is
// complete. Anything in Temp is unfinished and may be removed.
const Temp = ".tmp"

// Path returns the path in dir where an entry for x is cached:
// dir/xx/elem..., where xx is the last two hex digits of x. Using the last
// digits spreads the entries over 256 folders, because UUIDv7s share their
// first digits.
func Path(dir string, x id.ID, elem ...string) string {
	s := x.String()
	return filepath.Join(append([]string{dir, s[len(s)-2:]}, elem...)...)
}

// Leaf names the folder, inside a file's cache folder, that holds the
// file's result for one generation. The name is the generation, followed by
// "v" and the version when the version is above 0. A result of an earlier
// version counts as not made.
func Leaf(gen int64, version int) string {
	leaf := strconv.FormatInt(gen, 10)
	if version > 0 {
		leaf += "v" + strconv.Itoa(version)
	}
	return leaf
}

// MakeTemp makes a folder inside dir's Temp folder to build a
// result in. Results.Place, or atomicfile.RenameDir, then moves it into
// place.
func MakeTemp(dir string) (string, error) {
	parent := filepath.Join(dir, Temp)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return "", err
	}
	return os.MkdirTemp(parent, "")
}

// Results keeps one result per file in a cache folder, for the file's
// current generation, at Dir/xx/<file>/<Leaf>/ (see Path and Leaf). A
// result is built in a folder from MakeTemp and placed whole, so a
// result that is in place is complete. It's safe for concurrent use once
// its fields are set.
type Results struct {
	// Dir is the cache folder.
	Dir string
	// Version is the current version of the results, as in Leaf.
	Version int
	// Generations reads the current generation of each given file that
	// still has a row.
	Generations func(ctx context.Context, files []id.ID) (map[id.ID]int64, error)
	// Room watches the disk Dir is on. A Place that runs out of space
	// tells it.
	Room *health.Room
}

// Path returns the folder of a file's result at a generation.
func (r Results) Path(file id.ID, gen int64) string {
	return Path(r.Dir, file, file.String(), Leaf(gen, r.Version))
}

// Has reports whether a file's result is made at a generation.
func (r Results) Has(file id.ID, gen int64) bool {
	_, err := os.Stat(r.Path(file, gen))
	return err == nil
}

// Place moves a result built in tmp into place if the file is still at
// gen, the generation it was built from, and reports whether it did.
// Otherwise the result is left for the caller to remove, and the file's
// current generation stays pending. A result already in place counts as
// placed, and tmp is left for the caller to remove.
func (r Results) Place(ctx context.Context, tmp string, file id.ID, gen int64) (bool, error) {
	gens, err := r.Generations(ctx, []id.ID{file})
	if err != nil {
		return false, err
	}
	cur, ok := gens[file]
	if !ok {
		return false, nil // the file was deleted, or a move merged it into another file
	}
	if cur != gen {
		// Dir's last element names the feature, as in "previews".
		slog.DebugContext(ctx, filepath.Base(r.Dir)+": stale result dropped", "file", file)
		return false, nil
	}
	return true, r.Room.Err(atomicfile.RenameDir(tmp, r.Path(file, gen)))
}

// pruneStep is how many files Prune looks at between progress reports
// and checks of ctx.
const pruneStep = 500

// Prune removes the results that no file needs. Those are all results of a
// file with no row, and a file's results of other generations and
// versions. It returns how many results it removed and how many files
// still have a result. A file's generation is read again before each
// removal, so a result placed meanwhile is kept.
//
// progress, if set, is told how many of the files with results have been
// looked at, with an empty step. It has the same shape as tasks.Progress,
// so a prune task passes that.
func (r Results) Prune(ctx context.Context, progress func(ctx context.Context, step string, done, total int)) (removed, kept int, err error) {
	all, err := cachedFiles(r.Dir)
	if err != nil {
		return 0, 0, err
	}
	ids := make([]id.ID, len(all))
	for i, c := range all {
		ids[i] = c.file
	}
	gens, err := r.Generations(ctx, ids)
	if err != nil {
		return 0, 0, err
	}
	for i, c := range all {
		if i%pruneStep == 0 {
			if progress != nil {
				progress(ctx, "", i, len(all))
			}
			if err := ctx.Err(); err != nil {
				return removed, 0, err
			}
		}
		gen, ok := gens[c.file]
		if !ok {
			gen = -1
		}
		n, err := c.prune(gen, r.Version, func() (int64, error) { return r.latestGen(ctx, c.file) })
		removed += n
		if err != nil {
			return removed, 0, err
		}
	}
	return removed, len(gens), nil
}

// latestGen reads a file's current generation, or -1 once its row is gone.
func (r Results) latestGen(ctx context.Context, file id.ID) (int64, error) {
	g, err := r.Generations(ctx, []id.ID{file})
	if gen, ok := g[file]; ok {
		return gen, err
	}
	return -1, err
}

// cachedFile is the folder dir/xx/<file> that holds one file's results for
// every generation.
type cachedFile struct {
	dir  string
	file id.ID
}

// cachedFiles lists the files with results in dir, where Path placed
// them. A folder whose name isn't a file ID isn't one of ours. It gets the
// zero ID, so it's removed.
func cachedFiles(dir string) ([]cachedFile, error) {
	buckets, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []cachedFile
	for _, b := range buckets {
		if b.Name() == Temp || !b.IsDir() {
			continue
		}
		files, err := os.ReadDir(filepath.Join(dir, b.Name()))
		if err != nil {
			return nil, err
		}
		for _, f := range files {
			file, _ := id.Parse(f.Name())
			out = append(out, cachedFile{dir: filepath.Join(dir, b.Name(), f.Name()), file: file})
		}
	}
	return out, nil
}

// prune removes every result of the file except the one for gen at
// version, and returns how many it removed. When gen is -1 it removes them
// all. latest reads the file's current generation, or -1 once its row is
// gone.
func (c cachedFile) prune(gen int64, version int, latest func() (int64, error)) (int, error) {
	if gen < 0 {
		l, err := latest()
		if err != nil || l >= 0 {
			return 0, err
		}
		return 1, os.RemoveAll(c.dir)
	}
	entries, err := os.ReadDir(c.dir)
	if err != nil {
		return 0, err
	}
	removed := 0
	for _, e := range entries {
		if e.Name() == Leaf(gen, version) {
			continue
		}
		l, err := latest()
		if err != nil {
			return removed, err
		}
		if e.Name() == Leaf(l, version) {
			continue
		}
		if err := os.RemoveAll(filepath.Join(c.dir, e.Name())); err != nil {
			return removed, err
		}
		removed++
	}
	return removed, nil
}
