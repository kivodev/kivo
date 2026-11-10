package transcode

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/mmcdole/kivo/internal/media"
)

// Stream is one play's transcode: a planned playlist, the segments on
// disk, and at most one running job.
type Stream struct {
	t         *Transcoder
	spec      Spec
	plan      []span
	dir       string
	playlists map[string][]byte
	fileCues  [][]media.Cue // each text track's cues when it's read from a file, read as the stream opens

	mu       sync.Mutex
	changed  chan struct{} // closed and replaced whenever a waiting request should look again
	done     []bool        // whether each segment is on disk
	initDone []bool        // whether each part's init segment is on disk
	job      *job          // nil when no job is running
	closed   bool
	idle     *time.Timer
	jobs     sync.WaitGroup // tracks every started job until it exits
	count    int            // the number of jobs started, used to number their scratch files
}

// Playlist returns one of the stream's playlists: PlaylistName, or one of
// the playlists it names.
func (s *Stream) Playlist(name string) ([]byte, error) {
	b, ok := s.playlists[name]
	if !ok {
		return nil, fmt.Errorf("%w: playlist %s", ErrNotFound, name)
	}
	s.Touch()
	return b, nil
}

// Segment returns the path of a segment (a video segment, a text segment,
// or an init segment) once it's on disk. It returns at once if the segment
// is already there. It waits if the running job is about to write it. For
// a far seek, it restarts the job at that segment. The newest request
// wins: once another request has replaced the job, this one only waits for
// the running job and doesn't restart it. A job that began at the
// segment and went past it without writing it fails the request with
// ErrJobFailed, rather than starting again. Every text track's
// segment is written along with its video, so text segments behave the
// same way.
func (s *Stream) Segment(ctx context.Context, name string) (string, error) {
	s.Touch()
	if p, ok := parseInit(name); ok && p < len(s.spec.Parts) {
		return s.init(ctx, name, p)
	}
	i, text, ok := parseSegment(name)
	if !ok || i >= len(s.plan) || text >= len(s.spec.Text) {
		return "", fmt.Errorf("%w: segment %s", ErrNotFound, name)
	}
	path := s.path(i)
	if text >= 0 {
		path = s.textPath(text, i)
	}
	have := func() bool {
		s.readLocked(i)
		return s.done[i]
	}
	return s.wait(ctx, name, path, have, func(j *job) (int, error) {
		if err := s.refusedLocked(j, i); err != nil {
			return 0, err
		}
		if j == nil || j.exited || i < j.next || i > j.next+nearSegments {
			return i, nil
		}
		return -1, nil
	})
}

// wait returns path once have reports that it's on disk. Until then it
// looks again each time the stream changes. from is given the stream's job.
// It returns the segment to start a new job at, -1 to wait for the job, or
// an error when the request can't be met. Once the job the request waited
// on has been replaced, from isn't asked while another job is running, so
// the newer request keeps the job. have and from run under the lock. name
// is the requested segment, which errors name.
func (s *Stream) wait(ctx context.Context, name, path string, have func() bool, from func(j *job) (int, error)) (string, error) {
	var on *job // the job the request last waited on
	look := func() (<-chan struct{}, error) {
		s.mu.Lock()
		defer s.mu.Unlock()
		switch {
		case s.closed:
			return nil, fmt.Errorf("%w: segment %s", ErrClosed, name)
		case have():
			return nil, nil
		case on != nil && on != s.job && s.job != nil && !s.job.exited:
			// A newer request replaced the job, so this one waits for the
			// running job rather than take it back.
			return s.changed, nil
		case on != nil && on == s.job && on.err != nil && !stopped(on.err):
			return nil, on.err
		}
		i, err := from(s.job)
		if err != nil {
			return nil, err
		}
		if i >= 0 {
			s.startLocked(i)
		}
		on = s.job
		return s.changed, nil
	}
	for {
		changed, err := look()
		switch {
		case err != nil:
			return "", err
		case changed == nil:
			return path, nil
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-changed:
		}
	}
}

// wakeLocked makes every waiting request look again. Whatever changes what
// a request waits for calls it: a segment or an init segment landing, a job
// ending or being replaced, and the stream closing.
func (s *Stream) wakeLocked() {
	close(s.changed)
	s.changed = make(chan struct{})
}

// refusedLocked returns why segment i can't come from the stream's job j:
// the job's own failure, or that j began at i and went past it without
// writing it. Starting a job at i again would only do the same, so the
// request fails and playback can fall back. A job that began before i and
// went past it is started again at i, once, since a job that seeks there
// may yet write it. It returns nil when j may still write i, or a new job
// may start at it.
func (s *Stream) refusedLocked(j *job, i int) error {
	switch {
	case j == nil:
		return nil
	case j.failed():
		return j.err
	case i == j.from && i < j.next && !stopped(j.err):
		if !j.passed {
			j.passed = true
			slog.Warn("transcode: job passed a segment without writing it", "stream", filepath.Base(s.dir),
				"file", s.spec.Parts[s.plan[i].Part].ID, "segment", i, "from", j.from, "next", j.next)
		}
		return fmt.Errorf("%w: segment %d was passed by the job from segment %d", ErrJobFailed, i, j.from)
	}
	return nil
}

