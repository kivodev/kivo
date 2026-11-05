package transcode

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/mmcdole/kivo/internal/atomicfile"
	"github.com/mmcdole/kivo/internal/media"
	"github.com/mmcdole/kivo/internal/media/subtitle"
)

// job is one ffmpeg run, producing segments from one point onward.
type job struct {
	seq        int // the job's number among the stream's jobs
	from, next int // the first segment, and the next one it will produce
	reading    int // the furthest segment requested since the job started
	ready      map[int]chan struct{}
	first      chan struct{} // closed once the job's first segment is on disk
	cancel     context.CancelFunc
	exited     chan struct{}
	err        error        // set before exited closes
	out        media.Output // nil until ffmpeg has started
	paused     bool
	// worked and began track the time ffmpeg has spent working, to measure
	// its speed. worked is the time before the last pause. began is when
	// the current stretch of work started, and is zero while paused.
	worked time.Duration
	began  time.Time
}

// JobState is the state of a stream's job.
type JobState string

// Job states. A stopped job (by a far seek or an idle kill) is gone, so it
// has no state.
const (
	JobRunning JobState = "running"
	JobPaused  JobState = "paused" // far enough ahead of the player
	JobDone    JobState = "done"   // reached the end of the file
	JobFailed  JobState = "failed"
)

// JobInfo describes the stream's job, for diagnostics and so the playback
// package can see why a stream failed.
type JobInfo struct {
	State      JobState // empty when there's no job
	From, Next int
	// Encoded is how far the job's segments reach on the parts' timeline.
	Encoded time.Duration
	// Speed is how many seconds of media the job makes per second of work,
	// not counting pauses. 1.8 means 1.8× realtime. It's zero until the
	// first segment.
	Speed       float64
	Err         error // for a failed job, wraps ErrAcceleratorFailed or ErrJobFailed
	Diagnostics media.Diagnostics
}

// run runs one job. It starts ffmpeg at the job's first segment and
// appends each fragment the media package yields to its planned segment. A
// segment is renamed into place, and its waiters woken, as soon as the
// next segment's first fragment arrives. That way it's served without
// waiting for the segment after it. The matching text segment is written
// just before, from the cues read by then.
func (s *Stream) run(ctx context.Context, j *job) {
	start := time.Now()
	file := s.spec.Parts[s.plan[j.from].Part].ID
	slog.DebugContext(ctx, "transcode: job started", "file", file, "from", j.from, "at", s.plan[j.from].Start,
		"video", s.spec.Video.Codec, "accelerator", s.spec.Accelerator)
	err := s.spec.classify(s.produce(ctx, j), j.next > j.from)
	s.mu.Lock()
	defer s.mu.Unlock()
	attrs := []any{"file", file, "from", j.from, "to", j.next, "duration", time.Since(start).Round(time.Millisecond)}
	switch {
	case err == nil || stopped(err):
		slog.DebugContext(ctx, "transcode: job ended", attrs...)
	default:
		// Log the command line and ffmpeg's last output, so the failure can
		// be reproduced by hand.
		var d media.Diagnostics
		if j.out != nil {
			d = j.out.Diagnostics()
		}
		slog.WarnContext(ctx, "transcode: job failed", append(attrs, "accelerator", s.spec.Accelerator, "err", err, "command", d.Command, "stderr", d.Stderr)...)
	}
	j.err = err
	j.restLocked()
	close(j.exited)
	for _, ch := range j.ready {
		close(ch) // waiters for segments this job didn't produce will look again
	}
	j.ready = nil
}

// classify maps a job's media error to what the playback package does
// about it. An accelerator that's busy or fails before the job's first
// segment becomes ErrAcceleratorFailed, so playback can switch to
// software. An unreadable source, a timeout and a cancellation pass
// through unchanged. Any other error fails the job.
func (s Spec) classify(err error, produced bool) error {
	switch {
	case err == nil, stopped(err), errors.Is(err, media.ErrTimeout), errors.Is(err, media.ErrSourceUnreadable):
		return err
	case s.accelerated() && !produced && (errors.Is(err, media.ErrEncoderBusy) || errors.Is(err, media.ErrEncoderFailed)):
		return fmt.Errorf("%w: %w", ErrAcceleratorFailed, err)
	default:
		return fmt.Errorf("%w: %w", ErrJobFailed, err)
	}
}

