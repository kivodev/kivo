package ffmpeg

import "golang.org/x/sys/unix"

// runningKernel returns the running kernel's version, or zero when it
// can't be read.
func runningKernel() kernelVersion {
	var u unix.Utsname
	if unix.Uname(&u) != nil {
		return kernelVersion{}
	}
	return parseKernelVersion(unix.ByteSliceToString(u.Release[:]))
}
