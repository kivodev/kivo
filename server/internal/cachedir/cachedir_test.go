package cachedir

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/mmcdole/kivo/internal/health"
	"github.com/mmcdole/kivo/internal/id"
)

// files is the generation of each file that has a row.
type files struct {
	mu   sync.Mutex
	gens map[id.ID]int64
}

func (f *files) generations(_ context.Context, ids []id.ID) (map[id.ID]int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := map[id.ID]int64{}
	for _, x := range ids {
		if g, ok := f.gens[x]; ok {
			out[x] = g
		}
	}
	return out, nil
}

func newResults(t *testing.T, version int, gens map[id.ID]int64) (Results, *files) {
	t.Helper()
	f := &files{gens: gens}
	dir := filepath.Join(t.TempDir(), "results")
	return Results{Dir: dir, Version: version, Generations: f.generations, Room: &health.Room{Dir: dir}}, f
}

// build makes a result in a temp folder holding one file with body.
func build(t *testing.T, r Results, body string) string {
	t.Helper()
	tmp, err := MakeTemp(r.Dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmp, "result"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return tmp
}

func TestPath(t *testing.T) {
	file := id.New()
	s := file.String()
	r := Results{Dir: "cache", Version: 2}
	if got, want := r.Path(file, 7), filepath.Join("cache", s[len(s)-2:], s, "7v2"); got != want {
		t.Fatalf("Path = %q, want %q", got, want)
	}
	r.Version = 0
	if got, want := r.Path(file, 7), filepath.Join("cache", s[len(s)-2:], s, "7"); got != want {
		t.Fatalf("Path = %q, want %q", got, want)
	}
}

func TestPlaceMovesTheResultIntoPlace(t *testing.T) {
	file := id.New()
	r, _ := newResults(t, 0, map[id.ID]int64{file: 3})
	if r.Has(file, 3) {
		t.Fatal("Has before Place")
	}
	tmp := build(t, r, "made")
	made, err := r.Place(context.Background(), tmp, file, 3)
	if err != nil || !made {
		t.Fatalf("Place = %v, %v", made, err)
	}
	if !r.Has(file, 3) || r.Has(file, 2) {
		t.Fatal("Has doesn't see the result at its generation only")
	}
	if b, err := os.ReadFile(filepath.Join(r.Path(file, 3), "result")); err != nil || string(b) != "made" {
		t.Fatalf("result = %q, %v", b, err)
	}
	if _, err := os.Stat(tmp); !os.IsNotExist(err) {
		t.Fatal("the temp folder is still there")
	}
}

func TestPlaceDropsAStaleResult(t *testing.T) {
	file, gone := id.New(), id.New()
	r, _ := newResults(t, 0, map[id.ID]int64{file: 4})
	for _, c := range []struct {
		file id.ID
		gen  int64
	}{{file, 3}, {gone, 1}} {
		made, err := r.Place(context.Background(), build(t, r, "stale"), c.file, c.gen)
		if err != nil || made {
			t.Fatalf("Place = %v, %v, want not made", made, err)
		}
		if r.Has(c.file, c.gen) {
			t.Fatal("a stale result was put in place")
		}
	}
}

// Places of the same file at once each put a whole result in place or
// count as done, and the result in place is one of theirs, whole.
func TestConcurrentPlaces(t *testing.T) {
	file := id.New()
	r, _ := newResults(t, 0, map[id.ID]int64{file: 1})
	var wg sync.WaitGroup
	for i := range 8 {
		tmp := build(t, r, "made")
		wg.Go(func() {
			if made, err := r.Place(context.Background(), tmp, file, 1); err != nil || !made {
				t.Errorf("place %d = %v, %v", i, made, err)
			}
		})
	}
	wg.Wait()
	if b, err := os.ReadFile(filepath.Join(r.Path(file, 1), "result")); err != nil || string(b) != "made" {
		t.Fatalf("result = %q, %v", b, err)
	}
}

func TestPruneKeepsOnlyCurrentResults(t *testing.T) {
	kept, changed, gone := id.New(), id.New(), id.New()
	r, f := newResults(t, 2, map[id.ID]int64{kept: 5, changed: 2, gone: 1})
	ctx := context.Background()
	place := func(file id.ID, gen int64) {
		t.Helper()
		if made, err := r.Place(ctx, build(t, r, "made"), file, gen); err != nil || !made {
			t.Fatalf("Place = %v, %v", made, err)
		}
	}
	place(kept, 5)
	place(changed, 2)
	place(gone, 1)
	oldVersion := Path(r.Dir, kept, kept.String(), leaf(5, 1))
	if err := os.MkdirAll(oldVersion, 0o755); err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	f.gens[changed] = 3
	delete(f.gens, gone)
	f.mu.Unlock()

	var reported []int
	removed, still, err := r.Prune(ctx, func(_ context.Context, _ string, done, _ int) { reported = append(reported, done) })
	if err != nil {
		t.Fatal(err)
	}
	if removed != 3 || still != 2 {
		t.Fatalf("Prune = %d removed, %d kept, want 3 and 2", removed, still)
	}
	if len(reported) != 1 || reported[0] != 0 {
		t.Fatalf("progress = %v", reported)
	}
	if !r.Has(kept, 5) {
		t.Fatal("the current result was removed")
	}
	for _, p := range []string{oldVersion, r.Path(changed, 2), filepath.Dir(r.Path(gone, 1))} {
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Fatalf("%s wasn't removed", p)
		}
	}
}
