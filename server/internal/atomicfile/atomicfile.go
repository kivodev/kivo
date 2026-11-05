// Package atomicfile writes a file, or moves a built folder into place, so
// that it's never seen partly written.
package atomicfile

import (
	"os"
	"path/filepath"
)

// Write writes data to path through a hidden temporary file in the same
// directory, which is renamed over path once complete. So a reader sees the
// old file or the new one, never a partial one. The directory is made if
// it's missing. The file's mode is 0600.
func Write(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+"-*.tmp")
	if err != nil {
		return err
	}
	_, err = tmp.Write(data)
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Rename(tmp.Name(), path)
	}
	if err != nil {
		os.Remove(tmp.Name())
	}
	return err
}

// RenameDir moves the fully built folder src to dst, so a reader sees all
// of dst or none of it. It makes dst's parent if it's missing. If dst
// already exists, the move counts as done and src is left for the caller to
// remove.
func RenameDir(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	if err := os.Rename(src, dst); err != nil {
		if _, serr := os.Stat(dst); serr == nil {
			return nil
		}
		return err
	}
	return nil
}
