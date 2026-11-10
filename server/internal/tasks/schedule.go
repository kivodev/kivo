package tasks

import (
	"context"
	"log/slog"
	"time"
)

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
//   - An on-demand task looks for pending items once an hour.
func (r *Runner) look(ctx context.Context, now time.Time) {
	r.mu.Lock()
	open := r.window().Contains(now)
	var begun []*task
	closed := map[*task]*run{}
	for _, t := range r.tasks {
		switch {
		case r.dueLocked(t, now, open):
			t.open = &run{started: now}
			begun = append(begun, t)
		case t.Schedule == Nightly && !open && t.open != nil && !t.open.toEnd:
			closed[t] = r.cutLocked(t)
			continue
		}
		r.goOnLocked(t, now)
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

// dueLocked reports whether t's schedule begins a run now. A run never
// begins while another is open or ending. A night task is due while the
// window is open and none of its runs has ended since the window opened. So
// a server started inside the window does tonight's work, but not twice. An
// interval task is due once its time comes.
func (r *Runner) dueLocked(t *task, now time.Time, open bool) bool {
	switch {
	case t.open != nil || t.ending != nil:
		return false
	case t.Schedule == Nightly:
		return open && (t.last == nil || t.last.At.Before(r.window().Opened(now)))
	}
	return t.Schedule.every > 0 && !now.Before(r.nextLocked(t, now))
}

// goOnLocked lets t's work go on where it may. A Func task's loop is woken
// while its open run is working. An Items task gets a pass when it has
// none.
func (r *Runner) goOnLocked(t *task, now time.Time) {
	if t.items == nil {
		if t.open != nil && r.workingLocked(t, now) {
			t.signal()
		}
		return
	}
	r.keepPassingLocked(t, now)
}

// cutLocked cancels the open run's work in flight and ends the run,
// returning it. The cancelled work stays pending. Items that were enqueued
// go on.
func (r *Runner) cutLocked(t *task) *run {
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
	return t.endOpenLocked()
}

// workingLocked reports whether t may work on its own now. An on-demand
// task always may. Any other task may only while it has an open run, and a
// nightly run also needs the window open unless Run now asked it to finish.
func (r *Runner) workingLocked(t *task, now time.Time) bool {
	switch {
	case t.Schedule == OnDemand:
		return true
	case t.open == nil:
		return false
	}
	return t.open.toEnd || t.Schedule != Nightly || r.window().Contains(now)
}

// nextLocked is when t's schedule next begins a run, as of now. For a night
// task it is the window's next opening. For an interval task it is the
// interval after the later of the last run's end and the server's start.
// For an on-demand task it is zero.
func (r *Runner) nextLocked(t *task, now time.Time) time.Time {
	switch {
	case t.Schedule == Nightly:
		return r.window().Next(now)
	case t.Schedule.every > 0:
		from := r.boot
		if t.last != nil && t.last.At.After(from) {
			from = t.last.At
		}
		return from.Add(t.Schedule.every)
	}
	return time.Time{}
}

// window is the maintenance window in the settings, read each time so a
// change applies without a restart.
func (r *Runner) window() Window {
	st := r.settings.Get()
	return Window{Start: st.MaintenanceStart, End: st.MaintenanceEnd}
}
