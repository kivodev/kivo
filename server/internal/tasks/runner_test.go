package tasks

import (
	"context"
	"errors"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/mmcdole/kivo/internal/config"
	"github.com/mmcdole/kivo/internal/events"
	"github.com/mmcdole/kivo/internal/id"
	"github.com/mmcdole/kivo/internal/store"
	"github.com/mmcdole/kivo/internal/store/storetest"
)

// feature stands in for an item task's tables, in memory. It holds the
// items left and when each item was settled.
type feature struct {
	mu      sync.Mutex
	left    []id.ID
	settled map[id.ID]time.Time
	failing map[id.ID]bool // Do fails these, leaving them pending
	took    time.Duration
	order   []id.ID // every Do, in order
}

func newFeature(n int, took time.Duration) *feature {
	f := &feature{settled: map[id.ID]time.Time{}, failing: map[id.ID]bool{}, took: took}
	for range n {
		f.left = append(f.left, id.New())
	}
	return f
}

func (f *feature) task(name string) Task {
	return Task{
		Name: name, Nightly: true,
		Pending: func(context.Context) ([]id.ID, error) {
			f.mu.Lock()
			defer f.mu.Unlock()
			return slices.Clone(f.left), nil
		},
		Do: func(ctx context.Context, i id.ID) error {
			f.mu.Lock()
			f.order = append(f.order, i)
			fail, pending := f.failing[i], slices.Contains(f.left, i)
			f.mu.Unlock()
			if !pending {
				return nil
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(f.took):
			}
			if fail {
				return errors.New("ffmpeg crashed")
			}
			f.mu.Lock()
			defer f.mu.Unlock()
			f.left = slices.DeleteFunc(f.left, func(x id.ID) bool { return x == i })
			f.settled[i] = time.Now()
			return nil
		},
		Left: func(context.Context) (int, error) {
			f.mu.Lock()
			defer f.mu.Unlock()
			return len(f.left), nil
		},
	}
}

func (f *feature) done() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.settled)
}

// harness runs a runner inside a synctest bubble. The database outlives
// the runner, the same way it outlives a server restart.
type harness struct {
	t        *testing.T
	db       *store.DB
	settings *config.Store
	tasks    []Task
	opt      Options
	r        *Runner
	cancel   context.CancelFunc
	stopped  chan struct{}
}

func newHarness(t *testing.T, tasks ...Task) *harness {
	t.Helper()
	db := storetest.New(t)
	settings, err := config.Open(context.Background(), db, config.Options{})
	if err != nil {
		t.Fatal(err)
	}
	return &harness{t: t, db: db, settings: settings, tasks: tasks}
}

// window sets the maintenance window to open in `in` hours, for `long`.
func (h *harness) window(in, long int) {
	h.t.Helper()
	start := time.Now().Add(time.Duration(in) * time.Hour).Local().Hour()
	err := h.settings.Update(context.Background(), func(s *config.Settings) error {
		s.MaintenanceStart, s.MaintenanceEnd = start, (start+long)%24
		return nil
	})
	if err != nil {
		h.t.Fatal(err)
	}
}

func (h *harness) start() {
	h.r = New(h.db, h.settings, events.NewBus(), h.opt)
	for _, t := range h.tasks {
		h.r.Add(t)
	}
	ctx, cancel := context.WithCancel(context.Background())
	h.cancel, h.stopped = cancel, make(chan struct{})
	go func() {
		defer close(h.stopped)
		if err := h.r.Run(ctx); err != nil {
			h.t.Error(err)
		}
	}()
	synctest.Wait()
}

func (h *harness) stop() {
	h.cancel()
	<-h.stopped
}

func (h *harness) status(name string) Status {
	h.t.Helper()
	all, err := h.r.Tasks(context.Background())
	if err != nil {
		h.t.Fatal(err)
	}
	for _, s := range all {
		if s.Name == name {
			return s
		}
	}
	h.t.Fatalf("no task %s", name)
	return Status{}
}

