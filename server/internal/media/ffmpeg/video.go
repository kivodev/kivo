package ffmpeg

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/mmcdole/kivo/internal/media"
)

// stripDolbyVision drops Dolby Vision's metadata from HEVC, leaving the
// HDR10 base layer.
const stripDolbyVision = "dovi_rpu=strip=1"

// deinterlace turns each frame of interlaced video into one progressive
// frame, at the same rate, whichever field the frame says comes first.
const deinterlace = "bwdif=mode=send_frame:parity=auto:deint=all"

// device is one encoding device, either an accelerator or the CPU, with
// what its test passed. The video pipeline reads only this and the request.
type device struct {
	b        *backend
	node     string // the render node or GPU index, or empty for the CPU
	encoders map[media.Codec]media.GOPMode
	decoders map[media.VideoFormat]bool // source formats decoded on the device
	toneMap  bool                       // the device's tone-map filter passed
	burnIn   bool                       // the device's overlay passed
	kernel   kernelVersion              // the Linux kernel the device's driver runs in
}

// untestedCPU is the CPU device that Transcode uses before the accelerator
// test has run. libx264 and libx265 honor forced keyframes.
func untestedCPU() *device {
	return &device{b: software, encoders: map[media.Codec]media.GOPMode{
		media.H264: media.GridGOP, media.HEVC: media.GridGOP,
	}}
}

// format returns the source format a track's decoder sees.
func format(t media.Track) media.VideoFormat {
	return media.VideoFormat{Codec: t.Codec, BitDepth: max(t.BitDepth, 8)}
}

// decodes reports whether d decodes src on the device. Interlaced video is
// decoded by the CPU, which deinterlaces it.
func (d *device) decodes(src media.Track) bool {
	return d.decoders[format(src)] && !src.Interlaced
}

// videoIn is what a request asks of its video.
type videoIn struct {
	track     int         // the source track's Index from Probe
	source    media.Track // the track as Probe described it
	video     media.VideoRendition
	burnIn    *media.SubtitleRef
	start     time.Duration // where output begins, on the source's timeline
	keyframes time.Duration // how far apart encoded video's keyframes are
	// rate is how the encode spends its cap. The zero value is a stream's.
	rate rateMode
}

// videoPlan is a request's video, worked out. The video is either copied,
// or encoded on its device with every number its filters and encoder need.
type videoPlan struct {
	in       videoIn
	copied   bool
	dev      *device
	onDevice bool // decoded and filtered on the device, rather than by the CPU
	encoder  string
	profile  string
	mode     media.GOPMode
	w, h     int     // output size
	resize   bool    // whether w×h differs from the source's size
	rate     string  // the frame rate to cap at, or "" to keep the source's rate
	fps      float64 // the output frame rate
	toneMap  bool
	stripDV  bool // drop Dolby Vision profile 7's metadata before decoding
	// rateControl is the encoder's rate control.
	rateControl []string
}

// planVideo checks a request's video against its source and device. Copied
// video needs no device and can't have subtitles drawn on it. Encoded video
// runs everything on the device when the device's test passed this source
// format and every filter needed. Otherwise it is decoded and filtered on
// the CPU.
func planVideo(in videoIn, dev *device) (*videoPlan, error) {
	v, src := in.video, in.source
	if v.Codec == media.Copy {
		if in.burnIn != nil {
			return nil, unsupported("burning in subtitles needs a video transcode")
		}
		return &videoPlan{in: in, copied: true}, nil
	}
	p := &videoPlan{in: in, dev: dev, encoder: dev.b.encoders[v.Codec]}
	switch {
	case p.encoder == "":
		return nil, unsupported("video codec %q", v.Codec)
	case src.Kind != media.Video:
		return nil, unsupported("a video transcode needs the source's video track")
	case v.BitDepth != 0 && v.BitDepth != 8:
		return nil, unsupported("%d-bit video: encoded video is 8-bit", v.BitDepth)
	case v.Bitrate <= 0:
		return nil, unsupported("a video transcode needs a bitrate")
	case in.keyframes <= 0:
		return nil, unsupported("a video transcode needs a segment duration")
	}
	mode, ok := dev.encoders[v.Codec]
	if !ok {
		return nil, unsupported("%s didn't pass the accelerator test for %s", label(acceleratorOf(dev.b, dev.node)), v.Codec)
	}
	p.mode = mode
	switch p.profile = v.Profile; {
	case v.Codec == media.H264 && (p.profile == "" || p.profile == "high"):
		p.profile = "high"
	case p.profile == "" || p.profile == "main":
		p.profile = "main"
	default:
		return nil, unsupported("%s profile %q", v.Codec, v.Profile)
	}

	hdr := src.HDR()
	switch v.HDR {
	case media.PassThrough:
		if hdr {
			return nil, unsupported("8-bit video can't keep HDR; tone-map it")
		}
	case media.ToneMap:
		p.toneMap = hdr
		p.stripDV = src.DolbyVisionProfile == 7
	default:
		return nil, unsupported("only copied video keeps HDR10")
	}

	if src.Width > 0 && src.Height > 0 {
		p.w, p.h = fit(src.Width, src.Height, v.Width, v.Height)
	} else if v.Width > 0 && v.Height > 0 {
		p.w, p.h = v.Width&^1, v.Height&^1
	} else {
		return nil, unsupported("the source's size is unknown")
	}
	p.resize = p.w != src.Width || p.h != src.Height

	p.fps = src.FrameRate
	if v.FrameRate > 0 && (p.fps == 0 || v.FrameRate < p.fps*0.999) {
		p.fps = v.FrameRate
		p.rate = frameRate(v.FrameRate)
	}
	if p.fps == 0 {
		p.fps = 24 // a guess, used only for the GOP and the subtitle canvas
	}

	q := dev.b.quality[v.Fidelity][v.Codec]
	p.rateControl = dev.b.rateControl(in.rate, v.Codec, v.Bitrate, q)

	p.onDevice = dev.b != software && dev.decodes(src) &&
		(!p.toneMap || dev.toneMap) && (in.burnIn == nil || dev.burnIn)
	return p, nil
}

