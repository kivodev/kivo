package scan

import (
	"bufio"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
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
func (s *Scanner) Directories(path string) (dirs []Directory, found bool, err error) {
	if !filepath.IsAbs(path) {
		return nil, false, fault.About(fmt.Errorf("%w: %s", ErrRelativePath, path), fault.Field{Name: "path", Code: "invalid"})
	}
	path = filepath.Clean(path)
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

// pseudoFS are filesystem types that never hold media.
var pseudoFS = []string{
	"proc", "sysfs", "devtmpfs", "devpts", "tmpfs", "cgroup", "cgroup2", "mqueue", "securityfs",
	"debugfs", "tracefs", "pstore", "bpf", "configfs", "fusectl", "hugetlbfs", "autofs", "binfmt_misc",
	"overlay", "nsfs", "rpc_pipefs", "efivarfs", "ramfs", "squashfs",
}

// PickerStarts returns where the library folder picker starts: / plus every
// mount that could hold media. That way /media and /mnt/… show at once in a
// container.
func (s *Scanner) PickerStarts() []Directory {
	out := []Directory{{Name: "/", Path: "/"}}
	f, err := os.Open("/proc/self/mounts")
	if err != nil {
		return out // not on Linux
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) < 3 || slices.Contains(pseudoFS, fields[2]) {
			continue
		}
		mount := strings.ReplaceAll(fields[1], `\040`, " ")
		if mount == "/" || strings.HasPrefix(mount, "/proc") || strings.HasPrefix(mount, "/sys") ||
			strings.HasPrefix(mount, "/dev") || strings.HasPrefix(mount, "/etc") || strings.HasPrefix(mount, "/boot") {
			continue
		}
		if !slices.ContainsFunc(out, func(d Directory) bool { return d.Path == mount }) {
			out = append(out, Directory{Name: mount, Path: mount})
		}
	}
	slices.SortFunc(out[1:], func(a, b Directory) int { return strings.Compare(a.Path, b.Path) })
	return out
}