// sleep moves the bubble's clock on and lets everything settle.
func sleep(d time.Duration) {
	time.Sleep(d)
	synctest.Wait()
}

// A night run ends when the window closes, recording what it did and what
// is left. The next night's run is a separate run.
func TestANightRunEndsAtTheWindowsClose(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := newFeature(6, 25*time.Minute)
		h := newHarness(t, f.task("markers"))
		h.window(1, 2)
		h.start()
		if s := h.status("markers"); s.Running || s.Left != 6 {
			t.Fatalf("before the window: %+v, want 6 left and idle", s)
		}

		sleep(2 * time.Hour) // an hour into the window: two items done
		if s := h.status("markers"); !s.Running || s.Done != 2 || s.Total != 6 {
			t.Fatalf("inside the window: running %v, %d of %d, want 2 of 6", s.Running, s.Done, s.Total)
		}
		sleep(time.Hour + time.Minute) // the window closes, with the fifth in flight
		if s := h.status("markers"); s.Running || s.Left != 2 || s.Last == nil || s.Last.Summary != "4 done, 2 left" || f.done() != 4 {
			t.Fatalf("after the window: %+v with %d done, want the run ended with 4 done, 2 left", s, f.done())
		}

		h.stop() // a restart during the day
		h.start()
		if s := h.status("markers"); s.Running || s.Left != 2 || s.Last == nil || s.Last.Summary != "4 done, 2 left" {
			t.Fatalf("after a restart: %+v, want idle with the last run kept", s)
		}

		sleep(22*time.Hour + 30*time.Minute) // half an hour into the next night
		if s := h.status("markers"); !s.Running || s.Done != 1 || s.Total != 2 {
			t.Fatalf("the next night: %d of %d, want 1 of 2: a run of its own", s.Done, s.Total)
		}
		sleep(time.Hour)
		h.stop()
		s := h.status("markers")
		if s.Running || s.Last == nil || s.Last.Summary != "2 done" || f.done() != 6 {
			t.Fatalf("the end: %+v, want the run ended with 2 done", s.Last)
		}
	})
}

// A server started inside the window begins the night's run at startup, if
// no run has ended since the window opened.
func TestARestartInsideTheWindowBeginsTheNightsRun(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := newFeature(6, 25*time.Minute)
		h := newHarness(t, f.task("markers"))
		h.window(1, 3)
		h.start()
		sleep(90 * time.Minute) // half an hour into the window: one item done
		h.stop()                // a restart

		h.start()
		if s := h.status("markers"); !s.Running || s.Last != nil {
			t.Fatalf("after a restart inside the window: %+v, want a run begun", s)
		}
		sleep(2*time.Hour + 10*time.Minute)
		h.stop()
		if s := h.status("markers"); s.Running || s.Last == nil || s.Last.Summary != "5 done" || f.done() != 6 {
			t.Fatalf("the end: %+v with %d done, want the run ended with 5 done", s.Last, f.done())
		}
	})
}

// A night task whose run ended after the window opened doesn't run again
// that night, whether after a restart or on a later tick.
func TestARunEndedTonightIsntBegunAgain(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := newFeature(2, time.Minute)
		h := newHarness(t, f.task("markers"))
		h.window(1, 3)
		h.start()
		sleep(70 * time.Minute) // the run ends a few minutes into the window
		if s := h.status("markers"); s.Running || s.Last == nil || s.Last.Summary != "2 done" {
			t.Fatalf("inside the window: %+v, want the run ended with 2 done", s)
		}
		f.mu.Lock()
		f.left = append(f.left, id.New())
		f.mu.Unlock()
		h.stop() // a restart

		h.start()
		sleep(time.Hour)
		h.stop()
		if s := h.status("markers"); s.Running || s.Left != 1 || s.Last.Summary != "2 done" || f.done() != 2 {
			t.Fatalf("after a restart: %+v with %d done, want no run until the next night", s, f.done())
		}
	})
}

