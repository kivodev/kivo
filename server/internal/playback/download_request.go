package playback

import (
	"cmp"
	"context"
	"database/sql"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/mmcdole/kivo/internal/auth"
	"github.com/mmcdole/kivo/internal/id"
	"github.com/mmcdole/kivo/internal/playback/dbq"
	"github.com/mmcdole/kivo/internal/store"
	"github.com/mmcdole/kivo/internal/users"
)

// DownloadRequest is what a client asks to download. A video's quality is
// one of the names in DownloadQualities, or DefaultDownloadQuality when
// empty. A book is always downloaded as its original.
type DownloadRequest struct {
	Work          id.ID        `json:"work"`
	Version       id.ID        `json:"version,omitzero"`   // zero means the same version a play would pick
	AudioTrack    *id.ID       `json:"audio,omitempty"`    // nil means choose by preference
	SubtitleTrack *id.ID       `json:"subtitle,omitempty"` // nil means choose by preference; the zero ID means off
	Caps          Capabilities `json:"capabilities"`       // what the client plays offline
	Settings      Settings     `json:"settings"`
	Quality       string       `json:"quality"`
}

// asksFor reports whether two requests ask for the same download: the
// same version and tracks of the work, at the same quality, with the same
// settings. What the device plays isn't compared, since an app reports it
// anew each time it asks, and differently after an update.
func (r DownloadRequest) asksFor(o DownloadRequest) bool {
	same := func(a, b *id.ID) bool { return a == nil && b == nil || a != nil && b != nil && *a == *b }
	quality := func(name string) DownloadQuality {
		q, _ := DownloadQualityNamed(cmp.Or(name, DefaultDownloadQuality))
		return q
	}
	return r.Work == o.Work && r.Version == o.Version && same(r.AudioTrack, o.AudioTrack) && same(r.SubtitleTrack, o.SubtitleTrack) &&
		quality(r.Quality) == quality(o.Quality) && r.Settings == o.Settings
}

// sameDownload finds the newest of a device's downloads of a work, newest
// first, that asked for what r asks for.
func sameDownload(rows []dbq.Download, r DownloadRequest) (dbq.Download, bool) {
	for _, d := range rows {
		var asked DownloadRequest
		if err := json.Unmarshal([]byte(d.Request), &asked); err == nil && asked.asksFor(r) {
			return d, true
		}
	}
	return dbq.Download{}, false
}

// Request asks for a download of a work for an app. It's decided the same
// way as streaming, but always as whole files. The app gets the original
// when it plays the original and the original is no larger than the
// quality's estimate. Otherwise it gets a conversion to MP4. A download
// that needs work on the server (a conversion, or a subtitle to prepare)
// is queued, and the queue survives restarts. Other downloads are ready at
// once. Asking again for the same download, as an app does when an answer
// was lost or its user tapped twice, returns the same download, however
// close together the asks arrive.
func (s *Downloads) Request(ctx context.Context, p auth.Caller, v users.Viewer, r DownloadRequest) (DownloadStatus, error) {
	if p.Session.IsZero() {
		return DownloadStatus{}, ErrNotFound // a download belongs to a session, and an API key has none
	}
	user, err := s.downloader(ctx, v, p.User, r.Work)
	if err != nil {
		return DownloadStatus{}, err
	}
	request, err := json.Marshal(r)
	if err != nil {
		return DownloadStatus{}, err
	}
	rows, err := s.q.SameDownload(ctx, dbq.SameDownloadParams{SessionID: p.Session, WorkID: r.Work})
	if err != nil {
		return DownloadStatus{}, err
	}
	if same, ok := sameDownload(rows, r); ok {
		return s.status(ctx, same)
	}
	c, planned, err := s.planAsked(ctx, user, r)
	if err != nil {
		return DownloadStatus{}, err
	}
	now := time.Now()
	row := dbq.Download{
		ID: id.New(), UserID: p.User, SessionID: p.Session, WorkID: r.Work, VersionID: c.version.ID, Request: string(request), Delivered: planned,
		State: string(DownloadReady), CreatedAt: now.UnixMilli(), QueuedAt: now.UnixMilli(),
	}
	prepare := s.needsPreparing(ctx, c)
	if prepare {
		row.State = string(DownloadQueued)
	} else {
		row.SettledAt = sql.NullInt64{Int64: now.UnixMilli(), Valid: true}
	}
	stored, inserted, err := s.store(ctx, row, r, prepare)
	if err != nil {
		return DownloadStatus{}, err
	}
	if !inserted {
		return s.status(ctx, stored)
	}
	slog.InfoContext(ctx, "playback: download", "work", r.Work, "version", c.version.ID, "client", p.Name, "quality", c.input.Quality.Name,
		"delivery", c.decision.Delivery, "summary", c.decision.Summary(), "prepare", prepare)
	if prepare {
		s.changed(p.User)
	}
	return s.status(ctx, row)
}

