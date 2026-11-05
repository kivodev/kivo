package transcode

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/mmcdole/kivo/internal/fault"
	"github.com/mmcdole/kivo/internal/id"
	"github.com/mmcdole/kivo/internal/media"
)

var (
	// ErrStorageFull means the transcode disk is below its free-space
	// floor. The playback package refuses the play with that reason.
	ErrStorageFull = fault.New(fault.Internal, "transcode disk is below its free-space floor")
	// ErrUnsupported means the spec can't be planned. For example, a remux
	// needs each part's keyframes from RemuxKeyframes.
	ErrUnsupported = fault.New(fault.Internal, "can't plan this rendition")
	// ErrAcceleratorFailed means an accelerator reported busy or failed
	// before a job's first segment. This package never switches
	// accelerators. The playback package opens the stream's transcode again
	// in software instead.
	ErrAcceleratorFailed = fault.New(fault.Internal, "accelerator busy or failed before the first segment")
	// ErrJobFailed means the stream's job failed. The source is corrupt,
	// the rendition can't be made, or the encoder failed after the job's
	// first segment. The client falls back to a new play.
	ErrJobFailed = fault.New(fault.Internal, "job failed")
	// ErrNotFound means a playlist or segment name is invalid or out of
	// range.
	ErrNotFound = fault.New(fault.NotFound, "no such playlist or segment")
	// ErrClosed means the stream has ended.
	ErrClosed = fault.New(fault.Internal, "stream closed")
)

// Media is the part of the media package that this package uses.
type Media interface {
	Transcode(ctx context.Context, req media.TranscodeRequest) (media.Output, error)
	Convert(ctx context.Context, req media.ConvertRequest) error
}

// FileRef is a file a stream plays, described by stored probe results, so
// starting a job never runs ffprobe.
type FileRef struct {
	ID       id.ID
	Path     string
	Duration time.Duration
	Start    time.Duration // the file's first timestamp, where its copied timestamps begin
	Bitrate  int           // in bits per second, for the multivariant playlist
	// Keyframes are where copied video is cut into segments. They come
	// from RemuxKeyframes. Encoded video doesn't use them.
	Keyframes []media.Keyframe
}

// Spec is what a stream's job makes. The playback package chooses it.
type Spec struct {
	// Parts are the version's files in order, played as one timeline.
	// Every part has the same tracks as the first.
	Parts      []FileRef
	VideoTrack int
	Source     media.Track          // the video track as probed; required to encode video
	Video      media.VideoRendition // Codec is media.Copy for a remux or an audio-only transcode
	Audio      []media.AudioOut
	BurnIn     *media.SubtitleRef
	// Fonts are the font files a text BurnIn uses, wherever they're
	// stored. The job reads them from its own folder, named by BurnIn's
	// Fonts.
	Fonts []string
	// Text are the text tracks delivered with the video. A stream carries
	// each as a subtitle rendition, and a download's conversion carries
	// them in its file, in order.
	Text        []TextTrack
	Accelerator media.Accelerator // empty means the CPU
}

// TextTrack is a text subtitle track delivered with the video. A stream
// writes it as WebVTT segments that match the video's segments. A
// download's conversion carries it in its file.
//
// A stream's job reads an embedded track along with the video, so a
// player has it from the first segment, and after a far seek from the new
// job's first segment. A subtitle file, next to the video or downloaded
// for it, or an embedded track's extraction, belongs to the first part. A
// stream reads the whole file when it opens.
type TextTrack struct {
	ID       string               // the rendition's stable ID, the catalog track's
	Track    int                  // the source track's Index from Probe
	Path     string               // a subtitle file, read instead of Track
	Format   media.SubtitleFormat // the file's format: SRT, VTT or ASS
	Title    string               // what a player's own menu shows
	Language string               // a BCP 47 tag, if known
	Forced   bool                 // it shows only the parts in another language
	SDH      bool                 // it's for the deaf and hard of hearing
	Default  bool                 // the rendition the player shows from the start
}

// encoded reports whether the spec encodes video, rather than copying it.
func (s Spec) encoded() bool { return s.Video.Codec != media.Copy }

