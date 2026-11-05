package markers

import (
	"reflect"
	"testing"
	"time"

	"github.com/mmcdole/kivo/internal/catalog"
)

func TestTimelineJoinsEveryPartsChapters(t *testing.T) {
	mins := func(n int) time.Duration { return time.Duration(n) * time.Minute }
	part := func(rel string, d time.Duration, chapters ...catalog.Chapter) catalog.File {
		return catalog.File{RelPath: rel, Probe: catalog.ProbeSummary{Duration: d}, Chapters: chapters}
	}
	// In a book whose first file has its own chapters and whose other
	// files have none, each of the other files becomes one chapter, named
	// after the file.
	got := timeline([]catalog.File{
		part("Andy Weir/Artemis/01 Artemis.m4b", mins(20),
			catalog.Chapter{Index: 0, Start: 0, End: mins(5), Title: "Opening Credits"},
			catalog.Chapter{Index: 1, Start: mins(5), End: mins(20), Title: "Chapter 1"}),
		part("Andy Weir/Artemis/02 - Chapter 2.m4b", mins(30)),
		part("Andy Weir/Artemis/CD2/03.mp3", mins(10)),
	})
	want := []catalog.Chapter{
		{Index: 0, Start: 0, End: mins(5), Title: "Opening Credits"},
		{Index: 1, Start: mins(5), End: mins(20), Title: "Chapter 1"},
		{Index: 2, Start: mins(20), End: mins(50), Title: "02 - Chapter 2"},
		{Index: 3, Start: mins(50), End: mins(60), Title: "03"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got  %+v\nwant %+v", got, want)
	}

	// A single file without chapters has none. A second part's chapters
	// are shifted by the first part's length.
	if got := timeline([]catalog.File{part("Film.mkv", mins(90))}); got != nil {
		t.Errorf("one part without chapters: %+v", got)
	}
	got = timeline([]catalog.File{
		part("Film cd1.avi", mins(50), catalog.Chapter{Start: 0, End: mins(50), Title: "One"}),
		part("Film cd2.avi", mins(40), catalog.Chapter{Start: 0, End: mins(40), Title: "Two"}),
	})
	if len(got) != 2 || got[1].Start != mins(50) || got[1].End != mins(90) || got[1].Index != 1 {
		t.Errorf("two parts: %+v", got)
	}
}
