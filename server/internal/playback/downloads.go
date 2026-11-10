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
	"strings"
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

// DownloadStatus is a download as its app sees it.
type DownloadStatus struct {
	ID, Work, Version id.ID
	Quality           string
	State             DownloadState
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
	stored, inserted, err := s.storeDownload(ctx, row, r, prepare)
	if err != nil {
		return DownloadStatus{}, err
	}
	if !inserted {
		return s.status(ctx, stored)
	}
	slog.InfoContext(ctx, "playback: download", "work", r.Work, "version", c.version.ID, "client", p.Name, "quality", c.input.Quality.Name,
		"delivery", c.decision.Delivery, "summary", c.decision.Summary(), "prepare", prepare)
	if prepare {
		s.downloadsChanged(p.User)
	}
	return s.status(ctx, row)
}

// storeDownload stores a new download, and asks for its preparation when
// it needs it, unless the same download was stored while this one was
// planned. Writes take turns, so the first stored is the one, and it's
// returned instead.
func (s *Service) storeDownload(ctx context.Context, row dbq.Download, r DownloadRequest, prepare bool) (stored dbq.Download, inserted bool, err error) {
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

// prepareDownload prepares one download. It plans the download again for
// its files as they are now, extracts the subtitles it needs from its
// file, then converts it. If the accelerator fails before producing any
// output, the conversion runs again on the CPU. Finally it settles the
// download as ready, delivering what was made, or as failed with the
// reason. A file that can't be read is tried again on later passes before
// the download fails. A download that can't be planned any more, its
// version or a track gone, fails at once. After a server restart, the
// download waits in line and starts again.
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
	c, err := s.replan(ctx, d)
	if _, refused := errors.AsType[*RefusedError](err); refused || errors.Is(err, ErrNotFound) || errors.Is(err, catalog.ErrNotFound) {
		return s.settle(ctx, d, nil, nil, RefuseSourceUnreadable, err)
	}
	if err != nil {
		return err
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
	// A download put back in line is told of once it's out of preparing,
	// so its status reads queued.
	requeued := false
	defer func() {
		s.mu.Lock()
		delete(s.preparing, d.ID)
		s.mu.Unlock()
		if requeued {
			s.downloadsChanged(d.UserID)
		}
	}()
	s.downloadsChanged(d.UserID)

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
		requeued = true
		return ctx.Err()
	case errors.Is(err, media.ErrSourceUnreadable) && s.attempt(d.ID) < downloadAttempts:
		slog.WarnContext(ctx, "playback: download's file unreadable; trying again later", "work", d.WorkID, "err", err)
		s.requeue(ctx, d.ID)
		requeued = true
		return err
	}
	s.mu.Lock()
	delete(s.attempts, d.ID)
	s.mu.Unlock()
	err = s.settle(context.WithoutCancel(ctx), d, c, files, why, err)
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

// settle records a download as ready, delivering what c made with its
// converted files, or as failed with the reason. If the app removed the
// download in the meantime, the converted files are deleted instead.
func (s *Service) settle(ctx context.Context, d dbq.Download, c *choice, files []transcode.ConvertedFile, why Refusal, cause error) error {
	state, reason, made := DownloadReady, sql.NullString{}, sql.NullString{}
	if cause != nil {
		state, reason = DownloadFailed, sql.NullString{String: string(why), Valid: true}
		slog.WarnContext(ctx, "playback: download failed", "work", d.WorkID, "reason", why, "err", cause)
	} else {
		b, err := s.deliveredBy(ctx, c, files)
		if err != nil {
			return err
		}
		made = sql.NullString{String: b, Valid: true}
		if files != nil {
			slog.InfoContext(ctx, "playback: download ready", "work", d.WorkID, "took", time.Since(time.UnixMilli(d.QueuedAt)).Round(time.Second))
		}
	}
	var n int64
	err := s.db.Write(ctx, func(ctx context.Context, tx *store.Tx) error {
		q := dbq.New(tx)
		if now, err := q.GetDownload(ctx, d.ID); err == nil && made.Valid {
			made.String = moved(made.String, d.Delivered, now.Delivered)
		}
		var err error
		n, err = q.SettleDownload(ctx, dbq.SettleDownloadParams{
			ID: d.ID, State: string(state), Reason: reason, Delivered: made, SettledAt: sql.NullInt64{Int64: time.Now().UnixMilli(), Valid: true},
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

// moved renames, in what a download delivers as made, each track that was
// replaced while it was made. was is what the download delivered when its
// preparation read it, and now what it delivers since, the same tracks in
// the same order.
func moved(made, was, now string) string {
	var a, b delivered
	if was == now || json.Unmarshal([]byte(was), &a) != nil || json.Unmarshal([]byte(now), &b) != nil {
		return made
	}
	for i := range min(len(a.Tracks), len(b.Tracks)) {
		if from, to := a.Tracks[i].Track, b.Tracks[i].Track; from != to {
			made = strings.ReplaceAll(made, from.String(), to.String())
		}
	}
	return made
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

// downloaded is a choice's tracks as an app plays its download offline. A
// conversion's tracks are inside its file, the video first, then the
// audio, then each text track it carries, so each has its stream index
// there.
func (s *Service) downloaded(ctx context.Context, c *choice) []TrackDecision {
	tracks := slices.Clone(c.decision.Tracks)
	if c.decision.Delivery != DeliverConversion {
		return tracks
	}
	next := 0
	inFile := func(t *TrackDecision) {
		t.Delivery, t.Index = DeliverFile, next
		next++
	}
	for _, kind := range []media.TrackKind{media.Video, media.Audio} {
		if i := slices.IndexFunc(tracks, func(t TrackDecision) bool { return t.Type == kind && t.Selected }); i >= 0 {
			inFile(&tracks[i])
		}
	}
	for _, text := range s.textTracks(ctx, c) {
		if i := slices.IndexFunc(tracks, func(t TrackDecision) bool { return t.Track.String() == text.ID }); i >= 0 {
			inFile(&tracks[i])
			tracks[i].Codec = "mov_text"
		}
	}
	return tracks
}

// status is a download as its app sees it, from what it delivers and its
// files, subtitles and markers as they are now. Once it's ready, a
// subtitle delivered external that couldn't be extracted isn't in it.
func (s *Service) status(ctx context.Context, d dbq.Download) (DownloadStatus, error) {
	dl, files, err := s.kept(ctx, d)
	if err != nil {
		return DownloadStatus{}, err
	}
	out := DownloadStatus{ID: d.ID, Work: d.WorkID, Version: d.VersionID, Quality: dl.Quality, State: DownloadState(d.State), Decision: dl.decision()}
	out.ExternalSubtitles = s.externalSubtitles(ctx, files[0], s.styledOf(ctx, files[0]), out.Decision.Tracks)
	for i, t := range out.Decision.Tracks {
		if t.Delivery == DeliverExternal && out.State == DownloadReady &&
			!slices.ContainsFunc(out.ExternalSubtitles, func(x ExternalSubtitle) bool { return x.Track == t.Track && x.Ready }) {
			out.Decision.Tracks[i] = TrackDecision{Type: t.Type, Track: t.Track, Index: t.Index, Selected: t.Selected, Reason: ReasonSubtitlePreparing}
		}
	}
	if d.Reason.Valid {
		out.Reason = Refusal(d.Reason.String)
	}
	if d.SettledAt.Valid {
		out.Expires = time.UnixMilli(d.SettledAt.Int64).Add(keepDownload)
	}
	out.Chapters, out.Markers = s.deps.Markers.Markers(ctx, d.WorkID, files)
	out.Duration = duration(files)
	for i, f := range files {
		part := DownloadPart{Duration: f.Probe.Duration, Type: "video/mp4"}
		if dl.Delivery == DeliverFile {
			part.Size, part.Type = f.Size, mimeType(f.Probe.Container, f.RelPath)
			if fi, err := os.Stat(f.Path()); err == nil {
				part.Size, part.ETag = fi.Size(), fileETag(fi)
			}
		} else if made := dl.Parts[i]; made.SHA256 != "" {
			part.Size, part.ETag, part.SHA256 = made.Size, `"`+made.SHA256+`"`, made.SHA256
		}
		out.Size += part.Size
		out.Parts = append(out.Parts, part)
	}
	out.SizeExact = out.State == DownloadReady || dl.Delivery == DeliverFile
	if out.State != DownloadReady {
		out.Size = dl.Size
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
			out.State = DownloadQueued
		}
	}
	return out, nil
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

// RetryDownload asks again for a download that failed. It decides the
// download again, since the files may have changed, and puts it at the
// back of the line. Any other download is returned unchanged.
func (s *Service) RetryDownload(ctx context.Context, p auth.Caller, v users.Viewer, download id.ID) (DownloadStatus, error) {
	d, err := s.ownDownload(ctx, p.User, download)
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
		s.downloadsChanged(p.User)
	}
	return s.status(ctx, d)
}

// SubtitlesReplaced listens for the subtitles package replacing tracks.
// In tx, it moves every download that asked for or delivers a replaced
// track to the new track, so the download's status and a retry
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
	return nil
}

// DownloadedFile returns a part of one of the user's downloads, as long as
// the user may still have it (see Available). An original's part is
// returned at once. A conversion's part is returned once it's ready, and
// ErrNotReady before then.
func (s *Service) DownloadedFile(ctx context.Context, v users.Viewer, download id.ID, part int) (Media, error) {
	d, dl, files, err := s.servable(ctx, v, download)
	if err != nil {
		return Media{}, err
	}
	if part < 0 || part >= len(files) {
		return Media{}, fmt.Errorf("%w: part %d of download %s", ErrNotFound, part, download)
	}
	if dl.Delivery == DeliverFile {
		f := files[part]
		fi, err := os.Stat(f.Path())
		if err != nil {
			return Media{}, err
		}
		return Media{Path: f.Path(), Type: mimeType(f.Probe.Container, f.RelPath), ETag: fileETag(fi)}, nil
	}
	made := dl.Parts[part]
	if DownloadState(d.State) != DownloadReady || made.Path == "" {
		return Media{}, fmt.Errorf("%w: download %s", ErrNotReady, download)
	}
	return Media{Path: made.Path, Type: "video/mp4", ETag: `"` + made.SHA256 + `"`}, nil
}

// DownloadSubtitle returns a subtitle of one of the user's downloads by
// the name in its delivery, the same way Subtitle does for a play.
func (s *Service) DownloadSubtitle(ctx context.Context, v users.Viewer, download id.ID, name string) (Media, error) {
	_, _, files, err := s.servable(ctx, v, download)
	if err != nil {
		return Media{}, err
	}
	return s.subtitleFile(ctx, files[0], name)
}

// servable returns one of the viewer's downloads, what it delivers and its
// files, as long as the user may still have it.
func (s *Service) servable(ctx context.Context, v users.Viewer, download id.ID) (dbq.Download, delivered, []catalog.File, error) {
	d, err := s.ownDownload(ctx, v.User(), download)
	if err != nil {
		return d, delivered{}, nil, err
	}
	if _, err := s.downloader(ctx, v, v.User(), d.WorkID); err != nil {
		return d, delivered{}, nil, fmt.Errorf("%w: download %s", ErrNotFound, download)
	}
	dl, files, err := s.kept(ctx, d)
	if errors.Is(err, catalog.ErrNotFound) {
		return d, dl, nil, fmt.Errorf("%w: download %s", ErrNotFound, download)
	}
	return d, dl, files, err
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
