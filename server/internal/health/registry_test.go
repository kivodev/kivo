package health

import (
	"context"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"
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

// Open lists every component's problems newest first.
func TestRegistryOpen(t *testing.T) {
	now := time.Now()
	a := &reported{problems: []Problem{{Component: "scan", Code: "old", RaisedAt: now.Add(-time.Hour)}}}
	b := &reported{problems: []Problem{{Component: "metadata", Code: "new", RaisedAt: now}}}
	r := NewRegistry([]Reporter{a, b}, RegistryOptions{})
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
	r := NewRegistry([]Reporter{scan}, RegistryOptions{Raised: func(_ context.Context, raised []OpenProblem) {
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
		scan := &reported{}
		r := NewRegistry([]Reporter{scan}, RegistryOptions{
			Raised: func(_ context.Context, ps []OpenProblem) { raised.Add(int32(len(ps))) },
		})
		ctx, cancel := context.WithCancel(t.Context())
		done := make(chan error)
		go func() { done <- r.Run(ctx) }()
		scan.set(Problem{Component: "scan", Code: "folderUnreachable", RaisedAt: time.Now()})
		time.Sleep(recheckEvery - time.Second)
		synctest.Wait()
		if raised.Load() != 0 {
			t.Fatal("raised before the first recheck")
		}
		time.Sleep(time.Second)
		synctest.Wait()
		if raised.Load() != 1 {
			t.Fatalf("after 30 seconds raised %d; want 1", raised.Load())
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
		r := NewRegistry([]Reporter{scan}, RegistryOptions{
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
