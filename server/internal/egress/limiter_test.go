package egress

import (
	"context"
	"errors"
	"slices"
	"testing"
	"testing/synctest"
	"time"
)

func TestLimiterSpacesRequests(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		l := Limiter{Every: 100 * time.Millisecond}
		start := time.Now()
		var at []time.Duration
		for range 3 {
			if err := l.Wait(context.Background()); err != nil {
				t.Fatal(err)
			}
			at = append(at, time.Since(start))
		}
		if at[0] != 0 || at[1] != 100*time.Millisecond || at[2] != 200*time.Millisecond {
			t.Fatalf("requests at %v", at)
		}
		time.Sleep(time.Second) // idle time isn't saved up as a burst
		start = time.Now()
		l.Wait(context.Background())
		l.Wait(context.Background())
		if d := time.Since(start); d != 100*time.Millisecond {
			t.Fatalf("after idling, two requests took %v", d)
		}
		cctx, cancel := context.WithCancel(context.Background())
		cancel()
		if err := l.Wait(cctx); !errors.Is(err, context.Canceled) {
			t.Fatalf("a cancelled wait: %v", err)
		}
	})
}

// Failures in a row rest a service for 30 seconds, doubling up to 30
// minutes, or for its Retry-After when that is longer, up to an hour. A
// refused key rests it for an hour. An answer ends the outage. The outage
// remembers when it began.
func TestLimiterOutage(t *testing.T) {
	var l Limiter
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	if o := l.Outage(); o != (Outage{}) {
		t.Fatalf("a new limiter's outage: %+v", o)
	}
	var rests []time.Duration
	for i := range 8 {
		now := start.Add(time.Duration(i) * time.Minute)
		rests = append(rests, l.Failed(now, 0).Until.Sub(now))
	}
	want := []time.Duration{30 * time.Second, time.Minute, 2 * time.Minute, 4 * time.Minute, 8 * time.Minute, 16 * time.Minute, 30 * time.Minute, 30 * time.Minute}
	if !slices.Equal(rests, want) {
		t.Fatalf("rests %v, want %v", rests, want)
	}
	if o := l.Outage(); o.Failures != 8 || o.Rejected || !o.Since.Equal(start) {
		t.Fatalf("outage %+v", o)
	}

	l.Recovered()
	if o := l.Outage(); o != (Outage{}) {
		t.Fatalf("after recovering: %+v", o)
	}
	for _, c := range []struct{ after, rest time.Duration }{
		{10 * time.Second, 30 * time.Second}, // the backoff is longer
		{45 * time.Minute, 45 * time.Minute},
		{3 * time.Hour, time.Hour}, // capped
	} {
		l.Recovered()
		if got := l.Failed(start, c.after).Until.Sub(start); got != c.rest {
			t.Errorf("Retry-After %v rested %v, want %v", c.after, got, c.rest)
		}
	}

	l.Recovered()
	l.Failed(start, 0)
	o := l.Rejected(start.Add(time.Minute))
	if !o.Rejected || !o.Until.Equal(start.Add(time.Minute+time.Hour)) || !o.Since.Equal(start) {
		t.Fatalf("a refused key: %+v", o)
	}
}
