package identify

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"time"

	"github.com/mmcdole/kivo/internal/catalog"
	"github.com/mmcdole/kivo/internal/id"
	"github.com/mmcdole/kivo/internal/identify/dbq"
	"github.com/mmcdole/kivo/internal/parse"
)

// snapshot is one folder of a library as the database has it, classified
// into units. It holds the unit being identified and that unit's NFO
// files.
type snapshot struct {
	lib    catalog.Library
	parser parse.Parser
	unit   *unit
	nfos   map[string][]byte // NFO files by path, read from the first folder that has them
}

// folderKey names what is identified together with a file at rel. That is
// the whole folder the file is identified with, or the file alone when it
// is outside any folder. The files that need identifying are grouped by this
// key, so each folder is decided once.
func folderKey(lib catalog.Library, file id.ID, rel string) string {
	if folder := folderOf(lib.Kind, rel); folder != "" {
		return lib.ID.String() + "/" + folder
	}
	return lib.ID.String() + "/" + file.String()
}

// lockKey returns the key a file's identification locks while it runs. It
// is the file's folder, or for a loose file the name it parses to, so two
// loose copies of one movie aren't searched for twice.
func lockKey(lib catalog.Library, rel string) string {
	if folder := folderOf(lib.Kind, rel); folder != "" {
		return lib.ID.String() + "/" + folder
	}
	switch lib.Kind {
	case catalog.Audiobooks:
		return lib.ID.String() + "|" + rel // a one-file book
	}
	return lib.ID.String() + "|" + parse.Key(newParser().Parse(rel, parse.Kind(lib.Kind)).Title)
}

// newParser returns a parser that uses the current date, which bounds years
// and air dates.
func newParser() parse.Parser { return parse.Parser{Today: parse.DateOf(time.Now())} }

// readSnapshot returns the unit that file f belongs to in its folder, with
// the unit's NFO files. The unit is nil when f isn't catalogued, such as a
// sample.
func (i *Identifier) readSnapshot(ctx context.Context, lib catalog.Library, f catalog.File) (*snapshot, error) {
	p, units, err := i.classifyFolder(ctx, lib, f)
	if err != nil {
		return nil, err
	}
	for _, u := range units {
		if u.has(f.ID) {
			return i.unitSnapshot(lib, p, u)
		}
	}
	return &snapshot{lib: lib, parser: p}, nil
}

// classifyFolder gathers the present files of the folder that file f is
// identified with, from every folder of its library, and classifies them
// into units. A file outside any folder is a unit of its own while it is
// present.
func (i *Identifier) classifyFolder(ctx context.Context, lib catalog.Library, f catalog.File) (parse.Parser, []*unit, error) {
	p := newParser()
	folder := folderOf(lib.Kind, f.RelPath)
	var files []file
	if folder == "" && f.Present {
		files = []file{{ID: f.ID, Rel: f.RelPath, Size: f.Size, Gen: f.Generation, tags: f.Probe.Tags}}
	}
	if folder != "" {
		for _, r := range lib.Folders {
			rows, err := i.q.FolderFiles(ctx, dbq.FolderFilesParams{FolderID: r.ID, Dir: folder})
			if err != nil {
				return p, nil, err
			}
			for _, row := range rows {
				fl := file{ID: row.ID, Rel: row.RelPath, Size: row.Size, Gen: row.Generation}
				if row.Tags.Valid {
					if err := json.Unmarshal([]byte(row.Tags.String), &fl.tags); err != nil {
						return p, nil, err
					}
				}
				files = append(files, fl)
			}
		}
	}
	return p, classify(p, lib.Kind, folder, files), nil
}

// unitSnapshot returns a snapshot of u with its NFO files read.
func (i *Identifier) unitSnapshot(lib catalog.Library, p parse.Parser, u *unit) (*snapshot, error) {
	s := &snapshot{lib: lib, parser: p, unit: u, nfos: map[string][]byte{}}
	if len(u.nfos) == 0 {
		return s, nil
	}
	return s, i.readNFOs(lib, u.nfos, s.nfos)
}

// readNFOs reads the named NFO files, each from the first folder that holds
// it. A folder that can't be opened stops identification, because deciding
// without an NFO that exists could pick the wrong match.
func (i *Identifier) readNFOs(lib catalog.Library, names []string, into map[string][]byte) error {
	for _, r := range lib.Folders {
		fsys, closeFolder, err := i.openFolder(r.Path)
		if err != nil {
			return fmt.Errorf("%w: %s", catalog.ErrUnreachable, r.Path)
		}
		for _, name := range names {
			if _, ok := into[name]; ok {
				continue
			}
			b, err := readLimited(fsys, name)
			switch {
			case errors.Is(err, fs.ErrNotExist):
			case err != nil:
				closeFolder()
				return err
			default:
				into[name] = b
			}
		}
		closeFolder()
	}
	return nil
}

func readLimited(fsys fs.FS, name string) ([]byte, error) {
	f, err := fsys.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(io.LimitReader(f, maxNFO))
}
