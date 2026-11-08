package ffmpeg

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/mmcdole/kivo/internal/media"
)

// Probe describes the file at path with ffprobe. A failure that ffprobe
// doesn't name is reported as unreadable, so the caller tries again.
func (r *Runner) Probe(ctx context.Context, path string) (media.ProbeResult, error) {
	op := "probe " + path
	out, err := run(ctx, op, media.ErrSourceUnreadable, r.ffprobe, probeArgs(path), spawn{background: true})
	if err != nil {
		return media.ProbeResult{}, err
	}
	res, err := parseProbe(out)
	if err != nil {
		return media.ProbeResult{}, &media.Error{Op: op, Kind: media.ErrSourceCorrupt, Detail: "ffprobe output: " + err.Error()}
	}
	return res, nil
}

// probeJSON is the part of ffprobe's JSON output Kivo reads.
type probeJSON struct {
	Format struct {
		FormatName string            `json:"format_name"`
		StartTime  string            `json:"start_time"`
		Duration   string            `json:"duration"`
		BitRate    string            `json:"bit_rate"`
		Tags       map[string]string `json:"tags"`
	} `json:"format"`
	Streams  []probeStream `json:"streams"`
	Chapters []struct {
		StartTime string            `json:"start_time"`
		EndTime   string            `json:"end_time"`
		Tags      map[string]string `json:"tags"`
	} `json:"chapters"`
}

// probeStream is one stream of ffprobe's JSON output.
type probeStream struct {
	Index            int               `json:"index"`
	CodecType        string            `json:"codec_type"`
	CodecName        string            `json:"codec_name"`
	Profile          string            `json:"profile"`
	Level            int               `json:"level"`
	CodecTag         string            `json:"codec_tag_string"`
	Refs             int               `json:"refs"`
	FieldOrder       string            `json:"field_order"`
	SampleAspect     string            `json:"sample_aspect_ratio"`
	Width            int               `json:"width"`
	Height           int               `json:"height"`
	PixFmt           string            `json:"pix_fmt"`
	BitsPerRawSample string            `json:"bits_per_raw_sample"`
	AvgFrameRate     string            `json:"avg_frame_rate"`
	RFrameRate       string            `json:"r_frame_rate"`
	ColorTransfer    string            `json:"color_transfer"`
	SampleRate       string            `json:"sample_rate"`
	Channels         int               `json:"channels"`
	ChannelLayout    string            `json:"channel_layout"`
	BitRate          string            `json:"bit_rate"`
	Disposition      map[string]int    `json:"disposition"`
	Tags             map[string]string `json:"tags"`
	SideDataList     []struct {
		SideDataType string `json:"side_data_type"`
		DVProfile    int    `json:"dv_profile"`
		DVCompat     int    `json:"dv_bl_signal_compatibility_id"`
		DVEL         int    `json:"el_present_flag"`
	} `json:"side_data_list"`
}

var kinds = map[string]media.TrackKind{
	"video":      media.Video,
	"audio":      media.Audio,
	"subtitle":   media.Subtitle,
	"attachment": media.Attachment,
}

func parseProbe(b []byte) (media.ProbeResult, error) {
	var p probeJSON
	if err := json.Unmarshal(b, &p); err != nil {
		return media.ProbeResult{}, err
	}
	res := media.ProbeResult{
		Container: p.Format.FormatName,
		Start:     parseSeconds(p.Format.StartTime),
		Duration:  parseSeconds(p.Format.Duration),
		Bitrate:   atoi(p.Format.BitRate),
	}
	for name, aliases := range bookTags {
		for _, a := range aliases {
			if v := strings.TrimSpace(tag(p.Format.Tags, a)); v != "" {
				if res.Tags == nil {
					res.Tags = map[string]string{}
				}
				res.Tags[name] = v
				break
			}
		}
	}
	for _, s := range p.Streams {
		t := media.Track{
			Index:           s.Index,
			Kind:            kinds[s.CodecType],
			Codec:           media.Codec(s.CodecName),
			Profile:         s.Profile,
			Level:           max(s.Level, 0), // ffprobe says -99 for unknown
			Language:        tag(s.Tags, "language"),
			Title:           tag(s.Tags, "title"),
			Bitrate:         atoi(s.BitRate),
			Default:         s.Disposition["default"] == 1,
			Forced:          s.Disposition["forced"] == 1,
			HearingImpaired: s.Disposition["hearing_impaired"] == 1,
			Commentary:      s.Disposition["comment"] == 1,
			VisualImpaired:  s.Disposition["visual_impaired"] == 1,
			AttachedPicture: s.Disposition["attached_pic"] == 1,
			Channels:        s.Channels,
			ChannelLayout:   s.ChannelLayout,
			SampleRate:      atoi(s.SampleRate),
		}
		if t.Kind == "" {
			t.Kind = media.Data
		}
		if t.Bitrate == 0 {
			t.Bitrate = atoi(tag(s.Tags, "BPS")) // mkvmerge's statistics tags
		}
		if t.Kind == media.Audio {
			t.BitDepth = atoi(s.BitsPerRawSample)
		}
		if t.Kind == media.Video {
			videoFacts(&t, s)
		}
		res.Tracks = append(res.Tracks, t)
	}
	for _, c := range p.Chapters {
		res.Chapters = append(res.Chapters, media.Chapter{
			Start: parseSeconds(c.StartTime),
			End:   parseSeconds(c.EndTime),
			Title: tag(c.Tags, "title"),
		})
	}
	return res, nil
}

