package scan

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/mmcdole/kivo/internal/fault"
)

// Directory is a directory on the server an admin may pick as a library
// folder.
type Directory struct {
	Name, Path string
}

var (
	// ErrRelativePath means a directory to list was given by a relative
	// path.
	ErrRelativePath = fault.New(fault.Invalid, "path must be absolute")
	// ErrUnreadable means a directory to list exists but can't be read as
	// one.
	ErrUnreadable = fault.New(fault.Invalid, "path is not a readable folder")
)

// Directories lists the directories inside the directory at path, for the
// library folder picker. path must be absolute. Hidden directories are
// left out. It reads only the names of directories. It follows links,
// because a library folder may be a link. found is false when there's no
// such directory or it may not be opened.
func Directories(path string) (dirs []Directory, found bool, err error) {
	if !filepath.IsAbs(path) {
		return nil, false, fault.About(fmt.Errorf("%w: %s", ErrRelativePath, path), fault.Field{Name: "path", Code: "invalid"})
	}
	path = filepath.Clean(path)
	if info, err := os.Stat(path); err == nil && !info.IsDir() {
		return nil, false, fault.About(fmt.Errorf("%w: %s: not a folder", ErrUnreadable, path), fault.Field{Name: "path", Code: "invalid"})
	}
	entries, err := os.ReadDir(path)
	if errors.Is(err, fs.ErrNotExist) || errors.Is(err, fs.ErrPermission) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fault.About(fmt.Errorf("%w: %s: %w", ErrUnreadable, path, err), fault.Field{Name: "path", Code: "invalid"})
	}
	dirs = []Directory{}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".") {
			continue
		}
		full := filepath.Join(path, e.Name())
		if info, err := os.Stat(full); err == nil && info.IsDir() {
			dirs = append(dirs, Directory{Name: e.Name(), Path: full})
		}
	}
	return dirs, true, nil
}
