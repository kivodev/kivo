package media

import (
	"strings"
	"time"
)

// Codec names a codec in media terms, such as "h264", "aac" or "subrip".
// Probe reports whatever codec a track uses. A rendition may only name the
// codecs below.
type Codec string

// Codecs a rendition can ask for.
const (
	Copy Codec = "copy" // pass the source track through unchanged
	H264 Codec = "h264"
	HEVC Codec = "hevc"
	AAC  Codec = "aac"
	AC3  Codec = "ac3"  // Dolby Digital
	EAC3 Codec = "eac3" // Dolby Digital Plus
)

// HDRMode says what a transcode does with high dynamic range.
type HDRMode int

const (
	// PassThrough keeps the source's dynamic range. Encoded video is 8-bit,
	// so it can only pass SDR through.
	PassThrough HDRMode = iota
	// ToneMap converts HDR10, HLG and Dolby Vision to SDR. Dolby Vision
	// profile 7 is stripped to its HDR10 base first. Its metadata describes
	// the base and enhancement layers together, but only the base is
	// decoded.
	ToneMap
	// StripDolbyVision drops the Dolby Vision metadata from copied video,
	// keeping the HDR10 base.
	StripDolbyVision
)

// Fidelity is the picture a conversion aims for, so easy scenes take fewer
// bits than the bitrate cap allows. Each encoder reaches it
// with its own quality value. A transcode fills up to its cap whatever its
// Fidelity.
type Fidelity string

const (
	// FidelityHigh is for a download watched on any screen. It's the zero
	// value.
	FidelityHigh Fidelity = ""
	// FidelityStandard is for a download watched on a phone.
	FidelityStandard Fidelity = "standard"
)

// VideoRendition is the video a transcode produces. Only Codec and HDR are
// read when Codec is Copy.
//
// Encoded video is 8-bit H.264 or HEVC with its keyframes on the request's
// segment grid. Width and Height bound the picture, which keeps the
// source's aspect ratio and is never enlarged. FrameRate caps the source's
// frame rate. Bitrate caps the video and is required: a transcode's
// average never exceeds it. Fidelity is what a conversion's encoder aims
// for, under the cap where its quality mode allows one.
type VideoRendition struct {
	Codec     Codec
	Profile   string // for H.264, "high" (the default) or "main"; for HEVC, "main"
	BitDepth  int    // 8, or zero, which means 8
	Width     int    // zero for no bound
	Height    int    // zero for no bound
	FrameRate float64
	Bitrate   int // bits per second
	Fidelity  Fidelity
	HDR       HDRMode
}

// AudioRendition is one audio track a transcode produces. A zero Channels or
// SampleRate keeps the source's value. A zero Bitrate uses the encoder's
// default.
type AudioRendition struct {
	Codec      Codec
	Channels   int
	Bitrate    int // bits per second
	SampleRate int // Hz
	// DialogueBoost makes speech easier to follow. When the source has a
	// centre channel, where dialogue is, that channel is raised. Then loud
	// passages are compressed and the peaks limited. It applies only when
	// encoding.
	DialogueBoost bool
}

// AudioOut is one source audio track and how to render it. Encoding a track
// of more than two channels down to two keeps dialogue clear by boosting
// the centre channel. A plain downmix leaves dialogue too quiet beside the
// effects.
type AudioOut struct {
	Track          int    // the source track's Index from Probe
	Source         Codec  // the source track's codec, as Probe names it
	SourceChannels int    // the source track's channel count, as Probe counts it
	Language       string // the source track's language as a BCP 47 tag, if known
	Out            AudioRendition
}

// SubtitleRef names a subtitle to burn into the video. An image subtitle,
// such as PGS or DVD, is drawn from the source's track. A text subtitle,
// such as SRT, ASS or WebVTT, is drawn from a file. The file is either an
// subtitle file or the track as the subtitles package extracted it,
// because drawing text means reading every event in the file first.
type SubtitleRef struct {
	Track int    // the image subtitle's Index from Probe, read when Path is empty
	Path  string // the text subtitle file
	Fonts string // a directory of the fonts the text subtitle uses, gathered from the font store
	// Width and Height are the image subtitle's canvas, from Probe, or
	// zero when unknown.
	Width, Height int
}

// Accelerator names the accelerator that encodes video, as Capabilities
// lists it, such as "qsv:/dev/dri/renderD128" or "nvenc:0". The zero value
// means the CPU. Media never chooses an accelerator itself and never falls
// back to another.
type Accelerator string

// CuesOut is an embedded text subtitle track that a transcode reads
// alongside its video, so the cues come out with the segments. Subtitles
// then start with the first segment and follow a seek, without first
// reading the whole file as extraction must. Output.Cues returns each
// track's cues.
type CuesOut struct {
	Track int    // the source track's Index from Probe, which is SubRip, ASS, WebVTT or mov_text
	Path  string // a scratch file the job writes the track to as it reads it; Close removes it
}

// TextOut is a text subtitle that a conversion writes as a text track of
// the output file. It comes from an embedded track of the source, or from a
// separate SubRip, ASS or WebVTT file.
type TextOut struct {
	Track    int    // the source track's Index from Probe, used when Path is empty
	Path     string // a subtitle file, next to the media or downloaded for it
	Title    string // the name a player's own menu shows
	Language string // a BCP 47 tag, if known
}

