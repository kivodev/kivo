package ffmpeg

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/mmcdole/kivo/internal/media"
)

// quiet limits ffmpeg's error output to warnings and errors, which feed the
// diagnostics.
var quiet = []string{"-hide_banner", "-nostats", "-loglevel", "warning"}

// appendInput opens path as a plain file. The file: prefix stops a name
// such as "concat:a|b" or "-i" from being read as a protocol or an option.
// The whitelist stops anything inside the file, such as a playlist, from
// opening any other protocol.
func appendInput(args []string, path string) []string {
	return append(args, "-protocol_whitelist", "file", "-i", "file:"+path)
}

func probeArgs(path string) []string {
	args := []string{"-hide_banner", "-loglevel", "error", "-print_format", "json",
		"-show_format", "-show_streams", "-show_chapters"}
	return appendInput(args, path)
}

// transcodeArgs builds the arguments for a transcode job. In order, they
// are the input, the video, each audio track, the muxer, and each text
// track as an output of its own. The video is either copied, or filtered and encoded
// on dev.
func transcodeArgs(req media.TranscodeRequest, dev *device) ([]string, error) {
	args := append([]string(nil), quiet...)
	if len(req.Text) > 0 {
		// Never ask before replacing the scratch files, because stdin is
		// used for pausing.
		args = append(args, "-y")
	}
	video, err := planVideo(videoIn{track: req.VideoTrack, source: req.Source, video: req.Video, burnIn: req.BurnIn,
		start: req.Start, keyframes: req.SegmentDuration}, dev)
	if err != nil {
		return nil, err
	}
	args = video.appendInputOptions(args)
	// Copied timestamps keep segments on the source's timeline, which is
	// the timeline the playlist was written for, wherever the job starts.
	if req.Start > 0 {
		args = append(args, "-ss", seconds(req.Start))
	}
	args = appendInput(append(args, "-copyts"), req.Path)
	args = video.appendOutput(args)
	for i, a := range req.Audio {
		if args, err = appendAudio(args, i, a); err != nil {
			return nil, err
		}
	}
	args = appendMuxer(args, req.Audio)
	for _, t := range req.Text {
		args = appendCues(args, t)
	}
	return args, nil
}

// appendAudio adds output audio stream i.
func appendAudio(args []string, i int, a media.AudioOut) ([]string, error) {
	n := strconv.Itoa(i)
	args = append(args, "-map", "0:"+strconv.Itoa(a.Track))
	switch a.Out.Codec {
	case media.Copy:
		return append(args, "-c:a:"+n, "copy"), nil
	case media.AAC, media.AC3, media.EAC3:
		args = append(args, "-c:a:"+n, string(a.Out.Codec))
	default:
		return nil, unsupported("audio codec %q", a.Out.Codec)
	}
	if f := audioFilter(a); f != "" {
		args = append(args, "-filter:a:"+n, f)
	}
	for _, o := range []struct {
		flag  string
		value int
	}{{"-ac:a:", a.Out.Channels}, {"-ar:a:", a.Out.SampleRate}, {"-b:a:", a.Out.Bitrate}} {
		if o.value > 0 {
			args = append(args, o.flag+n, strconv.Itoa(o.value))
		}
	}
	return args, nil
}

// audioFilter returns the filter chain an encoded audio track goes through,
// or "" for none. Surround mixed to stereo gets a downmix. A dialogue boost
// raises the centre and evens out the dynamics.
func audioFilter(a media.AudioOut) string {
	downmix := a.Out.Channels == 2 && a.SourceChannels > 2
	switch {
	case !a.Out.DialogueBoost && downmix:
		return stereoDownmix
	case !a.Out.DialogueBoost:
		return ""
	case downmix:
		return dialogueDownmix + "," + dialogueDynamics
	case a.Out.Channels == 6:
		return centreLift + "," + dialogueDynamics
	}
	return dialogueDynamics
}

// stereoDownmix mixes surround down to stereo while keeping dialogue clear.
// It uses the coefficients known as Dave750's. Each side takes half the
// centre and the LFE, and 0.707 of its front and surround. ffmpeg's plain
// -ac 2 normalizes the mix, which leaves the centre, where dialogue is, at
// about 0.29.
//
// The source is first made 5.1 by ffmpeg's standard rematrixing, which
// folds 7.1's back and side channels together. So the coefficients' channel
// order holds for any layout.
const stereoDownmix = "aformat=channel_layouts=5.1|5.1(side)," +
	"pan=stereo|c0=0.5*c2+0.707*c0+0.707*c4+0.5*c3|c1=0.5*c2+0.707*c1+0.707*c5+0.5*c3"

// dialogueDownmix mixes surround down to stereo for a dialogue boost. Each
// side takes the centre at full level, the fronts and surrounds at 0.3, and
// no LFE. That puts dialogue about 10 dB above the rest. The source is made
// 5.1 first, as for stereoDownmix.
const dialogueDownmix = "aformat=channel_layouts=5.1|5.1(side)," +
	"pan=stereo|c0=c2+0.30*c0+0.30*c4|c1=c2+0.30*c1+0.30*c5"

// centreLift raises 5.1's centre channel by about 4 dB for a dialogue
// boost, leaving the others as they are.
const centreLift = "aformat=channel_layouts=5.1|5.1(side)," +
	"pan=5.1|c0=c0|c1=c1|c2=1.6*c2|c3=c3|c4=c4|c5=c5"

// dialogueDynamics evens out a dialogue boost's levels. A compressor brings
// loud passages down toward speech and raises the whole by 8 dB. A limiter
// keeps the peaks below full scale. Neither looks ahead more than a few
// milliseconds, so a job starts and a seek lands as quickly as without
// them.
const dialogueDynamics = "acompressor=threshold=-30dB:ratio=3:attack=5:release=250:makeup=8dB," +
	"alimiter=limit=-1dB:level=false"

