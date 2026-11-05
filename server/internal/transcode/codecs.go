package transcode

import (
	"fmt"
	"slices"
	"strings"

	"github.com/mmcdole/kivo/internal/media"
)

// codecs is the stream's CODECS attribute: the video and audio codec
// strings (RFC 6381), which let a player pick its decoders before the
// first segment. It's empty when any track's string isn't known, because
// a wrong string can stop a player from playing at all.
func (s Spec) codecs() string {
	var out []string
	if s.Video.Codec != "" {
		v := s.videoCodec()
		if v == "" {
			return ""
		}
		out = append(out, v)
	}
	for _, a := range s.Audio {
		c := a.Out.Codec
		if c == media.Copy {
			c = a.Source
		}
		v := audioCodec(c)
		if v == "" {
			return ""
		}
		if !slices.Contains(out, v) {
			out = append(out, v)
		}
	}
	return strings.Join(out, ",")
}

// videoCodec is the video's codec string. For copied video it's the
// source's string. For encoded video it's the encoder's profile at the
// level the picture size and frame rate need.
func (s Spec) videoCodec() string {
	if !s.encoded() {
		if s.Source.DolbyVisionProfile == 5 && s.Video.HDR == media.PassThrough {
			return "" // profile 5 has no base layer that another decoder can play
		}
		switch s.Source.Codec {
		case media.H264:
			return avcCodec(s.Source.Profile, s.Source.Level)
		case media.HEVC:
			return hevcCodec(s.Source.Profile, s.Source.Level)
		}
		return ""
	}
	w, h := s.outputSize()
	rate := s.Source.FrameRate
	if s.Video.FrameRate > 0 && s.Video.FrameRate < rate {
		rate = s.Video.FrameRate
	}
	switch s.Video.Codec {
	case media.H264:
		profile := "High"
		if s.Video.Profile == "main" {
			profile = "Main"
		}
		return avcCodec(profile, avcLevel(w, h, rate))
	case media.HEVC:
		return hevcCodec("Main", hevcLevel(w, h, rate))
	}
	return ""
}

// outputSize is the encoded picture's size. It's the source's size,
// scaled down to fit the rendition's bounds while keeping its aspect
// ratio.
func (s Spec) outputSize() (int, int) {
	w, h := float64(s.Source.Width), float64(s.Source.Height)
	scale := 1.0
	if s.Video.Width > 0 && w > 0 {
		scale = min(scale, float64(s.Video.Width)/w)
	}
	if s.Video.Height > 0 && h > 0 {
		scale = min(scale, float64(s.Video.Height)/h)
	}
	return int(w * scale), int(h * scale)
}

// avcProfiles maps H.264 profile names, as the prober reports them, to the
// profile_idc and constraint flags of an avc1 codec string.
var avcProfiles = map[string]string{
	"Constrained Baseline": "42E0",
	"Baseline":             "4200",
	"Main":                 "4D40",
	"High":                 "6400",
	"High 10":              "6E00",
}

// avcCodec is an H.264 codec string in the form avc1.PPCCLL. level uses
// the prober's numbering, such as 41 for 4.1.
func avcCodec(profile string, level int) string {
	p, ok := avcProfiles[profile]
	if !ok || level <= 0 || level > 0xff {
		return ""
	}
	return fmt.Sprintf("avc1.%s%02X", p, level)
}

// hevcCodec is an HEVC codec string for the Main or Main 10 profile at the
// main tier. level is general_level_idc, which is 30 times the level (153
// for 5.1).
func hevcCodec(profile string, level int) string {
	if level <= 0 {
		return ""
	}
	switch profile {
	case "Main":
		return fmt.Sprintf("hvc1.1.6.L%d.B0", level)
	case "Main 10":
		return fmt.Sprintf("hvc1.2.4.L%d.B0", level)
	}
	return ""
}

// codecLevel is a level an encoder may pick for a picture size and frame
// rate. The encoder picks the lowest level whose limits per frame and per
// second hold the picture.
type codecLevel struct {
	frame, second float64 // limits in macroblocks for H.264, or in luma samples for HEVC
	level         int
}

// avcLevels are H.264's levels from 3.1 up, as encoders pick them. An
// encode beyond 5.2 is labeled 5.2.
var avcLevels = []codecLevel{
	{3600, 108000, 31},
	{8192, 245760, 41},
	{8704, 522240, 42},
	{22080, 589824, 50},
	{36864, 983040, 51},
	{36864, 2073600, 52},
}

// hevcLevels are HEVC's levels from 3.1 up. An encode beyond 6.2 is
// labeled 6.2.
var hevcLevels = []codecLevel{
	{983040, 33177600, 93},
	{2228224, 66846720, 120},
	{2228224, 133693440, 123},
	{8912896, 267386880, 150},
	{8912896, 534773760, 153},
	{8912896, 1069547520, 156},
	{35651584, 1069547520, 180},
	{35651584, 2139095040, 183},
	{35651584, 4278190080, 186},
}

func avcLevel(w, h int, rate float64) int {
	mbs := float64(((w + 15) / 16) * ((h + 15) / 16))
	return pickLevel(avcLevels, mbs, mbs*rate)
}

func hevcLevel(w, h int, rate float64) int {
	samples := float64(w * h)
	return pickLevel(hevcLevels, samples, samples*rate)
}

func pickLevel(levels []codecLevel, frame, second float64) int {
	for _, l := range levels {
		if frame <= l.frame && second <= l.second {
			return l.level
		}
	}
	return levels[len(levels)-1].level
}

// audioCodecs maps the prober's audio codec names to codec strings. AAC is
// assumed to be AAC-LC, because players that decode AAC-LC also decode
// HE-AAC.
var audioCodecs = map[media.Codec]string{
	media.AAC: "mp4a.40.2",
	"ac3":     "ac-3",
	"eac3":    "ec-3",
	"mp3":     "mp4a.40.34",
	"opus":    "Opus",
	"flac":    "fLaC",
	"alac":    "alac",
}

func audioCodec(c media.Codec) string { return audioCodecs[c] }
