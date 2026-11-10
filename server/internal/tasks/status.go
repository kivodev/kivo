package tasks

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/mmcdole/kivo/internal/id"
)

// Failed lists a task's items that failed for good.
func (r *Runner) Failed(ctx context.Context, name string) ([]Failure, error) {
	t := r.task(name)
	if t == nil || t.items == nil || t.items.Failed == nil {
		return nil, ErrNotFound
	}
	return t.items.Failed(ctx)
}

// Items returns a task's items in flight, with the progress each reported,
// in the order they started.
func (r *Runner) Items(name string) []Item {
	r.mu.Lock()
	defer r.mu.Unlock()
	if t := r.byName[name]; t != nil && t.items != nil {
		return t.items.listLocked()
	}
	return nil
}

// Asked returns a task's enqueued items that haven't started yet.
func (r *Runner) Asked(name string) []id.ID {
	r.mu.Lock()
	defer r.mu.Unlock()
	if t := r.byName[name]; t != nil && t.items != nil {
		return slices.Clone(t.items.asked)
	}
	return nil
}

// Busy counts each task's work in flight: its items, or one for a Func
// task's run. Tasks with nothing in flight are left out.
func (r *Runner) Busy() map[string]int {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := map[string]int{}
	for _, t := range r.tasks {
		switch {
		case t.call != nil:
			out[t.Name] = 1
		case t.items != nil && len(t.items.busy) > 0:
			out[t.Name] = len(t.items.busy)
		}
	}
	return out
}

func (r *Runner) task(name string) *task {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.byName[name]
}

// Tasks returns every task as the Tasks page shows it, in the order they
// were added.
func (r *Runner) Tasks(ctx context.Context) ([]Status, error) {
	out := make([]Status, len(r.tasks))
	for i, t := range r.tasks {
		s, err := r.status(ctx, t)
		if err != nil {
			return nil, fmt.Errorf("status of task %s: %w", t.Name, err)
		}
		out[i] = s
	}
	return out, nil
}

// status builds one task's Status: its schedule, its last run, and the
// progress of its Func in flight or its items.
func (r *Runner) status(ctx context.Context, t *task) (Status, error) {
	now := time.Now()
	r.mu.Lock()
	s := Status{Task: t.Task, Last: t.last, Next: r.nextLocked(t, now)}
	if t.items == nil {
		s.Running = t.call != nil || t.open != nil && r.workingLocked(t, now) || t.ending != nil
		if c := t.call; c != nil {
			s.Step, s.Done, s.Total = c.step, c.done, c.total
		}
		r.mu.Unlock()
	} else {
		r.mu.Unlock()
		if err := r.itemStatus(ctx, t, &s, now); err != nil {
			return s, err
		}
	}
	if t.Wanted == nil || s.Next.IsZero() {
		return s, nil
	}
	wanted, err := t.Wanted(ctx)
	if !wanted {
		s.Next = time.Time{}
	}
	return s, err
}

// itemStatus fills in an Items task's Status. While its run is working,
// progress is the items settled out of those settled plus those left.
// Otherwise progress is that of the first item in flight, if it reports
// any. Enqueued items get no count of their own here, because whoever
// enqueued them shows their progress.
func (r *Runner) itemStatus(ctx context.Context, t *task, s *Status, now time.Time) error {
	r.mu.Lock()
	w := t.items
	s.Items = w.listLocked()
	working := t.open != nil && w.pass != nil && r.workingLocked(t, now)
	s.Running = len(w.busy) > 0 || len(w.queued) > 0 || working || t.ending != nil
	done := 0
	if working {
		done = t.open.done
	}
	r.mu.Unlock()

	c, err := r.count(ctx, t, now)
	if err != nil {
		return err
	}
	if !w.Uncounted {
		if working {
			s.Done, s.Total = done, done+c.pending
		} else {
			s.PendingCount = c.pending
		}
	}
	s.Failures = c.failures
	if len(s.Items) > 0 {
		s.Step = s.Items[0].Step
		if s.Total == 0 {
			s.Done, s.Total = s.Items[0].Done, s.Items[0].Total
		}
	}
	return nil
}

// listLocked returns the items in flight, in the order they started.
func (w *items) listLocked() []Item {
	out := make([]Item, 0, len(w.busy))
	for _, it := range w.busy {
		out = append(out, it.statusLocked())
	}
	slices.SortFunc(out, func(a, b Item) int { return a.started.Compare(b.started) })
	return out
}

// counted caches a task's pending count and its number of failed items for
// countFor.
type counted struct {
	at                time.Time
	pending, failures int
}

// count returns t's pending count and its number of failed items. A result is
// reused for countFor.
func (r *Runner) count(ctx context.Context, t *task, now time.Time) (counted, error) {
	r.mu.Lock()
	c := t.items.counted
	r.mu.Unlock()
	if now.Sub(c.at) < countFor {
		return c, nil
	}
	c = counted{at: now}
	if !t.items.Uncounted {
		var err error
		if c.pending, err = t.items.pendingCount(ctx); err != nil {
			return counted{}, err
		}
	}
	if t.items.Failed != nil {
		failed, err := t.items.Failed(ctx)
		if err != nil {
			return counted{}, err
		}
		c.failures = len(failed)
	}
	r.mu.Lock()
	t.items.counted = c
	r.mu.Unlock()
	return c, nil
}

// statusLocked returns the item as Status lists it.
func (it *item) statusLocked() Item {
	return Item{ID: it.id, Step: it.step, Done: it.done, Total: it.total, Since: it.since, started: it.started}
}

// flight is a Func or Do call in flight. It holds how to cancel the call
// and what the call reported through Progress and Summarize.
type flight struct {
	r      *Runner
	cancel context.CancelFunc

	// Runner.mu guards the fields below.
	step        string
	done, total int
	since       time.Time // when it reached step
	summary     string
	noticed     time.Time // when its progress was last published
}

// start prepares f to run under ctx. It returns the context to pass to the
// Func or Do, which carries f for Progress and Summarize.
func (f *flight) start(ctx context.Context, r *Runner) context.Context {
	ctx, f.cancel = context.WithCancel(ctx)
	f.r, f.since = r, time.Now()
	return context.WithValue(ctx, flightKey{}, f)
}

func (f *flight) progress(step string, done, total int) {
	r := f.r
	r.mu.Lock()
	if step != f.step {
		f.since = time.Now()
	}
	f.step, f.done, f.total = step, done, total
	tell := time.Since(f.noticed) >= noticeEvery
	if tell {
		f.noticed = time.Now()
	}
	r.mu.Unlock()
	if tell {
		r.changed()
	}
}

func (f *flight) summarize(summary string) {
	f.r.mu.Lock()
	f.summary = summary
	f.r.mu.Unlock()
}
