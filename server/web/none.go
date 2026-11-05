//go:build !webui

package web

import "io/fs"

// Files is nil in a build without the webui tag: the server serves a
// placeholder page.
func Files() fs.FS { return nil }
