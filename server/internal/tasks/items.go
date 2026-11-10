package tasks

import (
	"context"
	"fmt"
	"log/slog"
	"slices"
	"sync"
	"time"

	"github.com/mmcdole/kivo/internal/id"
)

// items is an Items task's work: what the task does, its pass, the items
// enqueued, and the items in flight.
type items struct {
	Items
	pass     *pass
	nextLook time.Time // when an on-demand task next looks for pending items

	asked  []id.ID
	queued map[id.ID]bool // the items in asked
	// requeue holds items enqueued while in flight. Each is done once more
	// after it finishes.
	requeue map[id.ID]bool

	busy    map[id.ID]*item // in flight
	counted counted
}

// pass is one walk through the items a task has left. It reads Pending and
// works through the list. Then it reads Pending again, less the items it
// already did, until nothing new turns up. A failed item counts as done for
// the pass, so the next pass tries it again, not this one.
type pass struct {
	list []id.ID
	pos  int
	did  map[id.ID]bool
}

// item is an item in flight.
type item struct {
	flight
	ctx     context.Context // the context Do gets, carrying the flight for Progress
	id      id.ID
	started time.Time // used to log how long the item took
	pass    *pass     // the pass it came from, or nil if it was enqueued
}

// itemLoop starts an Items task's items whenever one may start, then waits
// to be woken.
func (r *Runner) itemLoop(ctx context.Context, t *task, wg *sync.WaitGroup) {
	for {
		r.fill(ctx, t, wg)
		select {
		case <-ctx.Done():
			return
		case <-t.wake:
		}
	}
}

// fill starts every item that may start now. It reads Pending again
// whenever the pass runs out.
func (r *Runner) fill(ctx context.Context, t *task, wg *sync.WaitGroup) {
	for ctx.Err() == nil {
		r.mu.Lock()
		var it *item
		p := r.ranOutLocked(t)
		if p == nil {
			it = r.claimLocked(ctx, t)
			if it == nil {
				p = r.ranOutLocked(t) // the claim went through the rest of the list
			}
		}
		r.mu.Unlock()
		switch {
		case it != nil:
			wg.Go(func() { r.do(t, it) })
		case p != nil:
			list, err := t.items.Pending(ctx)
			r.mu.Lock()
			ended := r.refillLocked(t, p, list, err)
			r.mu.Unlock()
			if ended != nil {
				r.endItems(ctx, t, ended, "")
			}
		default:
			return
		}
	}
}

// ranOutLocked returns t's pass when it is time to read Pending again: the
// task is working, the pass's list is used up, and none of the pass's items
// are in flight. Otherwise it returns nil.
func (r *Runner) ranOutLocked(t *task) *pass {
	p := t.items.pass
	if p == nil || p.pos < len(p.list) || !r.workingLocked(t, time.Now()) {
		return nil
	}
	for _, it := range t.items.busy {
		if it.pass == p {
			return nil
		}
	}
	return p
}

// refillLocked sets a pass's next list from what Pending read, less the
// items in flight, enqueued, or already done this pass. If nothing new is
// left, the pass ends and so does the run, which refillLocked returns.
func (r *Runner) refillLocked(t *task, p *pass, list []id.ID, err error) *run {
	w := t.items
	if w.pass != p {
		return nil // the pass was stopped or ended meanwhile
	}
	if err != nil {
		slog.Error("tasks: read what's left", "task", t.Name, "err", err)
		w.pass = nil // a later tick begins another pass
		return nil
	}
	list = slices.DeleteFunc(list, func(i id.ID) bool { return p.did[i] || w.busy[i] != nil || w.queued[i] })
	if len(list) == 0 {
		w.pass = nil
		return t.endOpenLocked()
	}
	p.list, p.pos = list, 0
	return nil
}

// keepPassingLocked gives an Items task a pass when it has none and should
// have one. An on-demand task gets one each hour. Any other task gets one
// while its open run is working.
func (r *Runner) keepPassingLocked(t *task, now time.Time) {
	w := t.items
	switch {
	case w.pass != nil:
	case t.Schedule == OnDemand:
		if !now.Before(w.nextLook) {
			w.nextLook = now.Add(onDemandRun)
			r.startPassLocked(t)
		}
	case t.open != nil && r.workingLocked(t, now):
		r.startPassLocked(t)
	}
}

// startPassLocked begins a pass over the items t has left.
func (r *Runner) startPassLocked(t *task) {
	t.items.pass = &pass{did: map[id.ID]bool{}}
	t.signal()
	r.changed()
}

// claimLocked picks the next item t should start now, if its limit allows
// one. Enqueued items come first. Otherwise it takes the pass's next item,
// while the task is working.
func (r *Runner) claimLocked(ctx context.Context, t *task) *item {
	w, limit := t.items, r.limit(t)
	var it *item
	switch {
	case len(w.busy) >= limit:
	case len(w.asked) > 0:
		it = &item{id: w.asked[0]}
		w.asked = w.asked[1:]
	case w.pass != nil && r.workingLocked(t, time.Now()):
		it = w.nextListedLocked() // nil when the list ran out, so fill reads Pending again
	}
	if it == nil {
		return nil
	}
	delete(w.queued, it.id)
	it.ctx = it.start(ctx, r)
	it.started = it.since
	w.busy[it.id] = it
	r.changed()
	return it
}

