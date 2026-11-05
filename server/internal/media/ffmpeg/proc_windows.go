package ffmpeg

import (
	"os"
	"os/exec"

	"golang.org/x/sys/windows"
)

// prepare leaves exec's default cancellation, which is a plain kill.
// Windows has no process group to signal, and ffmpeg starts no children of
// its own. A process that outlives the server ends when its output pipe
// closes.
func prepare(*exec.Cmd) {}

// lowerPriority runs a process below playback, in the below-normal
// priority class.
func lowerPriority(p *os.Process) {
	h, err := windows.OpenProcess(windows.PROCESS_SET_INFORMATION, false, uint32(p.Pid))
	if err != nil {
		return
	}
	defer windows.CloseHandle(h)
	_ = windows.SetPriorityClass(h, windows.BELOW_NORMAL_PRIORITY_CLASS)
}
