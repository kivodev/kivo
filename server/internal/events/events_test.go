package events

import (
	"errors"
	"slices"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/mmcdole/kivo/internal/id"
)

func all(Scope) bool { return true }

func TestDelivery(t *testing.T) {
	b := NewBus()
	s := b.Subscribe(all)
	defer s.Close()
	b.Publish(Event{Kind: "a"})
	if e := <-s.C(); e.Kind != "a" {
		t.Fatalf("got %v", e)
	}
}

func TestPublishNeverBlocksAndLaggardIsClosed(t *testing.T) {
	b := NewBus()
	slow := b.Subscribe(all)
	ok := b.Subscribe(all)
	for i := range buffer + 10 {
		b.Publish(Event{Kind: "x", Data: i}) // would deadlock if Publish blocked
		<-ok.C()
	}
	n := 0
	for range slow.C() {
		n++
	}
	if n != buffer || !errors.Is(slow.Err(), ErrLagged) {
		t.Fatalf("slow got %d events, err %v; want %d, ErrLagged", n, slow.Err(), buffer)
	}
	if ok.Err() != nil {
		t.Fatalf("reader that kept up lagged: %v", ok.Err())
	}
	ok.Close()
}

func TestFilterRunsBeforebuffering(t *testing.T) {
	b := NewBus()
	lib := id.New()
	s := b.Subscribe(func(sc Scope) bool { return !sc.AdminOnly })
	defer s.Close()
	for range buffer * 4 {
		b.Publish(Event{Kind: "secret", Scope: Scope{AdminOnly: true}})
	}
	b.Publish(Event{Kind: "visible", Scope: Scope{Libraries: []id.ID{lib}}})
	if e := <-s.C(); e.Kind != "visible" || s.Err() != nil {
		t.Fatalf("got %v, err %v; rejected events must not fill the buffer", e, s.Err())
	}
}

func TestCloseIsIdempotentAndSafeWithPublish(t *testing.T) {
	b := NewBus()
	var wg sync.WaitGroup
	for range 50 {
		s := b.Subscribe(all)
		wg.Go(func() {
			for range 100 {
				b.Publish(Event{Kind: "x"})
			}
		})
		wg.Go(func() { s.Close(); s.Close() })
	}
	wg.Wait()
	if len(b.subs) != 0 {
		t.Fatalf("%d subscriptions left", len(b.subs))
	}
}

func TestCloseReportsNoError(t *testing.T) {
	b := NewBus()
	s := b.Subscribe(all)
	s.Close()
	if _, open := <-s.C(); open || s.Err() != nil {
		t.Fatalf("open=%v err=%v; want closed with no error", open, s.Err())
	}
}

// collect returns the events s has been sent so far.
func collect(s *Sub) []Event {
	var out []Event
	for {
		select {
		case e := <-s.C():
			out = append(out, e)
		default:
			return out
		}
	}
}

// Each limited kind goes out at most once per interval. A burst becomes one
// event when the interval ends, and a later change becomes another.
func TestLimitedKindsGoOutOncePerInterval(t *testing.T) {
	for kind, every := range limits {
		t.Run(string(kind), func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				b := NewBus()
				defer b.Close()
				s := b.Subscribe(all)
				defer s.Close()
				for range 100 {
					b.Publish(Event{Kind: kind, Scope: Scope{AdminOnly: true}})
				}
				time.Sleep(every - time.Millisecond)
				synctest.Wait()
				if got := collect(s); len(got) != 0 {
					t.Fatalf("%d events before the interval ended", len(got))
				}
				time.Sleep(time.Millisecond)
				synctest.Wait()
				if got := collect(s); len(got) != 1 || got[0].Kind != kind || !got[0].Scope.AdminOnly {
					t.Fatalf("after a burst got %+v; want one %s", got, kind)
				}
				b.Publish(Event{Kind: kind, Scope: Scope{AdminOnly: true}})
				time.Sleep(every)
				synctest.Wait()
				if got := collect(s); len(got) != 1 {
					t.Fatalf("after a later change got %+v; want one", got)
				}
			})
		})
	}
}

// libraryChanged goes out at most every 3 seconds. It names each library
// from the burst once.
func TestLibraryChangedJoinsLibraries(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		b := NewBus()
		defer b.Close()
		s := b.Subscribe(all)
		defer s.Close()
		x, y := id.New(), id.New()
		b.Publish(Event{Kind: LibraryChanged, Scope: Scope{Libraries: []id.ID{y}}})
		b.Publish(Event{Kind: LibraryChanged, Scope: Scope{Libraries: []id.ID{x, y}}})
		time.Sleep(2 * time.Second)
		b.Publish(Event{Kind: LibraryChanged, Scope: Scope{Libraries: []id.ID{x}}})
		time.Sleep(time.Second)
		synctest.Wait()
		got := collect(s)
		if len(got) != 1 || len(got[0].Scope.Libraries) != 2 ||
			!slices.Contains(got[0].Scope.Libraries, x) || !slices.Contains(got[0].Scope.Libraries, y) {
			t.Fatalf("got %+v; want one libraryChanged naming x and y", got)
		}
		b.Publish(Event{Kind: LibraryChanged, Scope: Scope{Libraries: []id.ID{x}}})
		time.Sleep(3 * time.Second)
		synctest.Wait()
		if got := collect(s); len(got) != 1 || !slices.Equal(got[0].Scope.Libraries, []id.ID{x}) {
			t.Fatalf("got %+v; want one naming x", got)
		}
	})
}

// downloadsChanged goes out at most once a second, as one event for each
// user whose downloads changed.
func TestDownloadsChangedIsOnePerUser(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		b := NewBus()
		defer b.Close()
		s := b.Subscribe(all)
		defer s.Close()
		u, v := id.New(), id.New()
		for range 10 {
			b.Publish(Event{Kind: DownloadsChanged, Scope: Scope{User: u}})
			b.Publish(Event{Kind: DownloadsChanged, Scope: Scope{User: v}})
		}
		time.Sleep(time.Second)
		synctest.Wait()
		got := collect(s)
		var users []id.ID
		for _, e := range got {
			users = append(users, e.Scope.User)
		}
		if len(got) != 2 || !slices.Contains(users, u) || !slices.Contains(users, v) {
			t.Fatalf("got %+v; want one event for each user", got)
		}
	})
}

// Other kinds go out at once, every one.
func TestOtherKindsGoOutAtOnce(t *testing.T) {
	b := NewBus()
	s := b.Subscribe(all)
	defer s.Close()
	for _, k := range []Kind{WorkChanged, UserStateChanged, PlayEnded, ProblemsChanged, WorkChanged} {
		b.Publish(Event{Kind: k})
	}
	if got := collect(s); len(got) != 5 {
		t.Fatalf("got %d events; want 5", len(got))
	}
}

// Close drops the events waiting on an interval and any published
// afterwards. Other kinds still go out.
func TestCloseDropsWaitingEvents(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		b := NewBus()
		s := b.Subscribe(all)
		defer s.Close()
		b.Publish(Event{Kind: TasksChanged})
		b.Close()
		b.Publish(Event{Kind: PlaysChanged})
		b.Publish(Event{Kind: WorkChanged})
		time.Sleep(5 * time.Second)
		synctest.Wait()
		if got := collect(s); len(got) != 1 || got[0].Kind != WorkChanged {
			t.Fatalf("got %+v; want only workChanged", got)
		}
	})
}
