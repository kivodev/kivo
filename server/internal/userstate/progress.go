package userstate

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/mmcdole/kivo/internal/activity"
	"github.com/mmcdole/kivo/internal/catalog"
	"github.com/mmcdole/kivo/internal/config"
	"github.com/mmcdole/kivo/internal/events"
	"github.com/mmcdole/kivo/internal/id"
	"github.com/mmcdole/kivo/internal/store"
	"github.com/mmcdole/kivo/internal/userstate/dbq"
)

const (
	floorMin      = 2 * time.Minute // "Continue Watching" hides positions before this…
	floorFraction = 0.05            // …or before this share of the version
	// A book is finished with this much or less left, whatever the played
	// threshold, because end credits run a minute or two. A book shorter
	// than ten times this is finished in its last tenth instead.
	bookTail = 5 * time.Minute
)

// Progress is a playback position report. Position is on the version's
// timeline, which is its parts joined in order.
type Progress struct {
	Play, User, Version id.ID
	Position            time.Duration
	// At is when the play made the report. A report older than one already
	// written for the same play is dropped. A report with the same time is
	// also dropped unless it stops the play, so a heartbeat that races the
	// stop never undoes it. A report older than another play's report
	// leaves that other play's position in place.
	At time.Time
	// Stopped marks the play's last report, which repeats its last
	// position. A play finished at this point is recorded as activity, and
	// the user's clients are told about the works it wrote.
	Stopped bool
	// Client and Local describe the play, for its activity.
	Client string
	Local  bool
}

// Service owns each user's watch state and play history. Each playback
// report is written as it arrives.
type Service struct {
	db   *store.DB
	q    *dbq.Queries
	deps Deps
}

// length is a version's duration, which is its parts' durations summed,
// and whether the version is a book. Both decide when a position finishes
// the version.
type length struct {
	dur  time.Duration
	book bool
}

// Deps are the service's collaborators.
type Deps struct {
	Catalog *catalog.Catalog
	// Settings hold the played threshold, which is read at each report.
	Settings *config.Store
	// Activity records finished plays.
	Activity *activity.Service
	// Events is told, once the write commits, about works whose state a
	// user changed or a play's last position wrote.
	Events *events.Bus
}

// New returns the service.
func New(db *store.DB, d Deps) *Service {
	return &Service{db: db, q: dbq.New(db.Reader()), deps: d}
}

// Report writes a play's position to each of its version's works, along
// with the play's event in the watch history. A play finishes a work only
// once, the first time it reaches the played threshold. A version that was
// regrouped away has nothing to attach to, so its report is dropped.
func (s *Service) Report(ctx context.Context, r Progress) error {
	// The version's works are looked up at each report, so a
	// re-identification never strands a play's later reports.
	p, err := s.position(ctx, r.Play, r.User, r.Version, r.Position, r.At)
	if errors.Is(err, catalog.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	return s.db.Write(ctx, func(ctx context.Context, tx *store.Tx) error {
		q := dbq.New(tx)
		at := p.at.UnixMilli()
		for _, w := range p.works {
			ev, err := q.PlayEvent(ctx, dbq.PlayEventParams{PlayID: p.play, WorkID: w})
			if err != nil && !errors.Is(err, sql.ErrNoRows) {
				return err
			}
			if ev.EndedAt > at || ev.EndedAt == at && !r.Stopped {
				continue // a later report of this play is already written
			}
			if err := writePosition(ctx, q, p, w, ev.Completed); err != nil {
				return err
			}
			if r.Stopped && p.finished {
				err := s.deps.Activity.Played(ctx, tx, activity.PlayFinished, activity.PlayActivity{User: p.user, Work: w, Client: r.Client, Local: r.Local})
				if err != nil {
					return err
				}
			}
		}
		if r.Stopped {
			s.changed(tx, p.user, p.works...)
		}
		return nil
	})
}

// writePosition writes a play's position to one of its works, on the
// version played, along with the play's event. A finishing position counts a play, unless the play
// already finished the work. The play's first report starts its event, so a
// crash leaves an open event rather than none.
func writePosition(ctx context.Context, q *dbq.Queries, p *position, work id.ID, counted bool) error {
	at, pos := p.at.UnixMilli(), p.pos.Milliseconds()
	var err error
	switch {
	case p.finished && !counted:
		err = q.SaveFinished(ctx, dbq.SaveFinishedParams{UserID: p.user, WorkID: work, VersionID: p.version, At: at})
	case !p.finished:
		err = q.SaveProgress(ctx, dbq.SaveProgressParams{UserID: p.user, WorkID: work, PositionMs: pos, Resumable: p.resumable, VersionID: p.version, At: at})
	}
	if err != nil {
		return err
	}
	return q.InsertEvent(ctx, dbq.InsertEventParams{
		ID: id.New(), UserID: p.user, WorkID: work, VersionID: p.version, PlayID: p.play,
		StartedAt: at, EndedAt: at, PositionMs: pos, Completed: p.finished,
	})
}

// position is a play's position, to be written to each of its version's
// works. Both a play's report and an offline position are written as a
// position.
type position struct {
	play, user, version id.ID
	works               []id.ID
	pos                 time.Duration
	at                  time.Time // when the position was reported
	finished            bool      // the position finishes the works
	resumable           bool      // the position is past the "Continue Watching" floor
}

// position places pos on a version's timeline and classifies it against
// the played threshold.
func (s *Service) position(ctx context.Context, play, user, version id.ID, pos time.Duration, at time.Time) (*position, error) {
	v, err := s.deps.Catalog.Version(ctx, version)
	if err != nil {
		return nil, err
	}
	l, err := s.lengthOf(ctx, v)
	if err != nil {
		return nil, err
	}
	p := &position{play: play, user: user, version: version, works: v.Works, pos: min(max(pos, 0), l.dur), at: at}
	p.finished, p.resumable = classify(p.pos, l.dur, l.book, s.deps.Settings.Get().PlayedThreshold)
	return p, nil
}

// changed tells the user's clients that the user's state for works
// changed. It waits until tx commits.
func (s *Service) changed(tx *store.Tx, user id.ID, works ...id.ID) {
	tx.AfterCommit(func() {
		s.deps.Events.Publish(events.Event{Kind: events.UserStateChanged, Scope: events.Scope{User: user}, Data: works})
	})
}

// classify decides whether a position finishes a work, and otherwise
// whether it's past the "Continue Watching" floor. A video is finished at
// the played threshold, and resumable after both 2 minutes and 5%. A book
// is finished with bookTail or less left, and resumable after 2 minutes.
func classify(pos, dur time.Duration, book bool, threshold float64) (finished, resumable bool) {
	if dur <= 0 || pos <= 0 {
		return false, false
	}
	if book {
		finished = dur-pos <= min(bookTail, dur/10)
		return finished, !finished && pos >= floorMin
	}
	finished = float64(pos) >= threshold*float64(dur)
	return finished, !finished && pos >= floorMin && float64(pos) >= floorFraction*float64(dur)
}

// lengthOf returns a version's length: its parts' durations summed, and
// whether the version is a book.
func (s *Service) lengthOf(ctx context.Context, v catalog.Version) (*length, error) {
	var out length
	for _, part := range v.Parts {
		f, err := s.deps.Catalog.File(ctx, part)
		if err != nil {
			return nil, err
		}
		out.dur += f.Probe.Duration
	}
	w, err := s.deps.Catalog.Work(ctx, v.Works[0])
	if err != nil {
		return nil, err
	}
	out.book = w.Kind == catalog.KindBook
	return &out, nil
}
