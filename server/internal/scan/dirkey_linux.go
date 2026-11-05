package scan

import (
	"io/fs"
	"syscall"
)

// dirKey identifies a directory however it's reached, through links or not.
type dirKey struct{ dev, ino uint64 }

func keyOf(info fs.FileInfo) (dirKey, bool) {
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return dirKey{}, false
	}
	return dirKey{uint64(st.Dev), st.Ino}, true
}