func TestRunNowGoesOnThroughTheWindowsClose(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := newFeature(10, time.Hour)
		h := newHarness(t, f.task("previews"))
		h.window(1, 1)
		h.start()
		if err := h.r.Start(context.Background(), "previews"); err != nil {
			t.Fatal(err)
		}
		sleep(4*time.Hour + time.Minute) // through the window
		if f.done() != 4 {
			t.Fatalf("%d done, want 4: Run now doesn't stop at the window's close", f.done())
		}
		sleep(7 * time.Hour)
		h.stop()
		if s := h.status("previews"); s.Last == nil || s.Last.Summary != "10 done" {
			t.Fatalf("last run %+v, want 10 done", s.Last)
		}
	})
}

func TestAsksRunNowWithoutTheBacklog(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := newFeature(5, time.Minute)
		task := f.task("markers")
		task.Limit = func(config.Settings) int { return 1 }
		h := newHarness(t, task)
		h.window(6, 2)
		h.start()

		// Asked items run in the order asked, one at a time within the
		// limit. An item asked for again while it waits keeps its place.
		ids := slices.Clone(f.left)
		h.r.Ask("markers", ids[3], ids[4])
		synctest.Wait()
		h.r.Ask("markers", ids[0], ids[4])
		synctest.Wait()
		if s := h.status("markers"); !s.Running || len(s.Items) != 1 || s.Items[0].ID != ids[3] {
			t.Fatalf("%+v: want the first asked in flight, alone", s.Items)
		}
		if asked := h.r.Asked("markers"); !slices.Equal(asked, []id.ID{ids[4], ids[0]}) {
			t.Fatalf("asked %v, want the rest in the order asked", asked)
		}
		sleep(3 * time.Minute)
		if want := []id.ID{ids[3], ids[4], ids[0]}; !slices.Equal(f.order, want) || f.done() != 3 {
			t.Fatalf("did %v, want %v and no more outside the window", f.order, want)
		}
		h.stop()
		if s := h.status("markers"); s.Last != nil {
			t.Fatalf("asks ended a run: %+v", s.Last)
		}
	})
}

func TestATransientFailureWaitsForTheNextPass(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := newFeature(3, time.Minute)
		f.failing[f.left[1]] = true
		h := newHarness(t, f.task("previews"))
		h.window(6, 2)
		h.start()
		if err := h.r.Start(context.Background(), "previews"); err != nil {
			t.Fatal(err)
		}
		sleep(time.Hour)
		h.stop()
		if s := h.status("previews"); s.Last == nil || s.Last.Summary != "2 done, 1 left" || len(f.order) != 3 {
			t.Fatalf("%+v after %d tries, want the run ended with the failure left, tried once", s.Last, len(f.order))
		}
	})
}

func TestStopEndsTheRun(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := newFeature(4, time.Hour)
		h := newHarness(t, f.task("previews"))
		h.window(6, 2)
		h.start()
		if err := h.r.Start(context.Background(), "previews"); err != nil {
			t.Fatal(err)
		}
		sleep(90 * time.Minute)
		if err := h.r.Stop(context.Background(), "previews"); err != nil {
			t.Fatal(err)
		}
		sleep(2 * time.Hour)
		h.stop()
		s := h.status("previews")
		if s.Running || s.Last == nil || s.Last.Summary != "Stopped: 1 done, 3 left" || f.done() != 1 {
			t.Fatalf("%+v, %d done: want stopped after the first", s.Last, f.done())
		}
	})
}

