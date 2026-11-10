package playback

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/mmcdole/kivo/internal/catalog"
	"github.com/mmcdole/kivo/internal/config"
	"github.com/mmcdole/kivo/internal/id"
	"github.com/mmcdole/kivo/internal/media"
	"github.com/mmcdole/kivo/internal/playback/dbq"
	"github.com/mmcdole/kivo/internal/store"
	"github.com/mmcdole/kivo/internal/tasks"
	"github.com/mmcdole/kivo/internal/transcode"
)

// preparingAtOnce is how many downloads are prepared at once.
func (s *Downloads) preparingAtOnce(config.Settings) int {
	if accel, _ := s.accelerators(false); len(accel) > 0 {
		return 2
	}
	return 1
}

// prepare prepares one download. It plans the download again for
// its files as they are now, extracts the subtitles it needs from its
// file, then converts it. If the accelerator fails before producing any
// output, the conversion runs again on the CPU. Finally it settles the
// download as ready, delivering what was made, or as failed with the
// reason. A file that can't be read is tried again on later passes before
// the download fails. A download that can't be planned any more, its
// version or a track gone, fails at once. After a server restart, the
// download waits in line and starts again.
func (s *Downloads) prepare(ctx context.Context, download id.ID) error {
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
	s.progress(ctx, "")
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
			s.changed(d.UserID)
		}
	}()
	s.changed(d.UserID)

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

// progress reports the Downloads task's step and count: the
// downloads prepared since none were waiting, of those and the ones
// waiting now, as other tasks count their items. ctx must be the task's.
func (s *Downloads) progress(ctx context.Context, step string) {
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
func (s *Downloads) preparedOne(ctx context.Context) {
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
func (s *Downloads) attempt(download id.ID) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.attempts[download]++
	return s.attempts[download]
}

// prepareFiles extracts a download's subtitles when it needs them, then
// converts it if its decision says so. It returns the converted files, or
// why it failed.
func (s *Downloads) prepareFiles(ctx context.Context, d dbq.Download, c *choice, p *preparation) ([]transcode.ConvertedFile, Refusal, error) {
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
	s.progress(ctx, StepConvert)
	reached := func(at time.Duration) {
		s.mu.Lock()
		p.reached = at
		tell := time.Since(p.notified) >= progressEvery
		if tell {
			p.notified = time.Now()
		}
		s.mu.Unlock()
		if tell {
			s.changed(p.user)
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
func (s *Downloads) extractSubtitles(ctx context.Context, c *choice) error {
	s.progress(ctx, StepExtractSubtitles)
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
func (s *Downloads) requeue(ctx context.Context, download id.ID) {
	err := s.db.Write(ctx, func(ctx context.Context, tx *store.Tx) error { return dbq.New(tx).RequeueDownload(ctx, download) })
	if err != nil {
		slog.WarnContext(ctx, "playback: requeue download", "err", err)
	}
}

// settle records a download as ready, delivering what c made with its
// converted files, or as failed with the reason. If the app removed the
// download in the meantime, the converted files are deleted instead.
func (s *Downloads) settle(ctx context.Context, d dbq.Download, c *choice, files []transcode.ConvertedFile, why Refusal, cause error) error {
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
	s.changed(d.UserID)
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
