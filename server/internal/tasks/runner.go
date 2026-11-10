package tasks

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"runtime/debug"
	"sync"
	"time"

	"github.com/mmcdole/kivo/internal/config"
	"github.com/mmcdole/kivo/internal/events"
	"github.com/mmcdole/kivo/internal/id"
	"github.com/mmcdole/kivo/internal/store"
	"github.com/mmcdole/kivo/internal/tasks/dbq"
)

const (
	tick        = time.Minute     // how often the schedule and the window are checked
	onDemandRun = time.Hour       // how often an on-demand task looks for pending items
	noticeEvery = 5 * time.Second // how often progress is published
	countFor    = 5 * time.Second // how long a task's pending count is reused
)

// Runner runs the tasks.
type Runner struct {
	db       *store.DB
	q        *dbq.Queries
	settings *config.Store // holds the window and the limits
	bus      *events.Bus
	ended    func(ctx context.Context, tx *store.Tx, e RunEnd) error // Options.Ended

	// Methods whose names end in Locked need mu held.
	mu      sync.Mutex
	tasks   []*task // in the order they were added
	byName  map[string]*task
	started bool
	boot    time.Time
}

// Options are the runner's optional hooks.
type Options struct {
	// Ended is called when a night run or a Run now run ends, if the run
	// did something or failed. It runs in the transaction that records the
	// run's end.
	Ended func(ctx context.Context, tx *store.Tx, e RunEnd) error
}

// New returns a runner over db's task table. The window and the limits are
// read from settings each time they're needed. The runner publishes
// tasksChanged on bus at every change the Tasks pages show, and for
// progress at most every 5 seconds. Add the tasks, then call Run.
func New(db *store.DB, settings *config.Store, bus *events.Bus, opt Options) *Runner {
	if opt.Ended == nil {
		opt.Ended = func(context.Context, *store.Tx, RunEnd) error { return nil }
	}
	return &Runner{db: db, q: dbq.New(db.Reader()), settings: settings, bus: bus, ended: opt.Ended, byName: map[string]*task{}}
}

// changed tells admins the Tasks pages changed.
func (r *Runner) changed() {
	r.bus.Publish(events.Event{Kind: events.TasksChanged, Scope: events.Scope{AdminOnly: true}})
}

// task is an added task and its state. Runner.mu guards the state.
type task struct {
	Task
	// wake is buffered 1. A send means the task's work may go on.
	wake chan struct{}

	open *run // the open run, or nil
	// ending is a run that has ended while end records how it ended.
	// No run begins meanwhile.
	ending *run
	last   *Outcome // how the last run ended, or nil if none has

	fn    Func    // a Func task's work, or nil
	call  *flight // fn in flight, or nil
	items *items  // an Items task's work, or nil
}

// run is a task's open run. It is held in memory only.
type run struct {
	started time.Time // used to log how long the run took
	// toEnd means Run now asked the run to finish, so it keeps working
	// after the window closes.
	toEnd bool
	done  int // items its passes settled
}

// Add adds a task. Call it while wiring the server, before Run.
func (r *Runner) Add(t Task) {
	r.mu.Lock()
	defer r.mu.Unlock()
	switch {
	case r.started:
		panic("tasks: Add after Run")
	case t.Name == "":
		panic("tasks: a task needs a name")
	case r.byName[t.Name] != nil:
		panic("tasks: task added twice: " + t.Name)
	}
	tk := &task{Task: t, wake: make(chan struct{}, 1)}
	switch w := t.Work.(type) {
	case Func:
		if w == nil {
			panic("tasks: a task needs Work, a Func or Items: " + t.Name)
		}
		if t.Schedule == OnDemand {
			panic("tasks: a Func needs a schedule: " + t.Name)
		}
		tk.fn = w
	case Items:
		if w.Pending == nil || w.Do == nil {
			panic("tasks: Items needs Pending and Do: " + t.Name)
		}
		tk.items = &items{Items: w, queued: map[id.ID]bool{}, requeue: map[id.ID]bool{}, busy: map[id.ID]*item{}}
	default:
		panic("tasks: a task needs Work, a Func or Items: " + t.Name)
	}
	r.tasks = append(r.tasks, tk)
	r.byName[t.Name] = tk
}

