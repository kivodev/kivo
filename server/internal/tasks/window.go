package tasks

import "time"

// Window is the nightly maintenance window, in whole hours of the server's
// local time. A night run begins while the window is open and ends when it
// closes. An End that is not after Start falls on the next day.
type Window struct{ Start, End int }

// Contains reports whether t is inside the window.
func (w Window) Contains(t time.Time) bool {
	h := t.Local().Hour()
	if w.Start < w.End {
		return h >= w.Start && h < w.End
	}
	return h >= w.Start || h < w.End
}

// Next is when the window next opens after t.
func (w Window) Next(t time.Time) time.Time {
	t = t.Local()
	open := time.Date(t.Year(), t.Month(), t.Day(), w.Start, 0, 0, 0, time.Local)
	if open.After(t) {
		return open
	}
	return time.Date(t.Year(), t.Month(), t.Day()+1, w.Start, 0, 0, 0, time.Local)
}

// Opened is when the window last opened at or before t.
func (w Window) Opened(t time.Time) time.Time {
	t = t.Local()
	open := time.Date(t.Year(), t.Month(), t.Day(), w.Start, 0, 0, 0, time.Local)
	if open.After(t) {
		return time.Date(t.Year(), t.Month(), t.Day()-1, w.Start, 0, 0, 0, time.Local)
	}
	return open
}
