package transcode

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/mmcdole/kivo/internal/health"
)

// floor is the free space below which new streams are refused, so a full
// disk never drops every running stream. There's no size cap, so this is
// the only guard.
const floor = 5 << 30

// storage keeps each stream's segments, and each download's conversion, in
// a separate directory under dir.
type storage struct {
	dir  string
	disk health.Room
}

// newStorage returns storage under dir. free reads the disk's free space,
// and nil reads the real disk.
func newStorage(dir string, free func(string) (uint64, error)) storage {
	return storage{dir: dir, disk: health.Room{Dir: dir, Free: free, Floor: floor}}
}

// reset removes whatever a crash left behind and makes dir.
func (s *storage) reset() error {
	if err := os.RemoveAll(s.dir); err != nil {
		return err
	}
	return os.MkdirAll(s.dir, 0o755)
}

// room returns ErrStorageFull while the disk is below the floor.
func (s *storage) room() error {
	if s.disk.Check() != nil {
		return ErrStorageFull
	}
	return nil
}

// open makes a stream's directory, if there's room. Its name is random
// rather than the play's ID, because the play ID is a credential and the
// directory shows up in logged ffmpeg command lines.
func (s *storage) open() (string, error) {
	if err := s.room(); err != nil {
		return "", err
	}
	dir, err := os.MkdirTemp(s.dir, "stream-")
	if err != nil {
		return "", err
	}
	return dir, nil
}

// named makes a fresh directory for a download, if there's room. It's
// named by the download's ID, which, unlike a play's ID, isn't a
// credential.
func (s *storage) named(name string) (string, error) {
	if !validName(name) {
		return "", fmt.Errorf("invalid download name %q", name)
	}
	if err := s.room(); err != nil {
		return "", err
	}
	dir := filepath.Join(s.dir, name)
	if err := os.RemoveAll(dir); err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return dir, nil
}
