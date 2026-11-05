package identify_test

import (
	"testing"

	"github.com/mmcdole/kivo/internal/activity"
	"github.com/mmcdole/kivo/internal/catalog"
)

// A folder's new files are recorded as added when they get their versions.
// A series' episodes are one event with their count, and a movie is an
// event of its own. Files identified again add nothing.
func TestAddedActivity(t *testing.T) {
	e := newEnv(t)
	added := func() []activity.Event {
		t.Helper()
		got, _, err := e.activity.List(ctx, activity.GroupAdded, activity.Position{}, 10)
		if err != nil {
			t.Fatal(err)
		}
		return got
	}
	shows := e.library(catalog.Shows)
	f := e.add(shows, "Firefly/S01E01.mkv", "Firefly/S01E02.mkv")
	e.run(f...)
	got := added()
	if len(got) != 1 {
		t.Fatalf("added %+v", got)
	}
	if a := got[0]; a.Kind != activity.Added || a.Count != 2 || a.WorkKind != "episode" || a.Title != "Firefly" || a.Library != shows.Name {
		t.Errorf("two episodes %+v", a)
	}
	e.again(f...)
	more := e.add(shows, "Firefly/S01E03.mkv")
	e.run(more...)
	if got := added(); len(got) != 2 || got[0].Count != 1 || got[0].Season != 1 || got[0].Episode != 3 {
		t.Errorf("a third episode %+v", got)
	}

	movies := e.library(catalog.Movies)
	e.run(e.add(movies, "Heat (1995)/Heat (1995).mkv")...)
	if got := added(); len(got) != 3 || got[0].Title != "Heat" || got[0].WorkKind != "movie" || got[0].Count != 1 {
		t.Errorf("a movie %+v", got)
	}
}
