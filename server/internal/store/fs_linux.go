package store

import "syscall"

// Filesystem magic numbers from statfs(2).
var fsMagic = map[int64]string{
	0x6969:     "NFS",
	0xFF534D42: "SMB",
	0xFE534D42: "SMB",
	0x517B:     "SMB",
	0x65735546: "FUSE",
}

// networkFS returns the name of the filesystem holding dir if SQLite isn't
// safe on it.
func networkFS(dir string) string {
	var st syscall.Statfs_t
	if syscall.Statfs(dir, &st) != nil {
		return ""
	}
	return fsMagic[int64(st.Type)]
}

// freeBytes is a variable so tests can simulate a full disk.
var freeBytes = func(dir string) (int64, bool) {
	var st syscall.Statfs_t
	if syscall.Statfs(dir, &st) != nil {
		return 0, false
	}
	return int64(st.Bavail) * int64(st.Bsize), true
}
