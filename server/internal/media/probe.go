package media

import (
	"math"
	"regexp"
	"strings"
	"time"
)

// ProbeResult describes a media file: its container, timing, tracks and
// chapters.
type ProbeResult struct {
	Container string        // such as "matroska,webm" or "mov,mp4,m4a,3gp,3g2,mj2"
	Start     time.Duration // the first timestamp, which is zero for most files
	Duration  time.Duration
	Bitrate   int // bits per second, over the whole file
	Tracks    []Track
	Chapters  []Chapter
	// Tags are the container's tags that describe an audiobook. They are
	// keyed by these names, whatever the container calls them: album,
	// artist, album_artist, composer, grouping, series, part, asin, date,
	// description, publisher, genre, track and disc. Only the tags the file
	// has are present.
	Tags map[string]string
}

// TrackKind is what a track carries.
type TrackKind string

// Track kinds.
const (
	Video      TrackKind = "video"
	Audio      TrackKind = "audio"
	Subtitle   TrackKind = "subtitle"
	Attachment TrackKind = "attachment" // fonts and other files carried in the container
	Data       TrackKind = "data"
)

// DynamicRange is the transfer a video's base layer uses.
type DynamicRange string

// Dynamic ranges.
const (
	SDR   DynamicRange = "sdr"
	HDR10 DynamicRange = "hdr10" // PQ
	HLG   DynamicRange = "hlg"
)

// Track is one track of a file. Fields that don't apply to its kind are
// zero. catalog stores its JSON form, with these snake_case keys, in
// media_track.info, and query reads it from there.
type Track struct {
	Index    int       `json:"index,omitempty"` // the container's stream index, which ffmpeg selects the track by
	Kind     TrackKind `json:"kind,omitempty"`
	Codec    Codec     `json:"codec,omitempty"`
	Profile  string    `json:"profile,omitempty"`  // as the codec names it, such as "Main 10", "High" or "LC"
	Level    int       `json:"level,omitempty"`    // as the codec numbers it, such as 41 for H.264 level 4.1
	Language string    `json:"language,omitempty"` // as tagged, usually ISO 639-2
	Title    string    `json:"title,omitempty"`
	Bitrate  int       `json:"bitrate,omitempty"` // bits per second, when the container records it

	Default         bool `json:"default,omitempty"`
	Forced          bool `json:"forced,omitempty"`
	HearingImpaired bool `json:"hearing_impaired,omitempty"`
	Commentary      bool `json:"commentary,omitempty"`      // as the file marks it
	VisualImpaired  bool `json:"visual_impaired,omitempty"` // an audio description, as the file marks it
	// AttachedPicture marks cover art stored as a video track.
	AttachedPicture bool `json:"attached_picture,omitempty"`

	Width     int          `json:"width,omitempty"`
	Height    int          `json:"height,omitempty"`
	BitDepth  int          `json:"bit_depth,omitempty"` // for audio too, when the codec records it
	FrameRate float64      `json:"frame_rate,omitempty"`
	Range     DynamicRange `json:"range,omitempty"`
	// DolbyVisionProfile is the Dolby Vision profile, such as 5, 7 or 8, or
	// zero when there is none. Range describes the base layer.
	DolbyVisionProfile int `json:"dolby_vision_profile,omitempty"`
	// DolbyVisionCompat is the base layer's signal compatibility ID: 1 for
	// HDR10, 2 for SDR, 4 for HLG, 6 for Blu-ray HDR10, and zero for none or
	// unknown.
	DolbyVisionCompat int    `json:"dolby_vision_compat,omitempty"`
	DolbyVisionEL     bool   `json:"dolby_vision_el,omitempty"` // the video has an enhancement layer
	Tag               string `json:"tag,omitempty"`             // the codec tag, such as "hvc1" or "hev1"; empty when unknown
	References        int    `json:"references,omitempty"`      // the reference frames it may use, when the codec records them
	Interlaced        bool   `json:"interlaced,omitempty"`
	Anamorphic        bool   `json:"anamorphic,omitempty"` // its pixels aren't square

	Channels      int    `json:"channels,omitempty"`
	ChannelLayout string `json:"channel_layout,omitempty"` // such as "5.1(side)" or "stereo"
	SampleRate    int    `json:"sample_rate,omitempty"`    // Hz
}

// SpecLevel is a video track's level as its codec's specification writes
// it, such as 4.1 or 5.1, from ffprobe's number for it. H.264 and VP9
// number levels ten times over, HEVC thirty times over, and AV1 by an
// index. It's zero when the level is unknown or the codec has no such
// numbering.
func (t Track) SpecLevel() float64 { return SpecLevel(t.Codec, t.Level) }

// SpecLevel converts ffprobe's level number for a codec into the
// specification's level. It's zero when it can't.
func SpecLevel(codec Codec, level int) float64 {
	if level <= 0 {
		return 0
	}
	switch strings.ToLower(string(codec)) {
	case "h264", "vp9":
		return float64(level) / 10
	case "hevc":
		return math.Round(float64(level)/3) / 10
	case "av1":
		return float64(2+level>>2) + float64(level&3)/10
	}
	return 0
}

// HDR reports whether a video track has high dynamic range: HDR10, HLG or
// Dolby Vision.
func (t Track) HDR() bool {
	return t.Range == HDR10 || t.Range == HLG || t.DolbyVisionProfile > 0
}

var commentaryWord = regexp.MustCompile(`(?i)\bcommentary\b`)

// IsCommentary reports whether a track is a commentary. Either the file
// marks it so, or its title says so, which is all most files do.
func (t Track) IsCommentary() bool {
	return t.Commentary || commentaryWord.MatchString(t.Title)
}

var descriptionWords = regexp.MustCompile(`(?i)\b(descriptive|audio description)\b`)

// IsDescription reports whether an audio track describes the picture, for
// viewers who can't see it. Either the file marks it so, or its title says
// so, as in "Descriptive Audio" or "Audio Description". Files probed before
// the mark was recorded have only their titles.
func (t Track) IsDescription() bool {
	return t.VisualImpaired || descriptionWords.MatchString(t.Title)
}

// AttachedFile is a file a container carries besides its tracks, such as a
// font for its styled subtitles or cover art.
type AttachedFile struct {
	Filename string
	MimeType string // as the container declares it
	Size     int64
	SHA256   string // the hash of its bytes, in lowercase hex
	Kind     AttachmentKind
}

// AttachmentKind is what an attachment's bytes are, judged from the bytes
// rather than what the container declares.
type AttachmentKind string

// Attachment kinds.
const (
	AttachmentFont  AttachmentKind = "font"
	AttachmentImage AttachmentKind = "image"
	AttachmentOther AttachmentKind = "other"
)

// Chapter is one chapter of a file.
type Chapter struct {
	Start time.Duration
	End   time.Duration
	Title string
}
