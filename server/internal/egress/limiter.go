package egress

import (
	"context"
	"sync"
	"time"
)

// The rests a service is given. After failures in a row, it rests for
// firstBackoff, doubling up to maxBackoff. If the service's own Retry-After
// is longer, that is used instead, up to maxRetryAfter. After the service
// refuses our key, it rests for rejectedRetry, so that a fixed key gets
// tried again.
const (
	firstBackoff  = 30 * time.Second
	maxBackoff    = 30 * time.Minute
	maxRetryAfter = time.Hour
	rejectedRetry = time.Hour
)

// Limiter tracks one outside service's pace and outage. It spaces the
// service's requests at least Every apart, well under the service's own
// limit, so a burst queues up instead of tripping that limit. It also
// tracks the service's outage. Once a request fails, or the service
// refuses our key, the service rests for a while. The first request after
// the rest is the trial. Callers send nothing to a resting service, and
// report its problems. A Limiter is safe for concurrent use.
type Limiter struct {
	Every time.Duration

	mu     sync.Mutex
	next   time.Time
	outage Outage
}

// Outage is a service's outage. There is none while Failures is zero and
// Rejected is false.
type Outage struct {
	Until    time.Time // the service rests and gets no requests before this
	Since    time.Time // when the outage began
	Failures int       // failures in a row
	Rejected bool      // the service refused our key
}

// Wait blocks until the caller's turn, or until ctx ends.
func (l *Limiter) Wait(ctx context.Context) error {
	l.mu.Lock()
	now := time.Now()
	at := now
	if l.next.After(now) {
		at = l.next
	}
	l.next = at.Add(l.Every)
	l.mu.Unlock()
	if !at.After(now) {
		return nil
	}
	t := time.NewTimer(at.Sub(now))
	defer t.Stop()
	select {
	case <-t.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Outage returns the service's current outage.
func (l *Limiter) Outage() Outage {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.outage
}

// Failed records a failure at now, with the Retry-After the service asked
// for, or zero for none. It returns the outage. The service rests for the
// backoff of its failures in a row, or for the Retry-After when that is
// longer.
func (l *Limiter) Failed(now time.Time, retryAfter time.Duration) Outage {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.outage.Failures++
	wait := firstBackoff
	for i := 1; i < l.outage.Failures && wait < maxBackoff; i++ {
		wait *= 2
	}
	wait = max(min(wait, maxBackoff), min(retryAfter, maxRetryAfter))
	l.outage.Until = now.Add(wait)
	if l.outage.Since.IsZero() {
		l.outage.Since = now
	}
	return l.outage
}

// Rejected records at now that the service refused our key, and returns the
// outage. The service rests until the key is tried again.
func (l *Limiter) Rejected(now time.Time) Outage {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.outage.Rejected = true
	l.outage.Until = now.Add(rejectedRetry)
	if l.outage.Since.IsZero() {
		l.outage.Since = now
	}
	return l.outage
}

// Recovered ends the outage, because the service answered or has a new key
// to try.
func (l *Limiter) Recovered() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.outage = Outage{}
}
