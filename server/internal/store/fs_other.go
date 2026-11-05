//go:build !linux && !windows

package store

// Linux is the supported platform. On other platforms these checks are
// skipped.

func networkFS(string) string { return "" }

var freeBytes = func(string) (int64, bool) { return 0, false }