// appendInputOptions creates the device and sets up decoding.
func (p *videoPlan) appendInputOptions(args []string) []string {
	if p.copied {
		return args
	}
	if p.dev.b.init != nil {
		args = append(args, p.dev.b.init(p.dev.node)...)
	}
	if p.onDevice {
		// Rotating would need a filter on the CPU. Files from phones,
		// which carry rotation, play directly anyway.
		args = append(append(args, p.dev.b.decode...), "-noautorotate") // a flag, because "-autorotate 0" makes "0" an output file
	}
	if p.stripDV {
		args = append(args, "-bsf:"+strconv.Itoa(p.in.track), stripDolbyVision)
	}
	return args
}

// appendOutput adds the copied stream, or the filter graph and the encoder.
func (p *videoPlan) appendOutput(args []string) []string {
	if p.copied {
		args = append(args, "-map", "0:"+strconv.Itoa(p.in.track), "-c:v", "copy")
		if p.in.video.HDR == media.StripDolbyVision {
			args = append(args, "-bsf:v", stripDolbyVision)
		}
		return args
	}
	args = append(args, "-filter_complex", p.graph(), "-map", "[v]",
		"-c:v", p.encoder, "-profile:v", p.profile)
	args = append(args, p.rateControl...)
	if p.dev.b == qsv && p.onDevice && p.toneMap && p.dev.kernel.hangsQSVAfterOpenCL() {
		args = append(args, "-async_depth", "1")
	}
	args = p.appendKeyframes(args)
	if p.in.video.Codec == media.HEVC {
		args = append(args, tagHVC1...)
	}
	return args
}

// tagHVC1 tags HEVC as hvc1, as Apple's players require.
var tagHVC1 = []string{"-tag:v", "hvc1"}

// appendKeyframes puts keyframes on the segment grid. It forces an IDR
// frame at each grid point, with a GOP limit of two segments so the encoder
// adds no keyframes of its own. Where forcing isn't honored, it sets a
// fixed GOP of one segment instead. The expression's t counts from the
// job's first frame, which is at the request's start, on the grid.
func (p *videoPlan) appendKeyframes(args []string) []string {
	d := p.in.keyframes
	gop := int(math.Ceil(d.Seconds()*p.fps - 1e-6))
	if p.mode == media.FixedGOP {
		return append(args, "-g:v", strconv.Itoa(gop))
	}
	args = append(args, "-force_key_frames:v", "expr:gte(t,n_forced*"+seconds(d)+")")
	return append(append(args, p.dev.b.forcedIDR...), "-g:v", strconv.Itoa(2*gop))
}

