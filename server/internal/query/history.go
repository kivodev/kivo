package query

import (
	"context"
	"database/sql"
	"time"

	"github.com/mmcdole/kivo/internal/id"
	"github.com/mmcdole/kivo/internal/users"
)

// historyLimit is how many plays a page of watch history holds when the
// request doesn't say.
const historyLimit = 50

// PlayEvent is one recorded play of a work, for the watch history pages.
type PlayEvent struct {
	ID             id.ID
	Work           WorkSummary
	User           id.ID
	Username       string // who played it
	Started, Ended time.Time
	Position       time.Duration // where it stopped
	Completed      bool          // whether the play reached the played threshold
}

// History lists the Viewer's own plays, newest first. Plays of works the
// Viewer can't see are left out.
func (s *Service) History(ctx context.Context, v users.Viewer, after Cursor, limit int) (Page[PlayEvent], error) {
	vw := viewerOf(v)
	return s.history(ctx, vw, vw.user, after, limit)
}

// ServerHistory lists every user's plays, newest first: the server-wide
// history an admin sees. Plays of works the Viewer can't see are left out.
func (s *Service) ServerHistory(ctx context.Context, v users.Viewer, after Cursor, limit int) (Page[PlayEvent], error) {
	return s.history(ctx, viewerOf(v), id.ID{}, after, limit)
}

// UserHistory lists one user's plays, newest first, as the Viewer sees
// them. The Viewer is an admin. Plays of works the Viewer can't see are
// left out.
func (s *Service) UserHistory(ctx context.Context, v users.Viewer, user id.ID, after Cursor, limit int) (Page[PlayEvent], error) {
	return s.history(ctx, viewerOf(v), user, after, limit)
}

// Watch history reads play_event, one row per play, newest first. It pages
// by keyset on (started_at DESC, id). A play of a work the Viewer can't see
// now isn't listed.

// plays lists one user's plays: those of @player.
var plays = stmt("History", `
SELECT e.id, e.work_id, e.user_id, u.username, e.started_at, e.ended_at, e.position_ms, e.completed
FROM play_event e
JOIN work w ON w.id = e.work_id
JOIN user u ON u.id = e.user_id
WHERE e.user_id = @player
  AND e.started_at <= @after_n
  AND (e.started_at < @after_n OR e.id > @after_id)
  AND `+visible+`
ORDER BY e.started_at DESC, e.id
LIMIT @limit`)

// serverPlays lists every user's plays, for the server-wide history.
var serverPlays = stmt("ServerHistory", `
SELECT e.id, e.work_id, e.user_id, u.username, e.started_at, e.ended_at, e.position_ms, e.completed
FROM play_event e
JOIN work w ON w.id = e.work_id
JOIN user u ON u.id = e.user_id
WHERE e.started_at <= @after_n
  AND (e.started_at < @after_n OR e.id > @after_id)
  AND `+visible+`
ORDER BY e.started_at DESC, e.id
LIMIT @limit`)

// history lists user's plays, or every user's plays when user is zero. It
// serves History, ServerHistory and UserHistory. A limit of 0 is
// historyLimit.
func (s *Service) history(ctx context.Context, vw viewer, user id.ID, after Cursor, limit int) (Page[PlayEvent], error) {
	const list = "history"
	if limit == 0 {
		limit = historyLimit
	}
	limit, err := pageLimit(limit)
	if err != nil {
		return Page[PlayEvent]{}, err
	}
	pos, more, err := after.decode(list, 1)
	if err != nil {
		return Page[PlayEvent]{}, err
	}
	if !more {
		pos = position{N: []int64{startDesc}}
	}
	st := plays
	if user.IsZero() {
		st = serverPlays
	}
	var items []PlayEvent
	err = s.db.Read(ctx, func(tx *sql.Tx) error {
		type play struct {
			PlayEvent
			work               id.ID
			startedAt, endedAt int64
			positionMs         int64
		}
		rows, err := rowsOf(ctx, tx, st, vw.args(sql.Named("player", user),
			sql.Named("after_n", pos.N[0]), sql.Named("after_id", pos.ID[:]), sql.Named("limit", limit+1)),
			func(r scanner, p *play) error {
				return r.Scan(&p.ID, &p.work, &p.User, &p.Username, &p.startedAt, &p.endedAt, &p.positionMs, &p.Completed)
			})
		if err != nil {
			return err
		}
		works := map[id.ID]WorkSummary{}
		for _, r := range rows {
			c, ok := works[r.work]
			if !ok {
				if c, err = summary(ctx, tx, vw, r.work); err != nil {
					return err
				}
				works[r.work] = c
			}
			p := r.PlayEvent
			p.Work, p.Started, p.Ended = c, time.UnixMilli(r.startedAt).UTC(), time.UnixMilli(r.endedAt).UTC()
			p.Position = time.Duration(r.positionMs) * time.Millisecond
			items = append(items, p)
		}
		return nil
	})
	if err != nil {
		return Page[PlayEvent]{}, err
	}
	works := make([]WorkSummary, len(items))
	for i, p := range items {
		works[i] = p.Work
	}
	if err := s.decorate(ctx, vw, works); err != nil {
		return Page[PlayEvent]{}, err
	}
	for i := range items {
		items[i].Work.Images = works[i].Images
	}
	return page(items, limit, 0, func(p PlayEvent) position {
		return position{List: list, N: []int64{p.Started.UnixMilli()}, ID: p.ID}
	}), nil
}
