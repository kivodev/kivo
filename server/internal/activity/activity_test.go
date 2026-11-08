package activity_test

import (
	"context"
	"errors"
	"net/netip"
	"testing"
	"testing/synctest"
	"time"

	"github.com/mmcdole/kivo/internal/store"

	"github.com/mmcdole/kivo/internal/activity"
	"github.com/mmcdole/kivo/internal/catalog"
	"github.com/mmcdole/kivo/internal/catalog/catalogtest"
	"github.com/mmcdole/kivo/internal/events"
	"github.com/mmcdole/kivo/internal/health"
	"github.com/mmcdole/kivo/internal/id"
	"github.com/mmcdole/kivo/internal/tasks"
	"github.com/mmcdole/kivo/internal/users"
)

var ctx = context.Background()

type fixture struct {
	t      *testing.T
	b      *catalogtest.Builder
	a      *activity.Service
	sub    *events.Sub // the activity's events
	lib    id.ID
	folder id.ID
}

func newFixture(t *testing.T) *fixture {
	f := &fixture{t: t, b: catalogtest.New(t)}
	bus := events.NewBus()
	t.Cleanup(bus.Close)
	f.sub = bus.Subscribe(func(events.Scope) bool { return true })
	t.Cleanup(f.sub.Close)
	f.a = activity.New(f.b.DB, bus)
	f.lib, f.folder = f.b.Library(catalog.Shows)
	return f
}

// changed returns how many activityChanged events went out after a second
// passed. It runs in a synctest bubble.
func (f *fixture) changed() int {
	f.t.Helper()
	time.Sleep(time.Second)
	synctest.Wait()
	n := 0
	for {
		select {
		case e := <-f.sub.C():
			if e.Kind != events.ActivityChanged || !e.Scope.AdminOnly {
				f.t.Fatalf("event %+v", e)
			}
			n++
		default:
			return n
		}
	}
}

func (f *fixture) user(name string) id.ID {
	f.t.Helper()
	var u users.User
	f.b.Write(func(ctx context.Context, tx *store.Tx) error {
		var err error
		u, err = users.New(f.b.DB, events.NewBus()).CreateUser(ctx, tx, users.NewUser{Username: name, Grants: users.Grants{All: true}})
		return err
	})
	return u.ID
}

func (f *fixture) list(g activity.Group) []activity.Event {
	f.t.Helper()
	events, more, err := f.a.List(ctx, g, activity.Position{}, 100)
	if err != nil || more {
		f.t.Fatalf("list %q: %v, more %v", g, err, more)
	}
	return events
}