// While a run's end is being recorded, the task still shows as running and
// its last run is unchanged. A Run now in that time begins a new run, which
// stays open after the old run's end is recorded.
func TestARunEndsInOneStep(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := newFeature(4, time.Hour)
		task := f.task("previews")
		left := task.Left
		var holding atomic.Bool
		gate := make(chan struct{})
		task.Left = func(ctx context.Context) (int, error) {
			if holding.CompareAndSwap(true, false) {
				<-gate
			}
			return left(ctx)
		}
		h := newHarness(t, task)
		h.window(6, 2)
		h.start()
		if err := h.r.Start(context.Background(), "previews"); err != nil {
			t.Fatal(err)
		}
		sleep(90 * time.Minute)
		holding.Store(true)
		go func() {
			if err := h.r.Stop(context.Background(), "previews"); err != nil {
				t.Error(err)
			}
		}()
		synctest.Wait()
		if s := h.status("previews"); !s.Running || s.Last != nil {
			t.Fatalf("while the end is recorded: %+v, want running with no last run yet", s)
		}
		if err := h.r.Start(context.Background(), "previews"); err != nil {
			t.Fatal(err)
		}
		close(gate)
		synctest.Wait()
		if s := h.status("previews"); !s.Running || s.Last == nil || s.Last.Summary != "Stopped: 1 done, 3 left" {
			t.Fatalf("after: %+v, want the stopped run recorded and the new one running", s.Last)
		}
		h.stop()
	})
}

func TestPlainRuns(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		runs := 0
		h := newHarness(t,
			Task{Name: "make-backup", Nightly: true, Maintenance: true, Run: func(ctx context.Context) error {
				runs++
				Summarize(ctx, "Backed up")
				return nil
			}},
			Task{Name: "optimize-database", Nightly: true, Maintenance: true, Run: func(ctx context.Context) error {
				return errors.New("disk I/O error")
			}},
		)
		h.window(1, 1)
		h.start()
		sleep(90 * time.Minute)
		h.stop()
		if s := h.status("make-backup"); runs != 1 || s.Last == nil || s.Last.Summary != "Backed up" || s.Section() != SectionMaintenance {
			t.Fatalf("backup ran %d times, last %+v", runs, s.Last)
		}
		if s := h.status("optimize-database"); s.Last == nil || s.Last.Error != "disk I/O error" {
			t.Fatalf("a failed run: %+v", s.Last)
		}
	})
}

// A scheduled task with nothing wanted of it shows no next run.
func TestUnwantedHasNoNextRun(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		wanted := false
		run := func(context.Context) error { return nil }
		h := newHarness(t,
			Task{Name: "make-previews", Nightly: true, Run: run, Wanted: func(context.Context) (bool, error) { return wanted, nil }},
			Task{Name: "make-backup", Nightly: true, Maintenance: true, Run: run},
		)
		h.start()
		defer h.stop()
		if s := h.status("make-previews"); !s.Next.IsZero() {
			t.Fatalf("next run %v of a task nothing wants", s.Next)
		}
		if s := h.status("make-backup"); s.Next.IsZero() {
			t.Fatal("a task without Wanted has no next run")
		}
		wanted = true
		if s := h.status("make-previews"); s.Next.IsZero() {
			t.Fatal("a wanted task has no next run")
		}
	})
}

func TestActivityDoesWhatItFindsAtStartup(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := newFeature(2, time.Minute)
		task := f.task("downloads")
		task.Nightly, task.Left, task.Finish = false, nil, true
		h := newHarness(t, task)
		h.window(6, 2)
		h.start()
		if s := h.status("downloads"); !s.Running || len(s.Items) != 1 || s.Total != 0 {
			t.Fatalf("%+v, want one found at startup in flight, with no count", s)
		}
		sleep(3 * time.Minute)
		if f.done() != 2 {
			t.Fatalf("%d done, want 2", f.done())
		}
		if err := h.r.Stop(context.Background(), "downloads"); !errors.Is(err, ErrNotStoppable) {
			t.Fatalf("stop: %v, want ErrNotStoppable", err)
		}
		h.stop()
	})
}

