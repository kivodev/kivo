package scan

import (
	"errors"
	"slices"
	"testing"

	"github.com/mmcdole/kivo/internal/catalog"
	"github.com/mmcdole/kivo/internal/catalog/catalogtest"
	"github.com/mmcdole/kivo/internal/id"
)

// An edit that adds a folder asks for a scan of the library, which walks
// the new folder too. An edit that adds no folder asks for nothing.
func TestEditLibraryScansAddedFolders(t *testing.T) {
	h := newHarness(t)
	l := h.library(catalog.Movies, "/a")
	h.folders["/a"].write("A/a.mkv", "a", old)
	h.scanLibrary(l.ID)
	h.tasks.reset()
	h.folders["/b"] = newMemFS()
	h.folders["/b"].write("B/b.mkv", "b", old)

	err := h.s.EditLibrary(ctx, l.ID, catalog.LibraryChange{Name: new("Films"), Folders: &[]string{catalogtest.Abs("/a"), catalogtest.Abs("/b")}})
	if err != nil {
		t.Fatal(err)
	}
	l, err = h.b.Catalog.Library(ctx, l.ID)
	if err != nil {
		t.Fatal(err)
	}
	if l.Name != "Films" || len(l.Folders) != 2 {
		t.Fatalf("library %+v", l)
	}
	var added id.ID
	for _, r := range l.Folders {
		if r.Path == catalogtest.Abs("/b") {
			added = r.ID
		}
	}
	if got := h.tasks.of(TaskScanLibraries); !slices.Equal(got, []id.ID{l.ID}) {
		t.Errorf("asked for scans of %v, want the library %v", got, l.ID)
	}
	h.scanLibrary(l.ID)
	if !h.files(added)["B/b.mkv"].Present {
		t.Fatalf("the added folder's files: %v", h.files(added))
	}
	h.tasks.reset()

	// An edit of one setting keeps the rest as they are now, whatever the
	// editor last read.
	if err := h.s.EditLibrary(ctx, l.ID, catalog.LibraryChange{UseLocalMetadata: new(false)}); err != nil {
		t.Fatal(err)
	}
	if l, _ = h.b.Catalog.Library(ctx, l.ID); l.Name != "Films" || len(l.Folders) != 2 || l.UseLocalMetadata {
		t.Fatalf("after editing one setting: %+v", l)
	}
	if got := h.tasks.of(TaskScanLibraries); len(got) != 0 {
		t.Fatalf("an edit adding no folder asked for %v", got)
	}
}

// Dropping a folder hides it at once and asks for its removal, which
// deletes its files. The library's scans skip the folder in the meantime.
func TestEditLibraryRemovesDroppedFolders(t *testing.T) {
	h := newHarness(t)
	l := h.library(catalog.Movies, "/a", "/b")
	b := l.Folders[1].ID
	h.folders["/a"].write("A/a.mkv", "a", old)
	h.folders["/b"].write("M/m.mkv", "m", old)
	h.scanLibrary(l.ID)
	h.tasks.reset()

	if err := h.s.EditLibrary(ctx, l.ID, catalog.LibraryChange{Folders: &[]string{catalogtest.Abs("/a")}}); err != nil {
		t.Fatal(err)
	}
	if got := h.tasks.of(catalog.TaskLibraryChanges); !slices.Equal(got, []id.ID{b}) {
		t.Fatalf("asked for removals of %v, want the dropped folder %v", got, b)
	}
	if got := h.tasks.of(TaskScanLibraries); len(got) != 0 {
		t.Fatalf("dropping a folder asked for scans of %v", got)
	}
	h.folders["/b"].write("N/n.mkv", "y", old)
	h.mustScan(b) // as a scan already asked for would run: it changes nothing
	h.scanLibrary(l.ID)
	if got := h.files(b); len(got) != 1 {
		t.Fatalf("files %v; want the scans to add none", got)
	}
	h.remove(b)
	if got := h.files(b); len(got) != 0 {
		t.Fatalf("files %v left", got)
	}
}

// A scan running when its folder is dropped stops at the next directory it
// commits, so the removal isn't racing new files in.
func TestScanStopsWhenItsFolderIsDropped(t *testing.T) {
	h := newHarness(t)
	l := h.library(catalog.Movies, "/a", "/b")
	b, fsys := l.Folders[1].ID, h.folders["/b"]
	fsys.write("A/a.mkv", "x", old)
	fsys.write("B/b.mkv", "y", old)
	fsys.onList = func(dir string) {
		if dir == "B" {
			if err := h.s.EditLibrary(ctx, l.ID, catalog.LibraryChange{Folders: &[]string{catalogtest.Abs("/a")}}); err != nil {
				t.Error(err)
			}
		}
	}
	h.mustScan(b)
	if got := h.files(b); len(got) != 1 || !got["A/a.mkv"].Present {
		t.Fatalf("files %v; want only the directory listed before the folder was dropped", got)
	}
}

// Deleting a library hides it at once and asks for its deletion. A scan of
// the library in the meantime does nothing.
func TestDeleteLibraryAsksForItsDeletion(t *testing.T) {
	h := newHarness(t)
	l := h.library(catalog.Movies, "/a")
	h.folders["/a"].write("M/m.mkv", "x", old)
	h.scanLibrary(l.ID)
	h.tasks.reset()
	if err := h.s.DeleteLibrary(ctx, l.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := h.b.Catalog.Library(ctx, l.ID); !errors.Is(err, catalog.ErrNotFound) {
		t.Fatalf("library being deleted: %v", err)
	}
	if err := h.s.DeleteLibrary(ctx, l.ID); !errors.Is(err, catalog.ErrNotFound) {
		t.Fatalf("deleting it twice: %v", err)
	}
	if got := h.tasks.of(catalog.TaskLibraryChanges); !slices.Equal(got, []id.ID{l.ID}) {
		t.Fatalf("asked for deletions of %v", got)
	}
	h.folders["/a"].write("N/n.mkv", "y", old)
	h.scanLibrary(l.ID)
	if got := h.files(l.Folders[0].ID); len(got) != 1 {
		t.Fatalf("files %v; want the scan to add none", got)
	}
	h.remove(l.ID)
	if got := h.files(l.Folders[0].ID); len(got) != 0 {
		t.Fatalf("files %v left", got)
	}
}
