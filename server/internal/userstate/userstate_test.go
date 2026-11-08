package userstate_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"testing"
	"testing/synctest"
	"time"

	"github.com/mmcdole/kivo/internal/store"

	"github.com/mmcdole/kivo/internal/activity"
	"github.com/mmcdole/kivo/internal/catalog"
	"github.com/mmcdole/kivo/internal/catalog/catalogtest"
	"github.com/mmcdole/kivo/internal/config"
	"github.com/mmcdole/kivo/internal/events"
	"github.com/mmcdole/kivo/internal/id"
	"github.com/mmcdole/kivo/internal/users"
	"github.com/mmcdole/kivo/internal/userstate"
	"github.com/mmcdole/kivo/internal/userstate/dbq"
)

var ctx = context.Background()

type fixture struct {
	t        *testing.T
	b        *catalogtest.Builder
	settings *config.Store
	activity *activity.Service
	s        *userstate.Service
	folder   id.ID
	user     id.ID
	bus      *events.Bus
	sub      *events.Sub // the bus's every event
}

// told is a userStateChanged event, reduced to the user it's scoped to and
// the works it names.
type told struct {
	user  id.ID
	works []id.ID
}

// told returns the userStateChanged events published since the last call.
func (f *fixture) told() []told {
	f.t.Helper()
	var out []told
	for {
		select {
		case e := <-f.sub.C():
			works, ok := e.Data.([]id.ID)
			if e.Kind != events.UserStateChanged || !ok {
				f.t.Fatalf("event %+v", e)
			}
			out = append(out, told{e.Scope.User, works})
		default:
			return out
		}
	}
}

func newFixture(t *testing.T) *fixture {
	f := &fixture{t: t, b: catalogtest.New(t), bus: events.NewBus()}
	f.sub = f.bus.Subscribe(func(events.Scope) bool { return true })
	t.Cleanup(f.sub.Close)
	f.activity = activity.New(f.b.DB, events.NewBus())
	settings, err := config.Open(ctx, f.b.DB, config.Options{})
	if err != nil {
		t.Fatal(err)
	}
	f.settings = settings
	f.s = f.service()
	_, f.folder = f.b.Library(catalog.Movies)
	f.user = f.newUser("anna")
	return f
}

// service returns a new service over the same database, as after a restart.
func (f *fixture) service() *userstate.Service {
	return userstate.New(f.b.DB, userstate.Deps{Catalog: f.b.Catalog, Settings: f.settings, Activity: f.activity, Events: f.bus})
}

// setThreshold sets the played threshold setting.
func (f *fixture) setThreshold(v float64) {
	f.t.Helper()
	if err := f.settings.Update(ctx, func(s *config.Settings) error {
		s.PlayedThreshold = v
		return nil
	}); err != nil {
		f.t.Fatal(err)
	}
}

func (f *fixture) newUser(name string) id.ID {
	u := users.New(f.b.DB, events.NewBus())
	var p id.ID
	f.b.Write(func(ctx context.Context, tx *store.Tx) error {
		user, err := u.CreateUser(ctx, tx, users.NewUser{Username: name, Grants: users.Grants{All: true}})
		p = user.ID
		return err
	})
	return p
}

// viewer is the Viewer of the fixture's user.
func (f *fixture) viewer() users.Viewer {
	v, err := users.New(f.b.DB, events.NewBus()).Viewer(ctx, f.user)
	if err != nil {
		f.t.Fatal(err)
	}
	return v
}

// movie adds a movie whose one file lasts dur.
func (f *fixture) movie(title string, dur time.Duration) catalogtest.Item {
	m := f.b.Movie(f.folder, title, 2000)
	f.setDuration(m.File, dur)
	return m
}

func (f *fixture) setDuration(file id.ID, dur time.Duration) {
	f.t.Helper()
	got, err := f.b.Catalog.File(ctx, file)
	if err != nil {
		f.t.Fatal(err)
	}
	f.b.Write(func(ctx context.Context, tx *store.Tx) error {
		return f.b.Catalog.SetProbe(ctx, tx, file, got.Generation, catalog.Probe{Summary: catalog.ProbeSummary{Duration: dur}})
	})
}

