package ffmpeg

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"iter"
	"os"
	"sync"
	"sync/atomic"

	"github.com/mmcdole/kivo/internal/media"
	"github.com/mmcdole/kivo/internal/media/container"
	"github.com/mmcdole/kivo/internal/media/subtitle"
)

// The bundled ffmpeg build reads these keys from stdin. p stops the job
// reading its input, and any key starts it again. u is used by convention.
const (
	pauseKey  = 'p'
	resumeKey = 'u'
)

// Transcode starts one ffmpeg that writes the request's video and audio,
// muxed together, as fragmented MP4. The job runs until ctx ends or the
// output is closed. A failure that ffmpeg doesn't name is reported as an
// encoder failure.
//
// Video is encoded on the request's accelerator, or on the CPU, in the
// pipeline the device's test passed for the source's format. An
// accelerator that hasn't passed the test is refused.
func (r *Runner) Transcode(ctx context.Context, req media.TranscodeRequest) (media.Output, error) {
	var dev *device
	if req.Video.Codec != media.Copy {
		var err error
		if dev, err = r.device(req.Accelerator); err != nil {
			return nil, refuse("transcode "+req.Path, err)
		}
	}
	return r.transcode(ctx, req, dev)
}

// transcode starts req's job on dev, which is nil for copied video.
func (r *Runner) transcode(ctx context.Context, req media.TranscodeRequest, dev *device) (*output, error) {
	op := "transcode " + req.Path
	args, err := transcodeArgs(req, dev)
	if err != nil {
		return nil, refuse(op, err)
	}
	p, err := start(ctx, op, r.ffmpeg, args, spawn{stdin: true})
	if err != nil {
		return nil, err
	}
	o := &output{op: op, p: p, split: container.NewSplitter(p.stdout, req.Start)}
	for _, t := range req.Text {
		o.text = append(o.text, &textFile{path: t.Path})
	}
	return o, nil
}

// output is a running transcode job.
type output struct {
	op     string
	p      *proc
	mu     sync.Mutex // serializes reads of the job's output
	split  *container.Splitter
	ranged atomic.Bool
	keys   sync.Mutex // serializes writes to stdin

	textMu sync.Mutex
	text   []*textFile // the text tracks' scratch files
}

// textFile is the scratch file ffmpeg writes a text track to, and how much
// of it has been read.
type textFile struct {
	path    string
	at      int64  // how many bytes of it have been read
	pending []byte // bytes read that don't yet make a whole cue
}

func (o *output) Init() ([]byte, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	b, err := o.split.Init()
	if err != nil {
		return nil, o.failure(err)
	}
	return b, nil
}

func (o *output) Segments() iter.Seq2[media.Segment, error] {
	return func(yield func(media.Segment, error) bool) {
		if !o.ranged.CompareAndSwap(false, true) {
			yield(media.Segment{}, fmt.Errorf("media: %s: segments read twice", o.op))
			return
		}
		for {
			o.mu.Lock()
			seg, err := o.split.Next()
			o.mu.Unlock()
			if err == io.EOF {
				// The output ended cleanly. Whether the job succeeded is
				// for ffmpeg's exit status to say.
				if o.p.wait() != nil {
					yield(media.Segment{}, o.p.failure(o.op, media.ErrEncoderFailed, nil))
				}
				return
			}
			if err != nil {
				yield(media.Segment{}, o.failure(err))
				return
			}
			if !yield(seg, nil) {
				return
			}
		}
	}
}

// failure ends the job after its output couldn't be read, and explains why.
// When ffmpeg failed, its error output says more than the broken output.
func (o *output) failure(err error) error {
	o.p.cancel()
	_ = o.p.wait()
	return o.p.failure(o.op, media.ErrEncoderFailed, err)
}

// maxText bounds how much one read of the text track takes in. A feature
// film's subtitles are well under 1 MB.
const maxText = 16 << 20

// Cues reads what ffmpeg has written of text track i since the last call.
// The file appears once the track's first cue is read.
func (o *output) Cues(i int) ([]media.Cue, error) {
	o.textMu.Lock()
	defer o.textMu.Unlock()
	if i < 0 || i >= len(o.text) {
		return nil, nil
	}
	t := o.text[i]
	f, err := os.Open(t.path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	b, err := io.ReadAll(io.NewSectionReader(f, t.at, maxText))
	if err != nil {
		return nil, err
	}
	t.at += int64(len(b))
	t.pending = append(t.pending, b...)
	cues, n := subtitle.ReadSRT(t.pending)
	t.pending = t.pending[n:]
	return cues, nil
}

func (o *output) Pause() error { return o.key(pauseKey, "pause") }

func (o *output) Resume() error { return o.key(resumeKey, "resume") }

func (o *output) key(k byte, what string) error {
	o.keys.Lock()
	defer o.keys.Unlock()
	if o.p.closed.Load() {
		return &media.Error{Op: o.op, Kind: media.ErrCancelled, Detail: what}
	}
	_, err := o.p.stdin.Write([]byte{k})
	if errors.Is(err, io.ErrClosedPipe) {
		return &media.Error{Op: o.op, Kind: media.ErrCancelled, Detail: what}
	}
	if err != nil {
		return fmt.Errorf("%s ffmpeg's %s: %w", what, o.op, err)
	}
	return nil
}

func (o *output) Diagnostics() media.Diagnostics { return o.p.diagnostics() }

// Close kills the job's process group and returns once it is reaped, then
// removes the text tracks' scratch files.
func (o *output) Close() error {
	o.p.close()
	var errs []error
	for _, t := range o.text {
		if err := os.Remove(t.path); err != nil && !errors.Is(err, fs.ErrNotExist) {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}