// graph returns the filter graph from the source's video to [v], the
// encoder's input. It sets the color properties, deinterlaces, caps the
// frame rate, sets the size and tone-maps, then draws the subtitle, then
// adds whatever the encoder needs.
func (p *videoPlan) graph() string {
	f := software
	if p.onDevice {
		f = p.dev.b
	}
	main := p.frames()
	w := 0
	if p.resize {
		w = p.w
	}
	main = appendFilter(main, f.scale(w, p.h, !p.toneMap && p.in.source.BitDepth > 8))
	if p.toneMap {
		main = append(main, f.toneMap)
	}
	burn := p.in.burnIn
	text := burn != nil && burn.Path != ""
	if text && !p.onDevice {
		main = append(main, "subtitles="+p.subtitleOptions())
	}

	var last []string // filters after the subtitle, leading into the encoder
	if !p.onDevice {
		last = appendFilter(last, p.dev.b.upload)
	}
	last = appendFilter(last, p.dev.b.tail)

	in := "[0:" + strconv.Itoa(p.in.track) + "]"
	if burn == nil || (text && !p.onDevice) {
		return in + chain(append(main, last...)) + "[v]"
	}
	var sub []string
	from := ""
	if text {
		// Text is drawn on a transparent canvas the size of the picture, on
		// the source's timeline, and laid over the picture on the device.
		sub = append(sub, fmt.Sprintf("alphasrc=s=%dx%d:r=%s:start=%s", p.w, p.h, frameRate(p.fps), seconds(p.in.start)),
			"format=yuva420p", "subtitles="+p.subtitleOptions()+":alpha=1")
	} else {
		// An image subtitle's frames are their own canvas, often the full
		// 1920x1080 of a film whose picture is cropped to 1920x800. overlay
		// needs them at the picture's size, so they're always scaled to it.
		from = "[0:" + strconv.Itoa(burn.Track) + "]"
		sub = append(sub, fmt.Sprintf("scale=w=%d:h=%d", p.w, p.h))
	}
	if p.onDevice {
		sub = append(sub, f.subUpload)
	}
	return in + chain(main) + "[main];" + from + chain(sub) + "[sub];[main][sub]" + chain(append([]string{f.overlay}, last...)) + "[v]"
}

// frames returns the filters that make the source's frames the frames the
// rest of the graph works on: their color properties, progressive, at the
// output frame rate.
func (p *videoPlan) frames() []string {
	fs := []string{colorParams(p.in.source, p.toneMap)}
	if p.in.source.Interlaced {
		fs = append(fs, deinterlace) // only on the CPU, as decodes says
	}
	if p.rate != "" {
		fs = append(fs, "fps="+p.rate)
	}
	return fs
}

// colorParams returns the filter that sets the color properties of a
// source's frames to the ones the later filters work in, whatever the file
// is tagged with. HDR that is about to be tone-mapped gets BT.2020 with its
// transfer. Everything else gets BT.709. A file tagged with a matrix the
// scaler can't convert, such as YCgCo, is read as BT.709 like any other.
func colorParams(src media.Track, toneMap bool) string {
	switch {
	case toneMap && src.Range == media.HLG:
		return "setparams=color_primaries=bt2020:color_trc=arib-std-b67:colorspace=bt2020nc"
	case toneMap:
		return "setparams=color_primaries=bt2020:color_trc=smpte2084:colorspace=bt2020nc"
	}
	return "setparams=color_primaries=bt709:color_trc=bt709:colorspace=bt709"
}

// subtitleOptions names the text subtitle file and its fonts for libass.
func (p *videoPlan) subtitleOptions() string {
	opts := "f=" + filterValue(p.in.burnIn.Path)
	if p.in.burnIn.Fonts != "" {
		opts += ":fontsdir=" + filterValue(p.in.burnIn.Fonts)
	}
	return opts
}

// appendFilter appends f unless it's empty.
func appendFilter(fs []string, f string) []string {
	if f == "" {
		return fs
	}
	return append(fs, f)
}

// chain joins filters. With no filters, it passes frames through.
func chain(fs []string) string {
	if len(fs) == 0 {
		return "null"
	}
	return strings.Join(fs, ",")
}

// filterValue escapes s as one option value inside a filter graph. It
// escapes once for the filter's option parser and again for the graph's.
func filterValue(s string) string {
	return escape(escape(s, `\':`), `\'[],;`)
}

func escape(s, special string) string {
	var b strings.Builder
	for _, r := range s {
		if strings.ContainsRune(special, r) {
			b.WriteByte('\\')
		}
		b.WriteRune(r)
	}
	return b.String()
}

// fit scales w×h to fit within maxW×maxH, where zero means no bound. It
// never enlarges, keeps the aspect ratio, and rounds to even numbers as
// 4:2:0 needs.
func fit(w, h, maxW, maxH int) (int, int) {
	s := 1.0
	if maxW > 0 {
		s = min(s, float64(maxW)/float64(w))
	}
	if maxH > 0 {
		s = min(s, float64(maxH)/float64(h))
	}
	even := func(x float64) int { return max(2, int(x/2+1e-6)*2) }
	return even(float64(w) * s), even(float64(h) * s)
}

// frameRate formats a rate as ffmpeg reads it. NTSC's 1000/1001 rates are
// written exactly.
func frameRate(r float64) string {
	for _, den := range []float64{1, 1001} {
		if n := math.Round(r * den); n > 0 && math.Abs(n/den-r) < 1e-6 {
			if den == 1 {
				return strconv.Itoa(int(n))
			}
			return strconv.Itoa(int(n)) + "/1001"
		}
	}
	return strconv.FormatFloat(r, 'f', 3, 64)
}