// report writes a report of a play's position, made at at.
func (f *fixture) report(playID, version id.ID, pos time.Duration, at time.Time, stopped bool) {
	f.t.Helper()
	err := f.s.Report(ctx, userstate.Progress{Play: playID, User: f.user, Version: version, Position: pos, At: at, Stopped: stopped})
	if err != nil {
		f.t.Fatal(err)
	}
}

// play stops a play at pos, writing the position.
func (f *fixture) play(playID, version id.ID, pos time.Duration) {
	f.t.Helper()
	f.report(playID, version, pos, time.Now(), true)
}

func (f *fixture) state(user, work id.ID) dbq.UserState {
	f.t.Helper()
	var st dbq.UserState
	err := f.b.DB.Reader().QueryRow(`SELECT played, play_count, position_ms, resumable, favorite, favorited_at, rating FROM user_state WHERE user_id = ? AND work_id = ?`, user, work).
		Scan(&st.Played, &st.PlayCount, &st.PositionMs, &st.Resumable, &st.Favorite, &st.FavoritedAt, &st.Rating)
	if err != nil && err != sql.ErrNoRows {
		f.t.Fatal(err)
	}
	return st
}

func TestAReportIsWrittenAtOnce(t *testing.T) {
	f := newFixture(t)
	m := f.movie("Heat", 2*time.Hour)
	f.report(id.New(), m.Version, 10*time.Minute, time.Now(), false)
	// After a crash at this point, a fresh service still sees the position.
	if pos, err := f.service().Resume(ctx, f.user, m.Work, m.Version); err != nil || pos != 10*time.Minute {
		t.Fatalf("after a crash: position %v, %v", pos, err)
	}
}

// A position belongs to the root work and resumes on any version of the
// edition it was saved on. A version of another edition starts from the
// beginning, and the version last played is remembered.
func TestResumeKeepsToItsEdition(t *testing.T) {
	f := newFixture(t)
	m := f.movie("Blade Runner", 2*time.Hour)
	uhd := f.b.File(f.folder, "Blade Runner (2000)/Blade Runner (2000) 2160p.mkv")
	f.setDuration(uhd, 2*time.Hour)
	same := f.b.Version([]id.ID{uhd}, m.Work)
	cutFile := f.b.File(f.folder, "Blade Runner (2000)/Blade Runner (2000) {edition-Final Cut}.mkv")
	f.setDuration(cutFile, 2*time.Hour)
	cut := f.b.Version([]id.ID{cutFile}, m.Work)
	f.b.Write(func(ctx context.Context, tx *store.Tx) error {
		return f.b.Catalog.Batch(ctx, tx, func(b *catalog.Batch) error { return b.SetVersionEdition(ctx, cut, "Final Cut") })
	})

	f.play(id.New(), m.Version, 30*time.Minute)
	if last, err := f.s.LastVersion(ctx, f.user, m.Work); err != nil || last != m.Version {
		t.Fatalf("last version %v, %v; want the one played", last, err)
	}
	for _, c := range []struct {
		name    string
		version id.ID
		want    time.Duration
	}{{"the version played", m.Version, 30 * time.Minute}, {"another of its edition", same, 30 * time.Minute}, {"another edition", cut, 0}} {
		if pos, err := f.s.Resume(ctx, f.user, m.Work, c.version); err != nil || pos != c.want {
			t.Errorf("%s resumes at %v, %v; want %v", c.name, pos, err, c.want)
		}
	}
}

func TestAPlayCountsOnce(t *testing.T) {
	f := newFixture(t)
	m := f.movie("Heat", 2*time.Hour)
	playID, at := id.New(), time.Now()
	f.report(playID, m.Version, 115*time.Minute, at, false)
	f.report(playID, m.Version, 116*time.Minute, at.Add(10*time.Second), false)
	f.report(playID, m.Version, 116*time.Minute, at.Add(10*time.Second), true)
	if st := f.state(f.user, m.Work); st.PlayCount != 1 {
		t.Fatalf("play count %d, want 1", st.PlayCount)
	}
}

