package previews

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/mmcdole/kivo/internal/catalog"
	"github.com/mmcdole/kivo/internal/config"
	"github.com/mmcdole/kivo/internal/health"
	"github.com/mmcdole/kivo/internal/id"
	"github.com/mmcdole/kivo/internal/media"
	"github.com/mmcdole/kivo/internal/previews/dbq"
	"github.com/mmcdole/kivo/internal/store"
	"github.com/mmcdole/kivo/internal/tasks"
)

// The tasks previews runs. Both run nightly.
const (
	TaskMakePreviews  = "make-previews"  // makes the previews that files are missing
	taskPrunePreviews = "prune-previews" // removes the previews no file needs
)

// makeTimeout bounds making one file's previews.
const makeTimeout = 2 * time.Hour

// Scrub previews are a frame every 10 seconds, 320 pixels wide, in sheets of
// 10 by 10.
const (
	previewInterval = 10 * time.Second
	previewWidth    = 320
	previewGrid     = 10
	manifestFile    = "manifest.json"
)

// Media makes previews. media's runner implements it.
type Media interface {
	Previews(ctx context.Context, req media.PreviewsRequest) (media.Previews, error)
}

// Tasks asks for items to be done now. tasks.Runner implements it.
type Tasks interface {
	Ask(task string, items ...id.ID)
}

// Deps are the service's collaborators and folders.
type Deps struct {
	Catalog *catalog.Catalog
	Media   Media
	Tasks   Tasks
	// CacheDir is the cache/ folder. Previews go in its previews/ folder.
	CacheDir string
}

// Service makes preview thumbnails and finds them for the packages that
// serve them.
type Service struct {
	db   *store.DB
	q    *dbq.Queries
	deps Deps
	dir  string // cache/previews
	room health.Room
}

// New returns the service. It removes anything an interrupted run left in
// the cache.
func New(db *store.DB, d Deps) *Service {
	dir := filepath.Join(d.CacheDir, "previews")
	_ = os.RemoveAll(filepath.Join(dir, config.CacheTemp))
	return &Service{
		db: db, q: dbq.New(db.Reader()), deps: d, dir: dir,
		room: health.Room{Dir: d.CacheDir, Problem: health.Problem{
			Component: "previews", Code: "cacheFull",
			Message: "The cache disk is almost full. Preview thumbnails are paused.",
		}},
	}
}

// Tasks returns the package's tasks for cmd/kivo to add. Previews are made
// at low OS priority, because reading a whole file shares the disk with
// playback. The previews-at-once setting says how many files are done at
// once.
func (s *Service) Tasks() []tasks.Task {
	return []tasks.Task{
		{Name: TaskMakePreviews, Nightly: true, Limit: func(st config.Settings) int { return st.PreviewsAtOnce }, Wanted: s.wanted,
			Pending: s.pending, Do: s.make, Left: s.left, Failed: s.failed, Retry: s.retry},
		{Name: taskPrunePreviews, Nightly: true, Maintenance: true, Run: s.prune},
	}
}

// Previews returns a file's scrub previews and the folder holding their
// sheets, if they're made. Missing previews are left to the nightly "Make
// preview thumbnails" task, as the library's setting says.
func (s *Service) Previews(ctx context.Context, file id.ID) (media.Previews, string, bool) {
	cur, err := s.current(ctx, file)
	if err != nil {
		return media.Previews{}, "", false
	}
	dir := s.result(file, cur)
	b, err := os.ReadFile(filepath.Join(dir, manifestFile))
	if err != nil {
		return media.Previews{}, "", false
	}
	var p media.Previews
	if err := json.Unmarshal(b, &p); err != nil || p.Sheets == 0 {
		return media.Previews{}, "", false
	}
	return p, dir, true
}

