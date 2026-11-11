package tasks

import (
	"slices"
	"time"

	"github.com/mmcdole/kivo/internal/id"
)

// owned is how far a step has got with the items one owner's item asked
// for: those done out of all it asked for, since it first asked.
type owned struct {
	done, total int
	since       time.Time
}

// EnqueueFor is Enqueue for a step's items asked for by owner, an item of
// the task the step is a step of. While any of them are in line or in
// flight, the owner's item shows on the step in Items and on its task's row,
// and stopping the owner's item stops them. An item already asked for by an
// owner counts once.
func (r *Runner) EnqueueFor(owner id.ID, name string, items ...id.ID) {
	r.mu.Lock()
	defer r.mu.Unlock()
	t := r.byName[name]
	if t == nil || t.StepOf.Task == "" {
		panic("tasks: enqueue for an owner on a task that isn't a step: " + name)
	}
	w := t.items
	w.fromIdleLocked()
	for _, i := range items {
		if _, ok := w.ownerOf[i]; ok {
			continue
		}
		o := w.owned[owner]
		if o == nil {
			o = &owned{since: time.Now()}
			w.owned[owner] = o
		}
		w.ownerOf[i] = owner
		o.total++
	}
	w.enqueueLocked(items...)
	t.signal()
	r.changed()
}

// settleOwnedLocked counts an item that is no longer in line or in flight
// done for its owner, if it has one. An owner whose items are all done no
// longer shows on the step.
func (w *items) settleOwnedLocked(i id.ID) {
	owner, ok := w.ownerOf[i]
	if !ok {
		return
	}
	delete(w.ownerOf, i)
	if o := w.owned[owner]; o != nil {
		if o.done++; o.done >= o.total {
			delete(w.owned, owner)
		}
	}
}

// stepsLocked returns the steps of t.
func (r *Runner) stepsLocked(t *task) []*task {
	var out []*task
	for _, s := range r.tasks {
		if s.StepOf.Task == t.Name {
			out = append(out, s)
		}
	}
	return out
}

// stepItemsLocked lists t's items that aren't in flight but whose step has
// items left that they asked for. Each is on the step, with the step's items
// done out of all it asked for, and Since when it first asked. They're in
// the order they first asked.
func (r *Runner) stepItemsLocked(t *task) []Item {
	var out []Item
	for _, s := range r.stepsLocked(t) {
		for owner, o := range s.items.owned {
			if t.items.busy[owner] == nil {
				out = append(out, Item{ID: owner, Step: s.StepOf.Step, Done: o.done, Total: o.total, Since: o.since, started: o.since})
			}
		}
	}
	slices.SortFunc(out, func(a, b Item) int { return a.started.Compare(b.started) })
	return out
}

// stopStepsLocked stops what t's steps were asked to do by the given items
// of t, or by any of t's items when none are given. Their items in flight
// are cancelled and those in line are dropped. A step's items that no item
// of t asked for go on.
func (r *Runner) stopStepsLocked(t *task, owners ...id.ID) {
	for _, s := range r.stepsLocked(t) {
		w := s.items
		for i, owner := range w.ownerOf {
			if len(owners) == 0 || slices.Contains(owners, owner) {
				delete(w.ownerOf, i)
				delete(w.owned, owner)
				w.stopLocked(i)
			}
		}
	}
}