// A heartbeat that raced the stop arrives after the stop, but is older, so
// it doesn't undo the stop.
func TestALateReportNeverUndoesTheStop(t *testing.T) {
	f := newFixture(t)
	m := f.movie("Heat", 2*time.Hour)
	playID, at := id.New(), time.Now()
	f.report(playID, m.Version, 30*time.Minute, at, true)
	f.report(playID, m.Version, 116*time.Minute, at.Add(-time.Second), false)
	f.report(playID, m.Version, 31*time.Minute, at, false)
	if st := f.state(f.user, m.Work); st.Played || st.PositionMs != (30*time.Minute).Milliseconds() {
		t.Fatalf("after a late report: %+v", st)
	}
}

func TestAFailedWriteReturnsItsError(t *testing.T) {
	f := newFixture(t)
	m := f.movie("Heat", 2*time.Hour)
	playID := id.New()
	failed, stop := context.WithCancel(ctx)
	stop()
	err := f.s.Report(failed, userstate.Progress{Play: playID, User: f.user, Version: m.Version, Position: 40 * time.Minute, At: time.Now()})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("a report with a cancelled context: %v", err)
	}
	// The next heartbeat writes the play's position.
	f.report(playID, m.Version, 40*time.Minute+10*time.Second, time.Now(), false)
	if pos, _ := f.s.Resume(ctx, f.user, m.Work, m.Version); pos != 40*time.Minute+10*time.Second {
		t.Fatalf("position %v after the next report", pos)
	}
}

func TestContinueWatchingFloorAndPlayedThreshold(t *testing.T) {
	f := newFixture(t)
	m := f.movie("Heat", 100*time.Minute)
	for _, c := range []struct {
		pos               time.Duration
		played, resumable bool
		position          time.Duration
	}{
		{90 * time.Second, false, false, 90 * time.Second}, // under 2 minutes: saved but hidden
		{4 * time.Minute, false, false, 4 * time.Minute},   // under 5%: saved but hidden
		{6 * time.Minute, false, true, 6 * time.Minute},
		{91 * time.Minute, true, false, 0}, // the threshold: played, back to the start
	} {
		f.play(id.New(), m.Version, c.pos)
		st := f.state(f.user, m.Work)
		if st.Played != c.played || st.Resumable != c.resumable || st.PositionMs != c.position.Milliseconds() {
			t.Errorf("at %v: %+v", c.pos, st)
		}
	}
	// The threshold is read from settings at each report.
	f.setThreshold(0.95)
	other := f.movie("Ronin", 100*time.Minute)
	f.play(id.New(), other.Version, 91*time.Minute)
	if st := f.state(f.user, other.Work); st.Played || !st.Resumable {
		t.Errorf("91%% at a 95%% threshold: %+v", st)
	}
	// Only the threshold counts, whatever the length or time left.
	clip := f.movie("Trailer", 90*time.Second)
	f.play(id.New(), clip.Version, 30*time.Second)
	if st := f.state(f.user, clip.Work); st.Played {
		t.Errorf("a 90-second clip 30 seconds in: %+v", st)
	}
	f.setThreshold(1.0)
	last := f.movie("Thief", 100*time.Minute)
	f.play(id.New(), last.Version, 99*time.Minute)
	if st := f.state(f.user, last.Work); st.Played {
		t.Errorf("a minute from the end at a 100%% threshold: %+v", st)
	}
}

