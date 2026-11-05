package subtitles

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"time"

	"github.com/mmcdole/kivo/internal/catalog"
	"github.com/mmcdole/kivo/internal/config"
	"github.com/mmcdole/kivo/internal/health"
	"github.com/mmcdole/kivo/internal/id"
	"github.com/mmcdole/kivo/internal/keylock"
	"github.com/mmcdole/kivo/internal/media"
	"github.com/mmcdole/kivo/internal/store"
	"github.com/mmcdole/kivo/internal/subtitles/dbq"
	"github.com/mmcdole/kivo/internal/tasks"
)

// The tasks subtitles runs.
const (
	TaskExtractSubtitles = "extract-subtitles" // extracts the embedded subtitles and fonts of files with ASS or PGS tracks
	taskPruneSubtitles   = "prune-subtitles"   // removes the subtitles and fonts no file needs, each night
)

// extractTimeout bounds extracting one file's subtitles.
const extractTimeout = 2 * time.Hour

// Media extracts subtitles. The media package's runner implements it.
type Media interface {
	Subtitles(ctx context.Context, req media.SubtitlesRequest) error
}

// Tasks asks for a task's items to be done now. tasks.Runner implements
// it.
type Tasks interface {
	Ask(task string, items ...id.ID)
}

// Deps are the service's collaborators and folders.
type Deps struct {
	Catalog *catalog.Catalog
	// Settings hold the OpenSubtitles credentials.
	Settings *config.Store
	Media    Media
	Tasks    Tasks
	// CacheDir is cache/. Extracted subtitles go in its subtitles/ folder,
	// and the fonts they use go in its fonts/ folder.
	CacheDir string
	// SubtitleDir is data/subtitles/, where downloaded subtitles are kept.
	SubtitleDir string
}

// Options are the service's optional hooks.
type Options struct {
	// Replaced is told which downloaded subtitles a new one replaced. It
	// runs in the transaction that drops them, and an error rolls that
	// transaction back.
	Replaced func(ctx context.Context, tx *store.Tx, replaced []id.ID, by id.ID) error
}

// Service extracts embedded subtitles and finds them for the packages that
// serve them. It also searches for, downloads, and keeps downloaded
// subtitles.
type Service struct {
	db   *store.DB
	q    *dbq.Queries
	deps Deps
	// openSubtitles searches for subtitles and downloads them.
	openSubtitles *openSubtitles
	room          health.Room
	replaced      func(ctx context.Context, tx *store.Tx, replaced []id.ID, by id.ID) error // Options.Replaced

	// fonts is held while an extraction adds fonts to the store and puts
	// its result, which lists them, in place. It's also held while a prune
	// deletes the fonts no result lists.
	fonts sync.Mutex
	// files holds a per-file lock while Prepare prepares that file.
	files keylock.Locks[id.ID]
}

// New returns the service, and removes whatever an interrupted extraction
// left in the cache.
func New(db *store.DB, d Deps, opt Options) *Service {
	if opt.Replaced == nil {
		opt.Replaced = func(context.Context, *store.Tx, []id.ID, id.ID) error { return nil }
	}
	_ = os.RemoveAll(filepath.Join(d.CacheDir, resultDir, config.CacheTemp))
	return &Service{
		db: db, q: dbq.New(db.Reader()), deps: d, openSubtitles: newOpenSubtitles(), replaced: opt.Replaced,
		room: health.Room{Dir: d.CacheDir, Problem: health.Problem{
			Component: "subtitles", Code: "cacheFull",
			Message: "The cache disk is almost full. Subtitle preparation is paused.",
		}},
	}
}

// Tasks returns the subtitles package's tasks, for cmd/kivo to add.
// Extraction runs one file at a time at low OS priority, because reading a
// whole file shares the disk with playback. In a library that extracts
// subtitles as media is added, a newly probed file is extracted at once.
// Whatever is left is extracted each night. A download prepares its own
// file through Prepare, outside the task.
func (s *Service) Tasks() []tasks.Task {
	return []tasks.Task{
		{Name: TaskExtractSubtitles, Nightly: true, Wanted: s.wanted, Pending: s.pending, Do: s.do, Left: s.left, Failed: s.failed, Retry: s.retry},
		{Name: taskPruneSubtitles, Nightly: true, Maintenance: true, Run: s.prune},
	}
}

