//go:build webui

// Package web is the built web client, embedded when the server is built
// with the webui tag after the client's own build has filled dist.
package web

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var dist embed.FS

// Files is the built client, index.html at its root.
func Files() fs.FS {
	files, _ := fs.Sub(dist, "dist")
	return files
}