// accelerated reports whether the spec encodes video on an accelerator.
func (s Spec) accelerated() bool { return s.encoded() && s.Accelerator != "" }

// Deps are a Transcoder's collaborators and folders.
type Deps struct {
	Media     Media
	Keys      KeyframeSource
	Dir       string // cache/transcode
	Downloads string // cache/downloads
}

// Options configures a Transcoder.
type Options struct {
	Floor uint64 // the free bytes below which streams and conversions are refused; defaults to 5 GB
}

// idleTimeout is how long a stream goes without a request before its job is
// killed.
const idleTimeout = 2 * time.Minute

// Transcoder runs one ffmpeg job per play's stream, and runs download
// conversions. It only knows about files and renditions. It never knows
// about users, plays' owners, or which accelerator to use.
type Transcoder struct {
	deps Deps
	st   storage // the streams' segments
	dl   storage // the downloads' conversions
}

// New returns a transcoder, and clears whatever a crash left in d.Dir.
// Download conversions in d.Downloads are kept across restarts.
func New(d Deps, opt Options) (*Transcoder, error) {
	if opt.Floor == 0 {
		opt.Floor = defaultFloor
	}
	t := &Transcoder{
		deps: d,
		st:   storage{dir: d.Dir, floor: opt.Floor},
		dl:   storage{dir: d.Downloads, floor: opt.Floor},
	}
	if err := errors.Join(t.st.reset(), os.MkdirAll(t.dl.dir, 0o755)); err != nil {
		return t, err
	}
	return t, nil
}

// Open plans a stream's playlist and starts its job at the segment
// containing start, which is a position on the parts' joined timeline.
// When the spec encodes video, Open waits for the job's first segment. An
// accelerator that is busy or fails at the start is then reported here as
// ErrAcceleratorFailed, and the playback package can open the stream again
// in software.
func (t *Transcoder) Open(ctx context.Context, s Spec, start time.Duration) (*Stream, error) {
	plan, err := planSpec(s)
	if err != nil {
		return nil, err
	}
	dir, err := t.st.open()
	if err != nil {
		return nil, err
	}
	if s, err = s.withFonts(dir); err != nil {
		os.RemoveAll(dir)
		return nil, err
	}
	ss := &Stream{
		t: t, spec: s, plan: plan, dir: dir, playlists: renderPlaylists(s, plan),
		done: make([]bool, len(plan)), initDone: make([]bool, len(s.Parts)),
	}
	for range s.Parts {
		ss.initReady = append(ss.initReady, make(chan struct{}))
	}
	ss.fileCues = make([][]media.Cue, len(s.Text))
	for k, t := range s.Text {
		if t.Path != "" {
			ss.fileCues[k] = fileCues(t, s.Parts[0].Start)
		}
	}
	ss.mu.Lock()
	ss.idle = time.AfterFunc(idleTimeout, ss.idleStop)
	j := ss.startLocked(segmentAt(plan, start))
	ss.mu.Unlock()
	if s.encoded() {
		if err := j.started(ctx); err != nil {
			ss.Close()
			return nil, err
		}
	}
	return ss, nil
}

// planSpec lays out a spec's playlist, one part after another. Encoded
// video is cut on the fixed grid, and copied video at the source's
// keyframes.
func planSpec(s Spec) ([]span, error) {
	if len(s.Parts) == 0 {
		return nil, fmt.Errorf("%w: no parts", ErrUnsupported)
	}
	var plan []span
	var at time.Duration
	for p, f := range s.Parts {
		var spans []span
		switch {
		case s.encoded():
			spans = planGrid(f.Duration)
		case len(f.Keyframes) == 0:
			return nil, fmt.Errorf("%w: part %d has no keyframes to remux at", ErrUnsupported, p)
		default:
			spans = planKeyframes(f.Duration, f.Keyframes)
		}
		for _, sp := range spans {
			sp.Part, sp.At = p, at+sp.Start
			plan = append(plan, sp)
		}
		at += f.Duration
	}
	return plan, nil
}
