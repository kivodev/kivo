//go:build !windows

package config

// home is the folder the default config, data and cache folders are in:
// the working directory.
func home() string { return "" }

// tool is the default path to ffmpeg or ffprobe, found on the PATH.
func tool(name string) string { return name }
