package ffmpeg

import (
	"errors"
	"fmt"
	"sync/atomic"

	"github.com/mmcdole/kivo/internal/media"
)

// Runner runs the ffmpeg and ffprobe binaries it was given.
type Runner struct {
	ffmpeg, ffprobe string
	root            string // where device nodes are looked for, which is "/" or a test's tree
	kernel          kernelVersion
	hw              atomic.Pointer[accelerators]
	chromaprintState
}

// New returns a Runner for the binaries at the given paths. They should be
// the bundled ffmpeg build, because pausing a job needs its pause command,
// and tone mapping and drawing subtitles on a device need its filters.
func New(ffmpeg, ffprobe string) *Runner {
	return &Runner{ffmpeg: ffmpeg, ffprobe: ffprobe, root: "/", kernel: runningKernel()}
}

// accelerators holds what the last accelerator test found.
type accelerators struct {
	caps    media.Capabilities
	devices map[media.Accelerator]*device
}

// Capabilities reports what the last accelerator test found. It is empty
// until DetectAccelerators has run.
func (r *Runner) Capabilities() media.Capabilities {
	if hw := r.hw.Load(); hw != nil {
		return hw.caps
	}
	return media.Capabilities{}
}

// device returns the record for d. The CPU always has one. An accelerator
// has one only once it has passed the accelerator test.
func (r *Runner) device(d media.Accelerator) (*device, error) {
	if _, _, err := parseAccelerator(d); err != nil {
		return nil, err
	}
	if hw := r.hw.Load(); hw != nil && hw.devices[d] != nil {
		return hw.devices[d], nil
	}
	if d == "" {
		return untestedCPU(), nil
	}
	return nil, unsupported("%s hasn't passed the accelerator test", d)
}

// unsupported returns an error for a request that can't be met, described
// in a few words. The exported call that refuses the request adds its own
// name with refuse.
func unsupported(format string, a ...any) error {
	return &media.Error{Kind: media.ErrUnsupported, Detail: fmt.Sprintf(format, a...)}
}

// refuse returns err, a refusal made by unsupported, as an error of op.
func refuse(op string, err error) error {
	var me *media.Error
	if errors.As(err, &me) {
		me.Op = op
	}
	return err
}