// appendMuxer writes fragmented MP4 to stdout, one fragment per video
// keyframe. Timestamps are the source's own, shifted by
// media.TimestampOffset. frag_discont starts the first fragment at its own
// time rather than zero. No edit list shifts what the samples say, and
// nothing renumbers timestamps that would be negative.
//
// Copied video with sparse audio overflows the default muxing queue, so the
// queue is enlarged. AC-3 and E-AC-3, copied or encoded, need their first
// packet parsed before the init segment can describe them. For those,
// delay_moov writes the init segment once the first fragment is cut.
// Without it, nothing is written.
func appendMuxer(args []string, audio []media.AudioOut) []string {
	movflags := "+frag_keyframe+empty_moov+default_base_moof+frag_discont"
	for _, a := range audio {
		codec := a.Out.Codec
		if codec == media.Copy {
			codec = a.Source
		}
		if codec == media.AC3 || codec == media.EAC3 {
			movflags = "+frag_keyframe+empty_moov+delay_moov+default_base_moof+frag_discont"
		}
	}
	return append(args,
		"-map_metadata", "-1", "-map_chapters", "-1",
		"-max_muxing_queue_size", "2048",
		"-avoid_negative_ts", "disabled",
		"-output_ts_offset", seconds(media.TimestampOffset),
		"-use_editlist", "0",
		"-movflags", movflags,
		"-f", "mp4", "pipe:1")
}

// appendCues writes a text track to its scratch file as SubRip. Each cue
// is flushed as it is read, so Output.Cues sees it at once. The cue
// timestamps stay on the source's timeline, without the media's offset.
func appendCues(args []string, t media.CuesOut) []string {
	return append(args, "-map", "0:"+strconv.Itoa(t.Track), "-c:s", "srt", "-flush_packets", "1", "-f", "srt", "file:"+t.Path)
}

// imageArgs re-encodes one image to WebP at each width, never wider than
// the source. It also writes a 32-pixel-wide PNG to stdout for the
// blurhash.
func imageArgs(src string, outs []media.ImageOut) ([]string, error) {
	args := append(append([]string(nil), quiet...), "-nostdin", "-y")
	args = appendInput(args, src)
	var graph strings.Builder
	fmt.Fprintf(&graph, "[0:v]split=%d", len(outs)+1)
	for i := range outs {
		fmt.Fprintf(&graph, "[s%d]", i)
	}
	graph.WriteString("[st]")
	for i, o := range outs {
		if o.Width <= 0 || o.Path == "" {
			return nil, unsupported("image output %d has width %d and path %q", i, o.Width, o.Path)
		}
		fmt.Fprintf(&graph, ";[s%d]scale=w='min(%d,iw)':h=-1[o%d]", i, o.Width, i)
	}
	graph.WriteString(";[st]scale=w=32:h=-1[tiny]")
	args = append(args, "-filter_complex", graph.String())
	for i, o := range outs {
		args = append(args, "-map", fmt.Sprintf("[o%d]", i), "-frames:v", "1",
			"-c:v", "libwebp", "-quality", "80", "-f", "webp", "file:"+o.Path)
	}
	return append(args, "-map", "[tiny]", "-frames:v", "1", "-c:v", "png", "-f", "image2pipe", "pipe:1"), nil
}

// subtitlesArgs extracts each subtitle track to its own file in one read.
// Text tracks are converted to SubRip. ASS and PGS are copied.
func subtitlesArgs(req media.SubtitlesRequest) ([]string, error) {
	args := appendInput(append(append([]string(nil), quiet...), "-nostdin", "-y"), req.Path)
	for _, t := range req.Tracks {
		codec := "copy"
		switch t.Format {
		case media.SRT:
			codec = "srt"
		case media.ASS, media.PGS:
		default:
			return nil, unsupported("subtitle format %q", t.Format)
		}
		args = append(args, "-map", "0:"+strconv.Itoa(t.Track), "-c:s", codec, "-f", t.Format.Ext(), "file:"+t.Path)
	}
	return args, nil
}

// previewsArgs decodes only one track's keyframes, on one thread because no
// one is waiting. It keeps a frame every interval, scales and tone-maps it
// on the CPU, and tiles the frames into WebP sheets. The last sheet is
// padded.
func previewsArgs(req media.PreviewsRequest, w, h int) []string {
	args := append(append([]string(nil), quiet...), "-nostdin", "-y", "-threads", "1", "-skip_frame", "nokey")
	src := req.Source
	hdr := src.HDR()
	if src.DolbyVisionProfile == 7 {
		args = append(args, "-bsf:"+strconv.Itoa(req.VideoTrack), stripDolbyVision)
	}
	args = appendInput(args, req.Path)
	filters := []string{colorParams(src, hdr), "fps=fps=1/" + seconds(req.Interval), software.scale(w, h, false)}
	if hdr {
		filters = append(filters, software.toneMap)
	}
	filters = append(filters, fmt.Sprintf("tile=%dx%d", req.Columns, req.Rows))
	return append(args,
		"-filter_complex", "[0:"+strconv.Itoa(req.VideoTrack)+"]"+chain(filters)+"[v]", "-map", "[v]",
		"-fps_mode", "passthrough", "-c:v", "libwebp", "-quality", "70",
		"-f", "image2", "-start_number", "0", "file:"+filepath.Join(req.Dir, "sheet-%d.webp")) // as media.SheetName names them
}

// seconds formats d for ffmpeg's time options.
func seconds(d time.Duration) string {
	return strconv.FormatFloat(d.Seconds(), 'f', -1, 64)
}