// make makes a file's scrub previews, unless its library is set to make
// none. It records them as made, or as failed for a file media can't read.
func (s *Service) make(ctx context.Context, file id.ID) error {
	f, err := s.deps.Catalog.File(ctx, file)
	switch {
	case errors.Is(err, catalog.ErrNotFound):
		return nil // the file was deleted, or a move merged it into another file
	case err != nil:
		return err
	case !f.Present:
		return nil
	}
	if failed, err := s.q.FailedNow(ctx, f.ID); failed || err != nil {
		return err
	}
	lib, err := s.deps.Catalog.Library(ctx, f.Library)
	if err != nil {
		return err
	}
	video, ok := videoTrack(f)
	switch {
	case lib.Previews == catalog.Never || !ok || f.Probe.Duration == 0:
		return nil // nothing to show, or the file isn't probed yet
	case s.has(f.ID, f.Generation):
		return s.settle(ctx, f, nil) // the previews were made before they were recorded
	}
	if err := s.room.Check(); err != nil {
		return err
	}
	tmp, err := config.MakeCacheTemp(s.dir)
	if err != nil {
		return s.room.Err(err)
	}
	defer os.RemoveAll(tmp)
	start := time.Now()
	mctx, cancel := context.WithTimeout(ctx, makeTimeout)
	defer cancel()
	p, err := s.deps.Media.Previews(mctx, media.PreviewsRequest{
		Path: f.Path(), VideoTrack: video.Index, Source: video,
		Interval: previewInterval, Width: previewWidth, Columns: previewGrid, Rows: previewGrid, Dir: tmp,
	})
	switch {
	case errors.Is(err, media.ErrSourceCorrupt), errors.Is(err, media.ErrUnsupported):
		return s.settle(ctx, f, err) // an unreadable file, an ffmpeg without tone mapping, or a picture of unknown size
	case err != nil:
		return s.room.Err(err)
	}
	b, _ := json.Marshal(p)
	if err := os.WriteFile(filepath.Join(tmp, manifestFile), b, 0o644); err != nil {
		return s.room.Err(err)
	}
	if made, err := s.commit(ctx, f, tmp); !made || err != nil {
		return err
	}
	slog.DebugContext(ctx, "previews: made", "file", f.ID, "sheets", p.Sheets, "duration", time.Since(start).Round(time.Millisecond))
	return s.settle(ctx, f, nil)
}

// settle records a file's previews as made at its generation, or as failed
// with failure.
func (s *Service) settle(ctx context.Context, f catalog.File, failure error) error {
	row := dbq.SettleParams{FileID: f.ID, Generation: f.Generation, State: "done", UpdatedAt: time.Now().UnixMilli()}
	if failure != nil {
		slog.WarnContext(ctx, "previews: can't make them", "file", f.RelPath, "err", failure)
		row.State, row.Reason = "failed", sql.NullString{String: failure.Error(), Valid: true}
	}
	return s.db.Write(ctx, func(ctx context.Context, tx *store.Tx) error {
		return dbq.New(tx).Settle(ctx, row)
	})
}

// pending lists what the task has left: the files missing previews, in
// every library that makes them.
func (s *Service) pending(ctx context.Context) ([]id.ID, error) {
	return s.q.Pending(ctx)
}

// left returns how many files are left.
func (s *Service) left(ctx context.Context) (int, error) {
	left, err := s.pending(ctx)
	return len(left), err
}

// failed lists the files whose previews failed at their generation.
func (s *Service) failed(ctx context.Context) ([]tasks.Failure, error) {
	rows, err := s.q.Failed(ctx)
	out := make([]tasks.Failure, len(rows))
	for i, r := range rows {
		out[i] = tasks.Failure{Item: r.FileID, Reason: r.Reason.String, At: time.UnixMilli(r.UpdatedAt)}
	}
	return out, err
}

// retry forgets the failures of the given files, or of every failed file
// when none are given. It returns the files to make previews for again.
func (s *Service) retry(ctx context.Context, files ...id.ID) ([]id.ID, error) {
	var out []id.ID
	err := s.db.Write(ctx, func(ctx context.Context, tx *store.Tx) error {
		var err error
		out, err = dbq.New(tx).Forget(ctx, dbq.ForgetParams{Ids: id.Set(files...), All: len(files) == 0})
		return err
	})
	return out, err
}

// current cheaply reads a file's current generation, for finding its
// result.
func (s *Service) current(ctx context.Context, file id.ID) (int64, error) {
	gen, err := s.q.FileGeneration(ctx, file)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, fmt.Errorf("%w: file %s", catalog.ErrNotFound, file)
	}
	return gen, err
}

// Probed asks for a file's previews, if its library makes previews as media
// is added. The ask waits until tx, the write that stores the probe,
// commits.
func (s *Service) Probed(tx *store.Tx, lib catalog.Library, file id.ID) {
	if lib.Previews == catalog.NightlyAndAdded {
		tx.AfterCommit(func() { s.deps.Tasks.Ask(TaskMakePreviews, file) })
	}
}

// wanted reports whether any library makes previews, which means the
// nightly task has something to do.
func (s *Service) wanted(ctx context.Context) (bool, error) {
	ok, err := s.q.Wanted(ctx)
	if err != nil {
		return false, err
	}
	return ok, nil
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
