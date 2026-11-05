package scan

import (
	"context"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/mmcdole/kivo/internal/catalog"
)

// An audiobooks library catalogues audio files, and nothing else beside
// them except cover images. It skips CUE sheets, rip-report NFO files,
// video, and the .bookarr- files Bookarr leaves while it replaces a book.
func TestAudiobookLibraryFiles(t *testing.T) {
	h := newHarness(t)
	lib := h.library(catalog.Audiobooks, "/books")
	fsys := h.folders["/books"]
	for _, p := range []string{
		"Andy Weir/Project Hail Mary/Project Hail Mary.m4b",
		"Andy Weir/Project Hail Mary/Project Hail Mary.cue",
		"Andy Weir/Project Hail Mary/Project Hail Mary.nfo",
		"Andy Weir/Project Hail Mary/cover.jpg",
		"Andy Weir/Project Hail Mary/.bookarr-3f2a.recycle",
		"Andy Weir/Project Hail Mary/.bookarr-extract/Project Hail Mary.m4b",
		"Stephen King/11.22.63/01.mp3", "Stephen King/11.22.63/02.m4a", "Stephen King/11.22.63/03.aac",
		"Stephen King/11.22.63/04.flac", "Stephen King/11.22.63/05.ogg", "Stephen King/11.22.63/06.opus",
		"Stephen King/11.22.63/trailer.mp4", "Stephen King/11.22.63/11.22.63.srt",
	} {
		fsys.write(p, "x", old)
	}
	h.prob.tags = map[string]string{"artist": "Andy Weir", "composer": "Ray Porter"}
	h.mustScan(lib.Folders[0].ID)
	var got []string
	for rel := range h.files(lib.Folders[0].ID) {
		got = append(got, rel)
	}
	slices.Sort(got)
	want := []string{
		"Andy Weir/Project Hail Mary/Project Hail Mary.m4b",
		"Stephen King/11.22.63/01.mp3", "Stephen King/11.22.63/02.m4a", "Stephen King/11.22.63/03.aac",
		"Stephen King/11.22.63/04.flac", "Stephen King/11.22.63/05.ogg", "Stephen King/11.22.63/06.opus",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("catalogued %v\nwant %v", got, want)
	}

	h.probeAll()
	f := h.files(lib.Folders[0].ID)["Andy Weir/Project Hail Mary/Project Hail Mary.m4b"]
	file, err := h.b.Catalog.File(context.Background(), f.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(file.Probe.Tags, h.prob.tags) {
		t.Errorf("tags %v", file.Probe.Tags)
	}

	// A changed cover sends the book back to identification. A CUE or NFO
	// file isn't local metadata for a book.
	h.identifyAll()
	fsys.write("Andy Weir/Project Hail Mary/Project Hail Mary.nfo", "Read By: Ray Porter", time.Now())
	h.mustScan(lib.Folders[0].ID)
	if n := len(h.unidentified(lib.Folders[0].ID)); n != 0 {
		t.Errorf("an NFO change sent back %d files", n)
	}
	fsys.write("Andy Weir/Project Hail Mary/cover.jpg", "y", time.Now())
	h.mustScan(lib.Folders[0].ID)
	if n := len(h.unidentified(lib.Folders[0].ID)); n != 1 {
		t.Errorf("a cover change sent back %d files", n)
	}

	// The library makes no preview thumbnails.
	if lib.Previews != catalog.Never {
		t.Errorf("library settings %+v", lib)
	}
}
