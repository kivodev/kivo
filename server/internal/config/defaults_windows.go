package config

import (
	"os"
	"path/filepath"

	"golang.org/x/sys/windows"
)

// home is the folder the default config, data and cache folders are in:
// %ProgramData%\Kivo.
func home() string {
	dir, err := windows.KnownFolderPath(windows.FOLDERID_ProgramData, 0)
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "Kivo")
}

// tool is the default path to ffmpeg or ffprobe: the one in the ffmpeg
// folder beside kivo.exe if there is one, or else the one on the PATH.
func tool(name string) string {
	exe, err := os.Executable()
	if err != nil {
		return name
	}
	bundled := filepath.Join(filepath.Dir(exe), "ffmpeg", name+".exe")
	if _, err := os.Stat(bundled); err != nil {
		return name
	}
	return bundled
}
