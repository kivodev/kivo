//go:build unix

package ffmpeg

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
)

// killGroup kills the process's group. exec calls it when the context ends,
// at worst just as the leader is reaped. Process IDs are handed out in
// sequence, so the group's ID isn't reused within that moment. A stream in
// progress has nothing worth flushing, so there is no gentler signal first.
func killGroup(cmd *exec.Cmd) error {
	err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	if errors.Is(err, syscall.ESRCH) {
		return os.ErrProcessDone
	}
	return err
}

// contain does nothing. Ending the process with the server is prepare's
// job here.
func contain(*os.Process) {}

// backgroundNice is the nice value of background processes. It is the
// value nice(1) sets by default.
const backgroundNice = 10

// lowerPriority runs a process below playback. It is niced for the CPU,
// and on Linux it gets the lowest best-effort I/O priority. It doesn't
// matter if the process has already exited.
func lowerPriority(p *os.Process) {
	_ = syscall.Setpriority(syscall.PRIO_PROCESS, p.Pid, backgroundNice)
	lowerIO(p.Pid)
}
