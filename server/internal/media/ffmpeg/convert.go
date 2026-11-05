package ffmpeg

import (
	"bufio"
	"context"
	"strconv"
	"strings"
	"time"

	"golang.org/x/text/language"

	"github.com/mmcdole/kivo/internal/media"
)

// convertKeyframes is how often encoded video in a conversion has a
// keyframe. A player seeks a converted file to any keyframe, so the
// keyframes need no grid.
const convertKeyframes = 4 * time.Second

// Convert runs one ffmpeg at low OS priority that writes the request's
// video and audio, muxed together, to one MP4 file. It returns once ffmpeg
// has exited. Video is encoded as in a transcode, on the request's
// accelerator or the CPU, aiming at the encoder's quality value.
func (r *Runner) Convert(ctx context.Context, req media.ConvertRequest) error {
	op := "convert " + req.Path
	var dev *device
	if req.Video.Codec != media.Copy {
		var err error
		if dev, err = r.device(req.Accelerator); err != nil {
			return refuse(op, err)
		}
	}
	args, err := convertArgs(req, dev)
	if err != nil {
		return refuse(op, err)
	}
	p, err := start(ctx, op, r.ffmpeg, args, spawn{background: true})
	if err != nil {
		return err
	}
	defer p.close()
	// ffmpeg reports progress as blocks of key=value lines, about twice a
	// second. out_time_us is how far the output has got.
	lines := bufio.NewScanner(p.stdout)
	for lines.Scan() {
		v, ok := strings.CutPrefix(lines.Text(), "out_time_us=")
		if !ok || req.Reached == nil {
			continue
		}
		if us, err := strconv.ParseInt(v, 10, 64); err == nil && us > 0 {
			req.Reached(time.Duration(us) * time.Microsecond)
		}
	}
	if err := lines.Err(); err != nil {
		p.cancel() // ffmpeg can't go on writing to a pipe no one reads
		_ = p.wait()
		return p.failure(op, media.ErrEncoderFailed, err)
	}
	if err := p.wait(); err != nil {
		return p.failure(op, media.ErrEncoderFailed, err)
	}
	return nil
}

// convertArgs builds the arguments for a conversion. In order, they are:
//   - the input;
//   - the video, either copied or filtered and encoded on dev as in a
//     transcode;
//   - each audio track;
//   - each text subtitle as a text track of the MP4, in mov_text, which
//     AVPlayer draws;
//   - the MP4 muxer, with the index at the front so a player can start
//     before it has the whole file;
//   - progress on stdout.
//
// The source's tags are dropped, because an encode makes their statistics
// stale. Each audio and text track is given its language and title again.
func convertArgs(req media.ConvertRequest, dev *device) (args []string, err error) {
	args = append(append([]string(nil), quiet...), "-nostdin", "-y")
	video, err := planVideo(videoIn{track: req.VideoTrack, source: req.Source, video: req.Video, burnIn: req.BurnIn,
		keyframes: convertKeyframes, rate: aimQuality}, dev)
	if err != nil {
		return nil, err
	}
	args = video.appendInputOptions(args)
	args = appendInput(args, req.Path)
	inputs := make([]string, len(req.Text))
	n := 1
	for i, t := range req.Text {
		inputs[i] = "0:" + strconv.Itoa(t.Track)
		if t.Path != "" {
			args = appendInput(args, t.Path)
			inputs[i] = strconv.Itoa(n) + ":0"
			n++
		}
	}
	args = video.appendOutput(args)
	if video.copied && req.Source.Codec == media.HEVC {
		args = append(args, tagHVC1...)
	}
	for i, a := range req.Audio {
		if args, err = appendAudio(args, i, a); err != nil {
			return nil, err
		}
	}
	for i, a := range req.Audio {
		if l := movLanguage(a.Language); l != "" {
			args = append(args, "-metadata:s:a:"+strconv.Itoa(i), "language="+l)
		}
	}
	if len(req.Text) > 0 {
		args = append(args, "-c:s", "mov_text")
	}
	for i, t := range req.Text {
		s := "-metadata:s:s:" + strconv.Itoa(i)
		args = append(args, "-map", inputs[i])
		if l := movLanguage(t.Language); l != "" {
			args = append(args, s, "language="+l)
		}
		if t.Title != "" {
			args = append(args, s, "handler_name="+t.Title)
		}
	}
	return append(args,
		"-map_metadata", "-1", "-map_chapters", "-1",
		"-max_muxing_queue_size", "2048",
		"-movflags", "+faststart",
		"-progress", "pipe:1",
		"-f", "mp4", "file:"+req.Dst), nil
}

// movLanguage returns a language tag as the MP4 muxer records it, in ISO
// 639-2's three letters, such as "jpn" for ja. It returns "" for no tag or
// a tag that doesn't parse.
func movLanguage(tag string) string {
	b := media.BaseLanguage(tag)
	if b == "" {
		return ""
	}
	base, err := language.ParseBase(b)
	if err != nil {
		return ""
	}
	return base.ISO3()
}
