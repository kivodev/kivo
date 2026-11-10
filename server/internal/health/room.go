package health

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"syscall"
	"time"
)

// minFree is the free space below which a Room's disk counts as full.
const minFree = 1 << 30

// ErrDiskFull means a Room's disk is almost full. The work that writes to
// it waits for a later pass.
var ErrDiskFull = errors.New("disk full")

// A Room watches the free space of a disk that work writes to. While the
// disk is almost full it reports a problem, until work finds room again.
// It's safe for concurrent use once its fields are set.
type Room struct {
	// Dir is a folder on the disk.
	Dir string
	// Problem is reported while the disk is full. It gives the component,
	// code and message.
	Problem Problem
	// Free reads the free space on a path's disk. Nil reads the real disk.
	Free func(path string) (uint64, error)

	mu   sync.Mutex
	full *Problem
}

// Check returns ErrDiskFull while the disk is almost full.
func (r *Room) Check() error {
	free := r.Free
	if free == nil {
		free = freeSpace
	}
	n, err := free(r.Dir)
	if err != nil || n >= minFree {
		r.setFull(false)
		return nil
	}
	r.setFull(true)
	return ErrDiskFull
}

// Err returns ErrDiskFull wrapping err when err means the disk ran out of
// space. Otherwise it returns err unchanged.
func (r *Room) Err(err error) error {
	if errors.Is(err, syscall.ENOSPC) {
		r.setFull(true)
		return fmt.Errorf("%w: %w", ErrDiskFull, err)
	}
	return err
}

// Health implements Reporter. It reports the disk while it is full.
func (r *Room) Health(context.Context) []Problem {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.full == nil {
		return nil
	}
	return []Problem{*r.full}
}

func (r *Room) setFull(full bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	switch {
	case !full:
		r.full = nil
	case r.full == nil:
		p := r.Problem
		p.Severity, p.RaisedAt = Warning, time.Now()
		r.full = &p
	}
}
