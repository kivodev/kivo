package playback

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/mmcdole/kivo/internal/auth"
	"github.com/mmcdole/kivo/internal/events"
	"github.com/mmcdole/kivo/internal/id"
	"github.com/mmcdole/kivo/internal/users"
	"github.com/mmcdole/kivo/internal/userstate"
)

const (
	playTimeout = 2 * time.Minute // a play with no heartbeat or request for this long ends
	keptSeeks   = 20              // how many seek timings a play keeps for diagnostics
)

// PlayState is what a client's player is doing, as a heartbeat reports it.
type PlayState string

// The play states clients report. Clients may send others, which leave a
// play's state as it was. "Now playing" shows a play's state.
const (
	StatePlaying   PlayState = "playing"
	StatePaused    PlayState = "paused"
	StateBuffering PlayState = "buffering"
	StateEnded     PlayState = "ended"
)

// Heartbeat is a client's progress report.
type Heartbeat struct {
	Position time.Duration
	State    PlayState
	// SeekToFirstFrame is how long the first frame took to show after a
	// seek. It feeds the diagnostics and the seek-time metrics.
	SeekToFirstFrame time.Duration
	Bandwidth        int // the client's measured download rate; zero when unknown
	// Subtitle is a subtitle track the client switched to without starting
	// a new play. A zero ID means subtitles off. It's nil when the client
	// shows the play's own subtitle.
	Subtitle *id.ID
	// Audio is an audio track the client switched to inside the file. It's
	// nil when the client plays the play's own.
	Audio *id.ID
}

// Heartbeat writes the play's position and keeps the play and its job
// alive. If the position fails to write, the heartbeat fails but the play
// keeps playing, and the next heartbeat writes it. A subtitle or an audio
// track the client switched to is remembered for an episode's series, the
// same as one chosen in a start request.
func (s *Service) Heartbeat(ctx context.Context, p auth.Caller, sid id.ID, h Heartbeat) error {
	pl, err := s.owned(p, sid)
	if err != nil {
		return err
	}
	return s.heartbeat(ctx, pl, h)
}

// heartbeat updates a play the caller owns. It returns ErrNoPlay when the
// play ended after it was found.
func (s *Service) heartbeat(ctx context.Context, pl *play, h Heartbeat) error {
	s.mu.Lock()
	report, ok := s.reportLocked(pl, h.Position)
	if !ok {
		s.mu.Unlock()
		return ErrNoPlay // the play ended after owned returned it
	}
	var ch chosen
	if h.Subtitle != nil && *h.Subtitle != pl.subtitle && pl.offersSubtitle(*h.Subtitle) {
		pl.subtitle, ch.subtitle = *h.Subtitle, h.Subtitle
	}
	if h.Audio != nil && *h.Audio != pl.audio && pl.offersAudio(*h.Audio) {
		pl.audio, ch.audio = *h.Audio, h.Audio
	}
	switched := ch.audio != nil || ch.subtitle != nil
	switch h.State {
	case StatePlaying, StatePaused, StateBuffering, StateEnded:
		pl.state = h.State
	}
	if h.Bandwidth > 0 {
		pl.bandwidth = h.Bandwidth
	}
	if h.SeekToFirstFrame > 0 {
		pl.seeks = append(pl.seeks, h.SeekToFirstFrame)
		if len(pl.seeks) > keptSeeks {
			pl.seeks = pl.seeks[1:]
		}
		s.seeks++
		s.seekTime += h.SeekToFirstFrame
	}
	s.mu.Unlock()
	if h.SeekToFirstFrame > 0 {
		slog.DebugContext(ctx, "playback: seek", "work", pl.work, "client", pl.client, "summary", pl.decision.Summary(),
			"first_frame", h.SeekToFirstFrame.Round(time.Millisecond))
	}
	if pl.tc != nil {
		pl.tc.Touch()
	}
	if switched {
		if user, err := s.deps.Users.User(ctx, pl.user); err != nil {
			slog.WarnContext(ctx, "playback: remember track choice", "work", pl.work, "err", err)
		} else {
			s.remember(ctx, pl, s.prefsFor(ctx, user, pl.work), ch)
		}
	}
	s.notify()
	return s.deps.Userstate.Report(ctx, report)
}

// owned returns a play the caller's user owns, and refreshes its last-seen
// time. A play that an admin stopped a short while ago returns a
// *StoppedError.
func (s *Service) owned(p auth.Caller, sid id.ID) (*play, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	pl := s.plays[sid]
	if st, ok := s.stopped[sid]; ok && pl == nil && st.user == p.User {
		return nil, &StoppedError{Message: st.message}
	}
	if pl == nil || pl.user != p.User || s.expiredLocked(pl, time.Now()) {
		return nil, fmt.Errorf("%w: %s", ErrNoPlay, sid)
	}
	pl.seen = time.Now()
	return pl, nil
}

