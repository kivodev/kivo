package tasks

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"runtime/debug"
	"slices"
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
	activityRun = time.Hour       // how often an Activity task looks for items left
	noticeEvery = 5 * time.Second // how often progress is published
	countFor    = 5 * time.Second // how long a task's Left count is reused
)

// Runner runs the tasks.
type Runner struct {
	db       *store.DB
	q        *dbq.Queries
	settings *config.Store // holds the window and the limits
	bus      *events.Bus
	ended    func(ctx context.Context, tx *store.Tx, e RunEnd) error // Options.Ended

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

	call  *flight // a plain task's Run in flight, or nil
	items *items  // an item task's work, or nil for a plain task
}

// run is a task's open run. It is held in memory only.
type run struct {
	started time.Time // used to log how long the run took
	// toEnd means Run now asked the run to finish, so it keeps working
	// after the window closes.
	toEnd bool
	done  int // items its passes settled
}

// items is an item task's work: its pass, the items asked for, and the
// items in flight.
type items struct {
	pass    *pass
	nextAct time.Time // when an Activity task next looks for items left

	asked  []id.ID
	queued map[id.ID]bool // the items in asked
	// again holds items asked for while in flight. Each is done once more
	// after it finishes.
	again map[id.ID]bool

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

// counted caches a task's Left count and its number of failed items for
// countFor.
type counted struct {
	at             time.Time
	left, failures int
}

// flight is a Run or Do call in flight. It holds how to cancel the call
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

// item is an item in flight.
type item struct {
	flight
	ctx     context.Context // the context Do gets, carrying the flight for Progress
	id      id.ID
	started time.Time // used to log how long the item took
	pass    *pass     // the pass it came from, or nil if it was asked for
}

// Add adds a task. Call it while wiring the server, before Run.
func (r *Runner) Add(t Task) {
	r.mu.Lock()
	defer r.mu.Unlock()
	plain, hasItems := t.Run != nil, t.Pending != nil && t.Do != nil
	switch {
	case r.started:
		panic("tasks: Add after Run")
	case t.Name == "" || plain == hasItems:
		panic("tasks: a task needs a name, and Run or Pending and Do: " + t.Name)
	case plain && t.Section() == SectionActivity:
		panic("tasks: a plain task needs a schedule: " + t.Name)
	case r.byName[t.Name] != nil:
		panic("tasks: task added twice: " + t.Name)
	}
	tk := &task{Task: t, wake: make(chan struct{}, 1)}
	if hasItems {
		tk.items = &items{queued: map[id.ID]bool{}, again: map[id.ID]bool{}, busy: map[id.ID]*item{}}
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
			wg.Go(func() { r.plainLoop(ctx, t) })
		} else {
			wg.Go(func() { r.itemLoop(ctx, t, &wg) })
		}
	}
	wg.Go(func() { r.schedule(ctx) })
	wg.Wait()
	return nil
}

