package api

import (
	"context"
	"net/http"
	"slices"
	"testing"
	"time"

	"github.com/mmcdole/kivo/internal/store"

	"github.com/mmcdole/kivo/internal/id"
	"github.com/mmcdole/kivo/internal/metadata"
	"github.com/mmcdole/kivo/internal/playback"
)

// task is a task's row from the Tasks page.
func (e *env) task(name string) map[string]any {
	e.t.Helper()
	rows := decode[list](e.t, e.do("GET", "/api/v1/tasks", e.adminToken, nil))
	i := slices.IndexFunc(rows.Items, func(t map[string]any) bool { return t["id"] == name })
	if i < 0 {
		e.t.Fatalf("tasks %v: no %s", rows.Items, name)
	}
	return rows.Items[i]
}

// Each row of the Tasks page has its section and schedule. Only tasks that
// keep failures have a failure count. Progress and Stop appear only while
// a task runs. Run now and Stop answer 204, and Stop records the run's
// end.
func TestTasks(t *testing.T) {
	e := newEnv(t, serving{})
	rows := decode[list](t, e.do("GET", "/api/v1/tasks", e.adminToken, nil))
	var names []string
	for _, r := range rows.Items {
		names = append(names, r["id"].(string))
	}
	if len(names) == 0 || names[0] != "scan-libraries" || names[len(names)-1] != "library-changes" {
		t.Fatalf("tasks in the order %v", names)
	}

	scans := e.task("scan-libraries")
	if scans["section"] != "library" || scans["nightly"] != false || scans["nextRunAt"] == nil || scans["failedCount"] != 0.0 ||
		scans["running"] != false || scans["stoppable"] != false || scans["progress"] != nil || scans["lastRun"] != nil {
		t.Errorf("scans %v", scans)
	}
	if prune := e.task("prune-previews"); prune["section"] != "maintenance" || prune["nightly"] != true || prune["nextRunAt"] == nil ||
		prune["failedCount"] != nil || prune["progress"] != nil {
		t.Errorf("previews cleanup %v", prune)
	}
	if subs := e.task("extract-subtitles"); subs["section"] != "library" || subs["nightly"] != true || subs["nextRunAt"] == nil || subs["failedCount"] != 0.0 {
		t.Errorf("subtitles %v", subs)
	}
	if downloads := e.task("prepare-downloads"); downloads["section"] != "onDemand" || downloads["failedCount"] != nil || downloads["running"] != false {
		t.Errorf("downloads %v", downloads)
	}

	// After Run now, the task is running, has progress, and can be
	// stopped.
	if w := e.do("POST", "/api/v1/tasks/prune-previews/run", e.adminToken, nil); w.Code != http.StatusNoContent || w.Body.Len() != 0 {
		t.Fatalf("run now: %d %s", w.Code, w.Body.String())
	}
	prune := e.task("prune-previews")
	if prune["running"] != true || prune["stoppable"] != true || prune["progress"] == nil {
		t.Errorf("running %v", prune)
	}
	if w := e.do("POST", "/api/v1/tasks/prune-previews/stop", e.adminToken, nil); w.Code != http.StatusNoContent {
		t.Fatalf("stop: %d %s", w.Code, w.Body.String())
	}
	prune = e.task("prune-previews")
	last, _ := prune["lastRun"].(map[string]any)
	if prune["running"] != false || prune["stoppable"] != false || prune["progress"] != nil || last == nil || last["finishedAt"] == nil || last["summary"] != "Stopped" || last["error"] != nil {
		t.Errorf("stopped %v", prune)
	}

	// Work that always finishes runs without Stop.
	e.d.Tasks.Enqueue(playback.TaskPrepareDownloads, id.New())
	downloads := e.task("prepare-downloads")
	if p, _ := downloads["progress"].(map[string]any); downloads["running"] != true || downloads["stoppable"] != false || p["total"] != 0.0 {
		t.Errorf("a download asked for %v", downloads)
	}
	for _, name := range []string{"prepare-downloads", "library-changes"} {
		if w := e.do("POST", "/api/v1/tasks/"+name+"/stop", e.adminToken, nil); w.Code != http.StatusConflict || problemCode(w) != "notStoppable" {
			t.Errorf("stop %s: %d %s", name, w.Code, problemCode(w))
		}
	}
	// An on-demand row has no runs.
	if w := e.do("POST", "/api/v1/tasks/library-changes/run", e.adminToken, nil); w.Code != http.StatusNotFound {
		t.Errorf("run an on-demand task: %d", w.Code)
	}
	if w := e.do("POST", "/api/v1/tasks/scan-libraries/run", e.nobodyToken, nil); w.Code != http.StatusForbidden {
		t.Errorf("run now as a viewer: %d", w.Code)
	}
}

