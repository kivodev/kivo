//go:build unix

package health

import "golang.org/x/sys/unix"

// freeSpace returns the space available to an unprivileged user on path's
// disk.
func freeSpace(path string) (uint64, error) {
	var st unix.Statfs_t
	if err := unix.Statfs(path, &st); err != nil {
		return 0, err
	}
	return st.Bavail * uint64(st.Bsize), nil
}
