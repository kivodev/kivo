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
// in the order they started. After them come its items that aren't in
// flight but whose step has items left that they asked for, each on the
// step with the step's items done out of all it asked for.
func (r *Runner) Items(name string) []Item {
	r.mu.Lock()
	defer r.mu.Unlock()
	if t := r.byName[name]; t != nil && t.items != nil {
		return append(t.items.listLocked(), r.stepItemsLocked(t)...)
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
	return r.listedLocked(name)
}

// listedLocked returns the task the Tasks page lists by name, or nil. A
// step isn't listed.
func (r *Runner) listedLocked(name string) *task {
	if t := r.byName[name]; t != nil && t.StepOf.Task == "" {
		return t
	}
	return nil
}

// Tasks returns every task as the Tasks page shows it, in the order they
// were added. A step shows on its owner's row.
func (r *Runner) Tasks(ctx context.Context) ([]Status, error) {
	var out []Status
	for _, t := range r.tasks {
		if t.StepOf.Task != "" {
			continue
		}
		s, err := r.status(ctx, t)
		if err != nil {
			return nil, fmt.Errorf("status of task %s: %w", t.Name, err)
		}
		out = append(out, s)
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
// enqueued them shows their progress. With no work of its own going on, the
// task runs while a step of it has items left that its items asked for: it
// is on that step, with the step's items done out of all those asked for.
func (r *Runner) itemStatus(ctx context.Context, t *task, s *Status, now time.Time) error {
	r.mu.Lock()
	w := t.items
	s.Items = w.listLocked()
	steps := r.stepItemsLocked(t)
	working := t.open != nil && w.pass != nil && r.workingLocked(t, now)
	s.Running = len(w.busy) > 0 || len(w.queued) > 0 || working || t.ending != nil
	done := 0
	if working {
		done = t.open.done
	}
	r.mu.Unlock()
	defer func() { s.Items = append(s.Items, steps...) }()
	if !s.Running && len(steps) > 0 {
		s.Running, s.Step = true, steps[0].Step
		for _, it := range steps {
			s.Done, s.Total = s.Done+it.Done, s.Total+it.Total
		}
	}

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
