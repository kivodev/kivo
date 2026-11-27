//go:build unix

package main

import (
	"os"
	"syscall"
)

// runAsOwner, when run as root, switches to the user and group that own the
// database file at path, just as the image's entrypoint does for the
// server. So files a command writes next to the database, such as SQLite's
// WAL and shared-memory files, stay owned by the server's user. This
// matters because `docker exec` runs as root unless given -u.
func runAsOwner(path string) error {
	if os.Geteuid() != 0 {
		return nil
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil // opening the database reports the error
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok || st.Uid == 0 {
		return nil
	}
	if err := syscall.Setgroups(nil); err != nil {
		return err
	}
	if err := syscall.Setgid(int(st.Gid)); err != nil {
		return err
	}
	return syscall.Setuid(int(st.Uid))
}