// StopRequest is how a play is stopped.
type StopRequest struct {
	// Position is where the owner stopped the play. Nil keeps the position
	// from the last heartbeat.
	Position *time.Duration
	// Message is an admin's message to the viewer of someone else's play.
	Message string
	// Failure, from the play's owner, says playback failed with nothing
	// left to try, as when its fallback failed too. The failure is logged
	// as a fallback's is, and the play ends as failed, so its player isn't
	// sent playEnded. Position is the failure's position.
	Failure *FailReport
}

// Stop ends one of the caller's plays. An admin, identified by the viewer,
// may also stop someone else's play. That play ends at the position its
// owner left it, and its player shows the request's message. A stop with
// a failure ends the play as failed. Only the owner reports a failure, so
// an admin's stop of someone else's play with one is ErrNoPlay.
func (s *Service) Stop(ctx context.Context, p auth.Caller, v users.Viewer, sid id.ID, r StopRequest) error {
	pl, err := s.owned(p, sid)
	if errors.Is(err, ErrNoPlay) && v.Admin() && r.Failure == nil {
		return s.endAny(ctx, sid, r.Message)
	}
	if err != nil {
		return err
	}
	if r.Position != nil {
		s.mu.Lock()
		s.reportLocked(pl, *r.Position) // if the play already ended, it keeps its end position, which its end wrote
		s.mu.Unlock()
	}
	if r.Failure != nil {
		s.logFailure(ctx, pl, *r.Failure, false)
		return s.end(ctx, pl, EndFailed, "")
	}
	return s.end(ctx, pl, EndStopped, "")
}

// reportLocked records a play's position and returns the report to write.
// It does nothing once the play has ended, because a report written after
// the end would overwrite the progress the end wrote. s.mu is held.
func (s *Service) reportLocked(pl *play, pos time.Duration) (userstate.Progress, bool) {
	if s.plays[pl.id] != pl {
		return userstate.Progress{}, false
	}
	pl.position, pl.reported = pos, time.Now()
	return pl.progress(false), true
}

// progress is the play's report of its last position. s.mu is held.
func (pl *play) progress(stopped bool) userstate.Progress {
	return userstate.Progress{
		Play: pl.id, User: pl.user, Version: pl.version.ID, Position: pl.position, At: pl.reported, Stopped: stopped,
		Client: pl.client, Local: pl.local,
	}
}

// endAny ends any user's play at the position of its last heartbeat. It's
// how an admin stops someone else's play.
func (s *Service) endAny(ctx context.Context, sid id.ID, message string) error {
	s.mu.Lock()
	pl := s.plays[sid]
	s.mu.Unlock()
	if pl == nil {
		return fmt.Errorf("%w: %s", ErrNoPlay, sid)
	}
	return s.end(ctx, pl, EndStoppedByAdmin, message)
}

// EndReason is why a play ended.
type EndReason string

// The end reasons. A play's own player already knows about a shutdown or
// a failure, so no Ended event is sent for those.
const (
	EndStopped        EndReason = "stopped"
	EndStoppedByAdmin EndReason = "stopped by an admin"
	EndExpired        EndReason = "expired"
	EndShutdown       EndReason = "shutdown"
	EndFailed         EndReason = "playback failed" // a fallback play replaces the failed one
)

// end ends a play and tells its player why, with an admin's message if
// there is one.
func (s *Service) end(ctx context.Context, pl *play, reason EndReason, message string) error {
	if !s.take(pl) {
		return nil // something else ended the play first
	}
	if reason == EndStoppedByAdmin {
		s.mu.Lock()
		s.stopped[pl.id] = stoppedPlay{user: pl.user, message: message, at: time.Now()}
		s.mu.Unlock()
	}
	return s.close(ctx, pl, reason, message)
}

// take removes a play from the map and reports whether it was there. When
// a stop, an expiry and a fallback race to end a play, only one takes it.
func (s *Service) take(pl *play) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.plays[pl.id]
	delete(s.plays, pl.id)
	return ok
}

// close finishes a play already taken from the map. It closes the
// transcode and writes the last position, if the play reported one.
func (s *Service) close(ctx context.Context, pl *play, reason EndReason, message string) error {
	slog.InfoContext(ctx, "playback: play ended", "work", pl.work, "client", pl.client, "reason", reason,
		"open", time.Since(pl.started).Round(time.Second))
	if pl.tc != nil {
		pl.tc.Close()
	}
	// TODO: no webhook is sent for a play's end.
	s.mu.Lock()
	report := pl.progress(true)
	s.mu.Unlock()
	var err error
	if !report.At.IsZero() {
		err = s.deps.Userstate.Report(ctx, report)
	}
	if reason != EndShutdown && reason != EndFailed {
		s.deps.Events.Publish(events.Event{Kind: events.PlayEnded, Scope: events.Scope{User: pl.user}, Data: Ended{Play: pl.id, Message: message}})
	}
	s.notify()
	return err
}

// FailReport is what a client says failed.
type FailReport struct {
	Position time.Duration
	Code     string // mediaDecodeError, mediaSourceError, networkError or unsupported
	Detail   string // the player's own error text
}

