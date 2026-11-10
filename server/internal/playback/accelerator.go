package playback

import (
	"context"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/mmcdole/kivo/internal/config"
	"github.com/mmcdole/kivo/internal/health"
	"github.com/mmcdole/kivo/internal/media"
)

// Accelerators is what the startup accelerator test found. It's the media
// package's runner, wired in cmd/kivo. It's empty until the test finishes,
// and the CPU always works in the meantime.
type Accelerators interface {
	Capabilities() media.Capabilities
}

// acceleratorFailure records the last time an accelerator failed a play,
// for the problems page. A failed accelerator is tested again at the next
// server start, so the problem shows until then, as long as it's recent.
type acceleratorFailure struct {
	mu          sync.Mutex
	at          time.Time
	accelerator media.Accelerator
	err         string
}

// failureShown is how long the problems page shows an accelerator failure.
const failureShown = 24 * time.Hour

// accelerators returns the accelerators plays may use under the
// accelerator setting, best first. When there are none, it also returns
// why the CPU is used.
func (s *planner) accelerators(cpu bool) ([]media.Caps, Reason) {
	caps := s.deps.Accelerators.Capabilities()
	accel := slices.Clone(caps.Accelerators)
	setting := s.deps.Settings.Get().Accelerator
	switch {
	case len(accel) == 0 && len(caps.Problems) == 0:
		return nil, "" // there's no accelerator, so the CPU is the only choice
	case setting == config.AcceleratorNone:
		return nil, ReasonNoAccelerator
	case cpu:
		return nil, ReasonAcceleratorFailed
	case setting != config.AcceleratorAuto:
		accel = slices.DeleteFunc(accel, func(a media.Caps) bool { return string(a.Accelerator) != setting })
	}
	if len(accel) == 0 {
		return nil, ReasonAcceleratorUnsupported
	}
	return accel, ""
}

// encoders lists the codecs a video transcode may produce. H.264 is always
// available, on an accelerator or the CPU. HEVC needs an accelerator,
// because the CPU's encoder is too slow for playback.
func (s *planner) encoders(cpu bool) []media.Codec {
	out := []media.Codec{media.H264}
	accel, _ := s.accelerators(cpu)
	if slices.ContainsFunc(accel, func(a media.Caps) bool { _, ok := a.Encoder(media.HEVC); return ok }) {
		out = append(out, media.HEVC)
	}
	return out
}

// chooseAccelerator chooses the encoder for a play's or a download's video
// transcode, if it has one, and records it on the decision's video track.
// It picks the best accelerator whose test passed the codec, plus tone
// mapping and subtitle drawing when the job needs them. Otherwise it falls
// back to the CPU with H.264, and records the reason when the server has
// an accelerator.
func (s *planner) chooseAccelerator(c *choice) {
	i := slices.IndexFunc(c.decision.Tracks, func(t TrackDecision) bool { return t.Type == media.Video && t.Selected && t.Transcoded() })
	if i < 0 {
		return
	}
	t, v := &c.decision.Tracks[i], c.decision.Tracks[i].Video
	burnIn := c.decision.burned()
	accel, why := s.accelerators(c.cpu)
	for _, codec := range []media.Codec{v.Codec, media.H264} {
		for _, a := range accel {
			if _, ok := a.Encoder(codec); !ok || v.HDR == media.ToneMap && a.ToneMap == "" || burnIn && a.BurnIn == "" {
				continue
			}
			if codec != v.Codec {
				v.Codec, v.Profile = media.H264, "high"
			}
			t.Accelerator, t.Encoder, t.SoftwareReason, t.Codec = a.Accelerator, a.Name, "", v.Codec
			return
		}
	}
	if why == "" && len(accel) > 0 {
		why = ReasonAcceleratorUnsupported
	}
	v.Codec, v.Profile = media.H264, "high"
	t.Accelerator, t.Encoder, t.SoftwareReason, t.Codec = "", "CPU", why, v.Codec
}

// acceleratorFailed records an accelerator failing a play or a download,
// for the log and the problems page.
func (s *planner) acceleratorFailed(c *choice, accelerator media.Accelerator, err error) {
	slog.Warn("playback: accelerator failed; using the CPU", "work", c.work, "accelerator", accelerator, "err", err)
	s.accelFailure.mu.Lock()
	defer s.accelFailure.mu.Unlock()
	s.accelFailure.at, s.accelFailure.accelerator, s.accelFailure.err = time.Now(), accelerator, err.Error()
}

// Health reports the accelerator problems that the transcoding settings
// and problems pages show. They are accelerators the server found but
// can't use, the accelerator setting being none, a chosen accelerator that
// isn't there, and a recent accelerator failure.
func (s *Service) Health(context.Context) []health.Problem {
	caps := s.deps.Accelerators.Capabilities()
	var out []health.Problem
	accelerated := len(caps.Accelerators) > 0
	// A GPU that can't be used is only a problem when no other GPU works.
	// With a working GPU, a second card (often a display-only card with no
	// encoder) is just listed with the accelerators.
	if len(caps.Problems) > 0 && !accelerated {
		var lines []string
		for _, p := range caps.Problems {
			lines = append(lines, p.Name+": "+problemText(p))
		}
		out = append(out, health.Problem{
			Component: "playback", Code: "gpuUnusable", Severity: health.Warning, RaisedAt: s.created,
			Message: "Hardware acceleration is unavailable. " + strings.Join(lines, ". ") + ".",
		})
	}
	switch setting := s.deps.Settings.Get().Accelerator; {
	case setting == config.AcceleratorNone && (accelerated || len(caps.Problems) > 0):
		out = append(out, health.Problem{
			Component: "playback", Code: "noAccelerator", Severity: health.Info, RaisedAt: s.created,
			Message: "Hardware acceleration is off. Video is transcoded on the CPU.",
		})
	case setting != config.AcceleratorNone && setting != config.AcceleratorAuto && caps.Tested &&
		!slices.ContainsFunc(caps.Accelerators, func(a media.Caps) bool { return string(a.Accelerator) == setting }):
		out = append(out, health.Problem{
			Component: "playback", Code: "acceleratorMissing", Severity: health.Warning, RaisedAt: s.created,
			Message: "The accelerator " + setting + " isn't available. Video is transcoded on the CPU.",
		})
	}
	if caps.Tested && len(caps.CPU.Encoders) == 0 {
		out = append(out, health.Problem{
			Component: "playback", Code: "cpuFailed", Severity: health.Error, RaisedAt: s.created,
			Message: "The CPU failed every test encode. Video that no accelerator can encode can't be transcoded.",
		})
	}
	s.accelFailure.mu.Lock()
	defer s.accelFailure.mu.Unlock()
	if f := &s.accelFailure; !f.at.IsZero() && time.Since(f.at) < failureShown {
		out = append(out, health.Problem{
			Component: "playback", Code: "gpuFailed", Severity: health.Warning, RaisedAt: f.at,
			Message: fmt.Sprintf("A stream switched to the CPU after accelerator %s failed: %s", f.accelerator, f.err),
		})
	}
	return out
}

// problemText says why an accelerator can't be used, for an admin.
func problemText(p media.AcceleratorProblem) string {
	switch p.Reason {
	case media.NoDeviceNode:
		return "device not found"
	case media.NoPermission:
		if p.Group != "" {
			return "permission denied for the device, which belongs to group " + p.Group
		}
		return "permission denied for the device"
	case media.NoDriver:
		return "driver not found"
	case media.TestFailed:
		return "test encode failed"
	}
	return string(p.Reason)
}
