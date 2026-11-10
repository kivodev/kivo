package tasks

import (
	"context"
	"time"

	"github.com/mmcdole/kivo/internal/config"
	"github.com/mmcdole/kivo/internal/fault"
	"github.com/mmcdole/kivo/internal/id"
)

var (
	// ErrNotFound means there is no such task, or the task has no runs to
	// start.
	ErrNotFound = fault.New(fault.NotFound, "task not found")
	// ErrNotStoppable means the task's work always runs to the end, because
	// a half-done removal or download is worse than a finished one.
	ErrNotStoppable = fault.Coded(fault.Conflict, "notStoppable", "this task's work isn't stopped midway")
)

// Section is the table of the Tasks page that a task is listed in.
type Section string

// The sections, in the order admins see them.
const (
	SectionOnDemand    Section = "onDemand"    // unscheduled work, done when enqueued
	SectionLibrary     Section = "library"     // scheduled work on the libraries' media
	SectionMaintenance Section = "maintenance" // cleanups and database upkeep
)

// Task is one row of the Tasks page, unless StepOf makes it a step of
// another task's row. Work says what it does: a Func, which is one call per
// run, or Items, a list worked through an item at a time. Schedule says when
// its runs begin: Nightly, Every so often, or OnDemand, which has no runs.
// Whatever the schedule, Runner.Enqueue has Items do the given items now.
type Task struct {
	Name     string
	Schedule Schedule
	Work     Work
	// Maintenance lists a scheduled task under Maintenance instead of
	// Library.
	Maintenance bool
	// Unstoppable means Stop doesn't apply, because the work is worse half
	// done.
	Unstoppable bool
	// Wanted reports whether a scheduled task has anything to do, which is
	// false when every library is set never to do its work. It only hides
	// the task's next run on the Tasks page. The schedule begins the run
	// either way. Nil means always wanted.
	Wanted func(ctx context.Context) (bool, error)
	// StepOf makes an Items task the last step of another task's items,
	// as identifying what a library's scan found is of the scan. Such a
	// task has no row of its own: the runner leaves it out of Tasks and
	// reports it not found. Its items enqueued with Runner.EnqueueFor
	// show on the owner's row and stop with the owner's items. Its other
	// work shows nowhere. The zero StepOf makes the task a row.
	StepOf StepOf
}

// StepOf names the task whose row shows a step task's work, and the step
// it shows it as.
type StepOf struct {
	Task string // the owner, an Items task
	Step string // the step as the owner's progress names it, such as "identify"
}

// Schedule is when a task's runs begin: Nightly, Every some interval, or
// OnDemand.
type Schedule struct {
	nightly bool
	every   time.Duration
}

var (
	// OnDemand is the zero Schedule. The task has no runs. It does items
	// when they are enqueued, and it also does any pending items it finds
	// at startup and then each hour. Only Items can be on demand.
	OnDemand = Schedule{}
	// Nightly runs the task in the nightly maintenance window. A run begins
	// when the window opens. If the server starts inside the window and no
	// run has ended since it opened, a run begins at startup. The run ends
	// when the window closes, unless Run now started it or asked it to
	// finish.
	Nightly = Schedule{nightly: true}
)

// Every begins a run d after the last one ended.
func Every(d time.Duration) Schedule { return Schedule{every: d} }

// Work is what a task does. It is a Func or Items.
type Work interface{ work() }

// Func is work that is one call. The call is the task's whole run.
type Func func(ctx context.Context) error

// Items is work on a list of items held in the tables of the feature that
// owns the task. A run works through Pending until nothing new turns up.
type Items struct {
	// Pending lists the items left, in the order to do them. It leaves out
	// items that are done and items that failed for good.
	Pending func(ctx context.Context) ([]id.ID, error)
	// Do does one item. It re-reads the item and returns nil at once if the
	// item is no longer pending. It records the outcome, done or failed for
	// good, in the task's own tables, so Pending leaves the item out
	// afterwards. A returned error leaves the item pending for a later pass.
	Do func(ctx context.Context, item id.ID) error
	// Limit is how many items may run at once. It is read each time an item
	// could start. Nil means one.
	Limit func(config.Settings) int
	// PendingCount counts the items Pending would list. Nil means the
	// runner counts what Pending returns.
	PendingCount func(ctx context.Context) (int, error)
	// Uncounted means the task reports no pending count, as when Pending
	// lists items whether or not they have work to do.
	Uncounted bool
	// Failed lists the items that failed for good.
	Failed func(ctx context.Context) ([]Failure, error)
	// Retry forgets the failures of the given items, or of every failed
	// item when none are given. It returns the items to do again.
	Retry func(ctx context.Context, items ...id.ID) ([]id.ID, error)
}

func (Func) work()  {}
func (Items) work() {}

// pendingCount counts the items left.
func (w Items) pendingCount(ctx context.Context) (int, error) {
	if w.PendingCount != nil {
		return w.PendingCount(ctx)
	}
	list, err := w.Pending(ctx)
	return len(list), err
}

// Section is the table the task is listed in.
func (t Task) Section() Section {
	switch {
	case t.Schedule == OnDemand:
		return SectionOnDemand
	case t.Maintenance:
		return SectionMaintenance
	}
	return SectionLibrary
}

// Failure is an item that failed for good.
type Failure struct {
	Item   id.ID
	Reason string
	At     time.Time
}

// Status is a task as the Tasks page shows it.
type Status struct {
	Task
	Last *Outcome // the last run, or nil if it never ran
	// Next is when the schedule next begins a run. It is zero for an
	// on-demand task and for a task that isn't wanted.
	Next time.Time
	// Running is true while the task has items in flight or enqueued, or
	// has a run working, or a step of it has items left that its items
	// asked for.
	Running bool
	// Step is what the running task is doing.
	Step string
	// Done of Total is how far the running task has got. For an item run
	// it is the items done out of those done plus those left. For a step
	// it is the step's items done out of those its items asked for.
	Done, Total int
	// PendingCount is how many items are left for the next run, while no
	// run is working.
	PendingCount int
	Failures     int    // items that failed for good
	Items        []Item // items in flight, as Runner.Items lists them
}

// Outcome is how a run ended.
type Outcome struct {
	At      time.Time
	Summary string // what the run did, or that it was stopped
	Error   string // why the run failed, or empty if it didn't
}

// RunEnd is what Options.Ended is told when a run ends.
type RunEnd struct {
	Task string
	Outcome
	// Failed is how many of the task's items have failed for good.
	Failed int
}

// Item is an item in flight, with the progress it reported.
type Item struct {
	ID          id.ID
	Step        string
	Done, Total int
	Since       time.Time // when it reached Step

	started time.Time
}

// flightKey is the context key that holds the flight of a Func or Do call.
type flightKey struct{}

// Progress reports what an item or a Func's run is doing, as a step such as
// "probe" and how far it has got. ctx must be the context Do or the Func
// was given.
func Progress(ctx context.Context, step string, done, total int) {
	if f, ok := ctx.Value(flightKey{}).(*flight); ok {
		f.progress(step, done, total)
	}
}

// Summarize sets a Func run's one-line summary, such as "Removed 12
// missing files". ctx must be the context the Func was given.
func Summarize(ctx context.Context, summary string) {
	if f, ok := ctx.Value(flightKey{}).(*flight); ok {
		f.summarize(summary)
	}
}
