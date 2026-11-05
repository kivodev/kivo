package events

import (
	"bytes"
	"slices"
	"sync"
	"time"

	"github.com/mmcdole/kivo/internal/fault"
	"github.com/mmcdole/kivo/internal/id"
)

// ErrLagged means a subscriber fell behind and its buffer overflowed. Its
// channel is closed, and the client should resync by refetching.
var ErrLagged = fault.New(fault.Internal, "subscriber lagged")

// buffer is how many events a subscriber can fall behind before it lags.
const buffer = 256

// Kind names an event type. Its value is the name clients see on the wire.
type Kind string

const (
	// WorkChanged says one work's detail changed. Data is the work's id.ID.
	WorkChanged Kind = "workChanged"
	// LibraryChanged says libraries were created or edited, root works were
	// added to or removed from them, or a library is being deleted. It is
	// scoped to those libraries, and api narrows it to the ones each viewer
	// sees.
	LibraryChanged Kind = "libraryChanged"
	// UserStateChanged says a user's state for some works changed. It is
	// scoped to the user. Data is the works, as an []id.ID.
	UserStateChanged Kind = "userStateChanged"
	// PlayEnded says a play ended for a reason other than the server
	// stopping: an admin or another client stopped it, or it expired. It is
	// scoped to the play's user. Data is a playback.Ended.
	PlayEnded Kind = "playEnded"
	// DownloadsChanged says a user's downloads on the server changed: one
	// moved in line, progressed in its preparation, or became ready or
	// failed. It is scoped to the user.
	DownloadsChanged Kind = "downloadsChanged"
	// TasksChanged says a task's work changed: an item started, finished
	// or reported progress, a run began or ended, or something was asked
	// for. It is admin-only. A library's scan is a task, so scan changes
	// send it too.
	TasksChanged Kind = "tasksChanged"
	// PlaysChanged says a play started or ended, or a heartbeat moved its
	// position or paused it. It is admin-only.
	PlaysChanged Kind = "playsChanged"
	// ProblemsChanged says the open problems changed. It is admin-only.
	// Data is the number of open problems, as an int.
	ProblemsChanged Kind = "problemsChanged"
	// LogsChanged says the server logged lines at Info or above. It is
	// admin-only.
	LogsChanged Kind = "logsChanged"
	// ActivityChanged says an event was recorded in activity. It is
	// admin-only.
	ActivityChanged Kind = "activityChanged"
)

// limits lists the kinds that go out at most once per interval, with each
// kind's interval. The first event of a kind after a quiet spell starts the
// interval. Events published during the interval merge into the waiting
// events, which go out when the interval ends. An event of a limited kind
// carries no Data. Two events merge when their scopes have the same User
// and AdminOnly, and their Libraries are joined.
var limits = map[Kind]time.Duration{
	// A scan adds root works one file at a time. A few seconds shows a new film
	// soon, without each viewer refetching Home for every file.
	LibraryChanged:   3 * time.Second,
	DownloadsChanged: time.Second,
	TasksChanged:     time.Second,
	PlaysChanged:     time.Second,
	LogsChanged:      time.Second,
	ActivityChanged:  time.Second,
}

// Scope says who may see an event. The zero value means everyone.
type Scope struct {
	Libraries []id.ID // if set, only viewers granted one of these
	User      id.ID   // if set, only this user
	AdminOnly bool
}

// Event is a hint that something changed. It carries only IDs and counts.
// Clients refetch the content through query.
type Event struct {
	Kind  Kind
	Scope Scope
	Data  any
}

// Bus fans events out to subscribers without ever blocking a publisher. It
// holds each limited kind to its interval.
type Bus struct {
	mu      sync.Mutex
	subs    map[*Sub]struct{}
	waiting map[Kind][]Event     // merged events of limited kinds, held until the interval ends
	timers  map[Kind]*time.Timer // the running intervals
	closed  bool                 // Close was called, so limited kinds no longer go out
}

// NewBus returns an empty bus.
func NewBus() *Bus {
	return &Bus{subs: map[*Sub]struct{}{}, waiting: map[Kind][]Event{}, timers: map[Kind]*time.Timer{}}
}