// Options.Ended is called, in the write that records the run, for night
// runs and Run now runs that did something or failed. It isn't called for
// a run with nothing to do, or for a scheduled run that isn't a night run.
func TestEndedHearsOfRunsThatDidSomething(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f, idle := newFeature(3, time.Minute), newFeature(0, time.Minute)
		f.failing[f.left[0]] = true
		every := 0
		h := newHarness(t, f.task("make-previews"), idle.task("find-markers"),
			Task{Name: "optimize-database", Nightly: true, Maintenance: true, Run: func(ctx context.Context) error {
				return errors.New("disk I/O error")
			}},
			Task{Name: "scan-libraries", Every: 2 * time.Hour, Run: func(ctx context.Context) error {
				every++
				Summarize(ctx, "Scanned")
				return nil
			}},
		)
		var told []RunEnd
		h.opt.Ended = func(_ context.Context, tx *store.Tx, e RunEnd) error {
			if tx == nil {
				t.Error("Ended was told outside the run's write")
			}
			told = append(told, e)
			return nil
		}
		h.window(1, 1)
		h.start()
		sleep(150 * time.Minute)
		if every != 1 {
			t.Fatalf("the scheduled run ran %d times", every)
		}
		if err := h.r.Start(context.Background(), "scan-libraries"); err != nil {
			t.Fatal(err)
		}
		sleep(time.Minute)
		h.stop()
		got := map[string]RunEnd{}
		for _, e := range told {
			got[e.Task] = e
		}
		if len(told) != 3 || got["make-previews"].Summary != "2 done, 1 left" || got["optimize-database"].Error != "disk I/O error" ||
			got["scan-libraries"].Summary != "Scanned" {
			t.Fatalf("told %+v", told)
		}
	})
}

// plain is a plain task's Run. It reports progress and takes a while,
// unless it's cancelled.
type plain struct {
	mu    sync.Mutex
	calls int
	ended int // calls that weren't cancelled
	took  time.Duration
	err   error
}

func (p *plain) task(name string) Task {
	return Task{Name: name, Nightly: true, Maintenance: true, Run: func(ctx context.Context) error {
		p.mu.Lock()
		p.calls++
		p.mu.Unlock()
		Progress(ctx, "", 1, 4)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(p.took):
		}
		p.mu.Lock()
		p.ended++
		p.mu.Unlock()
		Summarize(ctx, "Removed 3")
		return p.err
	}}
}

func (p *plain) counts() (calls, ended int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.calls, p.ended
}

func TestRunNowOnAPlainTask(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		p := &plain{took: time.Hour}
		h := newHarness(t, p.task("prune-previews"))
		var told []RunEnd
		h.opt.Ended = func(_ context.Context, _ *store.Tx, e RunEnd) error {
			told = append(told, e)
			return nil
		}
		h.window(6, 2)
		h.start()
		if err := h.r.Start(context.Background(), "prune-previews"); err != nil {
			t.Fatal(err)
		}
		synctest.Wait()
		s := h.status("prune-previews")
		if !s.Running || s.Done != 1 || s.Total != 4 || len(s.Items) != 0 || h.r.Busy()["prune-previews"] != 1 {
			t.Fatalf("in flight: %+v, busy %v, want running at 1 of 4 with no items", s, h.r.Busy())
		}
		sleep(time.Hour)
		h.stop()
		s = h.status("prune-previews")
		if calls, _ := p.counts(); calls != 1 || s.Running || s.Last == nil || s.Last.Summary != "Removed 3" || s.Last.Error != "" {
			t.Fatalf("after %d calls: %+v, want one run ended with its summary", calls, s)
		}
		if len(told) != 1 || told[0].Summary != "Removed 3" {
			t.Fatalf("told %+v", told)
		}
	})
}

// An error or panic from a plain task's Run becomes the run's error.
func TestAPlainRunFails(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		p := &plain{took: time.Minute, err: errors.New("disk I/O error")}
		h := newHarness(t, p.task("optimize-database"),
			Task{Name: "make-backup", Nightly: true, Maintenance: true, Run: func(context.Context) error { panic("no disk") }},
		)
		h.window(6, 2)
		h.start()
		for _, name := range []string{"optimize-database", "make-backup"} {
			if err := h.r.Start(context.Background(), name); err != nil {
				t.Fatal(err)
			}
		}
		sleep(2 * time.Minute)
		h.stop()
		if s := h.status("optimize-database"); s.Running || s.Last == nil || s.Last.Error != "disk I/O error" || s.Last.Summary != "Removed 3" {
			t.Fatalf("an error: %+v", s.Last)
		}
		if s := h.status("make-backup"); s.Running || s.Last == nil || s.Last.Error != "panic: no disk" {
			t.Fatalf("a panic: %+v", s.Last)
		}
	})
}

