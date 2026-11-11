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

// task is a nightly task whose work is f's items.
func (f *feature) task(name string) Task {
	return Task{Name: name, Schedule: Nightly, Work: f.items()}
}

func (f *feature) items() Items {
	return Items{
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
		PendingCount: func(context.Context) (int, error) {
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
		if s := h.status("markers"); s.Running || s.PendingCount != 6 {
			t.Fatalf("before the window: %+v, want 6 left and idle", s)
		}

		sleep(2 * time.Hour) // an hour into the window: two items done
		if s := h.status("markers"); !s.Running || s.Done != 2 || s.Total != 6 {
			t.Fatalf("inside the window: running %v, %d of %d, want 2 of 6", s.Running, s.Done, s.Total)
		}
		sleep(time.Hour + time.Minute) // the window closes, with the fifth in flight
		if s := h.status("markers"); s.Running || s.PendingCount != 2 || s.Last == nil || s.Last.Summary != "4 done, 2 left" || f.done() != 4 {
			t.Fatalf("after the window: %+v with %d done, want the run ended with 4 done, 2 left", s, f.done())
		}

		h.stop() // a restart during the day
		h.start()
		if s := h.status("markers"); s.Running || s.PendingCount != 2 || s.Last == nil || s.Last.Summary != "4 done, 2 left" {
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
		if s := h.status("markers"); s.Running || s.PendingCount != 1 || s.Last.Summary != "2 done" || f.done() != 2 {
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

func TestEnqueuedItemsRunNowWithoutTheBacklog(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := newFeature(5, time.Minute)
		w := f.items()
		w.Limit = func(config.Settings) int { return 1 }
		h := newHarness(t, Task{Name: "markers", Schedule: Nightly, Work: w})
		h.window(6, 2)
		h.start()

		// Enqueued items run in the order enqueued, one at a time within
		// the limit. An item enqueued again while it waits keeps its place.
		ids := slices.Clone(f.left)
		h.r.Enqueue("markers", ids[3], ids[4])
		synctest.Wait()
		h.r.Enqueue("markers", ids[0], ids[4])
		synctest.Wait()
		if s := h.status("markers"); !s.Running || len(s.Items) != 1 || s.Items[0].ID != ids[3] {
			t.Fatalf("%+v: want the first enqueued in flight, alone", s.Items)
		}
		if asked := h.r.Asked("markers"); !slices.Equal(asked, []id.ID{ids[4], ids[0]}) {
			t.Fatalf("asked %v, want the rest in the order enqueued", asked)
		}
		sleep(3 * time.Minute)
		if want := []id.ID{ids[3], ids[4], ids[0]}; !slices.Equal(f.order, want) || f.done() != 3 {
			t.Fatalf("did %v, want %v and no more outside the window", f.order, want)
		}
		h.stop()
		if s := h.status("markers"); s.Last != nil {
			t.Fatalf("enqueued items ended a run: %+v", s.Last)
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
		w := f.items()
		left := w.PendingCount
		var holding atomic.Bool
		gate := make(chan struct{})
		w.PendingCount = func(ctx context.Context) (int, error) {
			if holding.CompareAndSwap(true, false) {
				<-gate
			}
			return left(ctx)
		}
		h := newHarness(t, Task{Name: "previews", Schedule: Nightly, Work: w})
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

func TestFuncRuns(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		runs := 0
		h := newHarness(t,
			Task{Name: "make-backup", Schedule: Nightly, Maintenance: true, Work: Func(func(ctx context.Context) error {
				runs++
				Summarize(ctx, "Backed up")
				return nil
			})},
			Task{Name: "optimize-database", Schedule: Nightly, Maintenance: true, Work: Func(func(ctx context.Context) error {
				return errors.New("disk I/O error")
			})},
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
		run := Func(func(context.Context) error { return nil })
		h := newHarness(t,
			Task{Name: "make-previews", Schedule: Nightly, Work: run, Wanted: func(context.Context) (bool, error) { return wanted, nil }},
			Task{Name: "make-backup", Schedule: Nightly, Maintenance: true, Work: run},
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

func TestOnDemandDoesWhatItFindsAtStartup(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := newFeature(2, time.Minute)
		w := f.items()
		w.PendingCount, w.Uncounted = nil, true
		h := newHarness(t, Task{Name: "downloads", Schedule: OnDemand, Unstoppable: true, Work: w})
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
			Task{Name: "optimize-database", Schedule: Nightly, Maintenance: true, Work: Func(func(ctx context.Context) error {
				return errors.New("disk I/O error")
			})},
			Task{Name: "scan-libraries", Schedule: Every(2 * time.Hour), Work: Func(func(ctx context.Context) error {
				every++
				Summarize(ctx, "Scanned")
				return nil
			})},
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

// plain is a task's Func. It reports progress and takes a while, unless
// it's cancelled.
type plain struct {
	mu    sync.Mutex
	calls int
	ended int // calls that weren't cancelled
	took  time.Duration
	err   error
}

func (p *plain) task(name string) Task {
	return Task{Name: name, Schedule: Nightly, Maintenance: true, Work: Func(func(ctx context.Context) error {
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
	})}
}

func (p *plain) counts() (calls, ended int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.calls, p.ended
}

func TestRunNowOnAFuncTask(t *testing.T) {
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

// An error or panic from a task's Func becomes the run's error.
func TestAFuncRunFails(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		p := &plain{took: time.Minute, err: errors.New("disk I/O error")}
		h := newHarness(t, p.task("optimize-database"),
			Task{Name: "make-backup", Schedule: Nightly, Maintenance: true, Work: Func(func(context.Context) error { panic("no disk") })},
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

func TestStopDuringAFuncRun(t *testing.T) {
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

// A Func's night run cut off by the window's close ends as stopped. The
// window's next opening begins another run.
func TestAFuncRunAcrossTheWindow(t *testing.T) {
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
			t.Fatalf("%d calls, want a new run's Func called", calls)
		}
		sleep(2 * time.Hour)
		h.stop()
		s := h.status("prune-previews")
		if calls, ended := p.counts(); calls != 2 || ended != 1 || s.Last == nil || s.Last.Summary != "Removed 3" {
			t.Fatalf("%d calls, %d ended, last %+v: want the run ended in the window", calls, ended, s.Last)
		}
	})
}

// While a Func run's end is being recorded, the task still shows as
// running and its last run is unchanged.
func TestAFuncRunEndsInOneStep(t *testing.T) {
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

// A task with no PendingCount is counted by what Pending lists, a supplied
// PendingCount wins, and an Uncounted task reports no count.
func TestPendingCount(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := newFeature(3, time.Minute)
		listed, supplied, uncounted := f.items(), f.items(), f.items()
		listed.PendingCount = nil
		supplied.PendingCount = func(context.Context) (int, error) { return 7, nil }
		uncounted.PendingCount, uncounted.Uncounted = nil, true
		h := newHarness(t,
			Task{Name: "listed", Schedule: Nightly, Work: listed},
			Task{Name: "supplied", Schedule: Nightly, Work: supplied},
			Task{Name: "uncounted", Schedule: Nightly, Work: uncounted},
		)
		h.window(6, 2)
		h.start()
		for name, want := range map[string]int{"listed": 3, "supplied": 7, "uncounted": 0} {
			if s := h.status(name); s.PendingCount != want {
				t.Errorf("%s: %d pending, want %d", name, s.PendingCount, want)
			}
		}

		// A run's progress and summary use the same count.
		if err := h.r.Start(context.Background(), "listed"); err != nil {
			t.Fatal(err)
		}
		sleep(90 * time.Second)
		if s := h.status("listed"); s.Done != 1 || s.Total != 3 {
			t.Errorf("listed, running: %d of %d, want 1 of 3", s.Done, s.Total)
		}
		if err := h.r.Stop(context.Background(), "listed"); err != nil {
			t.Fatal(err)
		}
		h.stop()
		if s := h.status("listed"); s.Last == nil || s.Last.Summary != "Stopped: 1 done, 2 left" {
			t.Errorf("listed, stopped: %+v", s.Last)
		}
	})
}

// Add panics on what the compiler can't check.
func TestAddPanics(t *testing.T) {
	run := Func(func(context.Context) error { return nil })
	work := newFeature(0, time.Minute).items()
	noPending, noDo := work, work
	noPending.Pending, noDo.Do = nil, nil
	for _, c := range []struct {
		task Task
		want string
	}{
		{Task{Schedule: Nightly, Work: run}, "tasks: a task needs a name"},
		{Task{Name: "none", Schedule: Nightly}, "tasks: a task needs Work, a Func or Items: none"},
		{Task{Name: "nil-func", Schedule: Nightly, Work: Func(nil)}, "tasks: a task needs Work, a Func or Items: nil-func"},
		{Task{Name: "pointer", Schedule: Nightly, Work: &work}, "tasks: a task needs Work, a Func or Items: pointer"},
		{Task{Name: "no-pending", Work: noPending}, "tasks: Items needs Pending and Do: no-pending"},
		{Task{Name: "no-do", Work: noDo}, "tasks: Items needs Pending and Do: no-do"},
		{Task{Name: "on-demand", Schedule: OnDemand, Work: run}, "tasks: a Func needs a schedule: on-demand"},
		{Task{Name: "added", Schedule: Nightly, Work: run}, "tasks: task added twice: added"},
		{Task{Name: "func-step", Schedule: Nightly, Work: run, StepOf: StepOf{Task: "added", Step: "s"}}, "tasks: a step needs Items, another task and a step name: func-step"},
		{Task{Name: "unnamed-step", Work: work, StepOf: StepOf{Task: "added"}}, "tasks: a step needs Items, another task and a step name: unnamed-step"},
	} {
		h := newHarness(t)
		r := New(h.db, h.settings, events.NewBus(), Options{})
		r.Add(Task{Name: "added", Work: work})
		if got := panicOf(func() { r.Add(c.task) }); got != c.want {
			t.Errorf("Add(%+v) panicked with %v, want %q", c.task, got, c.want)
		}
	}

	synctest.Test(t, func(t *testing.T) {
		h := newHarness(t)
		h.start()
		defer h.stop()
		if got := panicOf(func() { h.r.Add(Task{Name: "late", Work: work}) }); got != "tasks: Add after Run" {
			t.Errorf("Add after Run panicked with %v", got)
		}
	})
}

// panicOf returns what f panicked with, or nil.
func panicOf(f func()) (p any) {
	defer func() { p = recover() }()
	f()
	return nil
}

// Asked items are counted the way a run's are: those settled since the
// task was last idle, out of those plus the items still in line or in
// flight. A single asked item has no count.
func TestAskedItemsAreCounted(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := newFeature(10, time.Minute)
		h := newHarness(t, f.task("refresh"))
		h.window(6, 2)
		h.start()
		ids := slices.Clone(f.left)
		count := func(done, total int) {
			t.Helper()
			if s := h.status("refresh"); !s.Running || s.Done != done || s.Total != total {
				t.Fatalf("running %v, %d of %d, want %d of %d", s.Running, s.Done, s.Total, done, total)
			}
		}

		h.r.Enqueue("refresh", ids[0])
		synctest.Wait()
		count(0, 0)
		sleep(time.Minute)

		// A burst starts from zero after the line drained, and one asked
		// while it works adds to its total.
		h.r.Enqueue("refresh", ids[1:4]...)
		synctest.Wait()
		count(0, 3)
		sleep(90 * time.Second)
		count(1, 3)
		h.r.Enqueue("refresh", ids[4:6]...)
		synctest.Wait()
		count(1, 5)
		sleep(2 * time.Minute)
		count(3, 5)
		sleep(2 * time.Minute)
		if s := h.status("refresh"); s.Running || s.Done != 0 || s.Total != 0 || s.PendingCount != 4 {
			t.Fatalf("drained: %+v, want idle with 4 left", s)
		}

		h.r.Enqueue("refresh", ids[6:8]...)
		synctest.Wait()
		count(0, 2)
		sleep(90 * time.Second)
		count(1, 2)
		sleep(time.Minute)
		h.stop()
		if s := h.status("refresh"); s.Last != nil || f.done() != 8 {
			t.Fatalf("%+v after %d done, want every asked item done and no run", s.Last, f.done())
		}
	})
}

// Items asked during a run are part of the run's count: settling one adds
// to done and takes it from what's left, so the total holds, and the pass
// doesn't do it again.
func TestItemsAskedDuringARunKeepItsCount(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := newFeature(6, time.Minute)
		h := newHarness(t, f.task("previews"))
		h.window(6, 2)
		h.start()
		if err := h.r.Start(context.Background(), "previews"); err != nil {
			t.Fatal(err)
		}
		sleep(90 * time.Second)
		if s := h.status("previews"); s.Done != 1 || s.Total != 6 {
			t.Fatalf("run: %d of %d, want 1 of 6", s.Done, s.Total)
		}
		h.r.Enqueue("previews", f.left[len(f.left)-1])
		sleep(time.Minute)
		if s := h.status("previews"); s.Done != 2 || s.Total != 6 {
			t.Fatalf("run with an asked item: %d of %d, want 2 of 6", s.Done, s.Total)
		}
		sleep(time.Hour)
		h.stop()
		if s := h.status("previews"); s.Last == nil || s.Last.Summary != "6 done" || len(f.order) != 6 {
			t.Fatalf("%+v after %d tries, want every item done once", s.Last, len(f.order))
		}
	})
}

// An Uncounted task's asked items show no count of the task's own, only
// the first item's progress.
func TestUncountedAskedItemsShowTheirOwnProgress(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := newFeature(3, time.Minute)
		w := f.items()
		w.Uncounted = true
		do := w.Do
		w.Do = func(ctx context.Context, i id.ID) error {
			Progress(ctx, "probe", 2, 9)
			return do(ctx, i)
		}
		h := newHarness(t, Task{Name: "scan", Schedule: Every(12 * time.Hour), Work: w})
		h.start()
		h.r.Enqueue("scan", f.left...)
		synctest.Wait()
		if s := h.status("scan"); !s.Running || s.Step != "probe" || s.Done != 2 || s.Total != 9 {
			t.Fatalf("%+v, want the first item's own progress", s)
		}
		h.stop()
	})
}
