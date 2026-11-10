package markers

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/mmcdole/kivo/internal/store"

	"github.com/mmcdole/kivo/internal/id"
	"github.com/mmcdole/kivo/internal/markers/dbq"
	"github.com/mmcdole/kivo/internal/media"
)

var ctx = context.Background()

// fakeMedia writes what ffmpeg would, and records what it was asked.
type fakeMedia struct {
	mu      sync.Mutex
	err     error // returned by every call
	samples []media.SampleRequest
	// sample answers a SampleRequest. When it is nil, Sample reads nothing.
	sample        func(media.SampleRequest) (media.Sample, error)
	noChromaprint bool
	caps          media.Capabilities
}

func (m *fakeMedia) Sample(_ context.Context, req media.SampleRequest) (media.Sample, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.samples = append(m.samples, req)
	if m.err != nil {
		return media.Sample{}, m.err
	}
	if m.sample != nil {
		return m.sample(req)
	}
	return media.Sample{}, nil
}

func (m *fakeMedia) Chromaprint(context.Context) bool { return !m.noChromaprint }

func (m *fakeMedia) Capabilities() media.Capabilities { return m.caps }

// fakeTasks records the items asked for.
type fakeTasks struct {
	mu    sync.Mutex
	asked []ask
}

// ask is one Enqueue call: the task and its items.
type ask struct {
	task  string
	items []id.ID
}

func (t *fakeTasks) Enqueue(task string, items ...id.ID) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.asked = append(t.asked, ask{task: task, items: items})
}

func (t *fakeTasks) take() []ask {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := t.asked
	t.asked = nil
	return out
}

func TestPrune(t *testing.T) {
	f := newShowFixture(t)
	file := f.file(0)
	current := f.a.result(file.ID, file.Generation)
	old := f.a.result(file.ID, file.Generation+7)
	deleted := f.a.result(id.New(), 1)
	for _, d := range []string{current, old, deleted} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	f.b.Write(func(ctx context.Context, tx *store.Tx) error {
		return dbq.New(tx).SetDecided(ctx, dbq.SetDecidedParams{FileID: file.ID, Generation: file.Generation + 7, Rules: 1, State: decidedNone})
	})
	if err := f.a.prune(ctx); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := f.b.DB.Reader().QueryRowContext(ctx, `SELECT count(*) FROM marker_file`).Scan(&n); err != nil || n != 0 {
		t.Errorf("an earlier generation's decision kept: %d, %v", n, err)
	}
	for d, keep := range map[string]bool{current: true, old: false, deleted: false, filepath.Dir(deleted): false} {
		if _, err := os.Stat(d); (err == nil) != keep {
			t.Errorf("%s: kept %v, want %v", d, err == nil, keep)
		}
	}
}
