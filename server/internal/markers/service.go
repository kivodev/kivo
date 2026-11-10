package markers

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/mmcdole/kivo/internal/catalog"
	"github.com/mmcdole/kivo/internal/config"
	"github.com/mmcdole/kivo/internal/health"
	"github.com/mmcdole/kivo/internal/id"
	"github.com/mmcdole/kivo/internal/markers/dbq"
	"github.com/mmcdole/kivo/internal/media"
	"github.com/mmcdole/kivo/internal/store"
	"github.com/mmcdole/kivo/internal/tasks"
)

// The tasks markers runs.
const (
	TaskFindMarkers  = "find-markers"  // nightly: find the markers episode files are missing
	taskPruneMarkers = "prune-markers" // nightly: remove the fingerprints and markers no file needs
)

// Media reads files for their markers. The media package's runner
// implements it.
type Media interface {
	Sample(ctx context.Context, req media.SampleRequest) (media.Sample, error)
	// Chromaprint reports whether Sample can fingerprint audio.
	Chromaprint(ctx context.Context) bool
	Capabilities() media.Capabilities
}

// Tasks enqueues items to be done now. tasks.Runner implements it.
type Tasks interface {
	Enqueue(task string, items ...id.ID)
}

// Deps are the service's collaborators and folders.
type Deps struct {
	Catalog *catalog.Catalog
	Media   Media
	Tasks   Tasks
	// Settings holds the accelerator setting, which picks the device that
	// may decode keyframes.
	Settings *config.Store
	// CacheDir is the cache/ folder. Fingerprints go in its markers/
	// folder.
	CacheDir string
}

// Service finds episodes' intro and credits markers, and gives a play
// its chapters and markers.
type Service struct {
	db   *store.DB
	q    *dbq.Queries
	deps Deps
	room health.Room
}

// New returns the service. It removes whatever an interrupted read left in
// the cache.
func New(db *store.DB, d Deps) *Service {
	_ = os.RemoveAll(filepath.Join(d.CacheDir, resultDir, config.CacheTemp))
	return &Service{
		db: db, q: dbq.New(db.Reader()), deps: d,
		room: health.Room{Dir: d.CacheDir, Problem: health.Problem{
			Component: "markers", Code: "cacheFull",
			Message: "The cache disk is almost full. Finding intro and credits is paused.",
		}},
	}
}

// Tasks returns the package's tasks, for cmd/kivo to add. Files are
// fingerprinted at low OS priority, because reading a file shares the disk
// with playback. The markers-at-once setting says how many run at once.
func (s *Service) Tasks() []tasks.Task {
	return []tasks.Task{
		{Name: TaskFindMarkers, Schedule: tasks.Nightly, Wanted: s.q.Wanted, Work: tasks.Items{
			Pending: s.pending, Do: s.find, Limit: func(st config.Settings) int { return st.MarkersAtOnce },
			PendingCount: s.pendingCount, Failed: s.failed, Retry: s.retry}},
		{Name: taskPruneMarkers, Schedule: tasks.Nightly, Maintenance: true, Work: tasks.Func(s.prune)},
	}
}

// Health implements health.Reporter. It reports a full cache disk.
func (s *Service) Health(ctx context.Context) []health.Problem {
	return s.room.Health(ctx)
}

// videoTrack returns a file's video track as probing described it. It is
// the first video track that isn't cover art.
func videoTrack(f catalog.File) (media.Track, bool) {
	for _, t := range f.Tracks {
		if media.TrackKind(t.Type) != media.Video {
			continue
		}
		var mt media.Track
		if err := json.Unmarshal(t.Info, &mt); err != nil {
			mt = media.Track{Kind: media.Video, Codec: media.Codec(t.Codec)}
		}
		mt.Index = t.Index
		if !mt.AttachedPicture {
			return mt, true
		}
	}
	return media.Track{}, false
}
