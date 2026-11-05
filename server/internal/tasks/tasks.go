// Package tasks runs the server's background work. Each task is one row of
// the Tasks page. An item task reads its remaining items from the tables of
// the feature that owns it each time it looks for work. So the only record
// a task keeps of its own is how its last run ended.
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
	SectionActivity    Section = "activity"    // unscheduled work, done when asked for
	SectionLibrary     Section = "library"     // scheduled work on the libraries' media
	SectionMaintenance Section = "maintenance" // cleanups and database upkeep
)

// Task is one row of the Tasks page. It says when its work runs and what
// the work is. A plain task sets Run. An item task sets Pending and Do.
type Task struct {
	Name string
	// Nightly makes the task run in the nightly maintenance window. A run
	// begins when the window opens. If the server starts inside the window
	// and no run has ended since it opened, a run begins at startup. The
	// run ends when the window closes, unless Run now started it or asked
	// it to finish.
	Nightly bool
	// Every is how long after a run ends the next one begins.
	//
	// A task with neither Nightly nor Every is an Activity row. It has no
	// runs. It does items when they are asked for, and it also does any
	// items it finds left at startup and then each hour.
	Every time.Duration
	// Maintenance lists a scheduled task under Maintenance instead of
	// Library.
	Maintenance bool
	// Limit is how many items may run at once. It is read each time an item
	// could start. Nil means one.
	Limit func(config.Settings) int
	// Finish means Stop doesn't apply, because the work is worse half done.
	Finish bool
	// Wanted reports whether a scheduled task has anything to do. It is
	// false when every library is set never to do the task's work, and then
	// the task shows no next run. Nil means always wanted.
	Wanted func(ctx context.Context) (bool, error)

	// Run is a plain task's whole run.
	Run func(ctx context.Context) error

	// Pending lists the items left, in the order to do them. It leaves out
	// items that are done and items that failed for good.
	Pending func(ctx context.Context) ([]id.ID, error)
	// Do does one item. It re-reads the item and returns nil at once if the
	// item is no longer pending. It records the outcome, done or failed for
	// good, in the task's own tables, so Pending leaves the item out
	// afterwards. A returned error leaves the item pending for a later pass.
	Do func(ctx context.Context, item id.ID) error
	// Left is how many items are left, counted as Pending would list them.
	Left func(ctx context.Context) (int, error)
	// Failed lists the items that failed for good.
	Failed func(ctx context.Context) ([]Failure, error)
	// Retry forgets the failures of the given items, or of every failed
	// item when none are given. It returns the items to do again.
	Retry func(ctx context.Context, items ...id.ID) ([]id.ID, error)
}

// Section is the table the task is listed in.
func (t Task) Section() Section {
	switch {
	case !t.Nightly && t.Every == 0:
		return SectionActivity
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
	// Activity task and for a task that isn't wanted.
	Next time.Time
	// Running is true while the task has items in flight or asked for, or
	// has a run working.
	Running bool
	// Step is what the running task is doing.
	Step string
	// Done of Total is how far the running task has got. For an item run
	// it is the items done out of those done plus those left.
	Done, Total int
	// Left is how many items are left for the next run, while no run is
	// working.
	Left     int
	Failures int    // items that failed for good
	Items    []Item // items in flight, in the order they started
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

// flightKey is the context key that holds the flight of a Run or Do call.
type flightKey struct{}

// Progress reports what an item or a plain run is doing, as a step such as
// "probe" and how far it has got. ctx must be the context Do or Run was
// given.
func Progress(ctx context.Context, step string, done, total int) {
	if f, ok := ctx.Value(flightKey{}).(*flight); ok {
		f.progress(step, done, total)
	}
}

// Summarize sets a plain run's one-line summary, such as "Removed 12
// missing files". ctx must be the context Run was given.
func Summarize(ctx context.Context, summary string) {
	if f, ok := ctx.Value(flightKey{}).(*flight); ok {
		f.summarize(summary)
	}
}
