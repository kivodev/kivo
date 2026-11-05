package scan

import (
	"context"
	"errors"
	"io/fs"

	"github.com/mmcdole/kivo/internal/catalog"
)

// moveSource finds the file this one was moved from. That is the one known
// file with the same size and edge hash that is gone, or whose path no
// longer exists. A file still at its path never counts, so copies and
// hardlinks stay separate. With two possible sources, the file is treated
// as new.
func (s *Scanner) moveSource(ctx context.Context, f catalog.File, edge []byte) (catalog.File, error) {
	same, err := s.deps.Catalog.FilesByHash(ctx, f.Size, edge)
	if err != nil {
		return catalog.File{}, err
	}
	var found []catalog.File
	for _, c := range same {
		if c.ID == f.ID {
			continue
		}
		if c.Present && s.exists(c) {
			continue
		}
		found = append(found, c)
	}
	if len(found) != 1 {
		return catalog.File{}, nil
	}
	return found[0], nil
}

// exists reports whether a file is still at its path. Anything but a clear
// "not there", such as an unreachable folder, counts as there, so a move is
// never guessed from an outage.
func (s *Scanner) exists(f catalog.File) bool {
	fsys, closeFolder, err := s.opt.OpenFolder(f.FolderPath)
	if err != nil {
		return true
	}
	defer closeFolder()
	_, err = fs.Stat(fsys, f.RelPath)
	return !errors.Is(err, fs.ErrNotExist)
}
