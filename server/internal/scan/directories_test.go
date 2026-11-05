package scan

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// Directories lists a directory's subdirectories, linked ones too, without
// hidden ones or files. A relative path is invalid, and a missing one is
// not found. The picker starts at a root it can list: / or a drive.
func TestDirectories(t *testing.T) {
	var s *Scanner
	dir := t.TempDir()
	for _, d := range []string{"b", "a", ".hidden"} {
		if err := os.Mkdir(filepath.Join(dir, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "file.mkv"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(dir, "a"), filepath.Join(dir, "c")); err != nil {
		t.Fatal(err)
	}
	dirs, found, err := s.Directories(dir)
	if err != nil || !found || !slices.Equal(dirs, []Directory{{"a", filepath.Join(dir, "a")}, {"b", filepath.Join(dir, "b")}, {"c", filepath.Join(dir, "c")}}) {
		t.Errorf("listing %v %v %v", dirs, found, err)
	}
	if _, _, err := s.Directories("relative"); !errors.Is(err, ErrRelativePath) {
		t.Errorf("relative: %v", err)
	}
	if _, _, err := s.Directories(filepath.Join(dir, "file.mkv")); !errors.Is(err, ErrUnreadable) {
		t.Errorf("a file: %v", err)
	}
	if _, found, err := s.Directories(filepath.Join(dir, "nope")); found || err != nil {
		t.Errorf("missing: %v %v", found, err)
	}
	starts := s.PickerStarts()
	if len(starts) == 0 || filepath.Dir(starts[0].Path) != starts[0].Path {
		t.Fatalf("picker starts %v", starts)
	}
	if _, found, err := s.Directories(starts[0].Path); !found || err != nil {
		t.Errorf("listing %s: %v %v", starts[0].Path, found, err)
	}
}
