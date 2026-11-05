package store

import (
	"path/filepath"

	"golang.org/x/sys/windows"
)

// networkFS returns "network" if dir is on a mapped network drive or a UNC
// share, where SQLite isn't safe.
func networkFS(dir string) string {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return ""
	}
	root, err := windows.UTF16PtrFromString(filepath.VolumeName(abs) + `\`)
	if err != nil || windows.GetDriveType(root) != windows.DRIVE_REMOTE {
		return ""
	}
	return "network"
}

// freeBytes is a variable so tests can simulate a full disk.
var freeBytes = func(dir string) (int64, bool) {
	p, err := windows.UTF16PtrFromString(dir)
	if err != nil {
		return 0, false
	}
	var free uint64
	if windows.GetDiskFreeSpaceEx(p, &free, nil, nil) != nil {
		return 0, false
	}
	return int64(free), true
}
