package userstate_test

import (
	"testing"
	"time"

	"github.com/mmcdole/kivo/internal/activity"
	"github.com/mmcdole/kivo/internal/id"
	"github.com/mmcdole/kivo/internal/userstate"
)

// A play that ends past the played threshold is recorded as finished, with
// its client, in the write of its last position. A play that ends before
// the threshold isn't recorded.
func TestPlayFinishedActivity(t *testing.T) {
	f := newFixture(t)
	m := f.movie("Heat", 100*time.Minute)
	finished := func() []activity.Event {
		t.Helper()
		got, _, err := f.activity.List(ctx, activity.GroupPlays, activity.Position{}, 10)
		if err != nil {
			t.Fatal(err)
		}
		return got
	}

	f.play(id.New(), m.Version, 30*time.Minute)
	if got := finished(); len(got) != 0 {
		t.Fatalf("a play ended before the threshold: %+v", got)
	}

	err := f.s.Report(ctx, userstate.Progress{
		Play: id.New(), User: f.user, Version: m.Version, Position: 95 * time.Minute, At: time.Now(), Stopped: true,
		Client: "Living Room TV", Local: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	got := finished()
	if len(got) != 1 {
		t.Fatalf("finished %+v", got)
	}
	if e := got[0]; e.Kind != activity.PlayFinished || e.Username != "anna" || e.Title != "Heat" || e.Work != m.Work ||
		e.Client != "Living Room TV" || !e.Local {
		t.Errorf("finished %+v", e)
	}
	if st := f.state(f.user, m.Work); !st.Played {
		t.Errorf("the play finished but its work isn't played: %+v", st)
	}
}