// TranscodeRequest is one transcode job. It reads one file and muxes the
// video and every selected audio track into fragmented MP4 segments. The
// cues of its text subtitle tracks come out beside them.
type TranscodeRequest struct {
	Path string
	// Start is where output begins on the source's timeline. Copied video
	// can only begin on a source keyframe, so output begins on the keyframe
	// the input seek lands on. Encoded video begins at Start, which should
	// be a grid point.
	Start time.Duration
	// Seek, for copied video, is where the input seek goes when Start is a
	// source keyframe: past Start, before the keyframe after it. ffmpeg's
	// input seek lands on the last keyframe before the time it's given,
	// less a margin for B-frames, so a seek to Start itself can land a
	// keyframe early. With Seek set, the audio and text begin where the
	// video does. Zero seeks to Start.
	Seek time.Duration
	// SegmentDuration is the grid encoded video places its keyframes on,
	// counted from Start. Copied video keeps the source's keyframes.
	SegmentDuration time.Duration
	VideoTrack      int // the source track's Index from Probe
	// Source is the video track as Probe described it. A video transcode
	// reads the codec, bit depth, size, frame rate and dynamic range from
	// here, rather than probing the file again.
	Source      Track
	Video       VideoRendition
	Audio       []AudioOut
	BurnIn      *SubtitleRef
	Text        []CuesOut
	Accelerator Accelerator
}

// ConvertRequest is one conversion. It reads one file and muxes the video
// and every selected audio track into one MP4 file that Apple's players
// play. Copied HEVC is tagged hvc1, as encoded HEVC is. A conversion runs
// from the start of the file as fast as its accelerator or the CPU allows,
// at low OS priority.
type ConvertRequest struct {
	Path       string
	VideoTrack int   // the source track's Index from Probe
	Source     Track // the video track as Probe described it, whether copied or encoded
	Video      VideoRendition
	Audio      []AudioOut
	BurnIn     *SubtitleRef
	// Text lists, in order, the text subtitles the output carries as its
	// own text tracks. They are for players that don't draw subtitle files
	// themselves.
	Text        []TextOut
	Accelerator Accelerator
	Dst         string // the output file; anything already there is replaced
	// Reached, if set, is called about twice a second with how far the
	// output has got on the source's timeline.
	Reached func(time.Duration)
}

// SubtitleFormat is a subtitle's format. It decides the file a subtitle is
// written as and what a client draws.
type SubtitleFormat string

// Subtitle formats.
const (
	SRT SubtitleFormat = "srt" // SubRip
	VTT SubtitleFormat = "vtt" // WebVTT
	ASS SubtitleFormat = "ass" // ASS or SSA, styled
	PGS SubtitleFormat = "pgs" // Blu-ray image subtitles, written as .sup
)

// Ext returns the format's file extension without the dot. It is "sup" for
// PGS and the format's name for the rest.
func (f SubtitleFormat) Ext() string {
	if f == PGS {
		return "sup"
	}
	return string(f)
}

// Text reports whether the format is text, which converts to WebVTT.
func (f SubtitleFormat) Text() bool { return f == SRT || f == VTT || f == ASS }

// SubtitleFormat returns the format of a subtitle track whose codec, as
// Probe names it, is c. The plain text codecs (SubRip, mov_text and text)
// are SRT. ASS and SSA are ASS. WebVTT and PGS map to themselves. Other
// subtitle codecs, such as DVD and DVB images, have no format.
func (c Codec) SubtitleFormat() (SubtitleFormat, bool) {
	switch strings.ToLower(string(c)) {
	case "subrip", "srt", "mov_text", "text":
		return SRT, true
	case "webvtt", "vtt":
		return VTT, true
	case "ass", "ssa":
		return ASS, true
	case "hdmv_pgs_subtitle", "pgs", "sup":
		return PGS, true
	}
	return "", false
}

// SubtitleOut is one embedded subtitle track to extract and the file it
// becomes. A plain text track becomes SRT. ASS and PGS tracks keep their
// format.
type SubtitleOut struct {
	Track  int // the source track's Index from Probe
	Format SubtitleFormat
	Path   string
}

// SubtitlesRequest extracts embedded subtitle tracks in one read of the
// file. One read serves them all, because a track's events are spread
// through the whole file.
type SubtitlesRequest struct {
	Path   string
	Tracks []SubtitleOut
}

// PreviewsRequest makes scrub previews. It takes a frame every Interval
// from keyframes, so only keyframes are decoded. Frames are scaled to Width
// pixels wide, never enlarged, and tiled Columns by Rows into WebP sheets.
// HDR is tone-mapped.
type PreviewsRequest struct {
	Path       string
	VideoTrack int   // the source track's Index from Probe
	Source     Track // the video track as Probe described it, for its size and dynamic range
	Interval   time.Duration
	Width      int
	Columns    int
	Rows       int
	Dir        string // where the sheets are written, named as Previews.SheetNames
}

// ImageOut is one re-encoded WebP copy of an image, written to Path. It is
// Width pixels wide, but never wider than the source.
type ImageOut struct {
	Width int
	Path  string
}

// SampleRequest reads a stretch of a file to find its skip markers. It
// returns a Chromaprint fingerprint and the silences of one audio track.
// When Video is set, it also returns the picture of every video keyframe in
// the stretch.
type SampleRequest struct {
	Path       string
	AudioTrack int // the track's Index from Probe
	Start      time.Duration
	Length     time.Duration
	Video      *SampleVideo
}

// SampleVideo is the video a SampleRequest measures. It measures the center
// of each keyframe, scaled to SampleWidth pixels wide. The center is the
// middle 90% of the width and 80% of the height, which skips letterboxing
// and corner logos.
type SampleVideo struct {
	Track  int   // the track's Index from Probe
	Source Track // the track as Probe described it
	// Accelerator decodes the keyframes on a device, with its backend's
	// decoder. Empty decodes them on the CPU.
	Accelerator Accelerator
}
