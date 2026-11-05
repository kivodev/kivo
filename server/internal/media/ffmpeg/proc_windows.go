package ffmpeg

import (
	"os"
	"os/exec"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

// prepare leaves exec's default cancellation, which is a plain kill.
// Windows has no process group to signal, and ffmpeg starts no children of
// its own.
func prepare(*exec.Cmd) {}

// job is a job object that kills every process in it when it closes.
// Windows closes it when the server exits, however it exits. It is zero if
// it couldn't be made.
var job = sync.OnceValue(func() windows.Handle {
	h, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return 0
	}
	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{
		BasicLimitInformation: windows.JOBOBJECT_BASIC_LIMIT_INFORMATION{LimitFlags: windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE},
	}
	if _, err := windows.SetInformationJobObject(h, windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info))); err != nil {
		_ = windows.CloseHandle(h)
		return 0
	}
	return h
})

// contain puts a started process in job, so it never outlives the server.
func contain(p *os.Process) {
	j := job()
	if j == 0 {
		return
	}
	h, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(p.Pid))
	if err != nil {
		return
	}
	defer windows.CloseHandle(h)
	_ = windows.AssignProcessToJobObject(j, h)
}

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