// Probed asks for a file's extraction once tx, the write that stores its
// probe, commits. It applies only in a library that extracts subtitles as
// media is added, and only when the file has an embedded ASS, SSA or PGS
// track. That way the subtitles are drawn from files from the first play.
func (s *Service) Probed(tx *store.Tx, lib catalog.Library, file id.ID, tracks []catalog.Track) {
	if lib.Subtitles == catalog.NightlyAndAdded && extractedAhead(tracks) {
		tx.AfterCommit(func() { s.deps.Tasks.Ask(TaskExtractSubtitles, file) })
	}
}

// extractedAhead reports whether tracks include an embedded ASS, SSA or
// PGS track. A file with one has its subtitles extracted ahead of its
// plays.
func extractedAhead(tracks []catalog.Track) bool {
	return slices.ContainsFunc(tracks, func(t catalog.Track) bool {
		f, _ := media.Codec(t.Codec).SubtitleFormat()
		return media.TrackKind(t.Type) == media.Subtitle && (f == media.ASS || f == media.PGS)
	})
}

// wanted reports whether any library extracts subtitles, so the nightly
// task has something to do.
func (s *Service) wanted(ctx context.Context) (bool, error) {
	ok, err := s.q.Wanted(ctx)
	if err != nil {
		return ok, err
	}
	return ok, nil
}

// settle records that a file's subtitles were extracted at its generation,
// or that extraction failed with failure.
func (s *Service) settle(ctx context.Context, f catalog.File, failure error) error {
	row := dbq.SettleParams{FileID: f.ID, Generation: f.Generation, State: "done", UpdatedAt: time.Now().UnixMilli()}
	if failure != nil {
		slog.WarnContext(ctx, "subtitles: can't extract them", "file", f.RelPath, "err", failure)
		row.State, row.Reason = "failed", sql.NullString{String: failure.Error(), Valid: true}
	}
	return s.db.Write(ctx, func(ctx context.Context, tx *store.Tx) error {
		return dbq.New(tx).Settle(ctx, row)
	})
}

// pending lists what the task has left: the files with an unextracted
// embedded ASS, SSA or PGS track, in every library that extracts
// subtitles.
func (s *Service) pending(ctx context.Context) ([]id.ID, error) {
	return s.q.Pending(ctx)
}

// left is how many files are left.
func (s *Service) left(ctx context.Context) (int, error) {
	left, err := s.pending(ctx)
	return len(left), err
}

// failed lists the files whose extraction failed at their generation.
func (s *Service) failed(ctx context.Context) ([]tasks.Failure, error) {
	rows, err := s.q.Failed(ctx)
	out := make([]tasks.Failure, len(rows))
	for i, r := range rows {
		out[i] = tasks.Failure{Item: r.FileID, Reason: r.Reason.String, At: time.UnixMilli(r.UpdatedAt)}
	}
	return out, err
}

// retry forgets the failures of the given files, or of every file when
// none are given, and returns the files to extract again.
func (s *Service) retry(ctx context.Context, files ...id.ID) ([]id.ID, error) {
	var out []id.ID
	err := s.db.Write(ctx, func(ctx context.Context, tx *store.Tx) error {
		var err error
		out, err = dbq.New(tx).Forget(ctx, dbq.ForgetParams{Ids: id.Set(files...), All: len(files) == 0})
		return err
	})
	return out, err
}

// current reads a file's current generation cheaply, to find its result.
func (s *Service) current(ctx context.Context, file id.ID) (int64, error) {
	gen, err := s.q.FileGeneration(ctx, file)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, fmt.Errorf("%w: file %s", catalog.ErrNotFound, file)
	}
	return gen, err
}
