//go:build !linux

package ffmpeg

// runningKernel returns zero: only a Linux kernel's version is used.
func runningKernel() kernelVersion { return kernelVersion{} }
