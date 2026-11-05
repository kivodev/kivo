package playback

import (
	"slices"

	"github.com/mmcdole/kivo/internal/id"
	"github.com/mmcdole/kivo/internal/users"
)

// limitFor is the lowest bitrate cap that applies to a play. The caps are
// the quality the client picked (its remote setting when the play is
// remote), the server's cap c, and, for "auto", the client's measured
// bandwidth.
func limitFor(r StartRequest, c Limit, local bool) Limit {
	var l Limit
	apply := func(bitrate int, why Reason) {
		if bitrate > 0 && (l.MaxBitrate == 0 || bitrate < l.MaxBitrate) {
			l = Limit{MaxBitrate: bitrate, Reason: why}
		}
	}
	q := r.qualityFor(local)
	apply(q.MaxBitrate, ReasonClientQualityLimit)
	apply(c.MaxBitrate, c.Reason)
	apply(q.Bandwidth*4/5, ReasonBandwidthLimit)
	return l
}

// capFor is the bitrate cap the server sets on a play regardless of the
// quality its client picks. It applies only to remote plays, and it's the
// user's remote cap, else the server's, as Jellyfin's user and server
// limits are.
func capFor(user users.User, server int64, local bool) Limit {
	c := RemoteCap(user, server)
	if local || c <= 0 {
		return Limit{}
	}
	return Limit{MaxBitrate: int(c), Reason: ReasonRemoteQualityLimit}
}

// RemoteCap is the most a user's stream outside the local networks may
// use: their own cap, else the server's. Zero means none.
func RemoteCap(user users.User, server int64) int64 {
	if user.Limits.RemoteQualityCap > 0 {
		return user.Limits.RemoteQualityCap
	}
	return server
}

// reserveStream holds one of the user's streams for a play about to
// start. Over the limit, it returns a StreamLimitError that lists the
// user's plays. Paused plays and plays still starting count toward the
// limit. Downloads don't.
func (s *Service) reserveStream(user id.ID, limit int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	var active []Play
	for _, pl := range s.plays {
		if pl.user == user {
			active = append(active, pl.infoLocked())
		}
	}
	if limit > 0 && len(active)+s.starting[user] >= limit {
		slices.SortFunc(active, func(a, b Play) int { return a.Started.Compare(b.Started) })
		return &StreamLimitError{Plays: active}
	}
	s.starting[user]++
	return nil
}

// reservation is what a play holds while it starts: one of its user's
// streams, and a video transcode when it needs one.
type reservation struct {
	user      id.ID
	transcode bool
}

// releaseLocked gives back what a starting play held. It runs once the
// play is published and counts itself, or once it fails to start.
func (s *Service) releaseLocked(r reservation) {
	if s.starting[r.user]--; s.starting[r.user] == 0 {
		delete(s.starting, r.user)
	}
	if r.transcode {
		s.reserved--
	}
}

// StreamLimitError lists the user's active plays, so the user can stop
// one.
type StreamLimitError struct{ Plays []Play }

func (e *StreamLimitError) Error() string { return ErrStreamLimit.Error() }

func (e *StreamLimitError) Unwrap() error { return ErrStreamLimit }

// reserveTranscode holds one of the server's video transcodes for a play
// about to start one, and refuses it over the limit. Paused plays count.
// Direct play, remux and audio transcodes never count.
func (s *Service) reserveTranscode() error {
	limit := s.deps.Settings.Get().VideoTranscodeLimit
	s.mu.Lock()
	defer s.mu.Unlock()
	if limit > 0 && s.videoTranscodesLocked() >= limit {
		return &RefusedError{Err: ErrTranscodeBusy, Reason: RefuseTranscodeLimit}
	}
	s.reserved++
	return nil
}

// videoTranscodesLocked counts the video transcodes held by running plays
// and reserved for plays still starting.
func (s *Service) videoTranscodesLocked() int {
	n := s.reserved
	for _, pl := range s.plays {
		if pl.transcodesVideo() {
			n++
		}
	}
	return n
}