// limit is how many of t's items may run at once now, at least one.
func (r *Runner) limit(t *task) int {
	if t.items.Limit == nil {
		return 1
	}
	return max(1, t.items.Limit(r.settings.Get()))
}

// nextListedLocked takes the pass's next item that isn't done, in flight,
// or enqueued. It returns nil when the list runs out.
func (w *items) nextListedLocked() *item {
	p := w.pass
	for p.pos < len(p.list) {
		i := p.list[p.pos]
		p.pos++
		if !p.did[i] && w.busy[i] == nil && !w.queued[i] {
			return &item{id: i, pass: p}
		}
	}
	return nil
}

// do does one item and then updates the in-memory state. The item's slot
// is freed and its pass marks it done. If it settled, its run counts it.
// If it was enqueued again while in flight, it goes back in line.
func (r *Runner) do(t *task, it *item) {
	err := guard(it.ctx, func() error { return t.items.Do(it.ctx, it.id) }, "task", t.Name, "item", it.id)
	cancelled := it.ctx.Err() != nil
	it.cancel()
	r.mu.Lock()
	w := t.items
	delete(w.busy, it.id)
	if p := it.pass; p != nil && p == w.pass {
		p.did[it.id] = true
		if t.open != nil && err == nil && !cancelled {
			t.open.done++
		}
	}
	if w.requeue[it.id] {
		delete(w.requeue, it.id)
		w.enqueueLocked(it.id)
	}
	r.mu.Unlock()
	if err != nil && !cancelled {
		slog.Warn("tasks: item left for a later pass", "task", t.Name, "item", it.id, "err", err)
	}
	t.signal()
	r.changed()
}

// endItems ends an Items task's run. Its summary says how many items the run
// settled and how many are left, after prefix, such as "Stopped".
func (r *Runner) endItems(ctx context.Context, t *task, ended *run, prefix string) {
	left := 0
	if !t.items.Uncounted {
		var err error
		if left, err = t.items.pendingCount(ctx); err != nil {
			slog.ErrorContext(ctx, "tasks: read what's left", "task", t.Name, "err", err)
		}
	}
	r.end(ctx, t, ended, Outcome{Summary: settledSummary(prefix, ended.done, left)}, ended.done > 0)
}

// settledSummary is an item run's summary: how many items it settled and
// how many it left, after prefix.
func settledSummary(prefix string, settled, left int) string {
	s := fmt.Sprintf("%d done", settled)
	if left > 0 {
		s += fmt.Sprintf(", %d left", left)
	}
	if prefix != "" {
		s = prefix + ": " + s
	}
	return s
}

// Enqueue has an Items task do the given items now, whether or not the
// window is open. They go ahead of the run's items, in the order enqueued
// and within the task's limit, and no run begins. An item enqueued while in
// flight is done once more afterwards. The line is held in memory only. An
// item lost to a restart is still pending in the feature's tables.
func (r *Runner) Enqueue(name string, items ...id.ID) {
	r.mu.Lock()
	defer r.mu.Unlock()
	t := r.byName[name]
	if t == nil || t.items == nil {
		panic("tasks: enqueue on a task with no items: " + name)
	}
	t.items.enqueueLocked(items...)
	t.signal()
	r.changed()
}

// enqueueLocked puts items in line, each once.
func (w *items) enqueueLocked(items ...id.ID) {
	for _, i := range items {
		switch {
		case w.busy[i] != nil:
			w.requeue[i] = true
		case !w.queued[i]:
			w.asked = append(w.asked, i)
			w.queued[i] = true
		}
	}
}

// stopLocked cancels an item in flight, removes it from the line, and marks
// it done for the current pass.
func (w *items) stopLocked(i id.ID) {
	if it := w.busy[i]; it != nil {
		it.cancel()
	}
	if w.queued[i] {
		delete(w.queued, i)
		w.asked = slices.DeleteFunc(w.asked, func(x id.ID) bool { return x == i })
	}
	delete(w.requeue, i)
	if w.pass != nil {
		w.pass.did[i] = true
	}
}

// Retry forgets the failures of a task's given items, or of all its failed
// items when none are given, and enqueues them. The task's counts are read
// afresh afterwards.
func (r *Runner) Retry(ctx context.Context, name string, items ...id.ID) error {
	t := r.task(name)
	if t == nil || t.items == nil || t.items.Retry == nil {
		return ErrNotFound
	}
	again, err := t.items.Retry(ctx, items...)
	if err != nil {
		return err
	}
	r.mu.Lock()
	t.items.counted = counted{}
	r.mu.Unlock()
	r.Enqueue(name, again...)
	return nil
}
