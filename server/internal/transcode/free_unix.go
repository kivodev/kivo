//go:build unix

package transcode

import (
	"io/fs"

	"golang.org/x/sys/unix"
)

// freeSpace is the free space available to an unprivileged user on the
// disk that holds path.
func freeSpace(path string) (uint64, error) {
	var st unix.Statfs_t
	if err := unix.Statfs(path, &st); err != nil {
		return 0, &fs.PathError{Op: "statfs", Path: path, Err: err}
	}
	return st.Bavail * uint64(st.Bsize), nil
}