// schedule begins the runs that are due and keeps night runs inside the
// window. It checks at once and then every tick.
func (r *Runner) schedule(ctx context.Context) {
	t := time.NewTicker(tick)
	defer t.Stop()
	for {
		r.look(ctx, time.Now())
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// look does one tick of the schedule:
//   - A night run begins while the window is open. It ends outside the
//     window, unless Run now asked it to finish.
//   - An interval run begins when it's due.
//   - A working run goes on.
//   - An Activity task looks for items left once an hour.
func (r *Runner) look(ctx context.Context, now time.Time) {
	r.mu.Lock()
	open := r.window().Contains(now)
	var begun []*task
	closed := map[*task]*run{}
	for _, t := range r.tasks {
		switch {
		case r.due(t, now, open):
			t.open = &run{started: now}
			begun = append(begun, t)
		case t.Nightly && !open && t.open != nil && !t.open.toEnd:
			closed[t] = r.cut(t)
			continue
		}
		r.goOn(t, now)
	}
	r.mu.Unlock()
	for _, t := range begun {
		slog.InfoContext(ctx, "tasks: run began", "task", t.Name)
	}
	for t, ended := range closed {
		if t.items == nil {
			r.end(ctx, t, ended, Outcome{Summary: "Stopped at the window's close"}, true)
		} else {
			r.endItems(ctx, t, ended, "")
		}
	}
}

// due reports whether t's schedule begins a run now. A run never begins
// while another is open or ending. A night task is due while the window is
// open and none of its runs has ended since the window opened. So a server
// started inside the window does tonight's work, but not twice. An
// interval task is due once its time comes.
func (r *Runner) due(t *task, now time.Time, open bool) bool {
	switch {
	case t.open != nil || t.ending != nil:
		return false
	case t.Nightly:
		return open && (t.last == nil || t.last.At.Before(r.window().Opened(now)))
	}
	return t.Every > 0 && !now.Before(r.next(t, now))
}

// goOn lets t's work go on where it may. A plain task's loop is woken while
// its open run is working. An item task gets a pass when it has none.
func (r *Runner) goOn(t *task, now time.Time) {
	if t.items == nil {
		if t.open != nil && r.working(t, now) {
			t.signal()
		}
		return
	}
	r.keepPassing(t, now)
}

// cut cancels the open run's work in flight and ends the run, returning
// it. The cancelled work stays pending. Items that were asked for go on.
// The caller holds Runner.mu.
func (r *Runner) cut(t *task) *run {
	if t.call != nil {
		t.call.cancel()
	}
	if t.items != nil {
		for _, it := range t.items.busy {
			if it.pass != nil {
				it.cancel()
			}
		}
		t.items.pass = nil
	}
	return t.endOpen()
}

// working reports whether t may work on its own now. An Activity task
// always may. Any other task may only while it has an open run, and a
// nightly run also needs the window open unless Run now asked it to
// finish.
func (r *Runner) working(t *task, now time.Time) bool {
	switch {
	case t.Section() == SectionActivity:
		return true
	case t.open == nil:
		return false
	}
	return t.open.toEnd || !t.Nightly || r.window().Contains(now)
}

// next is when t's schedule next begins a run, as of now. For a night task
// it is the window's next opening. For an interval task it is Every after
// the later of the last run's end and the server's start. For an Activity
// task it is zero.
func (r *Runner) next(t *task, now time.Time) time.Time {
	switch {
	case t.Nightly:
		return r.window().Next(now)
	case t.Every > 0:
		from := r.boot
		if t.last != nil && t.last.At.After(from) {
			from = t.last.At
		}
		return from.Add(t.Every)
	}
	return time.Time{}
}

// window is the maintenance window in the settings, read each time so a
// change applies without a restart.
func (r *Runner) window() Window {
	st := r.settings.Get()
	return Window{Start: st.MaintenanceStart, End: st.MaintenanceEnd}
}

// endOpen ends t's open run, if any, and returns it. The run stays in
// t.ending until end records how it ended, so no run begins meanwhile.
// The caller holds Runner.mu.
func (t *task) endOpen() *run {
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
	if (t.Nightly || ended.toEnd) && (did || out.Error != "") {
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
	if t.Failed != nil {
		if failed, err := t.Failed(ctx); err == nil {
			e.Failed = len(failed)
		}
	}
	return e
}

func nullString(s string) sql.NullString { return sql.NullString{String: s, Valid: s != ""} }

// plainLoop calls a plain task's Run while its open run is working, then
// waits to be woken.
func (r *Runner) plainLoop(ctx context.Context, t *task) {
	for {
		if ended, out := r.callRun(ctx, t); ended != nil {
			r.end(ctx, t, ended, out, true)
		}
		select {
		case <-ctx.Done():
			return
		case <-t.wake:
		}
	}
}

// callRun calls t's Run if its open run is working now. When Run returns on
// its own, the run ends, and callRun returns it with Run's summary or
// error. When the window's close or Stop cancels Run, they end the run
// themselves. A shutdown leaves the run unrecorded.
func (r *Runner) callRun(ctx context.Context, t *task) (*run, Outcome) {
	r.mu.Lock()
	if t.call != nil || t.open == nil || !r.working(t, time.Now()) {
		r.mu.Unlock()
		return nil, Outcome{}
	}
	open, c := t.open, &flight{}
	cctx := c.start(ctx, r)
	t.call = c
	r.mu.Unlock()
	r.changed()

	err := guard(cctx, func() error { return t.Run(cctx) }, "task", t.Name)
	cancelled := cctx.Err() != nil
	c.cancel()

	r.mu.Lock()
	t.call = nil
	var ended *run
	out := Outcome{Summary: c.summary}
	if !cancelled && t.open == open {
		ended = t.endOpen()
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

// itemLoop starts an item task's items whenever one may start, then waits
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
		p := r.ranOut(t)
		if p == nil {
			it = r.claim(ctx, t)
			if it == nil {
				p = r.ranOut(t) // the claim went through the rest of the list
			}
		}
		r.mu.Unlock()
		switch {
		case it != nil:
			wg.Go(func() { r.do(t, it) })
		case p != nil:
			list, err := t.Pending(ctx)
			r.mu.Lock()
			ended := r.refill(t, p, list, err)
			r.mu.Unlock()
			if ended != nil {
				r.endItems(ctx, t, ended, "")
			}
		default:
			return
		}
	}
}

// ranOut returns t's pass when it is time to read Pending again: the task
// is working, the pass's list is used up, and none of the pass's items are
// in flight. Otherwise it returns nil.
func (r *Runner) ranOut(t *task) *pass {
	p := t.items.pass
	if p == nil || p.pos < len(p.list) || !r.working(t, time.Now()) {
		return nil
	}
	for _, it := range t.items.busy {
		if it.pass == p {
			return nil
		}
	}
	return p
}

// refill sets a pass's next list from what Pending read, less the items in
// flight, asked for, or already done this pass. If nothing new is left,
// the pass ends and so does the run, which refill returns.
func (r *Runner) refill(t *task, p *pass, list []id.ID, err error) *run {
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
		return t.endOpen()
	}
	p.list, p.pos = list, 0
	return nil
}

// keepPassing gives an item task a pass when it has none and should have
// one. An Activity task gets one each hour. Any other task gets one while
// its open run is working.
func (r *Runner) keepPassing(t *task, now time.Time) {
	w := t.items
	switch {
	case w.pass != nil:
	case t.Section() == SectionActivity:
		if !now.Before(w.nextAct) {
			w.nextAct = now.Add(activityRun)
			r.startPass(t)
		}
	case t.open != nil && r.working(t, now):
		r.startPass(t)
	}
}

// startPass begins a pass over the items t has left.
func (r *Runner) startPass(t *task) {
	t.items.pass = &pass{did: map[id.ID]bool{}}
	t.signal()
	r.changed()
}

// claim picks the next item t should start now, if its limit allows one.
// Items asked for come first. Otherwise it takes the pass's next item,
// while the task is working.
func (r *Runner) claim(ctx context.Context, t *task) *item {
	w, limit := t.items, r.limit(t)
	var it *item
	switch {
	case len(w.busy) >= limit:
	case len(w.asked) > 0:
		it = &item{id: w.asked[0]}
		w.asked = w.asked[1:]
	case w.pass != nil && r.working(t, time.Now()):
		it = w.nextListed() // nil when the list ran out, so fill reads Pending again
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
	if t.Limit == nil {
		return 1
	}
	return max(1, t.Limit(r.settings.Get()))
}

// nextListed takes the pass's next item that isn't done, in flight, or
// asked for. It returns nil when the list runs out.
func (w *items) nextListed() *item {
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
// If it was asked for again while in flight, it goes back in line.
func (r *Runner) do(t *task, it *item) {
	err := guard(it.ctx, func() error { return t.Do(it.ctx, it.id) }, "task", t.Name, "item", it.id)
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
	if w.again[it.id] {
		delete(w.again, it.id)
		w.enqueue(it.id)
	}
	r.mu.Unlock()
	if err != nil && !cancelled {
		slog.Warn("tasks: item left for a later pass", "task", t.Name, "item", it.id, "err", err)
	}
	t.signal()
	r.changed()
}

// endItems ends an item task's run. Its summary says how many items the run
// settled and how many are left, after prefix, such as "Stopped".
func (r *Runner) endItems(ctx context.Context, t *task, ended *run, prefix string) {
	left := 0
	if t.Left != nil {
		var err error
		if left, err = t.Left(ctx); err != nil {
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

// guard calls Run or Do. It turns a panic into an error and logs the
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
	if t == nil || t.Section() == SectionActivity {
		r.mu.Unlock()
		return fmt.Errorf("%w: task %s", ErrNotFound, name)
	}
	now, begun := time.Now(), t.open == nil
	if begun {
		t.open = &run{started: now}
	}
	t.open.toEnd = true
	r.goOn(t, now)
	t.signal() // wakes a pass that the window's close had paused
	r.mu.Unlock()
	if begun {
		slog.InfoContext(ctx, "tasks: run began", "task", t.Name)
	}
	return nil
}

// Stop stops a task. It cancels the work in flight, which stays pending,
// drops the items asked for, and ends the run. Given items, it stops only
// those, as when one library's scan is stopped. A task whose work always
// finishes returns ErrNotStoppable.
func (r *Runner) Stop(ctx context.Context, name string, items ...id.ID) error {
	r.mu.Lock()
	t := r.byName[name]
	switch {
	case t == nil || len(items) > 0 && t.items == nil:
		r.mu.Unlock()
		return ErrNotFound
	case t.Finish:
		r.mu.Unlock()
		return ErrNotStoppable
	}
	if len(items) > 0 {
		for _, i := range items {
			t.items.stop(i)
		}
		r.mu.Unlock()
		r.changed()
		return nil
	}
	if w := t.items; w != nil {
		for i := range w.busy {
			w.stop(i)
		}
		for i := range w.queued {
			w.stop(i)
		}
	}
	ended := r.cut(t)
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

// stop cancels an item in flight, removes it from the line, and marks it
// done for the current pass.
func (w *items) stop(i id.ID) {
	if it := w.busy[i]; it != nil {
		it.cancel()
	}
	if w.queued[i] {
		delete(w.queued, i)
		w.asked = slices.DeleteFunc(w.asked, func(x id.ID) bool { return x == i })
	}
	delete(w.again, i)
	if w.pass != nil {
		w.pass.did[i] = true
	}
}

// Ask has an item task do the given items now, whether or not the window is
// open. They go ahead of the run's items, in the order asked and within
// the task's limit, and no run begins. An item asked for while in flight is
// done once more afterwards. Asks are held in memory only. An ask lost to a
// restart is still pending in the task's tables.
func (r *Runner) Ask(name string, items ...id.ID) {
	r.mu.Lock()
	defer r.mu.Unlock()
	t := r.byName[name]
	if t == nil || t.items == nil {
		panic("tasks: ask of a task with no items: " + name)
	}
	t.items.enqueue(items...)
	t.signal()
	r.changed()
}

// enqueue puts items in line, each once.
func (w *items) enqueue(items ...id.ID) {
	for _, i := range items {
		switch {
		case w.busy[i] != nil:
			w.again[i] = true
		case !w.queued[i]:
			w.asked = append(w.asked, i)
			w.queued[i] = true
		}
	}
}

// Failed lists a task's items that failed for good.
func (r *Runner) Failed(ctx context.Context, name string) ([]Failure, error) {
	t := r.task(name)
	if t == nil || t.Failed == nil {
		return nil, ErrNotFound
	}
	return t.Failed(ctx)
}

// Retry forgets the failures of a task's given items, or of all its failed
// items when none are given, and asks for them. The task's counts are read
// afresh afterwards.
func (r *Runner) Retry(ctx context.Context, name string, items ...id.ID) error {
	t := r.task(name)
	if t == nil || t.Retry == nil {
		return ErrNotFound
	}
	again, err := t.Retry(ctx, items...)
	if err != nil {
		return err
	}
	r.mu.Lock()
	t.items.counted = counted{}
	r.mu.Unlock()
	r.Ask(name, again...)
	return nil
}

// Items returns a task's items in flight, with the progress each reported,
// in the order they started.
func (r *Runner) Items(name string) []Item {
	r.mu.Lock()
	defer r.mu.Unlock()
	if t := r.byName[name]; t != nil && t.items != nil {
		return t.items.list()
	}
	return nil
}

// Asked returns a task's items asked for that haven't started yet.
func (r *Runner) Asked(name string) []id.ID {
	r.mu.Lock()
	defer r.mu.Unlock()
	if t := r.byName[name]; t != nil && t.items != nil {
		return slices.Clone(t.items.asked)
	}
	return nil
}

// Busy counts each task's work in flight: its items, or one for a plain
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
// progress of its Run in flight or its items.
func (r *Runner) status(ctx context.Context, t *task) (Status, error) {
	now := time.Now()
	r.mu.Lock()
	s := Status{Task: t.Task, Last: t.last, Next: r.next(t, now)}
	if t.items == nil {
		s.Running = t.call != nil || t.open != nil && r.working(t, now) || t.ending != nil
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

// itemStatus fills in an item task's Status. While its run is working,
// progress is the items settled out of those settled plus those left.
// Otherwise progress is that of the first item in flight, if it reports
// any. Asked-for items get no count of their own here, because whoever
// asked for them shows their progress.
func (r *Runner) itemStatus(ctx context.Context, t *task, s *Status, now time.Time) error {
	r.mu.Lock()
	w := t.items
	s.Items = w.list()
	working := t.open != nil && w.pass != nil && r.working(t, now)
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
	if t.Left != nil {
		if working {
			s.Done, s.Total = done, done+c.left
		} else {
			s.Left = c.left
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

// list returns the items in flight, in the order they started. The caller
// holds Runner.mu.
func (w *items) list() []Item {
	out := make([]Item, 0, len(w.busy))
	for _, it := range w.busy {
		out = append(out, it.status())
	}
	slices.SortFunc(out, func(a, b Item) int { return a.started.Compare(b.started) })
	return out
}

// count returns t's Left count and its number of failed items. A result is
// reused for countFor.
func (r *Runner) count(ctx context.Context, t *task, now time.Time) (counted, error) {
	r.mu.Lock()
	c := t.items.counted
	r.mu.Unlock()
	if now.Sub(c.at) < countFor {
		return c, nil
	}
	c = counted{at: now}
	if t.Left != nil {
		var err error
		if c.left, err = t.Left(ctx); err != nil {
			return counted{}, err
		}
	}
	if t.Failed != nil {
		failed, err := t.Failed(ctx)
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

// status returns the item as Status lists it. The caller holds Runner.mu.
func (it *item) status() Item {
	return Item{ID: it.id, Step: it.step, Done: it.done, Total: it.total, Since: it.since, started: it.started}
}

// start prepares f to run under ctx. It returns the context to pass to Run
// or Do, which carries f for Progress and Summarize.
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