// videoFacts fills in a video track's facts from its stream.
func videoFacts(t *media.Track, s probeStream) {
	t.Width, t.Height = s.Width, s.Height
	t.BitDepth = bitDepth(s.BitsPerRawSample, s.PixFmt)
	t.Tag = codecTag(s.CodecTag)
	t.References = s.Refs
	t.Interlaced = s.FieldOrder != "" && s.FieldOrder != "progressive" && s.FieldOrder != "unknown"
	t.Anamorphic = anamorphic(s.SampleAspect)
	t.PixelFormat = s.PixFmt
	t.FrameRate = rate(s.AvgFrameRate)
	if t.FrameRate == 0 {
		t.FrameRate = rate(s.RFrameRate)
	}
	t.Range = dynamicRange(s.ColorTransfer)
	for _, sd := range s.SideDataList {
		if sd.SideDataType == "DOVI configuration record" {
			t.DolbyVisionProfile, t.DolbyVisionCompat, t.DolbyVisionEL = sd.DVProfile, sd.DVCompat, sd.DVEL == 1
		}
	}
}

// bookTags are the container tags kept for describing audiobooks. Each is
// kept under one name, with the names containers use for it in order of
// preference. MP4's movement name and number (mvnm, mvin) are the series
// and part. Audible's ASIN is written as asin or AUDIBLE_ASIN.
var bookTags = map[string][]string{
	"album":        {"album"},
	"artist":       {"artist"},
	"album_artist": {"album_artist"},
	"composer":     {"composer"},
	"grouping":     {"grouping"},
	"series":       {"series", "mvnm", "movementname"},
	"part":         {"part", "series-part", "mvin", "movement"},
	"asin":         {"asin", "audible_asin"},
	"date":         {"date", "year"},
	"description":  {"description", "comment"},
	"publisher":    {"publisher"},
	"genre":        {"genre"},
	"track":        {"track"},
	"disc":         {"disc"},
}

// tag looks up a tag ignoring case, because containers disagree on case.
func tag(tags map[string]string, key string) string {
	if v, ok := tags[key]; ok {
		return v
	}
	for k, v := range tags {
		if strings.EqualFold(k, key) {
			return v
		}
	}
	return ""
}

func parseSeconds(s string) time.Duration {
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return time.Duration(f * float64(time.Second)).Round(time.Microsecond)
}

func atoi(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}

// rate parses a frame rate such as "24000/1001".
func rate(s string) float64 {
	num, den, ok := strings.Cut(s, "/")
	n, err1 := strconv.ParseFloat(num, 64)
	d, err2 := strconv.ParseFloat(den, 64)
	if !ok || err1 != nil || err2 != nil || d == 0 {
		return 0
	}
	return n / d
}

// bitDepth returns the coded bit depth, falling back to the depth of the
// pixel format, such as yuv420p10le.
func bitDepth(raw, pixFmt string) int {
	if n := atoi(raw); n > 0 {
		return n
	}
	for _, d := range []int{16, 14, 12, 10, 9} {
		if strings.Contains(pixFmt, "p"+strconv.Itoa(d)) {
			return d
		}
	}
	return 8
}

func dynamicRange(transfer string) media.DynamicRange {
	switch transfer {
	case "smpte2084":
		return media.HDR10
	case "arib-std-b67":
		return media.HLG
	}
	return media.SDR
}

// codecTag returns a codec tag as ffprobe prints it, or empty when the
// container gives none, which ffprobe prints as escaped zero bytes.
func codecTag(tag string) string {
	if strings.HasPrefix(tag, "[") || strings.HasPrefix(tag, "0x") {
		return ""
	}
	return tag
}

// anamorphic reports whether a sample aspect ratio, such as "32:27", is
// other than square. An unknown ratio ("0:1" or empty) counts as square.
func anamorphic(sar string) bool {
	w, h, ok := strings.Cut(sar, ":")
	return ok && w != h && w != "0" && h != "0"
}