// A book ignores the played threshold. It is finished with 5 minutes or
// less left, and in progress after 2 minutes whatever its length.
func TestBookFinishedWithFiveMinutesLeft(t *testing.T) {
	f := newFixture(t)
	_, folder := f.b.Library(catalog.Audiobooks)
	book := func(title string, parts ...time.Duration) (work, version id.ID) {
		work = f.b.Work(catalog.WorkInput{Kind: catalog.KindBook, Title: title})
		var files []id.ID
		for i, d := range parts {
			file := f.b.File(folder, fmt.Sprintf("%s/%d.mp3", title, i+1))
			f.setDuration(file, d)
			files = append(files, file)
		}
		return work, f.b.Version(files, work)
	}
	work, version := book("Dune", 10*time.Hour, 11*time.Hour)
	for _, c := range []struct {
		pos               time.Duration
		played, resumable bool
		position          time.Duration
	}{
		{90 * time.Second, false, false, 90 * time.Second}, // under 2 minutes: saved but hidden
		{3 * time.Minute, false, true, 3 * time.Minute},    // under 5% is in progress for a book
		{19 * time.Hour, false, true, 19 * time.Hour},      // past the played threshold, still listening
		{21*time.Hour - 6*time.Minute, false, true, 21*time.Hour - 6*time.Minute},
		{21*time.Hour - 5*time.Minute, true, false, 0}, // 5 minutes left: finished, back to the start
	} {
		f.play(id.New(), version, c.pos)
		st := f.state(f.user, work)
		if st.Played != c.played || st.Resumable != c.resumable || st.PositionMs != c.position.Milliseconds() {
			t.Errorf("at %v: %+v", c.pos, st)
		}
	}
	// A short book is finished in its last tenth, not from its first minute.
	short, shortVersion := book("Novella", 20*time.Minute)
	f.play(id.New(), shortVersion, 17*time.Minute)
	if st := f.state(f.user, short); st.Played || !st.Resumable {
		t.Errorf("a 20-minute book 3 minutes from the end: %+v", st)
	}
	f.play(id.New(), shortVersion, 18*time.Minute)
	if st := f.state(f.user, short); !st.Played {
		t.Errorf("a 20-minute book 2 minutes from the end: %+v", st)
	}
}

func TestTwoClientsLatestWinsAndNoPlayIsLost(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := newFixture(t)
		m := f.movie("Heat", 100*time.Minute)
		tv, phone := id.New(), id.New()
		phoneAt := time.Now()
		f.report(phone, m.Version, 40*time.Minute, phoneAt, false)
		time.Sleep(time.Second)
		f.report(tv, m.Version, 30*time.Minute, time.Now(), false)
		f.report(tv, m.Version, 30*time.Minute, time.Now(), true)
		f.report(phone, m.Version, 40*time.Minute, phoneAt, true) // older report, written later
		if pos, _ := f.s.Resume(ctx, f.user, m.Work, m.Version); pos != 30*time.Minute {
			t.Fatalf("position %v, want the latest report (30m)", pos)
		}
		f.play(tv, m.Version, 95*time.Minute)
		f.play(phone, m.Version, 99*time.Minute)
		if st := f.state(f.user, m.Work); st.PlayCount != 2 {
			t.Fatalf("play count %d, want 2", st.PlayCount)
		}
	})
}

func TestDoubleEpisodeIsOneCombinedItem(t *testing.T) {
	f := newFixture(t)
	_, folder := f.b.Library(catalog.Shows)
	series := f.b.Series("Lost")
	e1 := f.b.Episode(folder, series, 1, 1)
	e2 := f.b.Work(catalog.WorkInput{Kind: catalog.KindEpisode, Parent: mustParent(t, f, e1.Work), Title: "Pilot 2", ParentNumber: 1, Number: 2})
	file := f.b.File(folder, "Lost/S01E01-E02.mkv")
	f.setDuration(file, 80*time.Minute)
	v := f.b.Version([]id.ID{file}, e1.Work, e2)

	f.play(id.New(), v, 20*time.Minute)
	if a, b := f.state(f.user, e1.Work), f.state(f.user, e2); a.PositionMs != b.PositionMs || !a.Resumable {
		t.Fatalf("progress differs: %+v %+v", a, b)
	}
	f.play(id.New(), v, 79*time.Minute)
	if a, b := f.state(f.user, e1.Work), f.state(f.user, e2); !a.Played || !b.Played {
		t.Fatalf("finishing the file: %+v %+v", a, b)
	}
}

func mustParent(t *testing.T, f *fixture, work id.ID) id.ID {
	w, err := f.b.Catalog.Work(ctx, work)
	if err != nil {
		t.Fatal(err)
	}
	return w.Parent
}

