package auth

import (
	"net/netip"
	"strings"
	"sync"
	"time"
)

// Sign-in backoff. Failures are counted per client address, and per user
// plus address, so an attacker slows only themselves. Guessing one user's
// password from one address backs off soon. Spraying many users from one
// address backs off later. Nobody else is locked out of the user.
const (
	userFreeFailures = 5  // per user plus address, before backoff starts
	ipFreeFailures   = 20 // per address, which a household shares
	firstBackoff     = time.Second
	maxBackoff       = 15 * time.Minute
	pruneAfter       = time.Hour // how long a quiet entry is kept
	pruneEvery       = time.Minute
)

// throttle is an in-memory map that is swept lazily on use.
type throttle struct {
	mu        sync.Mutex
	entries   map[string]*strikes
	lastPrune time.Time
}

type strikes struct {
	failures int
	running  int       // attempts begun and not yet ended
	until    time.Time // no attempts before this
	last     time.Time
}

func newThrottle() *throttle {
	return &throttle{entries: map[string]*strikes{}}
}

// attempt is a sign-in attempt that has begun. It ends with fail, succeed
// or end. Until then it counts as a possible failure, so attempts run at
// the same time can't outpace the backoff.
type attempt struct {
	t     *throttle
	keys  throttleKeys
	ended bool
}

// begin starts an attempt at the user from the client's address, or
// returns how long the client must wait first.
func (t *throttle) begin(ip netip.Addr, user string) (*attempt, time.Duration) {
	t.mu.Lock()
	defer t.mu.Unlock()
	now := time.Now()
	t.pruneLocked(now)
	k := keysFor(ip, user)
	var d time.Duration
	for _, key := range []struct {
		name string
		free int
	}{{k.addr, ipFreeFailures}, {k.user, userFreeFailures}} {
		s := t.entries[key.name]
		if s == nil {
			continue
		}
		d = max(d, s.until.Sub(now))
		// Each attempt already running could fail. One more may begin only
		// if those possible failures would still leave it within the free
		// failures.
		if s.running > 0 && s.failures+s.running > key.free {
			d = max(d, firstBackoff)
		}
	}
	if d > 0 {
		return nil, d
	}
	for _, key := range []string{k.addr, k.user} {
		t.entryLocked(key, now).running++
	}
	return &attempt{t: t, keys: k}, 0
}

// fail ends the attempt as a failure, counted against both keys.
func (a *attempt) fail() {
	a.finish(func(now time.Time) {
		a.t.strikeLocked(a.keys.addr, ipFreeFailures, now)
		a.t.strikeLocked(a.keys.user, userFreeFailures, now)
	})
}

// succeed ends the attempt as a success, forgetting the user's failures
// from that address. The address's own count stays, so a valid sign-in
// doesn't reset a spray from it.
func (a *attempt) succeed() {
	a.finish(func(time.Time) {
		s := a.t.entries[a.keys.user]
		s.failures, s.until = 0, time.Time{}
	})
}

// end ends the attempt if it hasn't already failed or succeeded, as when
// the password couldn't be checked. It counts as neither.
func (a *attempt) end() { a.finish(func(time.Time) {}) }

func (a *attempt) finish(outcome func(now time.Time)) {
	a.t.mu.Lock()
	defer a.t.mu.Unlock()
	if a.ended {
		return
	}
	a.ended = true
	a.t.entries[a.keys.addr].running--
	a.t.entries[a.keys.user].running--
	outcome(time.Now())
}

func (t *throttle) entryLocked(key string, now time.Time) *strikes {
	s := t.entries[key]
	if s == nil {
		s = &strikes{}
		t.entries[key] = s
	}
	s.last = now
	return s
}

// strikeLocked counts a failure against key. Once key is past its free
// failures, it backs off.
func (t *throttle) strikeLocked(key string, free int, now time.Time) {
	s := t.entryLocked(key, now)
	s.failures++
	if n := s.failures - free; n > 0 {
		s.until = now.Add(backoff(n))
	}
}

func (t *throttle) pruneLocked(now time.Time) {
	if now.Sub(t.lastPrune) < pruneEvery {
		return
	}
	t.lastPrune = now
	for k, s := range t.entries {
		if s.running == 0 && now.Sub(s.last) > pruneAfter && !now.Before(s.until) {
			delete(t.entries, k)
		}
	}
}

// backoff is the wait after the nth failure past the free ones.
func backoff(n int) time.Duration {
	d := firstBackoff
	for i := 1; i < n && d < maxBackoff; i++ {
		d *= 2
	}
	return min(d, maxBackoff)
}

// throttleKeys are the two keys a sign-in attempt counts against.
type throttleKeys struct {
	addr string // the client's address
	user string // the user plus the address
}

// keysFor returns an attempt's keys.
func keysFor(ip netip.Addr, user string) throttleKeys {
	a := ClientAddr(ip)
	return throttleKeys{addr: "ip " + a, user: "user " + a + " " + strings.ToLower(user)}
}

// ClientAddr is the address a client is counted by. For IPv4 it is the
// client's own address. For IPv6 it is the client's /64, because one
// subscriber usually holds a whole /64.
func ClientAddr(ip netip.Addr) string {
	ip = ip.Unmap()
	if ip.Is6() {
		p, _ := ip.Prefix(64)
		return p.String()
	}
	return ip.String()
}
