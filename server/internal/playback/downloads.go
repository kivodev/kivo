package playback

import (
	"cmp"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"slices"
	"strconv"
	"time"

	"github.com/mmcdole/kivo/internal/auth"
	"github.com/mmcdole/kivo/internal/catalog"
	"github.com/mmcdole/kivo/internal/config"
	"github.com/mmcdole/kivo/internal/events"
	"github.com/mmcdole/kivo/internal/id"
	"github.com/mmcdole/kivo/internal/markers"
	"github.com/mmcdole/kivo/internal/media"
	"github.com/mmcdole/kivo/internal/playback/dbq"
	"github.com/mmcdole/kivo/internal/store"
	"github.com/mmcdole/kivo/internal/tasks"
	"github.com/mmcdole/kivo/internal/transcode"
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

// DownloadState is where a download is on the server.
type DownloadState string

const (
	DownloadQueued    DownloadState = "queued"    // waiting for its turn
	DownloadPreparing DownloadState = "preparing" // its subtitle or conversion is being made
	DownloadReady     DownloadState = "ready"     // its files can be fetched
	DownloadFailed    DownloadState = "failed"    // failed with a reason; Retry asks for it again
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

// DownloadStatus is a download as its app sees it.
type DownloadStatus struct {
	ID, Work, Version id.ID
	Quality           string
	State             DownloadState
	Position          int           // when queued, its place in line; 1 means it's next
	Progress          float64       // when preparing, a fraction from 0 to 1
	Remaining         time.Duration // when preparing, about how long is left; zero until known
	Reason            Refusal       // when failed, why
	// Decision says how each choice is in the download: a track inside its
	// file, with its stream index there, or its own file fetched with it.
	Decision Decision
	Duration time.Duration
	Parts    []DownloadPart
	// Size is the total bytes of every part. It's exact (SizeExact) once
	// the download is ready, and for an original sent as it is. Until a
	// conversion is ready, it's an estimate from the quality's cap.
	Size              int64
	SizeExact         bool
	Chapters          []catalog.Chapter
	Markers           []markers.Marker
	ExternalSubtitles []ExternalSubtitle // the subtitle tracks it delivers as files
	Expires           time.Time          // once ready or failed, when the server deletes its copy
}

// DownloadPart is one of a download's files. Its size and validators are
// known once the download is ready.
type DownloadPart struct {
	Duration time.Duration
	Type     string // the content type the file is served with
	Size     int64
	ETag     string // a strong ETag, for resuming by range
	SHA256   string // a conversion's hash, to check what was fetched
}

// preparation is a download this process is preparing. Its fields are
// guarded by the service's mu.
type preparation struct {
	user     id.ID
	started  time.Time     // when the conversion began; zero before that
	reached  time.Duration // how far the conversion has got
	total    time.Duration
	cancel   context.CancelFunc
	removed  bool // the app removed the download, so cancelling isn't a failure
	notified time.Time
}

// Download asks for a download of a work for an app. It's decided the same
// way as streaming, but always as whole files. The app gets the original
// when it plays the original and the original is no larger than the
// quality's estimate. Otherwise it gets a conversion to MP4. A download
// that needs work on the server (a conversion, or a subtitle to prepare)
// is queued, and the queue survives restarts. Other downloads are ready at
// once. Asking again for the same download, as an app does when an answer
// was lost or its user tapped twice, returns the same download, however
// close together the asks arrive.
func (s *Service) Download(ctx context.Context, p auth.Caller, v users.Viewer, r DownloadRequest) (DownloadStatus, error) {
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
	same, err := s.q.SameDownload(ctx, dbq.SameDownloadParams{SessionID: p.Session, WorkID: r.Work, Request: string(request)})
	switch {
	case err == nil:
		return s.status(ctx, same)
	case !errors.Is(err, sql.ErrNoRows):
		return DownloadStatus{}, err
	}
	c, err := s.planDownload(ctx, user, r)
	if err != nil {
		return DownloadStatus{}, err
	}
	plan, err := json.Marshal(planOf(c))
	if err != nil {
		return DownloadStatus{}, err
	}
	now := time.Now()
	row := dbq.Download{
		ID: id.New(), UserID: p.User, SessionID: p.Session, WorkID: r.Work, VersionID: c.version.ID, Request: string(request), Plan: string(plan),
		State: string(DownloadReady), CreatedAt: now.UnixMilli(), QueuedAt: now.UnixMilli(),
	}
	prepare := s.needsPreparing(ctx, c)
	if prepare {
		row.State = string(DownloadQueued)
	} else {
		row.SettledAt = sql.NullInt64{Int64: now.UnixMilli(), Valid: true}
	}
	stored, inserted, err := s.storeDownload(ctx, row, prepare)
	if err != nil {
		return DownloadStatus{}, err
	}
	if !inserted {
		return s.status(ctx, stored)
	}
	slog.InfoContext(ctx, "playback: download", "work", r.Work, "version", c.version.ID, "client", p.Name, "quality", c.input.Quality.Name,
		"delivery", c.decision.Delivery, "summary", c.decision.Summary(), "prepare", prepare)
	if prepare {
		s.queueMoved(ctx)
	}
	return s.status(ctx, row)
}

// storeDownload stores a new download, and asks for its preparation when
// it needs it, unless the same download was stored while this one was
// planned. Writes take turns, so the first stored is the one, and it's
// returned instead.
func (s *Service) storeDownload(ctx context.Context, row dbq.Download, prepare bool) (stored dbq.Download, inserted bool, err error) {
	stored = row
	err = s.db.Write(ctx, func(ctx context.Context, tx *store.Tx) error {
		q := dbq.New(tx)
		same, err := q.SameDownload(ctx, dbq.SameDownloadParams{SessionID: row.SessionID, WorkID: row.WorkID, Request: row.Request})
		if err == nil {
			stored = same
			return nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if err := q.InsertDownload(ctx, dbq.InsertDownloadParams{
			ID: row.ID, UserID: row.UserID, SessionID: row.SessionID, WorkID: row.WorkID, VersionID: row.VersionID, Request: row.Request, Plan: row.Plan,
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

// downloader checks that a viewer may download a work. The viewer must see
// the work, and the user must be allowed downloads.
func (s *Service) downloader(ctx context.Context, v users.Viewer, user, work id.ID) (users.User, error) {
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

// enqueue asks for a download's preparation once tx commits.
func (s *Service) enqueue(tx *store.Tx, download id.ID) {
	tx.AfterCommit(func() { s.deps.Tasks.Ask(TaskPrepareDownloads, download) })
}

// Tasks returns playback's tasks, for cmd/kivo to add. Downloads are
// prepared in the order they were asked for, at low OS priority, two at a
// time when an accelerator encodes video and one at a time on the CPU.
// They don't count against the server's video transcode limit. Expired
// downloads are removed each night.
func (s *Service) Tasks() []tasks.Task {
	return []tasks.Task{
		{Name: TaskPrepareDownloads, Finish: true, Limit: s.preparingAtOnce, Pending: s.q.OpenDownloads, Do: s.prepareDownload},
		{Name: taskPruneDownloads, Nightly: true, Maintenance: true, Run: s.PruneDownloads},
	}
}

// preparingAtOnce is how many downloads are prepared at once.
func (s *Service) preparingAtOnce(config.Settings) int {
	if accel, _ := s.accelerators(false); len(accel) > 0 {
		return 2
	}
	return 1
}

// prepareDownload prepares one download. It extracts the subtitles the
// download needs from its file, then converts the download. If the
// accelerator fails before producing any output, the conversion runs
// again on the CPU. Finally it settles the download as ready, or as
// failed with the reason. A file that can't be read is tried again on
// later passes before the download fails. A download whose plan can't be
// read fails at once. After a server restart, the download waits in line
// and starts again.
func (s *Service) prepareDownload(ctx context.Context, download id.ID) error {
	d, err := s.q.GetDownload(ctx, download)
	if errors.Is(err, sql.ErrNoRows) {
		return nil // the app removed the download
	}
	if err != nil {
		return err
	}
	if state := DownloadState(d.State); state != DownloadQueued && state != DownloadPreparing {
		return nil
	}
	c, err := s.restore(ctx, d)
	if why := unrestorable(err); why != "" {
		return s.settle(ctx, d, nil, why, err)
	}
	if err != nil {
		return err
	}
	for _, f := range c.files {
		if !f.Present {
			return s.settle(ctx, d, nil, RefuseSourceUnreadable, fmt.Errorf("%s is missing", f.RelPath))
		}
	}
	if err := s.db.Write(ctx, func(ctx context.Context, tx *store.Tx) error {
		_, err := dbq.New(tx).StartDownload(ctx, d.ID)
		return err
	}); err != nil {
		return err
	}
	s.downloadProgress(ctx, "")
	pctx, cancel := context.WithCancel(ctx)
	defer cancel()
	p := &preparation{user: d.UserID, cancel: cancel, total: duration(c.files)}
	s.mu.Lock()
	s.preparing[d.ID] = p
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		delete(s.preparing, d.ID)
		s.mu.Unlock()
		s.queueMoved(context.WithoutCancel(ctx))
	}()
	s.queueMoved(ctx)

	files, why, err := s.prepareFiles(pctx, d, c, p)
	s.mu.Lock()
	removed := p.removed
	s.mu.Unlock()
	switch {
	case removed:
		return nil
	case errors.Is(ctx.Err(), context.Canceled):
		// The server is stopping. The download is prepared again after a
		// restart.
		s.requeue(context.WithoutCancel(ctx), d.ID)
		return ctx.Err()
	case errors.Is(err, media.ErrSourceUnreadable) && s.attempt(d.ID) < downloadAttempts:
		slog.WarnContext(ctx, "playback: download's file unreadable; trying again later", "work", d.WorkID, "err", err)
		s.requeue(ctx, d.ID)
		return err
	}
	s.mu.Lock()
	delete(s.attempts, d.ID)
	s.mu.Unlock()
	err = s.settle(context.WithoutCancel(ctx), d, files, why, err)
	s.preparedOne(context.WithoutCancel(ctx))
	return err
}

// StepConvert is the step a download's preparation reports while its
// conversion is made.
const StepConvert = "convert"

// downloadProgress reports the Downloads task's step and count: the
// downloads prepared since none were waiting, of those and the ones
// waiting now, as other tasks count their items. ctx must be the task's.
func (s *Service) downloadProgress(ctx context.Context, step string) {
	open, err := s.q.OpenDownloads(ctx)
	if err != nil {
		return
	}
	s.mu.Lock()
	done := s.downloadsPrepared
	s.mu.Unlock()
	tasks.Progress(ctx, step, done, done+len(open))
}

// preparedOne counts a download prepared, or starts the count again once
// none are waiting.
func (s *Service) preparedOne(ctx context.Context) {
	open, err := s.q.OpenDownloads(ctx)
	s.mu.Lock()
	defer s.mu.Unlock()
	if err == nil && len(open) == 0 {
		s.downloadsPrepared = 0
	} else {
		s.downloadsPrepared++
	}
}

// unrestorable returns why a download that restore failed on should fail
// at once: its plan can't be read, or its files are gone. For any other
// error it returns "", and the download stays pending.
func unrestorable(err error) Refusal {
	switch {
	case errors.Is(err, errPlanUnreadable):
		return RefusePlanUnreadable
	case errors.Is(err, catalog.ErrNotFound):
		return RefuseSourceUnreadable
	}
	return ""
}

// attempt counts one more attempt at a download whose file couldn't be
// read, and returns how many there have been.
func (s *Service) attempt(download id.ID) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.attempts[download]++
	return s.attempts[download]
}

// prepareFiles extracts a download's subtitles when it needs them, then
// converts it if its decision says so. It returns the converted files, or
// why it failed.
func (s *Service) prepareFiles(ctx context.Context, d dbq.Download, c *choice, p *preparation) ([]transcode.ConvertedFile, Refusal, error) {
	if s.subtitlesToExtract(ctx, c) {
		if err := s.extractSubtitles(ctx, c); err != nil {
			return nil, RefuseSubtitlesUnavailable, err
		}
	}
	if c.decision.Delivery != DeliverConversion {
		return nil, "", nil
	}
	s.mu.Lock()
	p.started = time.Now()
	s.mu.Unlock()
	s.downloadProgress(ctx, StepConvert)
	reached := func(at time.Duration) {
		s.mu.Lock()
		p.reached = at
		tell := time.Since(p.notified) >= progressEvery
		if tell {
			p.notified = time.Now()
		}
		s.mu.Unlock()
		if tell {
			s.downloadsChanged(p.user)
		}
	}
	var files []transcode.ConvertedFile
	err := s.transcodeWithFallback(ctx, c, func(spec transcode.Spec) error {
		var err error
		files, err = s.deps.Transcoder.Download(ctx, d.ID.String(), spec, reached)
		return err
	})
	switch {
	case errors.Is(err, transcode.ErrStorageFull):
		return nil, RefuseStorageFull, err
	case errors.Is(err, media.ErrSourceUnreadable):
		return nil, RefuseSourceUnreadable, err
	case err != nil:
		return nil, RefuseTranscodeFailed, err
	}
	return files, "", nil
}

// StepExtractSubtitles is the step a download's preparation reports while
// its file's subtitles are extracted.
const StepExtractSubtitles = "extractSubtitles"

// errSubtitleUnprepared means the extraction of a download's file didn't
// make the download's subtitle.
var errSubtitleUnprepared = errors.New("subtitle wasn't prepared")

// extractSubtitles has the subtitles package prepare a download's file,
// then reads its styled subtitles again. If the download's own subtitle
// still isn't prepared, the download fails. Other tracks it delivers as
// separate files are left out if they're missing.
func (s *Service) extractSubtitles(ctx context.Context, c *choice) error {
	s.downloadProgress(ctx, StepExtractSubtitles)
	f := c.files[0]
	err := s.deps.Subtitles.Prepare(ctx, f.ID)
	if ctx.Err() != nil {
		return ctx.Err()
	}
	c.styled = s.styledOf(ctx, f)
	missing := s.subtitleToExtract(ctx, c)
	switch {
	case missing && err != nil:
		return err
	case missing:
		return fmt.Errorf("%w: track %d of %s", errSubtitleUnprepared, c.input.Source.Subtitle.Index, f.ID)
	case err != nil:
		slog.WarnContext(ctx, "playback: download's subtitles weren't extracted; going without them", "file", f.ID, "err", err)
	}
	return nil
}

// requeue puts a download that stopped partway back in line.
func (s *Service) requeue(ctx context.Context, download id.ID) {
	err := s.db.Write(ctx, func(ctx context.Context, tx *store.Tx) error { return dbq.New(tx).RequeueDownload(ctx, download) })
	if err != nil {
		slog.WarnContext(ctx, "playback: requeue download", "err", err)
	}
}

// settle records a download as ready with its converted files, or as
// failed with the reason. If the app removed the download in the
// meantime, the converted files are deleted instead.
func (s *Service) settle(ctx context.Context, d dbq.Download, files []transcode.ConvertedFile, why Refusal, cause error) error {
	state, reason, kept := DownloadReady, sql.NullString{}, sql.NullString{}
	if cause != nil {
		state, reason = DownloadFailed, sql.NullString{String: string(why), Valid: true}
		slog.WarnContext(ctx, "playback: download failed", "work", d.WorkID, "reason", why, "err", cause)
	} else if files != nil {
		b, err := json.Marshal(files)
		if err != nil {
			return err
		}
		kept = sql.NullString{String: string(b), Valid: true}
		slog.InfoContext(ctx, "playback: download ready", "work", d.WorkID, "took", time.Since(time.UnixMilli(d.QueuedAt)).Round(time.Second))
	}
	var n int64
	err := s.db.Write(ctx, func(ctx context.Context, tx *store.Tx) error {
		var err error
		n, err = dbq.New(tx).SettleDownload(ctx, dbq.SettleDownloadParams{
			ID: d.ID, State: string(state), Reason: reason, Files: kept, SettledAt: sql.NullInt64{Int64: time.Now().UnixMilli(), Valid: true},
		})
		return err
	})
	if err != nil {
		return err
	}
	if n == 0 && files != nil {
		return s.deps.Transcoder.DeleteDownload(d.ID.String())
	}
	s.downloadsChanged(d.UserID)
	return nil
}

// queueMoved tells every user with downloads in line that their places
// may have changed.
func (s *Service) queueMoved(ctx context.Context) {
	queued, err := s.q.QueuedUsers(ctx)
	if err != nil {
		slog.WarnContext(ctx, "playback: downloads in line", "err", err)
		return
	}
	for _, u := range queued {
		s.downloadsChanged(u)
	}
}

// downloadsChanged tells a user's clients that their downloads changed.
func (s *Service) downloadsChanged(user id.ID) {
	s.deps.Events.Publish(events.Event{Kind: events.DownloadsChanged, Scope: events.Scope{User: user}})
}

// Downloads lists the downloads the server keeps for the caller's session,
// newest first. An app reattaches to them after a relaunch or a server
// restart. An API key has none.
func (s *Service) Downloads(ctx context.Context, p auth.Caller) ([]DownloadStatus, error) {
	rows, err := s.q.SessionDownloads(ctx, p.Session)
	if err != nil {
		return nil, err
	}
	out := make([]DownloadStatus, 0, len(rows))
	for _, d := range rows {
		s, err := s.status(ctx, d)
		if errors.Is(err, catalog.ErrNotFound) {
			continue // the files are gone; the nightly prune and the availability check handle it
		}
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, nil
}

// DownloadStatus returns one of the user's downloads.
func (s *Service) DownloadStatus(ctx context.Context, p auth.Caller, download id.ID) (DownloadStatus, error) {
	d, err := s.ownDownload(ctx, p.User, download)
	if err != nil {
		return DownloadStatus{}, err
	}
	return s.status(ctx, d)
}

// ownDownload returns one of the user's downloads. Anyone else's download
// returns ErrNotFound, the same as a missing one.
func (s *Service) ownDownload(ctx context.Context, user, download id.ID) (dbq.Download, error) {
	d, err := s.q.GetDownload(ctx, download)
	if errors.Is(err, sql.ErrNoRows) || err == nil && d.UserID != user {
		return dbq.Download{}, fmt.Errorf("%w: download %s", ErrNotFound, download)
	}
	if err != nil {
		return dbq.Download{}, err
	}
	return d, nil
}

// downloaded is a download's decision as its app plays it offline. A
// conversion's tracks are inside its file, the video first, then the
// audio, then each text track it carries, so each has its stream index
// there. Once the download is ready, a subtitle delivered external that
// couldn't be extracted isn't in it.
func (s *Service) downloaded(ctx context.Context, c *choice, externals []ExternalSubtitle, ready bool) Decision {
	d := c.decision
	d.Tracks = slices.Clone(d.Tracks)
	next := 0
	inFile := func(t *TrackDecision) {
		t.Delivery, t.Index = DeliverFile, next
		next++
	}
	if d.Delivery == DeliverConversion {
		for _, kind := range []media.TrackKind{media.Video, media.Audio} {
			if i := slices.IndexFunc(d.Tracks, func(t TrackDecision) bool { return t.Type == kind && t.Selected }); i >= 0 {
				inFile(&d.Tracks[i])
			}
		}
		for _, text := range s.textTracks(ctx, c) {
			if i := slices.IndexFunc(d.Tracks, func(t TrackDecision) bool { return t.Track.String() == text.ID }); i >= 0 {
				inFile(&d.Tracks[i])
				d.Tracks[i].Codec = "mov_text"
			}
		}
	}
	for i, t := range d.Tracks {
		if t.Delivery == DeliverExternal && ready && !slices.ContainsFunc(externals, func(x ExternalSubtitle) bool { return x.Track == t.Track && x.Ready }) {
			d.Tracks[i] = TrackDecision{Type: t.Type, Track: t.Track, Index: t.Index, Selected: t.Selected, Reason: ReasonSubtitlePreparing}
		}
	}
	return d
}

func (s *Service) status(ctx context.Context, d dbq.Download) (DownloadStatus, error) {
	c, err := s.restore(ctx, d)
	if errors.Is(err, errPlanUnreadable) {
		return unreadableStatus(d), nil
	}
	if err != nil {
		return DownloadStatus{}, err
	}
	out := DownloadStatus{
		ID: d.ID, Work: d.WorkID, Version: d.VersionID, Quality: c.input.Quality.Name, State: DownloadState(d.State),
		ExternalSubtitles: s.externalSubtitles(ctx, c),
	}
	out.Decision = s.downloaded(ctx, c, out.ExternalSubtitles, out.State == DownloadReady)
	if out.Quality == "" {
		out.Quality = "original"
	}
	if d.Reason.Valid {
		out.Reason = Refusal(d.Reason.String)
	}
	if d.SettledAt.Valid {
		out.Expires = time.UnixMilli(d.SettledAt.Int64).Add(keepDownload)
	}
	out.Chapters, out.Markers = s.deps.Markers.Markers(ctx, c.work, c.files)
	var converted []transcode.ConvertedFile
	if d.Files.Valid {
		if err := json.Unmarshal([]byte(d.Files.String), &converted); err != nil {
			return DownloadStatus{}, err
		}
	}
	out.Duration = duration(c.files)
	for i, f := range c.files {
		part := DownloadPart{Duration: f.Probe.Duration, Type: "video/mp4"}
		switch {
		case c.decision.Delivery == DeliverFile:
			part.Size, part.Type = f.Size, mimeType(f.Probe.Container, f.RelPath)
			if fi, err := os.Stat(f.Path()); err == nil {
				part.Size, part.ETag = fi.Size(), fileETag(fi)
			}
		case i < len(converted):
			cf := converted[i]
			part.Size, part.ETag, part.SHA256 = cf.Size, `"`+cf.SHA256+`"`, cf.SHA256
		}
		out.Size += part.Size
		out.Parts = append(out.Parts, part)
	}
	out.SizeExact = out.State == DownloadReady || c.decision.Delivery == DeliverFile
	if out.State != DownloadReady {
		out.Size = estimateBytes(c.input, c.decision, c.files)
	}
	if out.State == DownloadQueued || out.State == DownloadPreparing {
		s.mu.Lock()
		p := s.preparing[d.ID]
		if p != nil {
			out.State = DownloadPreparing
			out.Progress, out.Remaining = p.progressLocked(time.Now())
		}
		s.mu.Unlock()
		if p == nil {
			// This process isn't preparing it, so it's in line, or back in
			// line after a restart.
			ahead, err := s.q.QueueAhead(ctx, dbq.QueueAheadParams{QueuedAt: d.QueuedAt, ID: d.ID})
			if err != nil {
				return DownloadStatus{}, err
			}
			out.State, out.Position = DownloadQueued, int(ahead)+1
		}
	}
	return out, nil
}

// unreadableStatus builds the status of a download whose plan can't be
// read, from its row alone. It's always failed, whatever the stored state,
// so the app offers Retry.
func unreadableStatus(d dbq.Download) DownloadStatus {
	var r DownloadRequest
	_ = json.Unmarshal([]byte(d.Request), &r) // an unreadable request shows the default quality
	out := DownloadStatus{
		// Migration (downloads): remove after every client has updated past this release.
		ID: d.ID, Work: d.WorkID, Version: d.VersionID, Quality: currentQualityName(cmp.Or(r.Quality, DefaultDownloadQuality)),
		State: DownloadFailed, Reason: RefusePlanUnreadable,
	}
	if d.SettledAt.Valid {
		out.Expires = time.UnixMilli(d.SettledAt.Int64).Add(keepDownload)
	}
	return out
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

// fileETag is an original file's strong ETag, built from its current size
// and modification time on disk. A file replaced since it was scanned is
// then fetched again rather than resumed.
func fileETag(fi os.FileInfo) string {
	return `"` + strconv.FormatInt(fi.Size(), 10) + "-" + strconv.FormatInt(fi.ModTime().UnixNano(), 36) + `"`
}

// RetryDownload asks again for a download that failed or whose plan can't
// be read. It decides the download again, since the files may have
// changed, and puts it at the back of the line. Any other download is
// returned unchanged.
func (s *Service) RetryDownload(ctx context.Context, p auth.Caller, v users.Viewer, download id.ID) (DownloadStatus, error) {
	d, err := s.ownDownload(ctx, p.User, download)
	if err != nil {
		return DownloadStatus{}, err
	}
	if _, err := readPlan(d); err == nil && DownloadState(d.State) != DownloadFailed {
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
	c, err := s.planDownload(ctx, user, r)
	if err != nil {
		return DownloadStatus{}, err
	}
	plan, err := json.Marshal(planOf(c))
	if err != nil {
		return DownloadStatus{}, err
	}
	now := time.Now().UnixMilli()
	was := d.State
	d.VersionID, d.Plan, d.QueuedAt, d.State, d.SettledAt = c.version.ID, string(plan), now, string(DownloadReady), sql.NullInt64{Int64: now, Valid: true}
	prepare := s.needsPreparing(ctx, c)
	if prepare {
		d.State, d.SettledAt = string(DownloadQueued), sql.NullInt64{}
	}
	d.Reason, d.Files = sql.NullString{}, sql.NullString{}
	err = s.db.Write(ctx, func(ctx context.Context, tx *store.Tx) error {
		n, err := dbq.New(tx).ReplanDownload(ctx, dbq.ReplanDownloadParams{
			ID: d.ID, Was: was, VersionID: d.VersionID, Plan: d.Plan, State: d.State, QueuedAt: d.QueuedAt, SettledAt: d.SettledAt,
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
		s.queueMoved(ctx)
	}
	return s.status(ctx, d)
}

// SubtitlesReplaced listens for the subtitles package replacing tracks.
// In tx, it moves every download that chose or was planned with a
// replaced track to the new track, so the download's status and a retry
// name a track that exists.
func (s *Service) SubtitlesReplaced(ctx context.Context, tx *store.Tx, replaced []id.ID, by id.ID) error {
	for _, old := range replaced {
		if _, err := dbq.New(tx).MoveDownloadSubtitle(ctx, dbq.MoveDownloadSubtitleParams{FromTrack: old.String(), ToTrack: by.String()}); err != nil {
			return fmt.Errorf("move downloads off subtitle %s: %w", old, err)
		}
	}
	return nil
}

// DeleteDownload deletes one of the user's downloads on the server, once
// its app has every file or no longer wants it. Its preparation stops,
// and its conversion is deleted.
func (s *Service) DeleteDownload(ctx context.Context, p auth.Caller, download id.ID) error {
	d, err := s.ownDownload(ctx, p.User, download)
	if err != nil {
		return err
	}
	if err := s.db.Write(ctx, func(ctx context.Context, tx *store.Tx) error {
		_, err := dbq.New(tx).DeleteDownload(ctx, d.ID)
		return err
	}); err != nil {
		return err
	}
	s.mu.Lock()
	if prep := s.preparing[d.ID]; prep != nil {
		prep.removed = true
		prep.cancel()
	}
	s.mu.Unlock()
	if err := s.deps.Transcoder.DeleteDownload(d.ID.String()); err != nil {
		return err
	}
	if state := DownloadState(d.State); state == DownloadQueued || state == DownloadPreparing {
		s.queueMoved(ctx)
	}
	return nil
}

// DownloadedFile returns a part of one of the user's downloads, as long as
// the user may still have it (see Available). An original's part is
// returned at once. A conversion's part is returned once it's ready, and
// ErrNotReady before then.
func (s *Service) DownloadedFile(ctx context.Context, v users.Viewer, download id.ID, part int) (Media, error) {
	d, c, err := s.servable(ctx, v, download)
	if err != nil {
		return Media{}, err
	}
	if part < 0 || part >= len(c.files) {
		return Media{}, fmt.Errorf("%w: part %d of download %s", ErrNotFound, part, download)
	}
	if c.decision.Delivery == DeliverFile {
		f := c.files[part]
		fi, err := os.Stat(f.Path())
		if err != nil {
			return Media{}, err
		}
		return Media{Path: f.Path(), Type: mimeType(f.Probe.Container, f.RelPath), ETag: fileETag(fi)}, nil
	}
	if DownloadState(d.State) != DownloadReady || !d.Files.Valid {
		return Media{}, fmt.Errorf("%w: download %s", ErrNotReady, download)
	}
	var converted []transcode.ConvertedFile
	if err := json.Unmarshal([]byte(d.Files.String), &converted); err != nil {
		return Media{}, fmt.Errorf("%w: download %s: %w", ErrNotFound, download, err)
	}
	if part >= len(converted) {
		return Media{}, fmt.Errorf("%w: part %d of download %s", ErrNotFound, part, download)
	}
	cf := converted[part]
	return Media{Path: cf.Path, Type: "video/mp4", ETag: `"` + cf.SHA256 + `"`}, nil
}

// DownloadSubtitle returns a subtitle of one of the user's downloads by
// the name in its delivery, the same way Subtitle does for a play.
func (s *Service) DownloadSubtitle(ctx context.Context, v users.Viewer, download id.ID, name string) (Media, error) {
	_, c, err := s.servable(ctx, v, download)
	if err != nil {
		return Media{}, err
	}
	return s.subtitleFile(ctx, c.files[0], name)
}

// servable returns one of the viewer's downloads and its plan, as long as
// the user may still have it. A download whose plan can't be read returns
// ErrNotReady.
func (s *Service) servable(ctx context.Context, v users.Viewer, download id.ID) (dbq.Download, *choice, error) {
	d, err := s.ownDownload(ctx, v.User(), download)
	if err != nil {
		return d, nil, err
	}
	if _, err := s.downloader(ctx, v, v.User(), d.WorkID); err != nil {
		return d, nil, fmt.Errorf("%w: download %s", ErrNotFound, download)
	}
	c, err := s.restore(ctx, d)
	switch {
	case errors.Is(err, errPlanUnreadable):
		return d, nil, fmt.Errorf("%w: download %s", ErrNotReady, download)
	case errors.Is(err, catalog.ErrNotFound):
		return d, nil, fmt.Errorf("%w: download %s", ErrNotFound, download)
	case err != nil:
		return d, nil, err
	}
	return d, c, nil
}

// PruneDownloads deletes downloads that became ready or failed more than
// keepDownload ago, with their conversions. It also deletes any conversion
// that has no download. It's the nightly "Clean downloads" task.
func (s *Service) PruneDownloads(ctx context.Context) error {
	before := time.Now().Add(-keepDownload).UnixMilli()
	removed := 0
	for {
		expired, err := s.q.ExpiredDownloads(ctx, dbq.ExpiredDownloadsParams{Before: sql.NullInt64{Int64: before, Valid: true}, Limit: store.BatchSize})
		if err != nil {
			return err
		}
		if len(expired) == 0 {
			break
		}
		if err := s.db.Write(ctx, func(ctx context.Context, tx *store.Tx) error {
			for _, d := range expired {
				if _, err := dbq.New(tx).DeleteDownload(ctx, d); err != nil {
					return err
				}
			}
			return nil
		}); err != nil {
			return err
		}
		for _, d := range expired {
			if err := s.deps.Transcoder.DeleteDownload(d.String()); err != nil {
				return err
			}
		}
		removed += len(expired)
	}
	names, err := s.deps.Transcoder.Downloads()
	if err != nil {
		return err
	}
	var kept []id.ID
	for _, n := range names {
		if d, err := id.Parse(n); err == nil {
			kept = append(kept, d)
		}
	}
	known, err := s.q.DownloadsAmong(ctx, id.Set(kept...))
	if err != nil {
		return err
	}
	for _, n := range names {
		if d, err := id.Parse(n); err == nil && slices.Contains(known, d) {
			continue
		}
		if err := s.deps.Transcoder.DeleteDownload(n); err != nil {
			return err
		}
		removed++
	}
	if removed > 0 {
		slog.InfoContext(ctx, "playback: removed expired downloads", "count", removed)
	}
	return nil
}

// KeptVersion is a version of a work a client keeps downloaded, for the
// availability check.
type KeptVersion struct{ Work, Version id.ID }

// Available says whether the user may still have each download. The user
// must be allowed downloads, the work must be visible to them, and the
// version must still exist as a version of that work. A missing work and
// a hidden work give the same answer.
func (s *Service) Available(ctx context.Context, v users.Viewer, downloads []KeptVersion) ([]bool, error) {
	out := make([]bool, len(downloads))
	user, err := s.deps.Users.User(ctx, v.User())
	if err != nil {
		return nil, err
	}
	if !user.Downloads {
		return out, nil
	}
	for i, d := range downloads {
		ok, err := s.visible(ctx, v, d.Work)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		ver, err := s.deps.Catalog.Version(ctx, d.Version)
		if errors.Is(err, catalog.ErrNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		out[i] = slices.Contains(ver.Works, d.Work)
	}
	return out, nil
}
