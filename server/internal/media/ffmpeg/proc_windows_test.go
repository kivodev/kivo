package ffmpeg

import (
	"slices"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

// A process the server starts is in a job that kills it when the job's
// last handle closes, which Windows does when the server exits.
func TestProcessIsInTheKillOnCloseJob(t *testing.T) {
	p, err := start(t.Context(), "test", "ping.exe", []string{"-n", "30", "127.0.0.1"}, spawn{})
	if err != nil {
		t.Fatal(err)
	}
	defer p.close()

	var limits windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION
	if err := windows.QueryInformationJobObject(job(), windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&limits)), uint32(unsafe.Sizeof(limits)), nil); err != nil {
		t.Fatal(err)
	}
	if limits.BasicLimitInformation.LimitFlags&windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE == 0 {
		t.Error("the job doesn't kill its processes when it closes")
	}

	// JOBOBJECT_BASIC_PROCESS_ID_LIST, with room for this test's processes.
	var list struct {
		assigned, listed uint32
		pids             [64]uintptr
	}
	if err := windows.QueryInformationJobObject(job(), windows.JobObjectBasicProcessIdList,
		uintptr(unsafe.Pointer(&list)), uint32(unsafe.Sizeof(list)), nil); err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(list.pids[:list.listed], uintptr(p.cmd.Process.Pid)) {
		t.Errorf("process %d isn't in the job, which holds %v", p.cmd.Process.Pid, list.pids[:list.listed])
	}
}
