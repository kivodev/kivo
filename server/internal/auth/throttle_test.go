package auth

import (
	"fmt"
	"net/netip"
	"testing"
	"testing/synctest"
	"time"
)

// strike makes one failed attempt, waiting out any backoff first.
func strike(th *throttle, ip netip.Addr, user string) {
	for {
		a, d := th.begin(ip, user)
		if d == 0 {
			a.fail()
			return
		}
		time.Sleep(d)
	}
}

// waitFor is how long an attempt must wait, without making one.
func waitFor(th *throttle, ip netip.Addr, user string) time.Duration {
	a, d := th.begin(ip, user)
	if d == 0 {
		a.end()
	}
	return d
}

func TestThrottlePerUserAndAddress(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		th := newThrottle()
		attacker := netip.MustParseAddr("203.0.113.9")
		for range userFreeFailures {
			strike(th, attacker, "anna")
		}
		if d := waitFor(th, attacker, "anna"); d != 0 {
			t.Fatalf("waits %v within the free failures", d)
		}

		// Past the free failures, each one doubles the wait.
		for i, want := range []time.Duration{time.Second, 2 * time.Second, 4 * time.Second} {
			strike(th, attacker, "ANNA")
			if d := waitFor(th, attacker, "anna"); d != want {
				t.Fatalf("failure %d: wait %v, want %v", userFreeFailures+i+1, d, want)
			}
		}
		// Only that user from that address waits.
		if d := waitFor(th, home, "anna"); d != 0 {
			t.Fatalf("anna from home waits %v", d)
		}
		if d := waitFor(th, attacker, "ben"); d != 0 {
			t.Fatalf("ben from the attacker's address waits %v", d)
		}

		time.Sleep(4 * time.Second)
		a, d := th.begin(attacker, "anna")
		if d != 0 {
			t.Fatalf("still waits %v after the backoff", d)
		}
		a.succeed()
		strike(th, attacker, "anna")
		if d := waitFor(th, attacker, "anna"); d != 0 {
			t.Fatalf("a success didn't reset the user's failures: waits %v", d)
		}
	})
}

// Attempts running at the same time each count as a possible failure. So
// no more of them begin than could fail before the backoff starts.
func TestThrottleCountsRunningAttempts(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		th := newThrottle()
		var running []*attempt
		for range userFreeFailures + 1 {
			a, d := th.begin(home, "anna")
			if d != 0 {
				t.Fatalf("attempt %d waits %v", len(running)+1, d)
			}
			running = append(running, a)
		}
		if d := waitFor(th, home, "anna"); d == 0 {
			t.Fatal("one more attempt began than could fail before the backoff")
		}
		for _, a := range running {
			a.fail()
		}
		if d := waitFor(th, home, "anna"); d != firstBackoff {
			t.Fatalf("after they failed: wait %v, want %v", d, firstBackoff)
		}
		// An attempt that ends without an answer counts as nothing.
		time.Sleep(firstBackoff)
		a, _ := th.begin(home, "anna")
		a.end()
		a.fail() // already ended
		if d := waitFor(th, home, "anna"); d != 0 {
			t.Fatalf("an ended attempt counted: wait %v", d)
		}
	})
}

func TestThrottleSprayBacksOffTheAddress(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		th := newThrottle()
		attacker := netip.MustParseAddr("2001:db8:1:2::9")
		neighbour := netip.MustParseAddr("2001:db8:1:2::abcd") // same /64
		for i := range ipFreeFailures + 1 {
			strike(th, attacker, fmt.Sprint("user", i))
		}
		if d := waitFor(th, neighbour, "someone-new"); d != firstBackoff {
			t.Fatalf("the spraying /64 waits %v for a new user, want %v", d, firstBackoff)
		}
		if d := waitFor(th, netip.MustParseAddr("2001:db8:1:3::9"), "user0"); d != 0 {
			t.Fatalf("another /64 waits %v", d)
		}
	})
}

func TestThrottleBackoffIsCapped(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		th := newThrottle()
		for range userFreeFailures + 30 {
			strike(th, home, "anna")
		}
		if d := waitFor(th, home, "anna"); d != maxBackoff {
			t.Fatalf("wait %v, want the cap %v", d, maxBackoff)
		}
	})
}

func TestThrottlePrunesQuietEntries(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		th := newThrottle()
		for range userFreeFailures + 1 {
			strike(th, home, "anna")
		}
		strike(th, other, "ben")
		time.Sleep(pruneAfter + time.Second)
		waitFor(th, home, "anna") // prunes, then makes the entries it asks about
		if n := len(th.entries); n != 2 {
			t.Fatalf("%d entries after an hour of quiet, want only the 2 just asked about", n)
		}
	})
}
