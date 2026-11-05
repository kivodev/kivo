package health

import (
	"context"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/mmcdole/kivo/internal/events"
)

var ctx = context.Background()

// reported is a component whose problems a test sets.
type reported struct {
	mu       sync.Mutex
	problems []Problem
}

func (r *reported) Health(context.Context) []Problem {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.problems
}

func (r *reported) set(ps ...Problem) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.problems = ps
}

// problemsTold subscribes to bus. It returns a func that reports the open
// count carried by each problemsChanged event received so far.
func problemsTold(t *testing.T, bus *events.Bus) func() []int {
	sub := bus.Subscribe(func(events.Scope) bool { return true })
	t.Cleanup(sub.Close)
	var told []int
	return func() []int {
		for {
			select {
			case e, ok := <-sub.C():
				if !ok {
					return told
				}
				if e.Kind == events.ProblemsChanged {
					told = append(told, e.Data.(int))
				}
			default:
				return told
			}
		}
	}
}

// problemsChanged is published when a read finds that the open problems
// changed, not on every read. The test raises a problem, dismisses it,
// resolves it and raises it again.
func TestRegistryTellsChanges(t *testing.T) {
	bus := events.NewBus()
	told := problemsTold(t, bus)
	scan := &reported{}
	r := NewRegistry([]Reporter{scan}, bus, RegistryOptions{})
	r.Open(ctx) // none open, which is the same as before any read
	raised := time.Now()
	scan.problems = []Problem{{Component: "scan", Code: "folderUnreachable", Message: "broken", RaisedAt: raised}}
	r.Open(ctx)
	r.Open(ctx)
	if !slices.Equal(told(), []int{1}) {
		t.Fatalf("told %v after a problem was raised; want [1]", told())
	}
	if r.Dismiss(ctx, "scan.nope") {
		t.Fatal("dismissed a problem that isn't open")
	}
	if !r.Dismiss(ctx, "scan.folderUnreachable") || len(r.Open(ctx)) != 0 || !slices.Equal(told(), []int{1, 0}) {
		t.Fatalf("after dismissing it: open %v, told %v", r.Open(ctx), told())
	}
	scan.problems = nil
	r.Open(ctx)
	if len(told()) != 2 {
		t.Fatalf("told %v when a dismissed problem resolved", told())
	}
	scan.problems = []Problem{{Component: "scan", Code: "folderUnreachable", Message: "broken", RaisedAt: raised.Add(time.Minute)}}
	r.Open(ctx)
	if open := r.Open(ctx); len(open) != 1 || open[0].ID != "scan.folderUnreachable" || !slices.Equal(told(), []int{1, 0, 1}) {
		t.Fatalf("raised again: open %v, told %v", open, told())
	}
}

// Open lists every component's problems newest first. A dismissal hides
// the problem only as it was raised at that time.
func TestRegistryOpen(t *testing.T) {
	now := time.Now()
	a := &reported{problems: []Problem{{Component: "scan", Code: "old", RaisedAt: now.Add(-time.Hour)}}}
	b := &reported{problems: []Problem{{Component: "metadata", Code: "new", RaisedAt: now}}}
	r := NewRegistry([]Reporter{a, b}, events.NewBus(), RegistryOptions{})
	ids := func() []string {
		var out []string
		for _, p := range r.Open(ctx) {
			out = append(out, p.ID)
		}
		return out
	}
	if got := ids(); !slices.Equal(got, []string{"metadata.new", "scan.old"}) {
		t.Fatalf("open %v", got)
	}
	r.Dismiss(ctx, "scan.old")
	a.problems[0].RaisedAt = now.Add(time.Minute) // raised again
	if got := ids(); !slices.Equal(got, []string{"scan.old", "metadata.new"}) {
		t.Fatalf("after it was raised again: open %v", got)
	}
}

// Raised receives each problem when a read first finds it open, and again
// when it's raised anew. It isn't called on every read.
func TestRegistryTellsRaised(t *testing.T) {
	var told []string
	scan := &reported{}
	r := NewRegistry([]Reporter{scan}, events.NewBus(), RegistryOptions{Raised: func(_ context.Context, raised []OpenProblem) {
		for _, p := range raised {
			told = append(told, p.ID)
		}
	}})
	at := time.Now()
	scan.problems = []Problem{{Component: "scan", Code: "folderUnreachable", RaisedAt: at}}
	r.Open(ctx)
	r.Open(ctx)
	scan.problems = append(scan.problems, Problem{Component: "scan", Code: "watchLimit", RaisedAt: at})
	r.Open(ctx)
	scan.problems = nil
	r.Open(ctx)
	scan.problems = []Problem{{Component: "scan", Code: "folderUnreachable", RaisedAt: at.Add(time.Minute)}}
	r.Open(ctx)
	if want := []string{"scan.folderUnreachable", "scan.watchLimit", "scan.folderUnreachable"}; !slices.Equal(told, want) {
		t.Fatalf("told %v, want %v", told, want)
	}
}

// Run rechecks every 30 seconds, so a problem raised while nobody reads the
// problems is reported within that time.
func TestRegistryRechecksEvery30Seconds(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var raised atomic.Int32
		bus := events.NewBus()
		told := problemsTold(t, bus)
		scan := &reported{}
		r := NewRegistry([]Reporter{scan}, bus, RegistryOptions{
			Raised: func(_ context.Context, ps []OpenProblem) { raised.Add(int32(len(ps))) },
		})
		ctx, cancel := context.WithCancel(t.Context())
		done := make(chan error)
		go func() { done <- r.Run(ctx) }()
		scan.set(Problem{Component: "scan", Code: "folderUnreachable", RaisedAt: time.Now()})
		time.Sleep(recheckEvery - time.Second)
		synctest.Wait()
		if len(told()) != 0 {
			t.Fatal("told before the first recheck")
		}
		time.Sleep(time.Second)
		synctest.Wait()
		if len(told()) != 1 || raised.Load() != 1 {
			t.Fatalf("after 30 seconds told %v, raised %d; want 1 and 1", told(), raised.Load())
		}
		scan.set()
		time.Sleep(recheckEvery)
		synctest.Wait()
		if len(told()) != 2 {
			t.Fatalf("told %v after it resolved; want 2", told())
		}
		cancel()
		if err := <-done; err != nil {
			t.Fatalf("Run: %v", err)
		}
	})
}

// Concurrent reads and the ticker check one at a time. A problem is raised
// once, however many checks race to find it.
func TestRegistryChecksOneAtATime(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var raised atomic.Int32
		scan := &reported{}
		r := NewRegistry([]Reporter{scan}, events.NewBus(), RegistryOptions{
			Raised: func(_ context.Context, ps []OpenProblem) { raised.Add(int32(len(ps))) },
		})
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		go func() { _ = r.Run(ctx) }()
		scan.set(Problem{Component: "scan", Code: "folderUnreachable", RaisedAt: time.Now()})
		time.Sleep(recheckEvery - time.Millisecond)
		var wg sync.WaitGroup
		for range 20 {
			wg.Go(func() { r.Open(ctx) })
		}
		time.Sleep(time.Millisecond)
		wg.Wait()
		synctest.Wait()
		if n := raised.Load(); n != 1 {
			t.Fatalf("raised %d times; want once", n)
		}
	})
}
