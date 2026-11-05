package identify_test

import (
	"slices"
	"testing"
	"time"

	"github.com/mmcdole/kivo/internal/catalog"
	"github.com/mmcdole/kivo/internal/events"
	"github.com/mmcdole/kivo/internal/id"
)

// Films found and matched in a library reach its viewers as one
// libraryChanged event, which the catalog publishes on its bus. That holds
// however many writes placing and matching them took.
func TestIdentifyTellsTheLibraryOnce(t *testing.T) {
	e := newEnv(t)
	sub := e.b.Bus.Subscribe(func(events.Scope) bool { return true })
	defer sub.Close()
	lib := e.library(catalog.Movies)
	e.library(catalog.Movies) // another library, which hears nothing after it's made
	select {
	case ev := <-sub.C():
		if ev.Kind != events.LibraryChanged || len(ev.Scope.Libraries) != 2 {
			t.Fatalf("event %+v; want libraryChanged for the new libraries", ev)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no libraryChanged event for the new libraries")
	}

	files := e.add(lib, "Heat (1995)/Heat (1995).mkv", "Dune (2021)/Dune (2021).mkv")
	e.run(files...)
	for _, f := range files {
		if w := e.workOf(f); len(w) != 1 || w[0].Provisional {
			t.Fatalf("not matched: %+v", w)
		}
	}
	select {
	case ev := <-sub.C():
		if ev.Kind != events.LibraryChanged || !slices.Equal(ev.Scope.Libraries, []id.ID{lib.ID}) {
			t.Fatalf("event %+v; want libraryChanged for the films' library", ev)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no libraryChanged event")
	}
	select {
	case ev := <-sub.C():
		t.Fatalf("another event %+v; want one", ev)
	case <-time.After(1500 * time.Millisecond):
	}
}
