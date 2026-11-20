package scan

import (
	"errors"
	"testing"

	"github.com/mmcdole/kivo/internal/catalog"
	"github.com/mmcdole/kivo/internal/id"
)

var errIO = errors.New("input/output error")

// A directory that can't be read isn't known to be gone. What was recorded
// below it stays, so a later scan still walks it and finds a file added
// there.
func TestUnreadableDirectoryIsWalkedAgain(t *testing.T) {
	cases := []struct {
		name, broken string
		scan         func(h *harness, folder id.ID)
	}{
		{"it can't be stat'ed", "stat Show", func(h *harness, folder id.ID) { h.mustScan(folder) }},
		{"it can't be listed", "list Show", func(h *harness, folder id.ID) { h.scanDir(folder, "Show") }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h := newHarness(t)
			l := h.library(catalog.Movies, "/tv")
			folder, fsys := l.Folders[0].ID, h.folders["/tv"]
			fsys.write("Show/Season 01/e1.mkv", "1", old)
			h.mustScan(folder)

			fsys.broken = map[string]error{c.broken: errIO}
			c.scan(h, folder)
			if !h.files(folder)["Show/Season 01/e1.mkv"].Present {
				t.Fatal("a file below the unreadable directory was marked gone")
			}

			fsys.broken = nil
			fsys.write("Show/Season 01/e2.mkv", "2", old) // changes only Season 01's mtime
			h.mustScan(folder)
			if !h.files(folder)["Show/Season 01/e2.mkv"].Present {
				t.Fatal("the scan after the error didn't find a file added below the directory")
			}
		})
	}
}

// A new directory that can't be read is listed by the next scan, though its
// parent hasn't changed since.
func TestUnreadableNewDirectoryIsListedNextScan(t *testing.T) {
	for _, op := range []string{"stat", "list"} {
		t.Run(op, func(t *testing.T) {
			h := newHarness(t)
			l := h.library(catalog.Movies, "/m")
			folder, fsys := l.Folders[0].ID, h.folders["/m"]
			fsys.write("A/a.mkv", "a", old)
			h.mustScan(folder)

			fsys.write("B/b.mkv", "b", old)
			fsys.broken = map[string]error{op + " B": errIO}
			h.mustScan(folder)
			if _, ok := h.files(folder)["B/b.mkv"]; ok {
				t.Fatal("a file in the unreadable directory was catalogued")
			}

			fsys.broken = nil
			h.mustScan(folder)
			if !h.files(folder)["B/b.mkv"].Present {
				t.Fatal("the scan after the error didn't list the new directory")
			}
		})
	}
}

// A listed file that can't be examined isn't known to be gone. Its
// directory is left as it was, and listed again by the next scan.
func TestUnreadableFileIsNotMarkedGone(t *testing.T) {
	h := newHarness(t)
	l := h.library(catalog.Movies, "/m")
	folder, fsys := l.Folders[0].ID, h.folders["/m"]
	fsys.write("M/a.mkv", "a", old)
	h.mustScan(folder)

	fsys.write("M/b.mkv", "b", old) // so M is listed
	fsys.broken = map[string]error{"info M/a.mkv": errIO}
	h.mustScan(folder)
	if !h.files(folder)["M/a.mkv"].Present {
		t.Fatal("the file that couldn't be examined was marked gone")
	}

	fsys.broken = nil
	h.mustScan(folder)
	got := h.files(folder)
	if !got["M/a.mkv"].Present || !got["M/b.mkv"].Present {
		t.Fatalf("after the error cleared: a.mkv present %v, b.mkv present %v; want both", got["M/a.mkv"].Present, got["M/b.mkv"].Present)
	}
}
