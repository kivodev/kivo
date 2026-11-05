// Package keylock holds one lock per key. A holder of a key runs alone,
// while holders of other keys run alongside it.
package keylock

import (
	"context"
	"sync"
)

// Locks lets one holder per key run at a time. Others wait their turn. The
// zero value is ready to use. A key's entry is kept only while the key is
// held or waited for.
type Locks[K comparable] struct {
	mu   sync.Mutex
	held map[K]*holder
}

type holder struct {
	turn    chan struct{} // holds a token while the key is free
	waiting int           // holders plus waiters; the entry is removed at zero
}

// Lock waits for key, or for ctx to end, and returns the unlock function.
func (l *Locks[K]) Lock(ctx context.Context, key K) (func(), error) {
	l.mu.Lock()
	if l.held == nil {
		l.held = map[K]*holder{}
	}
	h := l.held[key]
	if h == nil {
		h = &holder{turn: make(chan struct{}, 1)}
		h.turn <- struct{}{}
		l.held[key] = h
	}
	h.waiting++
	l.mu.Unlock()

	select {
	case <-h.turn:
		return func() { l.release(key, h, true) }, nil
	case <-ctx.Done():
		l.release(key, h, false)
		return nil, ctx.Err()
	}
}

func (l *Locks[K]) release(key K, h *holder, held bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if held {
		h.turn <- struct{}{}
	}
	if h.waiting--; h.waiting == 0 {
		delete(l.held, key)
	}
}