func TestMultiPartVersionResumesInTheRightPart(t *testing.T) {
	f := newFixture(t)
	movie := f.b.Work(catalog.WorkInput{Kind: catalog.KindMovie, Title: "Lawrence"})
	cd1, cd2 := f.b.File(f.folder, "L/L cd1.mkv"), f.b.File(f.folder, "L/L cd2.mkv")
	f.setDuration(cd1, 110*time.Minute)
	f.setDuration(cd2, 112*time.Minute)
	v := f.b.Version([]id.ID{cd1, cd2}, movie)

	f.play(id.New(), v, 150*time.Minute)
	pos, err := f.s.Resume(ctx, f.user, movie, v)
	if err != nil {
		t.Fatal(err)
	}
	if part, off := locate([]time.Duration{110 * time.Minute, 112 * time.Minute}, pos); part != 1 || off != 40*time.Minute {
		t.Fatalf("resume at part %d offset %v, want part 1 at 40m", part, off)
	}
}

// locate finds which of a version's parts holds a position on the
// version's timeline, and the offset into that part.
func locate(parts []time.Duration, pos time.Duration) (part int, offset time.Duration) {
	for i, d := range parts {
		if pos < d || i == len(parts)-1 {
			return i, min(pos, d)
		}
		pos -= d
	}
	return 0, 0
}

// Each Update that commits publishes userStateChanged to its user, naming
// the works it changed. A series marked played is named with its episodes.
// An Update that fails publishes nothing.
func TestUpdateTellsChanged(t *testing.T) {
	f := newFixture(t)
	m := f.movie("Heat", 2*time.Hour)
	if err := f.s.Update(ctx, f.viewer(), m.Work, userstate.Change{Favorite: new(true), Played: new(true)}); err != nil {
		t.Fatal(err)
	}
	if got := f.told(); len(got) != 1 || got[0].user != f.user || !slices.Equal(got[0].works, []id.ID{m.Work}) {
		t.Fatalf("told %+v; want the user and Heat", got)
	}
	other := f.movie("Ronin", 2*time.Hour)
	off := &userstate.Offline{Version: other.Version, Position: time.Minute, At: time.Now()}
	if err := f.s.Update(ctx, f.viewer(), m.Work, userstate.Change{Offline: off}); err == nil {
		t.Fatal("an offline position of another work's version was written")
	}
	if got := f.told(); len(got) != 0 {
		t.Fatalf("told %+v after a failed update", got)
	}

	_, folder := f.b.Library(catalog.Shows)
	series := f.b.Series("The Wire")
	e1, e2 := f.b.Episode(folder, series, 1, 1), f.b.Episode(folder, series, 2, 1)
	if err := f.s.Update(ctx, f.viewer(), series, userstate.Change{Played: new(true)}); err != nil {
		t.Fatal(err)
	}
	if got := f.told(); len(got) != 1 || !slices.Equal(got[0].works, []id.ID{series, e1.Work, e2.Work}) {
		t.Fatalf("told %+v; want the series and its episodes", got)
	}
}

// A play's stop publishes userStateChanged to its user once its last
// position is written, naming its version's works. A report during the
// play publishes nothing.
func TestStopTellsChanged(t *testing.T) {
	f := newFixture(t)
	m := f.movie("Heat", 2*time.Hour)
	play := id.New()
	f.report(play, m.Version, 10*time.Minute, time.Now(), false)
	if got := f.told(); len(got) != 0 {
		t.Fatalf("told %+v before the stop", got)
	}
	f.play(play, m.Version, 20*time.Minute)
	if got := f.told(); len(got) != 1 || got[0].user != f.user || !slices.Equal(got[0].works, []id.ID{m.Work}) {
		t.Fatalf("told %+v; want the user and Heat", got)
	}
}

