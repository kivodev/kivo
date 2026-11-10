package tasks

import (
	"context"
	"errors"
	"slices"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/mmcdole/kivo/internal/config"
	"github.com/mmcdole/kivo/internal/id"
)

// slow is items work that is only done when enqueued. Each item takes
// took, reporting step as it goes, unless it is cancelled first.
type slow struct {
	step  string
	took  time.Duration
	limit int

	mu        sync.Mutex
	done      []id.ID
	cancelled []id.ID
}

func (s *slow) items() Items {
	return Items{
		Pending: func(context.Context) ([]id.ID, error) { return nil, nil },
		Do: func(ctx context.Context, i id.ID) error {
			Progress(ctx, s.step, 0, 1)
			select {
			case <-ctx.Done():
				s.mu.Lock()
				s.cancelled = append(s.cancelled, i)
				s.mu.Unlock()
				return ctx.Err()
			case <-time.After(s.took):
			}
			s.mu.Lock()
			s.done = append(s.done, i)
			s.mu.Unlock()
			return nil
		},
		Limit:     func(config.Settings) int { return s.limit },
		Uncounted: true,
	}
}

func (s *slow) outcome() (done, cancelled []id.ID) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.done), slices.Clone(s.cancelled)
}

// scanAndIdentify is a scan task whose libraries are scanned for scanFor,
// and an identify task, a step of it, whose files take a minute each.
func scanAndIdentify(t *testing.T, scans, identifies int, scanFor time.Duration) (*harness, *slow, *slow) {
	sc := &slow{step: "walk", took: scanFor, limit: scans}
	ident := &slow{step: "match", took: time.Minute, limit: identifies}
	h := newHarness(t,
		Task{Name: "scan", Schedule: Every(12 * time.Hour), Work: sc.items()},
		Task{Name: "identify", Schedule: OnDemand, Work: ident.items(), StepOf: StepOf{Task: "scan", Step: "identify"}},
	)
	h.window(6, 2)
	return h, sc, ident
}

// A step has no row and isn't found by name. While the files a library's
// scan asked for are identified, the scan's row runs on the step, with the
// files done out of those asked for, and the library's item shows the
// same. Files no scan asked for don't make the scan's row run.
func TestAStepShowsOnItsOwnersRow(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h, _, ident := scanAndIdentify(t, 2, 1, time.Hour)
		h.start()
		defer h.stop()
		ctx := context.Background()
		all, err := h.r.Tasks(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if len(all) != 1 || all[0].Name != "scan" {
			t.Fatalf("tasks %+v, want the scan alone", all)
		}
		if err := h.r.Start(ctx, "identify"); !errors.Is(err, ErrNotFound) {
			t.Errorf("start: %v", err)
		}
		if err := h.r.Stop(ctx, "identify"); !errors.Is(err, ErrNotFound) {
			t.Errorf("stop: %v", err)
		}
		if _, err := h.r.Failed(ctx, "identify"); !errors.Is(err, ErrNotFound) {
			t.Errorf("failed: %v", err)
		}
		if err := h.r.Retry(ctx, "identify"); !errors.Is(err, ErrNotFound) {
			t.Errorf("retry: %v", err)
		}

		refresh, lib, files := id.New(), id.New(), []id.ID{id.New(), id.New(), id.New()}
		h.r.Enqueue("identify", refresh)
		synctest.Wait()
		if s := h.status("scan"); s.Running || len(s.Items) != 0 || len(h.r.Items("scan")) != 0 {
			t.Fatalf("%+v while a file no scan asked for is identified, want idle", s)
		}
		h.r.EnqueueFor(lib, "identify", files...)
		h.r.EnqueueFor(lib, "identify", files[0])
		synctest.Wait()
		want := Item{ID: lib, Step: "identify", Done: 0, Total: 3, Since: time.Now()}
		if s := h.status("scan"); !s.Running || s.Step != "identify" || s.Done != 0 || s.Total != 3 || !sameItems(s.Items, want) {
			t.Fatalf("%+v, want identifying 0 of 3", s)
		}
		sleep(150 * time.Second)
		want.Done = 1
		if s := h.status("scan"); !s.Running || s.Done != 1 || s.Total != 3 || !sameItems(h.r.Items("scan"), want) {
			t.Fatalf("%+v, want identifying 1 of 3", s)
		}
		sleep(2 * time.Minute)
		if s := h.status("scan"); s.Running || len(s.Items) != 0 {
			t.Fatalf("%+v once every file is identified, want idle", s)
		}
		if done, _ := ident.outcome(); !slices.Equal(done, append([]id.ID{refresh}, files...)) {
			t.Fatalf("identified %v", done)
		}
	})
}

