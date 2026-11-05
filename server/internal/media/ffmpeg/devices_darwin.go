package ffmpeg

import "github.com/mmcdole/kivo/internal/media"

// findDevices offers VideoToolbox, which every Mac has.
func (r *Runner) findDevices() ([]foundDevice, []media.AcceleratorProblem) {
	return []foundDevice{{b: videoToolbox, name: "Apple VideoToolbox"}}, nil
}
