package ffmpeg

import (
	"context"
	"os/exec"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/mmcdole/kivo/internal/media"
)

// smiTimeout bounds nvidia-smi, so a hung driver doesn't hold up the
// accelerator test.
const smiTimeout = 10 * time.Second

// findDevices lists the NVIDIA GPUs that nvidia-smi reports, and the Intel
// GPUs that DXGI lists for Quick Sync.
func (r *Runner) findDevices() ([]foundDevice, []media.AcceleratorProblem) {
	found, problems := findNVIDIA()
	return append(found, intelDevices(adapters())...), problems
}

// findNVIDIA lists the NVIDIA GPUs that nvidia-smi reports. nvidia-smi
// comes with the NVIDIA driver, so a PC without the driver lists nothing.
// Each GPU is reached through CUDA by its index.
func findNVIDIA() ([]foundDevice, []media.AcceleratorProblem) {
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

// intelVendor is Intel's PCI vendor ID.
const intelVendor = 0x8086

// intelDevices offers each Intel adapter for Quick Sync, by its DXGI index.
// Two would share a name, so they're told apart by index.
func intelDevices(list []adapter) []foundDevice {
	list = slices.DeleteFunc(slices.Clone(list), func(a adapter) bool { return a.vendor != intelVendor })
	var found []foundDevice
	for _, a := range list {
		idx := strconv.Itoa(a.index)
		name := "Intel Quick Sync (QSV)"
		if len(list) > 1 {
			name += " (" + idx + ")"
		}
		found = append(found, foundDevice{b: qsvD3D11, node: idx, name: name})
	}
	return found
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