// Stopping one library's scan stops the files it asked for, in flight and
// in line, and no others. Stopping the scan task stops what every scan
// asked for, and leaves files no scan asked for going.
func TestStoppingAScanStopsWhatItAskedFor(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h, _, ident := scanAndIdentify(t, 2, 2, time.Hour)
		h.start()
		defer h.stop()
		ctx := context.Background()
		refresh, retry, a, b := id.New(), id.New(), id.New(), id.New()
		libA, libB := id.New(), id.New()
		h.r.Enqueue("identify", refresh)
		h.r.EnqueueFor(libA, "identify", a)
		h.r.EnqueueFor(libB, "identify", b)
		h.r.Enqueue("identify", retry)
		synctest.Wait()

		if err := h.r.Stop(ctx, "scan", libA); err != nil {
			t.Fatal(err)
		}
		synctest.Wait()
		if items := h.r.Items("scan"); !sameItems(items, Item{ID: libB, Step: "identify", Total: 1, Since: time.Now()}) {
			t.Fatalf("items %+v after stopping library A, want B's alone", items)
		}
		if err := h.r.Stop(ctx, "scan"); err != nil {
			t.Fatal(err)
		}
		synctest.Wait()
		if s := h.status("scan"); s.Running {
			t.Fatalf("%+v after Stop, want idle", s)
		}
		sleep(3 * time.Minute)
		done, cancelled := ident.outcome()
		if !sameIDs(done, refresh, retry) || !sameIDs(cancelled, a, b) {
			t.Fatalf("done %v, cancelled %v: want the scans' files stopped and the others done", done, cancelled)
		}
	})
}

// A library being identified doesn't take a scan's slot, and its next scan
// starts at once, showing its own progress.
func TestAStepTakesNoSlotOfItsOwner(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h, sc, _ := scanAndIdentify(t, 1, 1, 10*time.Minute)
		h.start()
		defer h.stop()
		libA, libB := id.New(), id.New()
		h.r.EnqueueFor(libA, "identify", id.New(), id.New())
		sleep(time.Second)
		h.r.EnqueueFor(libB, "identify", id.New())
		sleep(time.Second)

		h.r.Enqueue("scan", libA)
		synctest.Wait()
		items := h.r.Items("scan")
		if len(items) != 2 || items[0].ID != libA || items[0].Step != "walk" || items[1].ID != libB || items[1].Step != "identify" {
			t.Fatalf("items %+v, want A scanning and B identifying", items)
		}
		if s := h.status("scan"); !s.Running || s.Step != "walk" {
			t.Fatalf("%+v, want the scan's own step", s)
		}
		sleep(11 * time.Minute)
		if done, _ := sc.outcome(); !slices.Equal(done, []id.ID{libA}) {
			t.Fatalf("scanned %v", done)
		}
	})
}

// sameIDs reports whether got holds want, in any order.
func sameIDs(got []id.ID, want ...id.ID) bool {
	return len(got) == len(want) && !slices.ContainsFunc(want, func(i id.ID) bool { return !slices.Contains(got, i) })
}

// sameItems reports whether got holds want, ignoring when each started.
func sameItems(got []Item, want ...Item) bool {
	return slices.EqualFunc(got, want, func(a, b Item) bool {
		return a.ID == b.ID && a.Step == b.Step && a.Done == b.Done && a.Total == b.Total && a.Since.Equal(b.Since)
	})
}