func TestSetPlayedCascades(t *testing.T) {
	f := newFixture(t)
	_, folder := f.b.Library(catalog.Shows)
	series := f.b.Series("The Wire")
	e1, e2 := f.b.Episode(folder, series, 1, 1), f.b.Episode(folder, series, 2, 1)
	if err := f.s.Update(ctx, f.viewer(), series, userstate.Change{Played: new(true)}); err != nil {
		t.Fatal(err)
	}
	for _, e := range []id.ID{e1.Work, e2.Work} {
		if st := f.state(f.user, e); !st.Played || st.PlayCount != 1 {
			t.Errorf("episode %+v", st)
		}
	}
	if st := f.state(f.user, series); st.Played {
		t.Error("the series itself got a row")
	}
	if err := f.s.Update(ctx, f.viewer(), e1.Work, userstate.Change{Played: new(false)}); err != nil {
		t.Fatal(err)
	}
	if st := f.state(f.user, e1.Work); st.Played || st.PlayCount != 1 {
		t.Errorf("unplayed %+v: history keeps the count", st)
	}
}

// episodes adds two episodes of one series, each with a version of the
// given duration.
func (f *fixture) episodes(d time.Duration) (a, b catalogtest.Item) {
	f.t.Helper()
	_, folder := f.b.Library(catalog.Shows)
	series := f.b.Series("Dark")
	a, b = f.b.Episode(folder, series, 1, 1), f.b.Episode(folder, series, 1, 2)
	f.setDuration(a.File, d)
	f.setDuration(b.File, d)
	return a, b
}

func TestFollowVersionMovesOnlyTheFilesPlays(t *testing.T) {
	f := newFixture(t)
	a, b := f.episodes(100 * time.Minute)
	file, err := f.b.Catalog.File(ctx, a.File)
	if err != nil {
		t.Fatal(err)
	}
	other := f.b.File(file.Folder, "Dark/Season 01/S01E01 4K.mkv")
	f.setDuration(other, 100*time.Minute)
	keep := f.b.Version([]id.ID{other}, a.Work)

	f.play(id.New(), keep, 99*time.Minute)      // a play of the version that stays
	f.play(id.New(), a.Version, 99*time.Minute) // a play of the version that moves
	f.play(id.New(), a.Version, 40*time.Minute) // and the resume position
	if err := f.s.Update(ctx, f.viewer(), a.Work, userstate.Change{Favorite: new(true)}); err != nil {
		t.Fatal(err)
	}

	f.b.Write(func(ctx context.Context, tx *store.Tx) error {
		return f.b.Catalog.Batch(ctx, tx, func(cb *catalog.Batch) error {
			r, err := cb.ReassignVersion(ctx, a.Version, []id.ID{b.Work})
			if err != nil {
				return err
			}
			return f.s.FollowVersion(ctx, tx, r)
		})
	})
	sa, sb := f.state(f.user, a.Work), f.state(f.user, b.Work)
	if sa.PlayCount != 1 || !sa.Played || !sa.Favorite || sa.Resumable {
		t.Errorf("the old episode keeps the rest: %+v", sa)
	}
	if sb.PlayCount != 1 || sb.PositionMs != (40*time.Minute).Milliseconds() || sb.Favorite {
		t.Errorf("the new episode gets the file's play and position: %+v", sb)
	}
}

func TestMergeEpisodeUnionsState(t *testing.T) {
	f := newFixture(t)
	a, b := f.episodes(100 * time.Minute)
	f.play(id.New(), a.Version, 99*time.Minute)
	f.play(id.New(), a.Version, 99*time.Minute)
	f.play(id.New(), b.Version, 99*time.Minute)
	f.play(id.New(), b.Version, 20*time.Minute) // the latest position
	if err := f.s.Update(ctx, f.viewer(), a.Work, userstate.Change{Favorite: new(true)}); err != nil {
		t.Fatal(err)
	}
	favorited := f.state(f.user, a.Work).FavoritedAt
	second := f.newUser("ben")
	ben, err := users.New(f.b.DB, events.NewBus()).Viewer(ctx, second)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.s.Update(ctx, ben, a.Work, userstate.Change{Played: new(true)}); err != nil { // by hand: no event
		t.Fatal(err)
	}

	f.b.Write(func(ctx context.Context, tx *store.Tx) error {
		if err := f.s.MergeEpisode(ctx, tx, a.Work, b.Work); err != nil {
			return err
		}
		return f.b.Catalog.Batch(ctx, tx, func(cb *catalog.Batch) error { return cb.MergeEpisode(ctx, a.Work, b.Work) })
	})
	st := f.state(f.user, b.Work)
	if !st.Played || st.PlayCount != 3 || !st.Favorite || st.PositionMs != (20*time.Minute).Milliseconds() {
		t.Fatalf("merged %+v; want played, 3 plays from the events, favorite, the latest position", st)
	}
	if !favorited.Valid || st.FavoritedAt != favorited {
		t.Errorf("merged favorited at %v, want %v", st.FavoritedAt, favorited)
	}
	if st := f.state(second, b.Work); !st.Played || st.PlayCount != 1 {
		t.Fatalf("a hand-marked play must survive the merge: %+v", st)
	}
	if _, err := f.b.Catalog.Work(ctx, a.Work); err == nil {
		t.Fatal("the merged episode survived")
	}
}

