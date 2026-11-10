package main

import (
	"context"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/mmcdole/kivo/internal/config"
	"github.com/mmcdole/kivo/internal/events"
	"github.com/mmcdole/kivo/internal/health"
	"github.com/mmcdole/kivo/internal/id"
	"github.com/mmcdole/kivo/internal/playback"
)

// TestSideEffects runs the real server and checks that each change's side
// effects happen, as its services publish them on the bus:
//   - Creating and renaming a library publishes libraryChanged.
//   - A user's state change and a play's end publish userStateChanged.
//   - Raising a problem (once the problems are read) and dismissing it
//     publish problemsChanged.
//
// It needs ffmpeg and ffprobe on the PATH.
func TestSideEffects(t *testing.T) {
	if testing.Short() {
		t.Skip("end to end")
	}
	ffmpeg, err1 := exec.LookPath("ffmpeg")
	ffprobe, err2 := exec.LookPath("ffprobe")
	if err1 != nil || err2 != nil {
		t.Skip("needs ffmpeg and ffprobe")
	}
	dir := t.TempDir()
	media, empty := filepath.Join(dir, "media"), filepath.Join(dir, "empty")
	makeMovie(t, ffmpeg, filepath.Join(media, "Short Film (1999)", "Short Film (1999).mp4"), 10*time.Second)
	if err := os.MkdirAll(empty, 0o755); err != nil {
		t.Fatal(err)
	}

	boot := config.DefaultBootstrap()
	boot.Listen = []string{freeAddr(t)}
	boot.DataDir, boot.CacheDir, boot.BackupDir = filepath.Join(dir, "data"), filepath.Join(dir, "cache"), filepath.Join(dir, "backups")
	boot.FFmpeg, boot.FFprobe = ffmpeg, ffprobe
	bus := events.NewBus()
	w := &busWatcher{t: t, bus: bus, sub: bus.Subscribe(everyone)}
	defer func() { w.sub.Close() }()
	ctx, stop := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- run(ctx, boot, health.NewLogs(100, bus), bus) }()
	defer func() {
		stop()
		if err := <-done; err != nil {
			t.Errorf("shutdown: %v", err)
		}
	}()

	c := newClient(t, "http://"+boot.Listen[0])
	c.waitHealthy()
	c.call("POST", "/api/v1/setup", map[string]any{"username": "admin", "password": "correct horse battery"}, http.StatusCreated, nil)

	// A library created, then renamed.
	var lib struct{ ID id.ID }
	c.call("POST", "/api/v1/libraries", map[string]any{"name": "Movies", "kind": "movies", "folders": []string{media}}, http.StatusCreated, &lib)
	names := func(lib id.ID) func(events.Event) bool {
		return func(e events.Event) bool {
			return e.Kind == events.LibraryChanged && slices.Contains(e.Scope.Libraries, lib)
		}
	}
	w.waitFor("libraryChanged for a new library", names(lib.ID))
	var work string
	for deadline := time.Now().Add(60 * time.Second); work == "" && time.Now().Before(deadline); time.Sleep(200 * time.Millisecond) {
		var page struct{ Items []struct{ ID string } }
		c.call("POST", "/api/v1/works/query", map[string]any{"query": map[string]any{"kinds": []string{"movie"}}, "library": lib.ID}, http.StatusOK, &page)
		if len(page.Items) > 0 {
			work = page.Items[0].ID
		}
	}
	if work == "" {
		t.Fatal("the film was never added")
	}
	w.drain()
	c.call("PATCH", "/api/v1/libraries/"+lib.ID.String(), map[string]any{"name": "Films"}, http.StatusOK, nil)
	w.waitFor("libraryChanged for a renamed library", names(lib.ID))

	// A user's state changed by hand, and by a play's end.
	changed := func(e events.Event) bool {
		works, _ := e.Data.([]id.ID)
		return e.Kind == events.UserStateChanged && slices.ContainsFunc(works, func(w id.ID) bool { return w.String() == work })
	}
	c.call("PATCH", "/api/v1/works/"+work+"/user-state", map[string]any{"played": true}, http.StatusOK, nil)
	w.waitFor("userStateChanged after marking played", changed)
	caps := capsFor([]string{"mp4"}, true, h264(), []string{"aac"})
	var play struct{ ID string }
	c.call("POST", "/api/v1/plays", map[string]any{"workId": work, "capabilities": caps}, http.StatusCreated, &play)
	c.call("POST", "/api/v1/plays/"+play.ID+"/stop", map[string]any{"positionMs": 4000}, http.StatusNoContent, nil)
	w.waitFor("userStateChanged after a play ended", changed) // published once the play's last position is written
	w.waitFor("playEnded", func(e events.Event) bool {
		d, ok := e.Data.(playback.Ended)
		return e.Kind == events.PlayEnded && ok && d.Play.String() == play.ID
	})

	// A folder found empty raises a problem, and dismissing it clears it.
	opened := func(n int) func(events.Event) bool {
		return func(e events.Event) bool { return e.Kind == events.ProblemsChanged && e.Data == n }
	}
	var problems struct{ Items []struct{ ID string } }
	c.call("GET", "/api/v1/problems", nil, http.StatusOK, &problems)
	before := len(problems.Items)
	c.call("POST", "/api/v1/libraries", map[string]any{"name": "Empty", "kind": "movies", "folders": []string{empty}}, http.StatusCreated, nil)
	i := -1
	for deadline := time.Now().Add(30 * time.Second); i < 0 && time.Now().Before(deadline); time.Sleep(200 * time.Millisecond) {
		c.call("GET", "/api/v1/problems", nil, http.StatusOK, &problems)
		i = slices.IndexFunc(problems.Items, func(p struct{ ID string }) bool { return p.ID == "scan.folderUnreachable" })
	}
	if i < 0 {
		t.Fatalf("problems %+v", problems)
	}
	w.waitFor("problemsChanged for an unreachable folder", opened(before+1))
	c.call("DELETE", "/api/v1/problems/"+problems.Items[i].ID, nil, http.StatusNoContent, nil)
	w.waitFor("problemsChanged after a dismissal", opened(before))

}

func everyone(events.Scope) bool { return true }

// busWatcher reads what the server publishes on its bus.
type busWatcher struct {
	t   *testing.T
	bus *events.Bus
	sub *events.Sub
}

// waitFor reads events until one matches, failing after 30 seconds.
func (w *busWatcher) waitFor(what string, match func(events.Event) bool) {
	w.t.Helper()
	deadline := time.After(30 * time.Second)
	for {
		select {
		case e, ok := <-w.sub.C():
			if !ok { // the subscription lagged and lost events, so start over
				w.sub = w.bus.Subscribe(everyone)
				continue
			}
			if match(e) {
				return
			}
		case <-deadline:
			w.t.Fatalf("no %s", what)
		}
	}
}

// drain discards the events published so far.
func (w *busWatcher) drain() {
	for {
		select {
		case _, ok := <-w.sub.C():
			if !ok {
				w.sub = w.bus.Subscribe(everyone)
				return
			}
		default:
			return
		}
	}
}