// nearSegments is how many segments ahead of a running job a request may
// be and still wait for the job rather than restart it. A seek this close
// is cheaper to wait out than to restart.
const nearSegments = 3

// init returns the path of part p's init segment once it's on disk. If no
// job is running in the part, it starts one at the part's start. name is
// the requested segment, which errors name.
func (s *Stream) init(ctx context.Context, name string, p int) (string, error) {
	have := func() bool { return s.initDone[p] }
	return s.wait(ctx, name, filepath.Join(s.dir, initName(p)), have, func(j *job) (int, error) {
		switch {
		case j != nil && j.failed():
			return 0, j.err
		case j == nil || j.exited || s.plan[j.from].Part != p:
			return s.firstOf(p), nil
		}
		return -1, nil
	})
}

// Touch marks the stream active by resetting the idle timer. Heartbeats
// call it.
func (s *Stream) Touch() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.closed {
		s.idle.Reset(idleTimeout)
	}
}

// Job describes the stream's job.
func (s *Stream) Job() JobInfo {
	s.mu.Lock()
	defer s.mu.Unlock()
	j := s.job
	if j == nil {
		return JobInfo{}
	}
	info := JobInfo{State: JobRunning, From: j.from, Next: j.next, Encoded: s.at(j.next)}
	if w := j.workingLocked(); j.next > j.from && w > 0 {
		info.Speed = float64(s.at(j.next)-s.at(j.from)) / float64(w)
	}
	switch {
	case j.failed():
		info.State, info.Err = JobFailed, j.err
	case j.exited:
		info.State = JobDone
	case j.paused:
		info.State = JobPaused
	}
	if j.out != nil {
		info.Diagnostics = j.out.Diagnostics()
	}
	return info
}

// Close kills the job and removes the stream's segments. It waits until
// every job the stream started, including stopped ones, has exited.
func (s *Stream) Close() {
	s.mu.Lock()
	s.closed = true
	s.idle.Stop()
	s.stopLocked()
	s.mu.Unlock()
	s.jobs.Wait()
	_ = os.RemoveAll(s.dir)
}

// idleStop kills the job of a stream that has had no request for
// idleTimeout, whether the job is paused or not. The stream stays, and its
// next request starts a new job.
func (s *Stream) idleStop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stopLocked()
}

func (s *Stream) path(i int) string { return filepath.Join(s.dir, segmentName(i)) }

// textPath is the path of text track k's segment i.
func (s *Stream) textPath(k, i int) string { return filepath.Join(s.dir, textName(k, i)) }

// textSpan is the span that text segment i's cues are cut to, on the
// source's timestamps. It's normally the segment's own span. For the last
// segment of a part it runs from the segment's start to the end of time,
// so it includes any cue running past the end of the video.
func (s *Stream) textSpan(i int) (from, to time.Duration) {
	sp := s.plan[i]
	from = sp.Start + s.spec.Parts[sp.Part].Start
	if i+1 == len(s.plan) || s.plan[i+1].Part != sp.Part {
		return from, math.MaxInt64
	}
	return from, from + sp.Duration
}

// firstOf returns the index of part p's first segment.
func (s *Stream) firstOf(p int) int {
	for i, sp := range s.plan {
		if sp.Part == p {
			return i
		}
	}
	return len(s.plan)
}

// startLocked replaces the running job with one starting at segment i.
func (s *Stream) startLocked(i int) *job {
	s.stopLocked()
	ctx, cancel := context.WithCancel(context.Background())
	s.count++
	j := &job{seq: s.count, from: i, next: i, reading: i, cancel: cancel, began: time.Now()}
	s.job = j
	s.jobs.Go(func() { s.run(ctx, j) })
	return j
}

// stopLocked cancels the running job, if any.
func (s *Stream) stopLocked() {
	if s.job != nil {
		s.job.cancel()
		s.job = nil
	}
	s.wakeLocked()
}

// readLocked records a request for segment i. It resumes a paused job once
// the player has caught up with it.
func (s *Stream) readLocked(i int) {
	j := s.job
	if j == nil || i < j.from {
		return
	}
	j.reading = max(j.reading, i)
	if j.paused && s.leadLocked(j) < resumeLead {
		if err := j.out.Resume(); err != nil {
			slog.Debug("transcode: can't resume the job", "err", err)
		}
		j.paused, j.began = false, time.Now()
	}
}

// leadLocked is how far the job has encoded beyond the furthest segment the
// player has asked for.
func (s *Stream) leadLocked(j *job) time.Duration {
	return s.at(j.next) - s.at(j.reading)
}

// at is where segment i starts on the parts' timeline. Past the last
// segment, it's the end of the timeline.
func (s *Stream) at(i int) time.Duration {
	if i >= len(s.plan) {
		last := s.plan[len(s.plan)-1]
		return last.At + last.Duration
	}
	return s.plan[i].At
}
