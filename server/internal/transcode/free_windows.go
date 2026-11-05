package transcode

import (
	"io/fs"

	"golang.org/x/sys/windows"
)

// freeSpace is the free space available to the server's user on the disk
// that holds path.
func freeSpace(path string) (uint64, error) {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return 0, &fs.PathError{Op: "GetDiskFreeSpaceEx", Path: path, Err: err}
	}
	var free uint64
	if err := windows.GetDiskFreeSpaceEx(p, &free, nil, nil); err != nil {
		return 0, &fs.PathError{Op: "GetDiskFreeSpaceEx", Path: path, Err: err}
	}
	return free, nil
}