// Run runs the tasks until ctx ends and everything in flight has returned.
// A run that was open when the server stopped isn't resumed. The schedule
// begins the next one.
func (r *Runner) Run(ctx context.Context) error {
	rows, err := r.q.Tasks(ctx)
	if err != nil {
		return fmt.Errorf("read task runs: %w", err)
	}
	now := time.Now()
	r.mu.Lock()
	if r.started {
		r.mu.Unlock()
		panic("tasks: Run called twice")
	}
	r.started, r.boot = true, now
	for _, row := range rows {
		t := r.byName[row.Name]
		if t == nil {
			continue // a task no longer added
		}
		t.last = &Outcome{At: time.UnixMilli(row.Finished), Summary: row.Summary.String, Error: row.Error.String}
	}
	r.mu.Unlock()

	var wg sync.WaitGroup
	for _, t := range r.tasks {
		if t.items == nil {
			wg.Go(func() { r.funcLoop(ctx, t) })
		} else {
			wg.Go(func() { r.itemLoop(ctx, t, &wg) })
		}
	}
	wg.Go(func() { r.schedule(ctx) })
	wg.Wait()
	return nil
}

// endOpenLocked ends t's open run, if any, and returns it. The run stays in
// t.ending until end records how it ended, so no run begins meanwhile.
func (t *task) endOpenLocked() *run {
	ended := t.open
	if ended != nil {
		t.open, t.ending = nil, ended
	}
	return ended
}

// end records how a run ended. If the run was a night run or a Run now run,
// and it did something or failed, end also calls Options.Ended.
func (r *Runner) end(ctx context.Context, t *task, ended *run, out Outcome, did bool) {
	out.At = time.Now()
	var tell *RunEnd
	if (t.Schedule == Nightly || ended.toEnd) && (did || out.Error != "") {
		e := r.runEnd(ctx, t, out)
		tell = &e
	}
	err := r.db.Write(context.WithoutCancel(ctx), func(ctx context.Context, tx *store.Tx) error {
		err := dbq.New(tx).EndRun(ctx, dbq.EndRunParams{
			Name: t.Name, Finished: out.At.UnixMilli(), Summary: nullString(out.Summary), Error: nullString(out.Error),
		})
		if err != nil || tell == nil {
			return err
		}
		return r.ended(ctx, tx, *tell)
	})
	if err != nil {
		slog.ErrorContext(ctx, "tasks: record a run's end", "task", t.Name, "err", err)
	}
	r.mu.Lock()
	t.last = &out
	if t.ending == ended {
		t.ending = nil
	}
	r.mu.Unlock()
	if out.Error != "" {
		slog.WarnContext(ctx, "tasks: run failed", "task", t.Name, "err", out.Error)
	} else {
		slog.InfoContext(ctx, "tasks: run ended", "task", t.Name, "summary", out.Summary, "took", out.At.Sub(ended.started).Round(time.Second))
	}
	r.changed()
}

// runEnd builds the RunEnd for Options.Ended, counting the task's failed
// items.
func (r *Runner) runEnd(ctx context.Context, t *task, out Outcome) RunEnd {
	e := RunEnd{Task: t.Name, Outcome: out}
	if t.items != nil && t.items.Failed != nil {
		if failed, err := t.items.Failed(ctx); err == nil {
			e.Failed = len(failed)
		}
	}
	return e
}

func nullString(s string) sql.NullString { return sql.NullString{String: s, Valid: s != ""} }

// funcLoop calls a Func task's work while its open run is working, then
// waits to be woken.
func (r *Runner) funcLoop(ctx context.Context, t *task) {
	for {
		if ended, out := r.callFunc(ctx, t); ended != nil {
			r.end(ctx, t, ended, out, true)
		}
		select {
		case <-ctx.Done():
			return
		case <-t.wake:
		}
	}
}