// Each producer's event is written in the producer's transaction, with the
// names as they were then. Events are listed newest first and can be
// filtered by group.
func TestRecordAndList(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := newFixture(t)
		ben, admin := f.user("ben"), f.user("admin")
		series := f.b.Series("Shōgun")
		ep := f.b.Episode(f.folder, series, 1, 4)
		movie := f.b.Movie(f.folder, "Runner", 2024)
		f.b.Write(func(ctx context.Context, tx *store.Tx) error {
			return errors.Join(
				f.a.Played(ctx, tx, activity.PlayStarted, activity.PlayActivity{User: ben, Work: ep.Work, Client: "Firefox", Local: false}),
				f.a.FilesAdded(ctx, tx, f.lib, movie.Work, 1),
				f.a.SignedIn(ctx, tx, activity.SignIn{User: ben, Client: "ben's iPhone", Address: netip.MustParseAddr("::ffff:203.0.113.24")}),
				f.a.Joined(ctx, tx, ben, admin),
				f.a.TaskRunEnded(ctx, tx, tasks.RunEnd{Task: "make-previews", Outcome: tasks.Outcome{At: time.Now(), Summary: "214 done"}, Failed: 2}),
			)
		})
		if n := f.changed(); n != 1 {
			t.Errorf("%d activityChanged events for a write of five; want 1", n)
		}
		err := f.a.ProblemsRaised(ctx, []health.OpenProblem{
			{ID: "playback.gpuFailed", Problem: health.Problem{Component: "playback", Code: "gpuFailed", Message: "Quick Sync stopped working", Severity: health.Error, RaisedAt: time.Now()}},
			{ID: "activity.signInsFailed", Problem: health.Problem{Component: "activity", Code: "signInsFailed", Message: "5 failed sign-ins", RaisedAt: time.Now()}},
		})
		if err != nil {
			t.Fatal(err)
		}

		all := f.list("")
		var kinds []activity.Kind
		for _, e := range all {
			kinds = append(kinds, e.Kind)
		}
		want := []activity.Kind{activity.ProblemRaised, activity.TaskEnded, activity.UserJoined, activity.SignedIn, activity.Added, activity.PlayStarted}
		if len(kinds) != len(want) {
			t.Fatalf("kinds %v, want %v (newest first, activity's own problem left out)", kinds, want)
		}
		for i := range want {
			if kinds[i] != want[i] && !(i < 5 && all[i].At.Equal(all[i+1].At)) {
				t.Errorf("kinds %v, want %v", kinds, want)
				break
			}
		}
		byKind := map[activity.Kind]activity.Event{}
		for _, e := range all {
			byKind[e.Kind] = e
		}
		if e := byKind[activity.PlayStarted]; e.Username != "ben" || e.Title != "Shōgun" || e.Season != 1 || e.Episode != 4 || e.WorkKind != "episode" || e.Client != "Firefox" || e.Local {
			t.Errorf("play started %+v", e)
		}
		if e := byKind[activity.Added]; e.Title != "Runner" || e.Library == "" || e.Count != 1 || e.WorkKind != "movie" {
			t.Errorf("added %+v", e)
		}
		if e := byKind[activity.SignedIn]; e.Username != "ben" || e.Client != "ben's iPhone" || e.Address != "203.0.113.24" {
			t.Errorf("signed in %+v", e)
		}
		if e := byKind[activity.UserJoined]; e.Username != "ben" || e.By != "admin" {
			t.Errorf("joined %+v", e)
		}
		if e := byKind[activity.TaskEnded]; e.Task != "make-previews" || e.Summary != "214 done" || e.Failed != 2 {
			t.Errorf("task ended %+v", e)
		}
		if e := byKind[activity.ProblemRaised]; e.Problem != "playback.gpuFailed" || e.Code != "gpuFailed" || e.Severity != "error" {
			t.Errorf("problem raised %+v", e)
		}

		if got := f.list(activity.GroupPlays); len(got) != 1 || got[0].Kind != activity.PlayStarted {
			t.Errorf("plays %+v", got)
		}
		if got := f.list(activity.GroupSignIns); len(got) != 2 {
			t.Errorf("sign-ins %+v", got)
		}
		if got := f.list(activity.GroupServer); len(got) != 2 {
			t.Errorf("server %+v", got)
		}
		if _, _, err := f.a.List(ctx, "nope", activity.Position{}, 10); !errors.Is(err, activity.ErrUnknownGroup) {
			t.Errorf("an unknown group: %v", err)
		}
	})
}

// An event exists exactly when its change does. A rolled back transaction
// leaves no event and notifies nobody.
func TestRolledBack(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := newFixture(t)
		ben := f.user("ben")
		err := f.b.DB.Write(ctx, func(ctx context.Context, tx *store.Tx) error {
			if err := f.a.SignedIn(ctx, tx, activity.SignIn{User: ben}); err != nil {
				return err
			}
			return errors.New("the change failed")
		})
		if err == nil {
			t.Fatal("the write should fail")
		}
		if got, n := f.list(""), f.changed(); len(got) != 0 || n != 0 {
			t.Errorf("after a rollback: %+v, %d activityChanged events", got, n)
		}
	})
}