// store stores a new download, and asks for its preparation when
// it needs it, unless the same download was stored while this one was
// planned. Writes take turns, so the first stored is the one, and it's
// returned instead.
func (s *Downloads) store(ctx context.Context, row dbq.Download, r DownloadRequest, prepare bool) (stored dbq.Download, inserted bool, err error) {
	stored = row
	err = s.db.Write(ctx, func(ctx context.Context, tx *store.Tx) error {
		q := dbq.New(tx)
		rows, err := q.SameDownload(ctx, dbq.SameDownloadParams{SessionID: row.SessionID, WorkID: row.WorkID})
		if err != nil {
			return err
		}
		if same, ok := sameDownload(rows, r); ok {
			stored = same
			return nil
		}
		if err := q.InsertDownload(ctx, dbq.InsertDownloadParams{
			ID: row.ID, UserID: row.UserID, SessionID: row.SessionID, WorkID: row.WorkID, VersionID: row.VersionID, Request: row.Request, Delivered: row.Delivered,
			State: row.State, CreatedAt: row.CreatedAt, QueuedAt: row.QueuedAt, SettledAt: row.SettledAt,
		}); err != nil {
			return err
		}
		if prepare {
			s.enqueue(tx, row.ID)
		}
		inserted = true
		return nil
	})
	return stored, inserted, err
}

// enqueue asks for a download's preparation once tx commits.
func (s *Downloads) enqueue(tx *store.Tx, download id.ID) {
	tx.AfterCommit(func() { s.deps.Tasks.Enqueue(TaskPrepareDownloads, download) })
}

// Retry asks again for a download that failed. It decides the
// download again, since the files may have changed, and puts it at the
// back of the line. Any other download is returned unchanged.
func (s *Downloads) Retry(ctx context.Context, p auth.Caller, v users.Viewer, download id.ID) (DownloadStatus, error) {
	d, err := s.own(ctx, p.User, download)
	if err != nil {
		return DownloadStatus{}, err
	}
	if DownloadState(d.State) != DownloadFailed {
		return s.status(ctx, d)
	}
	var r DownloadRequest
	if err := json.Unmarshal([]byte(d.Request), &r); err != nil {
		return DownloadStatus{}, err
	}
	user, err := s.downloader(ctx, v, p.User, r.Work)
	if err != nil {
		return DownloadStatus{}, err
	}
	c, planned, err := s.planAsked(ctx, user, r)
	if err != nil {
		return DownloadStatus{}, err
	}
	now := time.Now().UnixMilli()
	was := d.State
	d.VersionID, d.Delivered, d.QueuedAt, d.State, d.SettledAt = c.version.ID, planned, now, string(DownloadReady), sql.NullInt64{Int64: now, Valid: true}
	prepare := s.needsPreparing(ctx, c)
	if prepare {
		d.State, d.SettledAt = string(DownloadQueued), sql.NullInt64{}
	}
	d.Reason = sql.NullString{}
	err = s.db.Write(ctx, func(ctx context.Context, tx *store.Tx) error {
		n, err := dbq.New(tx).ReplanDownload(ctx, dbq.ReplanDownloadParams{
			ID: d.ID, Was: was, VersionID: d.VersionID, Delivered: d.Delivered, State: d.State, QueuedAt: d.QueuedAt, SettledAt: d.SettledAt,
		})
		if err == nil && n > 0 && prepare {
			s.enqueue(tx, d.ID)
		}
		return err
	})
	if err != nil {
		return DownloadStatus{}, err
	}
	slog.InfoContext(ctx, "playback: download retried", "work", d.WorkID, "delivery", c.decision.Delivery)
	if prepare {
		s.changed(p.User)
	}
	return s.status(ctx, d)
}