// callFunc calls t's Func if its open run is working now. When the Func
// returns on its own, the run ends, and callFunc returns it with the Func's
// summary or error. When the window's close or Stop cancels the Func, they
// end the run themselves. A shutdown leaves the run unrecorded.
func (r *Runner) callFunc(ctx context.Context, t *task) (*run, Outcome) {
	r.mu.Lock()
	if t.call != nil || t.open == nil || !r.workingLocked(t, time.Now()) {
		r.mu.Unlock()
		return nil, Outcome{}
	}
	open, c := t.open, &flight{}
	cctx := c.start(ctx, r)
	t.call = c
	r.mu.Unlock()
	r.changed()

	err := guard(cctx, func() error { return t.fn(cctx) }, "task", t.Name)
	cancelled := cctx.Err() != nil
	c.cancel()

	r.mu.Lock()
	t.call = nil
	var ended *run
	out := Outcome{Summary: c.summary}
	if !cancelled && t.open == open {
		ended = t.endOpenLocked()
	}
	r.mu.Unlock()
	if ended == nil {
		r.changed()
		return nil, Outcome{}
	}
	if err != nil {
		out.Error = err.Error()
	}
	return ended, out
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

// guard calls a Func or Do. It turns a panic into an error and logs the
// stack.
func guard(ctx context.Context, f func() error, attrs ...any) (err error) {
	defer func() {
		if p := recover(); p != nil {
			slog.ErrorContext(ctx, "tasks: panicked", append(attrs, "panic", p, "stack", string(debug.Stack()))...)
			err = fmt.Errorf("panic: %v", p)
		}
	}()
	return f()
}

func (t *task) signal() {
	select {
	case t.wake <- struct{}{}:
	default:
	}
}

// Start is Run now. It begins a run of a scheduled task, or marks the open
// run to keep going past the window's close. Either way the run lasts
// until its work is done or Stop is called.
func (r *Runner) Start(ctx context.Context, name string) error {
	r.mu.Lock()
	t := r.byName[name]
	if t == nil || t.Schedule == OnDemand {
		r.mu.Unlock()
		return fmt.Errorf("%w: task %s", ErrNotFound, name)
	}
	now, begun := time.Now(), t.open == nil
	if begun {
		t.open = &run{started: now}
	}
	t.open.toEnd = true
	r.goOnLocked(t, now)
	t.signal() // wakes a pass that the window's close had paused
	r.mu.Unlock()
	if begun {
		slog.InfoContext(ctx, "tasks: run began", "task", t.Name)
	}
	return nil
}

// Stop stops a task. It cancels the work in flight, which stays pending,
// drops the items enqueued, and ends the run. Given items, it stops only
// those, as when one library's scan is stopped. A task whose work always
// finishes returns ErrNotStoppable.
func (r *Runner) Stop(ctx context.Context, name string, items ...id.ID) error {
	r.mu.Lock()
	t := r.byName[name]
	switch {
	case t == nil || len(items) > 0 && t.items == nil:
		r.mu.Unlock()
		return ErrNotFound
	case t.Unstoppable:
		r.mu.Unlock()
		return ErrNotStoppable
	}
	if len(items) > 0 {
		for _, i := range items {
			t.items.stopLocked(i)
		}
		r.mu.Unlock()
		r.changed()
		return nil
	}
	if w := t.items; w != nil {
		for i := range w.busy {
			w.stopLocked(i)
		}
		for i := range w.queued {
			w.stopLocked(i)
		}
	}
	ended := r.cutLocked(t)
	r.mu.Unlock()
	switch {
	case ended == nil:
	case t.items == nil:
		r.end(ctx, t, ended, Outcome{Summary: "Stopped"}, true)
	default:
		r.endItems(ctx, t, ended, "Stopped")
	}
	r.changed()
	return nil
}
