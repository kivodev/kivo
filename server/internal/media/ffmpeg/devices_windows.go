package ffmpeg

import (
	"context"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/mmcdole/kivo/internal/media"
)

// smiTimeout bounds nvidia-smi, so a hung driver doesn't hold up the
// accelerator test.
const smiTimeout = 10 * time.Second

// findDevices lists the NVIDIA GPUs that nvidia-smi reports. nvidia-smi
// comes with the NVIDIA driver, so a PC without the driver lists nothing.
// Each GPU is reached through CUDA by its index.
//
// Intel Quick Sync and AMD AMF aren't found on Windows.
func (r *Runner) findDevices() ([]foundDevice, []media.AcceleratorProblem) {
	smi, err := exec.LookPath("nvidia-smi.exe")
	if err != nil {
		return nil, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), smiTimeout)
	defer cancel()
	out, err := run(ctx, "list NVIDIA GPUs", media.ErrUnsupported, smi, []string{"--query-gpu=index", "--format=csv,noheader"}, spawn{})
	if err != nil {
		return nil, []media.AcceleratorProblem{{Accelerator: acceleratorOf(nvenc, "0"), Backend: media.NVENC, Name: "NVIDIA GPU",
			Reason: media.NoDriver, Detail: "nvidia-smi can't reach the NVIDIA driver: " + err.Error()}}
	}
	return nvidiaDevices(string(out)), nil
}

// nvidiaDevices reads nvidia-smi's GPU indexes, one per line. Two GPUs
// would share a name, so they're told apart by index.
func nvidiaDevices(out string) []foundDevice {
	var idxs []string
	for line := range strings.Lines(out) {
		idx := strings.TrimSpace(line)
		if _, err := strconv.Atoi(idx); err == nil {
			idxs = append(idxs, idx)
		}
	}
	var found []foundDevice
	for _, idx := range idxs {
		name := "NVIDIA NVENC"
		if len(idxs) > 1 {
			name += " (" + idx + ")"
		}
		found = append(found, foundDevice{b: nvenc, node: idx, name: name})
	}
	return found
}