func TestStopDuringAPlainRun(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		p := &plain{took: time.Hour}
		h := newHarness(t, p.task("prune-previews"))
		h.window(6, 2)
		h.start()
		if err := h.r.Start(context.Background(), "prune-previews"); err != nil {
			t.Fatal(err)
		}
		sleep(10 * time.Minute)
		if err := h.r.Stop(context.Background(), "prune-previews"); err != nil {
			t.Fatal(err)
		}
		synctest.Wait()
		s := h.status("prune-previews")
		if calls, ended := p.counts(); calls != 1 || ended != 0 || s.Running || s.Last == nil || s.Last.Summary != "Stopped" || s.Last.Error != "" {
			t.Fatalf("after %d calls, %d ended: %+v, want the run cancelled and recorded as stopped", calls, ended, s)
		}
		sleep(2 * time.Hour)
		h.stop()
		if calls, _ := p.counts(); calls != 1 {
			t.Fatalf("%d calls: a stopped run ran again", calls)
		}
	})
}

// A plain night run cut off by the window's close ends as stopped. The
// window's next opening begins another run.
func TestAPlainRunAcrossTheWindow(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		p := &plain{took: 90 * time.Minute}
		h := newHarness(t, p.task("prune-previews"))
		h.window(1, 1)
		h.start()
		sleep(90 * time.Minute) // half an hour into the window
		if s := h.status("prune-previews"); !s.Running || s.Done != 1 || s.Total != 4 {
			t.Fatalf("inside the window: %+v, want running at 1 of 4", s)
		}
		sleep(time.Hour) // the window closes
		if calls, ended := p.counts(); calls != 1 || ended != 0 {
			t.Fatalf("%d calls, %d ended: want the run cut off at the close", calls, ended)
		}
		if s := h.status("prune-previews"); s.Running || s.Last == nil || s.Last.Summary != "Stopped at the window's close" {
			t.Fatalf("after the window: %+v, want the run ended, stopped", s)
		}

		h.stop() // a restart, and a window that opens later
		h.window(1, 3)
		h.start()
		sleep(40 * time.Minute) // ten minutes into the window
		if calls, _ := p.counts(); calls != 2 {
			t.Fatalf("%d calls, want a new run's Run called", calls)
		}
		sleep(2 * time.Hour)
		h.stop()
		s := h.status("prune-previews")
		if calls, ended := p.counts(); calls != 2 || ended != 1 || s.Last == nil || s.Last.Summary != "Removed 3" {
			t.Fatalf("%d calls, %d ended, last %+v: want the run ended in the window", calls, ended, s.Last)
		}
	})
}

// While a plain run's end is being recorded, the task still shows as
// running and its last run is unchanged.
func TestAPlainRunEndsInOneStep(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		p := &plain{took: time.Minute}
		h := newHarness(t, p.task("prune-previews"))
		gate := make(chan struct{})
		h.opt.Ended = func(context.Context, *store.Tx, RunEnd) error {
			<-gate
			return nil
		}
		h.window(6, 2)
		h.start()
		if err := h.r.Start(context.Background(), "prune-previews"); err != nil {
			t.Fatal(err)
		}
		sleep(2 * time.Minute)
		if s := h.status("prune-previews"); !s.Running || s.Last != nil {
			t.Fatalf("while the end is recorded: %+v, want running with no last run yet", s)
		}
		close(gate)
		synctest.Wait()
		h.stop()
		if s := h.status("prune-previews"); s.Running || s.Last == nil || s.Last.Summary != "Removed 3" {
			t.Fatalf("after: %+v, want the run recorded", s)
		}
	})
}
