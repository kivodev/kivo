package health

import (
	"context"
	"encoding"
	"fmt"
	"io"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/mmcdole/kivo/internal/events"
)

// Logs keeps the newest log records in memory for admins to read. It
// publishes logsChanged for each line kept at Info or above. It is safe for
// concurrent use.
type Logs struct {
	bus   *events.Bus
	mu    sync.Mutex
	lines []LogLine // a ring once full, with the oldest line at start
	start int
	seq   int64
}

// NewLogs returns a Logs that keeps up to capacity lines and publishes on
// bus.
func NewLogs(capacity int, bus *events.Bus) *Logs {
	return &Logs{bus: bus, lines: make([]LogLine, 0, capacity)}
}

// Handler wraps next so that a copy of each record it handles is kept.
// Records that next doesn't enable, such as Debug records below the
// server's log level, aren't kept.
func (l *Logs) Handler(next slog.Handler) slog.Handler {
	return &logTee{logs: l, next: next}
}

// Lines returns the kept lines that match f, oldest first.
func (l *Logs) Lines(f LogFilter) []LogLine {
	l.mu.Lock()
	all := make([]LogLine, 0, len(l.lines))
	all = append(all, l.lines[l.start:]...)
	all = append(all, l.lines[:l.start]...)
	l.mu.Unlock()
	search := strings.ToLower(f.Search)
	return slices.DeleteFunc(all, func(line LogLine) bool {
		return line.Seq <= f.After || line.Level < f.MinLevel || search != "" && !strings.Contains(line.text(), search)
	})
}

// LogLine is one record the server logged.
type LogLine struct {
	Seq     int64 // counts up from 1 for each line kept
	Time    time.Time
	Level   slog.Level
	Message string
	// Attrs are the record's attributes and its logger's, flattened. A
	// group's keys are dotted, as in "req.id". Values other than numbers,
	// booleans and durations are stored as the text the text log writes, so
	// each value's String is what the text log shows.
	Attrs []slog.Attr
}

// text returns the line's message and attributes, lowercased, for search.
func (l LogLine) text() string {
	var b strings.Builder
	b.WriteString(l.Message)
	for _, a := range l.Attrs {
		b.WriteString(" " + a.Key + "=" + a.Value.String())
	}
	return strings.ToLower(b.String())
}

// LogFilter picks lines from Logs.
type LogFilter struct {
	MinLevel slog.Level // the zero value is Info
	// Search, when set, keeps lines whose message or attributes contain it,
	// ignoring case.
	Search string
	// After keeps lines whose Seq is greater than After.
	After int64
}

// WriteText writes lines as the server's text log writes them.
func WriteText(w io.Writer, lines []LogLine) error {
	h := slog.NewTextHandler(w, nil)
	for _, line := range lines {
		r := slog.NewRecord(line.Time, line.Level, line.Message, 0)
		r.AddAttrs(line.Attrs...)
		if err := h.Handle(context.Background(), r); err != nil {
			return err
		}
	}
	return nil
}

type logTee struct {
	logs   *Logs
	next   slog.Handler
	attrs  []slog.Attr // the logger's attributes, flattened
	prefix string      // the open groups, each followed by a dot
}

func (h *logTee) Enabled(ctx context.Context, level slog.Level) bool {
	return h.next.Enabled(ctx, level)
}

func (h *logTee) Handle(ctx context.Context, r slog.Record) error {
	line := LogLine{Time: r.Time, Level: r.Level, Message: r.Message, Attrs: slices.Clip(h.attrs)}
	r.Attrs(func(a slog.Attr) bool {
		line.Attrs = flatten(line.Attrs, h.prefix, a)
		return true
	})
	h.logs.add(line)
	return h.next.Handle(ctx, r)
}

// add keeps line, replacing the oldest line once the ring is full. A Debug
// line publishes nothing, because every request logs one, including a
// request for the logs.
func (l *Logs) add(line LogLine) {
	l.mu.Lock()
	l.seq++
	line.Seq = l.seq
	if len(l.lines) < cap(l.lines) {
		l.lines = append(l.lines, line)
	} else {
		l.lines[l.start] = line
		l.start = (l.start + 1) % len(l.lines)
	}
	l.mu.Unlock()
	if line.Level >= slog.LevelInfo {
		l.bus.Publish(events.Event{Kind: events.LogsChanged, Scope: events.Scope{AdminOnly: true}})
	}
}

func (h *logTee) WithAttrs(as []slog.Attr) slog.Handler {
	out := *h
	out.next = h.next.WithAttrs(as)
	out.attrs = slices.Clip(h.attrs)
	for _, a := range as {
		out.attrs = flatten(out.attrs, h.prefix, a)
	}
	return &out
}

func (h *logTee) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	out := *h
	out.next = h.next.WithGroup(name)
	out.prefix += name + "."
	return &out
}

// flatten appends a to dst as the text handler would write it. Groups
// become dotted keys, and other values become their text. So a line holds
// nothing that can change after it's logged.
func flatten(dst []slog.Attr, prefix string, a slog.Attr) []slog.Attr {
	a.Value = a.Value.Resolve()
	if a.Equal(slog.Attr{}) {
		return dst
	}
	if a.Value.Kind() == slog.KindGroup {
		if a.Key != "" {
			prefix += a.Key + "."
		}
		for _, g := range a.Value.Group() {
			dst = flatten(dst, prefix, g)
		}
		return dst
	}
	switch a.Value.Kind() {
	case slog.KindAny:
		a.Value = slog.StringValue(anyText(a.Value.Any()))
	case slog.KindTime:
		a.Value = slog.StringValue(a.Value.Time().Format("2006-01-02T15:04:05.000Z07:00"))
	}
	return append(dst, slog.Attr{Key: prefix + a.Key, Value: a.Value})
}

// anyText returns a value's text as the text handler writes it.
func anyText(v any) string {
	if tm, ok := v.(encoding.TextMarshaler); ok {
		b, err := tm.MarshalText()
		if err != nil {
			return "!ERROR:" + err.Error()
		}
		return string(b)
	}
	return fmt.Sprintf("%+v", v)
}