// A task has no next run when every library is set never to do its work.
// The other tasks keep their next runs.
func TestTasksWithEveryLibrarySetToNever(t *testing.T) {
	e := newEnv(t, serving{})
	err := e.b.DB.Write(context.Background(), func(ctx context.Context, tx *store.Tx) error {
		_, err := tx.ExecContext(ctx, `UPDATE library SET previews = 'never', markers = 'never', refresh_days = 0`)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	tasks := decode[list](t, e.do("GET", "/api/v1/tasks", e.adminToken, nil))
	for _, task := range tasks.Items {
		switch never := slices.Contains([]string{"refresh-metadata", "make-previews", "find-markers"}, task["id"].(string)); {
		case never && task["nextRunAt"] != nil:
			t.Errorf("%v runs next with every library set to never", task["id"])
		case !never && task["section"] != "onDemand" && task["nextRunAt"] == nil:
			t.Errorf("%v has no next run", task["id"])
		}
	}
}

// A task's failures are listed by what each item is. Retry forgets the
// failures of the named items, or of every item, and has them done now.
func TestTaskFailures(t *testing.T) {
	e := newEnv(t, serving{})
	failed := func(work id.ID, reason string, at time.Time) {
		e.b.Write(func(ctx context.Context, tx *store.Tx) error {
			_, err := tx.ExecContext(ctx, `UPDATE work SET refresh_error = ?, refreshed_at = ? WHERE id = ?`, reason, at.UnixMilli(), work)
			return err
		})
	}
	failed(e.heat.Work, "TMDb has no such movie", time.Now().Add(-time.Hour))
	failed(e.episode.Work, "TMDb timed out", time.Now())

	path := "/api/v1/tasks/" + metadata.TaskRefreshMetadata
	before := e.task(metadata.TaskRefreshMetadata)
	if before["failedCount"] != 2.0 {
		t.Errorf("refreshes %v", before)
	}
	got := decode[list](t, e.do("GET", path+"/failures", e.adminToken, nil))
	if len(got.Items) != 2 {
		t.Fatalf("failures %v", got.Items)
	}
	ep, heat := got.Items[0], got.Items[1] // newest first
	if ep["itemId"] != e.episode.Work.String() || ep["title"] != "Seeded Series · S1 · E1" || ep["reason"] != "TMDb timed out" || ep["failedAt"] == nil {
		t.Errorf("the episode's failure %v", ep)
	}
	if heat["itemId"] != e.heat.Work.String() || heat["title"] != "Heatwave Seeded" || heat["reason"] != "TMDb has no such movie" {
		t.Errorf("the movie's failure %v", heat)
	}

	// Retry one.
	if w := e.do("POST", path+"/retry", e.adminToken, map[string]any{"itemIds": []string{e.heat.Work.String()}}); w.Code != http.StatusNoContent {
		t.Fatalf("retry one: %d %s", w.Code, w.Body.String())
	}
	if got := decode[list](t, e.do("GET", path+"/failures", e.adminToken, nil)); len(got.Items) != 1 || got.Items[0]["itemId"] != e.episode.Work.String() {
		t.Errorf("after retrying the movie: %v", got.Items)
	}
	if task := e.task(metadata.TaskRefreshMetadata); task["failedCount"] != 1.0 || task["running"] != true || task["progress"].(map[string]any)["total"] != 0.0 {
		t.Errorf("refreshes after retrying the movie %v", task)
	}

	// Retry every one.
	if w := e.do("POST", path+"/retry", e.adminToken, nil); w.Code != http.StatusNoContent {
		t.Fatalf("retry all: %d %s", w.Code, w.Body.String())
	}
	if got := decode[list](t, e.do("GET", path+"/failures", e.adminToken, nil)); len(got.Items) != 0 {
		t.Errorf("after retrying all: %v", got.Items)
	}
	if task := e.task(metadata.TaskRefreshMetadata); task["failedCount"] != 0.0 || task["running"] != true || task["pendingCount"] != before["pendingCount"].(float64)+2 {
		t.Errorf("refreshes after retrying all %v", task)
	}

	// A task that keeps no failures has none to list or retry.
	if w := e.do("GET", "/api/v1/tasks/prune-previews/failures", e.adminToken, nil); w.Code != http.StatusNotFound {
		t.Errorf("failures of a cleanup: %d", w.Code)
	}
	if w := e.do("POST", "/api/v1/tasks/prepare-downloads/retry", e.adminToken, nil); w.Code != http.StatusNotFound {
		t.Errorf("retry downloads: %d", w.Code)
	}
}
