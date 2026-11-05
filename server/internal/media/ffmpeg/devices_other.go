//go:build !linux && !darwin

package ffmpeg

import "github.com/mmcdole/kivo/internal/media"

// findDevices finds nothing. Accelerators are supported on Linux, and on
// macOS as best effort.
func (r *Runner) findDevices() ([]foundDevice, []media.AcceleratorProblem) {
	return nil, nil
}
