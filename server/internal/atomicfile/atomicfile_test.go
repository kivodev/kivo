package atomicfile

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a.vtt")
	for _, body := range []string{"first", "second"} {
		if err := Write(path, []byte(body)); err != nil {
			t.Fatal(err)
		}
		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != body {
			t.Fatalf("got %q, want %q", got, body)
		}
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("%d files in the directory, want only the written one", len(entries))
	}
	nested := filepath.Join(dir, "sub", "b.vtt")
	if err := Write(nested, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(nested); err != nil {
		t.Fatal(err)
	}
}

func TestRenameDir(t *testing.T) {
	dir := t.TempDir()
	build := func() string {
		t.Helper()
		src, err := os.MkdirTemp(dir, "")
		if err == nil {
			err = os.WriteFile(filepath.Join(src, "manifest.json"), []byte("{}"), 0o644)
		}
		if err != nil {
			t.Fatal(err)
		}
		return src
	}
	dst := filepath.Join(dir, "ab", "file", "1")
	if err := RenameDir(build(), dst); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dst, "manifest.json")); err != nil {
		t.Fatal(err)
	}

	// When dst is already there, the move counts as done and the second
	// build is left where it is.
	again := build()
	if err := RenameDir(again, dst); err != nil {
		t.Fatalf("onto a made one: %v", err)
	}
	if _, err := os.Stat(again); err != nil {
		t.Fatalf("the second build: %v", err)
	}
	if err := RenameDir(filepath.Join(dir, "missing"), filepath.Join(dir, "other")); err == nil {
		t.Fatal("moved a missing folder")
	}
}
