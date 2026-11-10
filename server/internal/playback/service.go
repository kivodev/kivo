package playback

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/mmcdole/kivo/internal/activity"
	"github.com/mmcdole/kivo/internal/auth"
	"github.com/mmcdole/kivo/internal/catalog"
	"github.com/mmcdole/kivo/internal/config"
	"github.com/mmcdole/kivo/internal/events"
	"github.com/mmcdole/kivo/internal/fault"
	"github.com/mmcdole/kivo/internal/id"
	"github.com/mmcdole/kivo/internal/markers"
	"github.com/mmcdole/kivo/internal/previews"
	"github.com/mmcdole/kivo/internal/store"
	"github.com/mmcdole/kivo/internal/subtitles"
	"github.com/mmcdole/kivo/internal/transcode"
	"github.com/mmcdole/kivo/internal/users"
	"github.com/mmcdole/kivo/internal/userstate"
)

var (
	// ErrNoPlay means the play is unknown, expired, or owned by someone
	// else. The cases look the same so play IDs can't be probed.
	ErrNoPlay = fault.New(fault.NotFound, "no such play")
	// ErrNotFound means a work or version the viewer can't see, or a bad
	// media name.
	ErrNotFound = fault.New(fault.NotFound, "not found")
	// ErrNotReady means a subtitle file isn't extracted yet, or a
	// download's conversion isn't ready.
	ErrNotReady = fault.Coded(fault.NotFound, "notReady", "not ready")
	// ErrStreamLimit means the user is at their stream limit. The error is
	// a *StreamLimitError that lists the active plays.
	ErrStreamLimit = fault.Coded(fault.Conflict, "streamLimit", "too many streams")
	// ErrTranscodeBusy means the play needs a transcode that the server
	// can't start now but may start later. Either all of the server's
	// video transcodes are in use, or the transcode disk is full. The error
	// is a *RefusedError with the reason.
	ErrTranscodeBusy = fault.Coded(fault.Unavailable, "transcodeBusy", "transcoding busy")
	// ErrPlayStopped means an admin stopped the play, so its player should
	// stop rather than start it again. The error is a *StoppedError with
	// the admin's message.
	ErrPlayStopped = fault.Coded(fault.Gone, "playEnded", "play stopped by an admin")
	// ErrNotPlayable means no rung works for this client. The error is a
	// *RefusedError with the reason.
	ErrNotPlayable = fault.Coded(fault.Conflict, "notPlayable", "not playable on this client")
	// ErrNoFallback means the failed play already transcoded every track on
	// the CPU, so a fallback has nothing else to try.
	ErrNoFallback = fault.Coded(fault.Conflict, "noFallback", "nothing left to fall back to")
	// ErrJobFailed means the play's transcode failed mid-stream. The
	// client reports it through Fallback.
	ErrJobFailed = fault.Coded(fault.Internal, "transcodeFailed", "the play's transcode failed")
)

// RefusedError means a play can't start. Reason is the reason to show,
// and Unmet the capability the file failed, for RefuseFileUnplayable.
type RefusedError struct {
	Err    error
	Reason Refusal
	Unmet  *Unmet
}

func (e *RefusedError) Error() string { return fmt.Sprintf("%v: %s", e.Err, e.Reason) }

func (e *RefusedError) Unwrap() error { return e.Err }

// Deps are the collaborators of plays and downloads.
type Deps struct {
	Catalog    *catalog.Catalog
	Users      *users.Service
	Auth       *auth.Service
	Userstate  *userstate.Service
	Transcoder *transcode.Transcoder
	Settings   *config.Store
	// Markers gives a play its chapters and markers.
	Markers *markers.Service
	// Tasks prepares downloads.
	Tasks Tasks
	// Activity records each play's start.
	Activity *activity.Service
	// Subtitles gives the embedded subtitles the subtitles package
	// prepared, and says where downloaded subtitles are kept.
	Subtitles *subtitles.Service
	// Previews gives the scrub previews the previews package made.
	Previews *previews.Service
	// Accelerators is what the accelerator test found.
	Accelerators Accelerators
	// Events receives three kinds of event:
	//   - each play that ends before the server stops, except by a
	//     fallback, so a player whose play was stopped elsewhere or
	//     expired can say so;
	//   - each change "Now playing" shows: a play started or ended, or a
	//     heartbeat reported its position;
	//   - each user whose downloads changed: moved in the queue, progressed
	//     in preparation, or became ready or failed.
	Events *events.Bus
}

// Ended is a play that ended, as the playEnded event tells its user's
// clients.
type Ended struct {
	Play    id.ID
	Message string // the admin's message to the viewer, when an admin stopped the play
}

// Service runs plays. They live in memory, so a restart ends them and the
// client starts again from its saved position.
type Service struct {
	*planner
	db      *store.DB
	created time.Time

	// mu guards plays and the fields of each play that heartbeats and media
	// requests update, stopped, reserved, starting, seeks and seekTime.
	mu       sync.Mutex
	plays    map[id.ID]*play
	stopped  map[id.ID]stoppedPlay // plays an admin stopped, kept for a while so their players learn of it
	reserved int                   // video transcodes held for plays still starting
	starting map[id.ID]int         // streams held for plays still starting, per user
	seeks    int                   // number of seeks clients have timed
	seekTime time.Duration         // total time to the first frame across those seeks
}

// New returns the service.
func New(db *store.DB, d Deps) *Service {
	return &Service{
		planner: &planner{deps: d}, db: db, created: time.Now(),
		plays:    map[id.ID]*play{},
		stopped:  map[id.ID]stoppedPlay{},
		starting: map[id.ID]int{},
	}
}

// expiryCheck is how often Run looks for expired plays.
const expiryCheck = 30 * time.Second

// Run expires idle plays every expiryCheck, closing their jobs and saving
// their positions. It ends every play on shutdown.
func (s *Service) Run(ctx context.Context) error {
	t := time.NewTicker(expiryCheck)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			s.endAll(context.WithoutCancel(ctx))
			return nil
		case <-t.C:
			s.endExpired(ctx)
		}
	}
}

// notify tells admins that "Now playing" changed.
func (s *Service) notify() {
	s.deps.Events.Publish(events.Event{Kind: events.PlaysChanged, Scope: events.Scope{AdminOnly: true}})
}