// produce runs ffmpeg on the part the job starts in, up to that part's
// end.
func (s *Stream) produce(ctx context.Context, j *job) error {
	part := s.plan[j.from].Part
	req := media.TranscodeRequest{
		Path: s.spec.Parts[part].Path, Start: s.plan[j.from].Start, SegmentDuration: gridSegment,
		VideoTrack: s.spec.VideoTrack, Source: s.spec.Source, Video: s.spec.Video, Audio: s.spec.Audio,
		BurnIn: s.spec.BurnIn, Accelerator: s.spec.Accelerator,
	}
	text := make([]*textWriter, len(s.spec.Text))
	for k, t := range s.spec.Text {
		text[k] = &textWriter{s: s, k: k, out: -1}
		if t.Path == "" {
			text[k].out = len(req.Text)
			req.Text = append(req.Text, media.CuesOut{Track: t.Track, Path: filepath.Join(s.dir, fmt.Sprintf("text-%d-%d.srt", j.seq, k))})
		}
	}
	out, err := s.t.deps.Media.Transcode(ctx, req)
	if err != nil {
		return err
	}
	defer out.Close()
	s.mu.Lock()
	j.out = out
	s.mu.Unlock()

	init, err := out.Init()
	if err != nil {
		return err
	}
	if err := s.writeInit(part, init); err != nil {
		return err
	}

	cur := -1
	var f *os.File // the segment being written; nil while skipping a segment already on disk
	defer func() {
		if f != nil {
			f.Close()
			os.Remove(f.Name())
		}
	}()
	// finish puts the segment being written in place, after its text.
	finish := func() error {
		for _, w := range text {
			if err := w.write(out, cur); err != nil {
				return err
			}
		}
		if err := s.commit(j, cur, f); err != nil {
			return err
		}
		f = nil
		return nil
	}
	for seg, err := range out.Segments() {
		if err != nil {
			return err
		}
		i := s.segmentOf(j, part, cur, seg)
		if i != cur {
			if f != nil {
				if err := finish(); err != nil {
					return err
				}
			}
			cur = i
			s.mu.Lock()
			have := s.done[i]
			s.mu.Unlock()
			if !have {
				if f, err = os.CreateTemp(s.dir, "seg-*.tmp"); err != nil {
					return err
				}
			}
		}
		if f == nil {
			continue
		}
		if _, err := f.Write(seg.Data); err != nil {
			return err
		}
		// Encoded video has one fragment per segment, so a segment is done
		// when its fragment is. A copied video segment ends only when the
		// next segment's first fragment arrives.
		if s.spec.encoded() {
			if err := finish(); err != nil {
				return err
			}
		}
	}
	if f != nil {
		return finish()
	}
	return nil
}

// segmentOf returns the planned segment of part p that a fragment belongs
// to. cur is the previous fragment's segment. Copied video yields one
// fragment per source keyframe, placed by its measured start. Encoded
// video yields exactly one fragment per grid segment, so fragments are
// counted instead. At NTSC rates, a fixed GOP's whole frames drift from
// the grid by a few milliseconds per segment, and over a long job that
// would push a fragment's start into the next grid step.
func (s *Stream) segmentOf(j *job, p, cur int, seg media.Segment) int {
	if !s.spec.encoded() {
		return segmentIn(s.plan, p, seg.Start)
	}
	if cur < 0 {
		return j.from
	}
	if cur+1 < len(s.plan) && s.plan[cur+1].Part == p {
		return cur + 1
	}
	return cur
}

// textWriter writes one text track's segments for a job, from the cues
// its output reads or from the track's file.
//
// Cues arrive on the source's copied timestamps, which begin at the part's
// start time. That isn't zero in many MPEG-TS recordings. So each segment
// takes the cues in its span, shifted forward by that start, and cuts them
// to the span (subtitle.Segment). The cues keep their own times, just as
// the video keeps its timestamps, and the segment's timestamp map relates
// the two.
type textWriter struct {
	s      *Stream
	k      int         // the text track, in the spec's Text
	out    int         // the track's place in the job's request, or -1 when it's read from a file
	cues   []media.Cue // cues read so far that show in this segment or a later one
	broken bool        // the track couldn't be read, so the video goes on without it
}

// write puts segment i's text in place, from the cues read by then.
func (w *textWriter) write(out media.Output, i int) error {
	if w.out < 0 {
		return w.writeFile(i)
	}
	if !w.broken {
		cues, err := out.Cues(w.out)
		if err != nil {
			slog.Warn("transcode: text track unreadable", "file", w.s.spec.Parts[w.s.plan[i].Part].ID, "track", w.s.spec.Text[w.k].Track, "err", err)
			w.broken = true
		}
		w.cues = append(w.cues, cues...)
	}
	from, to := w.s.textSpan(i)
	b := subtitle.Segment(w.cues, from, to)
	w.cues = slices.DeleteFunc(w.cues, func(c media.Cue) bool { return c.End <= to })
	return atomicfile.Write(w.s.textPath(w.k, i), b)
}

