package playback

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/mmcdole/kivo/internal/events"
	"github.com/mmcdole/kivo/internal/id"
	"github.com/mmcdole/kivo/internal/playback/dbq"
	"github.com/mmcdole/kivo/internal/store"
	"github.com/mmcdole/kivo/internal/tasks"
	"github.com/mmcdole/kivo/internal/users"
)

// Tasks asks for a task's items to be done now. tasks.Runner implements
// it.
type Tasks interface {
	Ask(task string, items ...id.ID)
}

// The tasks playback runs.
const (
	TaskPrepareDownloads = "prepare-downloads" // prepares each download's subtitle, then its conversion
	taskPruneDownloads   = "prune-downloads"   // removes expired downloads each night
)

const (
	// keepDownload is how long a download stays on the server after it's
	// ready or failed, unless its app reports that it has the download
	// sooner.
	keepDownload = 7 * 24 * time.Hour
	// downloadAttempts is how many times a download whose file can't be
	// read is tried, each on a later pass, before it fails.
	downloadAttempts = 3
	// progressEvery is how often a download's owner is told that its
	// preparation progressed.
	progressEvery = 10 * time.Second
)

// Downloads plans, prepares and serves downloads. They are kept in the
// database, so a restart doesn't lose them. A task finds them there and
// prepares them. Downloads plans the way Service does and holds no play.
type Downloads struct {
	*planner
	db *store.DB
	q  *dbq.Queries

	// mu guards preparing and the fields of each preparation in it,
	// attempts, and downloadsPrepared.
	mu sync.Mutex
	// preparing holds the downloads this process is preparing now, by ID.
	preparing map[id.ID]*preparation
	// attempts counts the tries at each download whose file couldn't be
	// read.
	attempts map[id.ID]int
	// downloadsPrepared counts the downloads prepared since none were waiting, for
	// the Downloads task's count.
	downloadsPrepared int
}

// NewDownloads returns the downloads of the server whose plays s runs.
// They're planned with the same collaborators as its plays, and an
// accelerator that fails a conversion shows in its Health.
func NewDownloads(s *Service) *Downloads {
	return &Downloads{
		planner: s.planner, db: s.db, q: dbq.New(s.db.Reader()),
		preparing: map[id.ID]*preparation{}, attempts: map[id.ID]int{},
	}
}

// preparation is a download this process is preparing. Its fields are
// guarded by Downloads' mu.
type preparation struct {
	user     id.ID
	started  time.Time     // when the conversion began; zero before that
	reached  time.Duration // how far the conversion has got
	total    time.Duration
	cancel   context.CancelFunc
	removed  bool // the app removed the download, so cancelling isn't a failure
	notified time.Time
}

// progressLocked returns how far a preparation has got and about how long
// it has left. The time left is the time taken so far, scaled, and is
// only given once the preparation has run long enough to tell. The caller
// holds s.mu.
func (p *preparation) progressLocked(now time.Time) (float64, time.Duration) {
	if p.started.IsZero() || p.total <= 0 {
		return 0, 0
	}
	done := min(1, float64(p.reached)/float64(p.total))
	took := now.Sub(p.started)
	if done < 0.01 || took < 10*time.Second {
		return done, 0
	}
	return done, time.Duration(float64(took) * (1 - done) / done)
}

// Tasks returns the downloads' tasks, for cmd/kivo to add. Downloads are
// prepared in the order they were asked for, at low OS priority, two at a
// time when an accelerator encodes video and one at a time on the CPU.
// They don't count against the server's video transcode limit. Expired
// downloads are removed each night.
func (s *Downloads) Tasks() []tasks.Task {
	return []tasks.Task{
		{Name: TaskPrepareDownloads, Finish: true, Limit: s.preparingAtOnce, Pending: s.q.OpenDownloads, Do: s.prepare},
		{Name: taskPruneDownloads, Nightly: true, Maintenance: true, Run: s.Prune},
	}
}

// downloader checks that a viewer may download a work. The viewer must see
// the work, and the user must be allowed downloads.
func (s *Downloads) downloader(ctx context.Context, v users.Viewer, user, work id.ID) (users.User, error) {
	if ok, err := s.visible(ctx, v, work); err != nil {
		return users.User{}, err
	} else if !ok {
		return users.User{}, fmt.Errorf("%w: work %s", ErrNotFound, work)
	}
	u, err := s.deps.Users.User(ctx, user)
	if err != nil {
		return users.User{}, err
	}
	if !u.Downloads {
		return users.User{}, &RefusedError{Err: ErrNotPlayable, Reason: RefuseDownloadsNotAllowed}
	}
	return u, nil
}

// own returns one of the user's downloads. Anyone else's download
// returns ErrNotFound, the same as a missing one.
func (s *Downloads) own(ctx context.Context, user, download id.ID) (dbq.Download, error) {
	d, err := s.q.GetDownload(ctx, download)
	if errors.Is(err, sql.ErrNoRows) || err == nil && d.UserID != user {
		return dbq.Download{}, fmt.Errorf("%w: download %s", ErrNotFound, download)
	}
	if err != nil {
		return dbq.Download{}, err
	}
	return d, nil
}

// changed tells a user's clients that their downloads changed.
func (s *Downloads) changed(user id.ID) {
	s.deps.Events.Publish(events.Event{Kind: events.DownloadsChanged, Scope: events.Scope{User: user}})
}
