//go:build unix && !linux

package ffmpeg

import (
	"os/exec"
	"syscall"
)

// prepare puts the process in its own group, so cancelling also kills
// anything it started. There is no parent-death signal here, so a process
// that outlives the server ends when its output pipe closes.
func prepare(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return killGroup(cmd) }
}

// lowerIO does nothing, because I/O priorities exist only on Linux.
func lowerIO(int) {}
