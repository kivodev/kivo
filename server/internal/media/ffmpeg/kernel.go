package ffmpeg

import (
	"strconv"
	"strings"
)

// kernelVersion is the version of the Linux kernel the server runs on,
// which in a container is the host's. It is zero when it's unknown or the
// system isn't Linux.
type kernelVersion struct{ major, minor, patch int }

// parseKernelVersion reads the version at the start of a kernel release,
// such as "5.19.0-50-generic" or "6.6.78-Unraid". It returns zero when the
// release doesn't start with a major and a minor number.
func parseKernelVersion(release string) kernelVersion {
	parts := strings.SplitN(release, ".", 3)
	if len(parts) < 2 {
		return kernelVersion{}
	}
	var n [3]int
	for i, part := range parts {
		digits := part
		if end := strings.IndexFunc(part, func(r rune) bool { return r < '0' || r > '9' }); end >= 0 {
			digits = part[:end]
		}
		v, err := strconv.Atoi(digits)
		if err != nil {
			if i < 2 {
				return kernelVersion{}
			}
			break
		}
		n[i] = v
		if len(digits) < len(part) {
			if i == 0 {
				return kernelVersion{}
			}
			break // a suffix such as "-arch1" ends the version
		}
	}
	return kernelVersion{n[0], n[1], n[2]}
}

// less reports whether k is older than major.minor.patch.
func (k kernelVersion) less(major, minor, patch int) bool {
	if k.major != major {
		return k.major < major
	}
	if k.minor != minor {
		return k.minor < minor
	}
	return k.patch < patch
}

// hangsQSVAfterOpenCL reports whether k's i915 driver can hang the GPU when
// Quick Sync encodes frames that were decoded on the Intel GPU and
// tone-mapped with OpenCL, unless the encoder's async depth is 1. Kernels
// 5.18 to 6.1.3 can, except 6.0.18 and later 6.0 releases, which have the
// fix. https://github.com/intel/media-driver/issues/1456
func (k kernelVersion) hangsQSVAfterOpenCL() bool {
	if k.major == 6 && k.minor == 0 && k.patch >= 18 {
		return false
	}
	return !k.less(5, 18, 0) && k.less(6, 1, 4)
}
