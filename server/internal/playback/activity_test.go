package playback

import (
	"testing"

	"github.com/mmcdole/kivo/internal/activity"
)

// A play's start is recorded with its user, its client, and whether it's
// local. A fallback replacing a play doesn't count as another start.
func TestPlayStartedActivity(t *testing.T) {
	f := newFixture(t, 0)
	m := f.movie("Ronin", "mkv", 1)
	st, err := f.start(m.Work, lan)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Fallback(ctx, f.p, f.v, st.ID, FailReport{Code: "mediaDecodeError"}); err != nil {
		t.Fatal(err)
	}
	got, _, err := f.activity.List(ctx, activity.GroupPlays, activity.Position{}, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("plays %+v", got)
	}
	if e := got[0]; e.Kind != activity.PlayStarted || e.Username != "admin" || e.Title != "Ronin" || e.Work != m.Work || !e.Local {
		t.Errorf("started %+v", e)
	}
}