// Sub is one subscription.
type Sub struct {
	bus    *Bus
	filter func(Scope) bool
	ch     chan Event
	err    error // set under bus.mu when the subscription ends
	closed bool  // guarded by bus.mu
}

// Subscribe starts a subscription. filter is built from the subscriber's
// Viewer. It runs before buffering, so events it rejects take no space.
func (b *Bus) Subscribe(filter func(Scope) bool) *Sub {
	s := &Sub{bus: b, filter: filter, ch: make(chan Event, buffer)}
	b.mu.Lock()
	b.subs[s] = struct{}{}
	b.mu.Unlock()
	return s
}

// Publish delivers e to every subscriber whose filter accepts it. Most
// kinds go out at once. A limited kind goes out when its interval ends.
// Publish never blocks. A subscriber whose buffer is full is closed with
// ErrLagged.
func (b *Bus) Publish(e Event) {
	b.mu.Lock()
	defer b.mu.Unlock()
	every, limited := limits[e.Kind]
	if !limited {
		b.deliver(e)
		return
	}
	if b.closed {
		return
	}
	if b.timers[e.Kind] == nil {
		b.timers[e.Kind] = time.AfterFunc(every, func() { b.release(e.Kind) })
	}
	b.waiting[e.Kind] = merge(b.waiting[e.Kind], e)
}

// Close ends the intervals of the limited kinds. It drops their waiting
// events and any published afterwards. Other kinds still go out.
func (b *Bus) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.closed = true
	for _, t := range b.timers {
		t.Stop()
	}
	clear(b.timers)
	clear(b.waiting)
}

// release delivers the events of kind k waiting for its interval to end.
func (b *Bus) release(k Kind) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return
	}
	for _, e := range b.waiting[k] {
		b.deliver(e)
	}
	delete(b.waiting, k)
	delete(b.timers, k)
}

// deliver sends e to every subscriber whose filter accepts it. The caller
// holds b.mu. Sends and closes both happen under the mutex, because closing
// a channel during a send would panic.
func (b *Bus) deliver(e Event) {
	for s := range b.subs {
		if !s.filter(e.Scope) {
			continue
		}
		select {
		case s.ch <- e:
		default:
			s.end(ErrLagged)
		}
	}
}

// merge adds e to the waiting events of its kind. If a waiting event's
// scope has the same User and AdminOnly, e merges into it and their
// libraries are joined. Otherwise e is added as a new event.
func merge(waiting []Event, e Event) []Event {
	for i, w := range waiting {
		if w.Scope.User == e.Scope.User && w.Scope.AdminOnly == e.Scope.AdminOnly {
			waiting[i].Scope.Libraries = join(w.Scope.Libraries, e.Scope.Libraries)
			return waiting
		}
	}
	sc := e.Scope
	sc.Libraries = join(nil, e.Scope.Libraries)
	return append(waiting, Event{Kind: e.Kind, Scope: sc})
}

// join returns the IDs in a or b, each once, sorted, in a new slice.
func join(a, b []id.ID) []id.ID {
	if len(a)+len(b) == 0 {
		return nil
	}
	out := slices.Concat(a, b)
	slices.SortFunc(out, func(x, y id.ID) int { return bytes.Compare(x[:], y[:]) })
	return slices.Compact(out)
}

// C is the subscription's channel, closed when the subscription ends.
func (s *Sub) C() <-chan Event { return s.ch }

// Err returns ErrLagged if the subscription ended by overflowing, and nil
// otherwise.
func (s *Sub) Err() error {
	s.bus.mu.Lock()
	defer s.bus.mu.Unlock()
	return s.err
}

// Close ends the subscription. It is safe to call more than once and
// concurrently with Publish.
func (s *Sub) Close() {
	s.bus.mu.Lock()
	defer s.bus.mu.Unlock()
	s.end(nil)
}

// end closes s. The caller holds bus.mu.
func (s *Sub) end(err error) {
	if s.closed {
		return
	}
	s.closed = true
	s.err = err
	close(s.ch)
	delete(s.bus.subs, s)
}
