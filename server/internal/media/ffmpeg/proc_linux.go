package ffmpeg

import (
	"os/exec"
	"syscall"
)

// prepare puts the process in its own group, so cancelling also kills
// anything it started. It also has the kernel kill the process if the
// server dies. The death signal fires when the thread that started the
// process exits. That is safe because media never locks OS threads.
func prepare(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true, Pdeathsig: syscall.SIGKILL}
	cmd.Cancel = func() error { return killGroup(cmd) }
}

// ioprio_set(2)'s arguments: a process, and the best-effort class at its
// lowest level, 7. Unlike the idle class, it never starves on a busy disk.
const (
	ioprioWhoProcess = 1
	ioprioLowest     = 2<<13 | 7
)

func lowerIO(pid int) {
	_, _, _ = syscall.Syscall(syscall.SYS_IOPRIO_SET, ioprioWhoProcess, uintptr(pid), ioprioLowest)
}