// A work purged with nothing left takes its watch state, plays and track
// choices with it.
func TestPurgedWorkTakesItsState(t *testing.T) {
	f := newFixture(t)
	a, _ := f.episodes(100 * time.Minute)
	f.play(id.New(), a.Version, 99*time.Minute)
	if err := f.s.SetTrackChoice(ctx, f.user, a.Work, userstate.TrackChoice{AudioLanguage: "de"}); err != nil {
		t.Fatal(err)
	}
	f.b.Write(func(ctx context.Context, tx *store.Tx) error {
		_, err := tx.ExecContext(ctx, `UPDATE media_file SET version_id = NULL, part = NULL WHERE version_id = ?`, a.Version)
		if err == nil {
			_, err = tx.ExecContext(ctx, `DELETE FROM media_version WHERE id = ?`, a.Version)
		}
		if err == nil {
			_, err = f.b.Catalog.DeleteEmptyWorks(ctx, tx)
		}
		return err
	})
	var n int
	err := f.b.DB.Reader().QueryRow(`SELECT (SELECT count(*) FROM user_state WHERE work_id = ?1) + (SELECT count(*) FROM play_event WHERE work_id = ?1)
		+ (SELECT count(*) FROM track_choice WHERE series_id = ?1)`, a.Work).Scan(&n)
	if err != nil || n != 0 {
		t.Fatalf("%d rows of the purged work's state left (%v)", n, err)
	}
}

func TestTrackChoice(t *testing.T) {
	f := newFixture(t)
	series := f.b.Series("Dark")
	if _, ok, _ := f.s.TrackChoice(ctx, f.user, series); ok {
		t.Fatal("a choice before any was made")
	}
	want := userstate.TrackChoice{AudioLanguage: "de", SubtitleLanguage: "en", SubtitleKind: "full"}
	if err := f.s.SetTrackChoice(ctx, f.user, series, want); err != nil {
		t.Fatal(err)
	}
	if got, ok, err := f.s.TrackChoice(ctx, f.user, series); !ok || err != nil || got != want {
		t.Fatalf("got %+v %v %v", got, ok, err)
	}
}

