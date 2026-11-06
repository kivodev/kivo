//go:build !linux

package scan

import "io/fs"

type dirKey struct{}

// keyOf can't tell a directory's identity off Linux, the supported
// platform. So on other platforms a directory reached twice through links
// isn't recognised.
func keyOf(fs.FileInfo) (dirKey, bool) { return dirKey{}, false }
