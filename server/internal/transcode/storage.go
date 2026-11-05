package transcode

import (
	"fmt"
	"os"
	"path/filepath"
)

// defaultFloor is the free space below which new streams are refused, so a
// full disk never drops every running stream. There's no size cap, so this
// is the only guard.
const defaultFloor = 5 << 30

// storage keeps each stream's segments, and each download's conversion, in
// a separate directory under dir.
type storage struct {
	dir   string
	floor uint64
}

// reset removes whatever a crash left behind and makes dir.
func (s *storage) reset() error {
	if err := os.RemoveAll(s.dir); err != nil {
		return err
	}
	return os.MkdirAll(s.dir, 0o755)
}

// room checks the free space against the floor.
func (s *storage) room() error {
	free, err := freeSpace(s.dir)
	if err != nil {
		return err
	}
	if free < s.floor {
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
