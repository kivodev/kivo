package media

import (
	"regexp"
	"strings"
)

var layoutNumbers = regexp.MustCompile(`^\d+\.\d+(\.\d+)?$`)

// Layout returns an ffmpeg channel layout as its counts of main and LFE
// channels. It returns "5.1" for "5.1(side)", "4.0" for "quad", "2.0" for
// "stereo", and "7.1.4" unchanged. It returns "" for an unknown or empty
// layout.
func Layout(name string) string {
	name, _, _ = strings.Cut(strings.TrimSpace(name), "(")
	switch name {
	case "mono":
		return "1.0"
	case "stereo", "downmix":
		return "2.0"
	case "quad":
		return "4.0"
	case "hexagonal":
		return "6.0"
	case "octagonal":
		return "8.0"
	}
	if layoutNumbers.MatchString(name) {
		return name
	}
	return ""
}

// AudioProfile returns the part of an audio track's ffmpeg profile that
// people choose a track by, beyond its codec. It returns "atmos" for Dolby
// Atmos on TrueHD or E-AC-3, and "dtsX", "dtsHdMa" or "dtsHdHra" for DTS.
// It returns "" for any other profile or none.
func AudioProfile(profile string) string {
	switch {
	case strings.Contains(profile, "DTS:X"):
		return "dtsX"
	case strings.Contains(profile, "Atmos"):
		return "atmos"
	case strings.HasPrefix(profile, "DTS-HD MA"):
		return "dtsHdMa"
	case strings.HasPrefix(profile, "DTS-HD HRA"):
		return "dtsHdHra"
	}
	return ""
}
