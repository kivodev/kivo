package config

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"

	"github.com/mmcdole/kivo/internal/id"
)

// CacheTemp is the folder inside a cache folder where results are built
// before they're renamed into place. So a result that is in place is
// complete. Anything in CacheTemp is unfinished and may be removed.
const CacheTemp = ".tmp"

// CachePath returns the path in dir where an entry for x is cached:
// dir/xx/elem..., where xx is the last two hex digits of x. Using the last
// digits spreads the entries over 256 folders, because UUIDv7s share their
// first digits.
func CachePath(dir string, x id.ID, elem ...string) string {
	s := x.String()
	return filepath.Join(append([]string{dir, s[len(s)-2:]}, elem...)...)
}

// CacheLeaf names the folder, inside a file's cache folder, that holds the
// file's result for one generation. The name is the generation, followed by
// "v" and the version when the version is above 0. A result of an earlier
// version counts as not made.
func CacheLeaf(gen int64, version int) string {
	leaf := strconv.FormatInt(gen, 10)
	if version > 0 {
		leaf += "v" + strconv.Itoa(version)
	}
	return leaf
}

// MakeCacheTemp makes a folder inside dir's CacheTemp folder to build a
// result in. atomicfile.RenameDir then moves it into place.
func MakeCacheTemp(dir string) (string, error) {
	parent := filepath.Join(dir, CacheTemp)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return "", err
	}
	return os.MkdirTemp(parent, "")
}

// CachePrune says which results in a cache folder PruneCache keeps.
type CachePrune struct {
	// Version is the current version of the results, as in CacheLeaf.
	Version int
	// Generations reads the current generation of each given file that
	// still has a row.
	Generations func(ctx context.Context, files []id.ID) (map[id.ID]int64, error)
	// Progress, if set, is told how many of the files with results have
	// been looked at, with an empty step. It has the same shape as
	// tasks.Progress, so a prune task passes that.
	Progress func(ctx context.Context, step string, done, total int)
}

// pruneStep is how many files PruneCache looks at between progress reports
// and checks of ctx.
const pruneStep = 500

// PruneCache removes the results in dir that no file needs. Those are all
// results of a file with no row, and a file's results of other generations
// and versions. It returns how many results it removed and how many files
// still have a result. A file's generation is read again before each
// removal, so a result committed meanwhile is kept.
func PruneCache(ctx context.Context, dir string, p CachePrune) (removed, kept int, err error) {
	all, err := cachedFiles(dir)
	if err != nil {
		return 0, 0, err
	}
	gens, err := generations(ctx, p, all)
	if err != nil {
		return 0, 0, err
	}
	for i, c := range all {
		if i%pruneStep == 0 {
			if p.Progress != nil {
				p.Progress(ctx, "", i, len(all))
			}
			if err := ctx.Err(); err != nil {
				return removed, 0, err
			}
		}
		gen, ok := gens[c.file]
		if !ok {
			gen = -1
		}
		n, err := c.prune(gen, p.Version, func() (int64, error) { return latestGen(ctx, p, c.file) })
		removed += n
		if err != nil {
			return removed, 0, err
		}
	}
	return removed, len(gens), nil
}

// generations reads the current generation of each file with results that
// still has a row.
func generations(ctx context.Context, p CachePrune, all []cachedFile) (map[id.ID]int64, error) {
	ids := make([]id.ID, len(all))
	for i, c := range all {
		ids[i] = c.file
	}
	return p.Generations(ctx, ids)
}

// latestGen reads a file's current generation, or -1 once its row is gone.
func latestGen(ctx context.Context, p CachePrune, file id.ID) (int64, error) {
	g, err := p.Generations(ctx, []id.ID{file})
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

// cachedFiles lists the files with results in dir, where CachePath placed
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
		if b.Name() == CacheTemp || !b.IsDir() {
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
		if e.Name() == CacheLeaf(gen, version) {
			continue
		}
		l, err := latest()
		if err != nil {
			return removed, err
		}
		if e.Name() == CacheLeaf(l, version) {
			continue
		}
		if err := os.RemoveAll(filepath.Join(c.dir, e.Name())); err != nil {
			return removed, err
		}
		removed++
	}
	return removed, nil
}
