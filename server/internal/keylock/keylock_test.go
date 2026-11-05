package keylock

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestLocks(t *testing.T) {
	var l Locks[string]
	unlock, err := l.Lock(context.Background(), "a")
	if err != nil {
		t.Fatal(err)
	}
	other, err := l.Lock(context.Background(), "b") // another key doesn't wait
	if err != nil {
		t.Fatal(err)
	}
	other()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if _, err := l.Lock(ctx, "a"); err == nil {
		t.Fatal("a held key was taken")
	}
	var wg sync.WaitGroup
	got := make(chan struct{})
	wg.Go(func() {
		u, err := l.Lock(context.Background(), "a")
		if err == nil {
			close(got)
			u()
		}
	})
	unlock()
	wg.Wait()
	<-got
	if len(l.held) != 0 {
		t.Fatalf("%d keys left held", len(l.held))
	}
}