func TestOfflineProgressAppliesOnlyWhenNewer(t *testing.T) {
	f := newFixture(t)
	_, folder := f.b.Library(catalog.Audiobooks)
	work := f.b.Work(catalog.WorkInput{Kind: catalog.KindBook, Title: "Dune"})
	file := f.b.File(folder, "Dune/Dune.m4b")
	f.setDuration(file, 20*time.Hour)
	version := f.b.Version([]id.ID{file}, work)
	offline := func(pos time.Duration, at time.Time) error {
		return f.s.Update(ctx, f.viewer(), work, userstate.Change{Offline: &userstate.Offline{Version: version, Position: pos, At: at}})
	}
	plays := func() (n, completed int) {
		t.Helper()
		err := f.b.DB.Reader().QueryRow(`SELECT count(*), coalesce(sum(completed), 0) FROM play_event WHERE user_id = ? AND work_id = ?`, f.user, work).Scan(&n, &completed)
		if err != nil {
			t.Fatal(err)
		}
		return n, completed
	}

	// Listened to on the server just now, and offline two hours ago.
	f.play(id.New(), version, time.Hour)
	if err := offline(3*time.Hour, time.Now().Add(-2*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if st := f.state(f.user, work); st.PositionMs != time.Hour.Milliseconds() {
		t.Fatalf("an older offline position was applied: %+v", st)
	}
	if n, _ := plays(); n != 1 {
		t.Fatalf("an ignored position made a play: %d plays", n)
	}
	at := time.Now().Add(time.Second)
	if err := offline(2*time.Hour, at); err != nil {
		t.Fatal(err)
	}
	if st := f.state(f.user, work); st.PositionMs != 2*time.Hour.Milliseconds() || !st.Resumable {
		t.Fatalf("a newer one: %+v", st)
	}
	// The outbox sending the same entry again changes nothing.
	if err := offline(2*time.Hour, at); err != nil {
		t.Fatal(err)
	}
	if n, _ := plays(); n != 2 {
		t.Fatalf("%d plays, want the play's and the offline one", n)
	}
	if err := offline(20*time.Hour-5*time.Minute, at.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if st := f.state(f.user, work); !st.Played || st.PlayCount != 1 || st.PositionMs != 0 {
		t.Fatalf("5 minutes from the end: %+v", st)
	}
	if n, completed := plays(); n != 3 || completed != 1 {
		t.Fatalf("%d plays, %d completed", n, completed)
	}

	other := f.movie("Heat", 2*time.Hour)
	err := f.s.Update(ctx, f.viewer(), work, userstate.Change{Offline: &userstate.Offline{Version: other.Version, Position: time.Hour, At: time.Now()}})
	if !errors.Is(err, catalog.ErrNotFound) {
		t.Fatalf("another work's version: %v", err)
	}
}

// Removing a work from Continue Watching keeps its position and played
// state, and tells the user's clients. An episode marks its series.
func TestRemoveFromContinue(t *testing.T) {
	f := newFixture(t)
	m := f.movie("Heat", 2*time.Hour)
	f.play(id.New(), m.Version, 30*time.Minute)
	f.told()
	if err := f.s.RemoveFromContinue(ctx, f.viewer(), m.Work); err != nil {
		t.Fatal(err)
	}
	if st := f.state(f.user, m.Work); st.PositionMs != (30*time.Minute).Milliseconds() || !st.Resumable {
		t.Errorf("after removing: %+v, want the position kept", st)
	}
	if pos, _ := f.s.Resume(ctx, f.user, m.Work, m.Version); pos != 30*time.Minute {
		t.Errorf("resumes at %v after removing", pos)
	}
	if got := f.told(); len(got) != 1 || !slices.Equal(got[0].works, []id.ID{m.Work}) {
		t.Errorf("told %+v; want Heat", got)
	}

	a, _ := f.episodes(time.Hour)
	f.play(id.New(), a.Version, time.Hour)
	f.told()
	if err := f.s.RemoveFromContinue(ctx, f.viewer(), a.Work); err != nil {
		t.Fatal(err)
	}
	series := mustParent(t, f, mustParent(t, f, a.Work))
	var marked id.ID
	err := f.b.DB.Reader().QueryRow(`SELECT work_id FROM user_state WHERE user_id = ? AND continue_removed_at IS NOT NULL AND work_id <> ?`, f.user, m.Work).Scan(&marked)
	if err != nil || marked != series {
		t.Errorf("marked %v (%v), want the series %v", marked, err, series)
	}
	if st := f.state(f.user, a.Work); !st.Played || st.PlayCount != 1 {
		t.Errorf("the episode after removing: %+v, want it played", st)
	}
	if got := f.told(); len(got) != 1 || !slices.Equal(got[0].works, []id.ID{a.Work, series}) {
		t.Errorf("told %+v; want the episode and its series", got)
	}
}

// A position before the Continue Watching floor starts again from the
// beginning, because the pages show Play for it, not Resume.
func TestResumeBelowFloorStartsOver(t *testing.T) {
	f := newFixture(t)
	m := f.movie("Heat", 2*time.Hour)
	f.report(id.New(), m.Version, 3*time.Minute+53*time.Second, time.Now(), false)
	if pos, _ := f.s.Resume(ctx, f.user, m.Work, m.Version); pos != 0 {
		t.Fatalf("resumes at %v, before the floor", pos)
	}
}