// Each page starts after the last event of the page before, newest first.
func TestPages(t *testing.T) {
	f := newFixture(t)
	start := time.Now().Add(-time.Hour)
	f.b.Write(func(ctx context.Context, tx *store.Tx) error {
		for i := range 5 {
			// Two events share each time, so the ID breaks the tie.
			at := start.Add(time.Duration(i/2) * time.Minute)
			if err := f.a.TaskRunEnded(ctx, tx, tasks.RunEnd{Task: "make-backup", Outcome: tasks.Outcome{At: at, Summary: string(rune('a' + i))}}); err != nil {
				return err
			}
		}
		return nil
	})
	var got string
	p := activity.Position{}
	for range 3 {
		events, more, err := f.a.List(ctx, "", p, 2)
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range events {
			got += e.Summary
		}
		if !more {
			break
		}
		last := events[len(events)-1]
		p = activity.Position{At: last.At, ID: last.ID}
	}
	if got != "edcba" {
		t.Errorf("pages read %q, want edcba", got)
	}
}

// Failed sign-ins for one user from one address within the hour become one
// counted event. Five in a day raise a problem, timed at the latest
// failure.
func TestFailedSignIns(t *testing.T) {
	f := newFixture(t)
	ben := f.user("ben")
	home, away := netip.MustParseAddr("192.168.1.9"), netip.MustParseAddr("203.0.113.24")
	fail := func(in activity.SignIn) {
		t.Helper()
		f.b.Write(func(ctx context.Context, tx *store.Tx) error { return f.a.Failed(ctx, tx, in) })
	}
	for range 3 {
		fail(activity.SignIn{User: ben, Address: away})
	}
	fail(activity.SignIn{User: ben, Address: home, Local: true})
	if p := f.a.Health(ctx); len(p) != 0 {
		t.Errorf("four failures raise %v", p)
	}
	fail(activity.SignIn{Address: away}) // a username nobody has

	got := f.list(activity.GroupSignIns)
	if len(got) != 3 {
		t.Fatalf("failed sign-ins %+v", got)
	}
	if e := got[2]; e.Kind != activity.SignInFailed || e.Username != "ben" || e.Address != away.String() || e.Count != 3 {
		t.Errorf("ben's from away %+v", e)
	}
	if e := got[1]; e.Username != "ben" || e.Address != home.String() || e.Count != 1 || !e.Local {
		t.Errorf("ben's from home %+v", e)
	}
	if e := got[0]; e.Username != "" || !e.User.IsZero() || e.Count != 1 {
		t.Errorf("nobody's %+v", e)
	}
	p := f.a.Health(ctx)
	if len(p) != 1 || p[0].Code != "signInsFailed" || p[0].Severity != health.Warning || p[0].Message != "5 failed sign-ins in the last day" ||
		!p[0].RaisedAt.Equal(got[0].At) {
		t.Errorf("five failures raise %+v", p)
	}
}

// The nightly cleanup removes events older than 90 days.
func TestPrune(t *testing.T) {
	f := newFixture(t)
	f.b.Write(func(ctx context.Context, tx *store.Tx) error {
		for _, age := range []time.Duration{91 * 24 * time.Hour, 89 * 24 * time.Hour} {
			if err := f.a.TaskRunEnded(ctx, tx, tasks.RunEnd{Task: "make-backup", Outcome: tasks.Outcome{At: time.Now().Add(-age)}}); err != nil {
				return err
			}
		}
		return nil
	})
	ts := f.a.Tasks()
	if len(ts) != 1 || ts[0].Name != activity.TaskPrune || !ts[0].Nightly || !ts[0].Maintenance {
		t.Fatalf("tasks %+v", ts)
	}
	if err := ts[0].Run(ctx); err != nil {
		t.Fatal(err)
	}
	if got := f.list(""); len(got) != 1 || time.Since(got[0].At) > 90*24*time.Hour {
		t.Errorf("after the cleanup %+v", got)
	}
}