// writeFile puts segment i's text in place from the track's file, which
// belongs to the first part.
func (w *textWriter) writeFile(i int) error {
	var cues []media.Cue
	if w.s.plan[i].Part == 0 {
		cues = w.s.fileCues[w.k]
	}
	from, to := w.s.textSpan(i)
	return atomicfile.Write(w.s.textPath(w.k, i), subtitle.Segment(cues, from, to))
}

// fileCues reads a text track's file and shifts its cues onto the first
// part's source timestamps, where an embedded track's cues would be. If the
// file can't be read, the text track is left empty and the video goes on.
func fileCues(t TextTrack, start time.Duration) []media.Cue {
	b, err := os.ReadFile(t.Path)
	var cues []media.Cue
	if err == nil {
		cues, err = subtitle.Cues(b, t.Format)
	}
	if err != nil {
		slog.Warn("transcode: subtitle file unreadable", "path", t.Path, "err", err)
		return nil
	}
	for i := range cues {
		cues[i].Start += start
		cues[i].End += start
	}
	return cues
}

// A job pauses once it's pauseLead ahead of the furthest segment the player
// has asked for, and resumes when the player is within resumeLead. A paused
// job keeps its process and its accelerator's session.
const (
	pauseLead  = 90 * time.Second
	resumeLead = 60 * time.Second
)

// commit renames a finished segment into place, and wakes its waiters and
// the waiters of any earlier segment. It pauses the job once it's far
// enough ahead of the player.
func (s *Stream) commit(j *job, i int, f *os.File) error {
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Rename(f.Name(), s.path(i)); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.done[i] = true
	if j.next == j.from {
		close(j.first)
	}
	j.next = i + 1
	for k, ch := range j.ready {
		if k <= i { // waiters for segments the job skipped past will look again
			close(ch)
			delete(j.ready, k)
		}
	}
	if !j.paused && s.job == j && s.leadLocked(j) >= pauseLead {
		if err := j.out.Pause(); err != nil {
			slog.Debug("transcode: can't pause the job", "err", err)
		} else {
			j.paused = true
			j.restLocked()
		}
	}
	return nil
}

// writeInit puts part p's init segment in place, only once. A stream keeps
// one rendition and one encoder, so every job's init segment for a part is
// the same.
func (s *Stream) writeInit(p int, b []byte) error {
	s.mu.Lock()
	have := s.initDone[p]
	s.mu.Unlock()
	if have {
		return nil
	}
	// A stopped job may still be writing its own copy, so each job writes
	// its own temporary file.
	if err := atomicfile.Write(filepath.Join(s.dir, initName(p)), b); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.initDone[p] {
		s.initDone[p] = true
		close(s.initReady[p])
	}
	return nil
}

// waitLocked returns a channel that's closed when segment i is on disk or
// the job ends.
func (j *job) waitLocked(i int) chan struct{} {
	ch, ok := j.ready[i]
	if !ok {
		ch = make(chan struct{})
		j.ready[i] = ch
	}
	return ch
}

// started waits for the job's first segment. It returns the job's error if
// the job ends without one.
func (j *job) started(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-j.first:
		return nil
	case <-j.exited:
		select {
		case <-j.first:
			return nil
		default:
			return j.err
		}
	}
}

// ended reports whether the job has exited.
func (j *job) ended() bool {
	select {
	case <-j.exited:
		return true
	default:
		return false
	}
}

// failed reports whether the job exited with a failure of its own. A
// failed job stays the stream's job, so requests report the failure and
// playback can step down instead of restarting it forever. A temporarily
// unreadable source or a cancellation isn't a failure, so the next request
// starts a new job.
func (j *job) failed() bool {
	return j.ended() && jobFailure(j.err)
}

// jobFailure reports whether err is a job's own failure, as classify
// produces it: ErrJobFailed or ErrAcceleratorFailed.
func jobFailure(err error) bool {
	return errors.Is(err, ErrJobFailed) || errors.Is(err, ErrAcceleratorFailed)
}

// workingLocked is how long the job has spent encoding, not counting
// pauses.
func (j *job) workingLocked() time.Duration {
	if j.began.IsZero() {
		return j.worked
	}
	return j.worked + time.Since(j.began)
}

// restLocked stops counting the job's working time, when it pauses or
// exits.
func (j *job) restLocked() {
	j.worked, j.began = j.workingLocked(), time.Time{}
}

// stopped reports whether an error only means the job was stopped, by a
// far seek, an idle kill, or the stream's end. It checks for the media
// package's own cancellation error as well as the context's.
func stopped(err error) bool {
	return errors.Is(err, context.Canceled) || errors.Is(err, media.ErrCancelled)
}