// Fallback replaces one of the caller's plays whose playback failed,
// either in the client or in the server's transcode. The failed play ends.
// A new play starts at the client's position with the same version and
// tracks, and transcodes every track (Input.Fallback). A video transcode
// that ran on an accelerator runs on the CPU in the new play. A play that
// already transcoded every track on the CPU has nothing else to try, so it
// ends and Fallback returns ErrNoFallback.
func (s *Service) Fallback(ctx context.Context, p auth.Caller, v users.Viewer, sid id.ID, f FailReport) (Started, error) {
	pl, err := s.endFailed(ctx, p, sid, f)
	if err != nil {
		return Started{}, err
	}
	s.mu.Lock()
	r := pl.again(f.Position)
	s.mu.Unlock()
	return s.start(ctx, p, v, r, pl)
}

// endFailed ends one of the caller's plays whose playback failed, and
// returns it so its fallback can replace it.
func (s *Service) endFailed(ctx context.Context, p auth.Caller, sid id.ID, f FailReport) (*play, error) {
	pl, err := s.owned(p, sid)
	if err != nil {
		return nil, err
	}
	s.logFailure(ctx, pl, f, true)
	if !s.take(pl) {
		return nil, fmt.Errorf("%w: %s", ErrNoPlay, sid) // the play was stopped or expired in the meantime
	}
	if err := s.close(ctx, pl, EndFailed, ""); err != nil {
		return nil, err
	}
	if pl.transcodesAll() {
		return nil, ErrNoFallback
	}
	return pl, nil
}

// logFailure logs a client's report of a play's failure with the server's
// side of it, the transcode job's error, and records an accelerator that
// failed. fallingBack says whether a fallback play follows. When none
// does, the failure ended playback. Both are Warn, as an item that failed
// for good is, and only the message tells them apart.
func (s *Service) logFailure(ctx context.Context, pl *play, f FailReport, fallingBack bool) {
	var failure error
	if pl.tc != nil {
		failure = pl.tc.Job().Err
	}
	msg := "playback: playback failed"
	if !fallingBack {
		msg = "playback: playback failed, nothing left to try"
	}
	slog.WarnContext(ctx, msg, "work", pl.work, "client", pl.client, "summary", pl.decision.Summary(),
		"position", f.Position.Round(time.Second), "code", f.Code, "detail", f.Detail, "job_err", failure)
	if a := pl.accelerator(); a != "" && failure != nil {
		s.acceleratorFailed(&pl.choice, a, failure)
	}
}

// again is the request that starts the play again at pos, with the same
// version and the tracks showing now. The caller holds s.mu.
func (pl *play) again(pos time.Duration) StartRequest {
	r := pl.request
	r.Version, r.Position = pl.version.ID, &pos
	if !pl.audio.IsZero() {
		audio := pl.audio
		r.AudioTrack = &audio
	}
	sub := pl.subtitle // zero means subtitles off
	r.SubtitleTrack = &sub
	return r
}

// endExpired ends the plays with no heartbeat or request for playTimeout,
// closing their jobs and saving their positions.
func (s *Service) endExpired(ctx context.Context) {
	now := time.Now()
	s.endEach(ctx, EndExpired, func(pl *play) bool { return s.expiredLocked(pl, now) })
	s.mu.Lock()
	for sid, st := range s.stopped {
		if now.Sub(st.at) > stoppedKept {
			delete(s.stopped, sid)
		}
	}
	s.mu.Unlock()
}

// stoppedKept is how long a play an admin stopped answers its player with
// ErrPlayStopped rather than as a play that doesn't exist. It's longer
// than playTimeout.
const stoppedKept = 10 * time.Minute

// stoppedPlay is a play an admin stopped.
type stoppedPlay struct {
	user    id.ID
	message string
	at      time.Time
}

// StoppedError means a request is about a play an admin stopped. Message
// is the admin's message to the viewer, if any. It wraps ErrPlayStopped.
type StoppedError struct {
	Message string
}

func (e *StoppedError) Error() string { return ErrPlayStopped.Error() }

func (e *StoppedError) Unwrap() error { return ErrPlayStopped }

// endAll ends every play, as the server shuts down.
func (s *Service) endAll(ctx context.Context) {
	s.endEach(ctx, EndShutdown, func(*play) bool { return true })
}

// endEach ends the plays that match picks, with the given reason. match
// runs with s.mu held.
func (s *Service) endEach(ctx context.Context, reason EndReason, match func(*play) bool) {
	s.mu.Lock()
	var done []*play
	for _, pl := range s.plays {
		if match(pl) {
			done = append(done, pl)
		}
	}
	s.mu.Unlock()
	for _, pl := range done {
		if err := s.end(ctx, pl, reason, ""); err != nil {
			slog.ErrorContext(ctx, "playback: end play", "err", err)
		}
	}
}

// expiredLocked reports whether a play has had no heartbeat or request for
// playTimeout.
func (s *Service) expiredLocked(pl *play, now time.Time) bool {
	return now.Sub(pl.seen) > playTimeout
}
