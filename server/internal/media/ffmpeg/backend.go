package ffmpeg

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/mmcdole/kivo/internal/media"
)

// backend is one row of the backend table. Everything that differs between
// encoding devices lives here, and the pipeline only reads it.
//
// A video transcode takes one of two shapes. In the first, video is
// decoded, filtered and encoded on the device, and only the subtitle is
// drawn by the CPU. In the second, video is decoded and filtered by the CPU
// with the software row's filters, then uploaded to the device's encoder.
// The accelerator test decides, per source format, which shape a device
// runs.
type backend struct {
	kind media.Backend
	// init creates the device for node, which is its render node or GPU
	// index, and makes it the device that filters upload to.
	init func(node string) []string
	// decode has the decoder leave frames on the device.
	decode []string
	// scale resizes frames on the device to w×h, or keeps their size when
	// w is zero. It converts them to 8-bit when to8 is set. It returns ""
	// when there is nothing to do.
	scale func(w, h int, to8 bool) string
	// toneMap converts HDR frames to 8-bit SDR.
	toneMap string
	// overlay draws its second input over its first. subUpload turns the
	// CPU's subtitle frames into that second input.
	overlay   string
	subUpload string
	// upload turns 8-bit frames from the CPU into the encoder's input.
	upload string
	// tail adapts the device's frames for its encoder, in either shape.
	tail string
	// encoders names the encoder for each output codec.
	encoders map[media.Codec]string
	// vbr sets the encoder's speed and puts it in VBR, ahead of its
	// bitrates. fillVBR replaces it for a stream, where it's set. gop turns
	// scene-cut keyframes off and closes GOPs for c, after them.
	vbr     []string
	fillVBR []string
	gop     func(c media.Codec) []string
	// aim is a download's rate control between vbr and gop: video that
	// aims at the encoder's quality value q, capped at bitrate where the
	// encoder's quality mode allows a cap. It returns nil for a codec with
	// no quality mode, which then fills up to the cap as a stream does. Nil
	// means every codec does.
	aim func(c media.Codec, bitrate, q int) []string
	// quality is the encoder's quality value for each media.Fidelity and
	// codec. Lower is better.
	quality map[media.Fidelity]map[media.Codec]int
	// forcedIDR makes forced keyframes IDR frames, for encoders that need
	// to be told.
	forcedIDR []string
}

// toneMapTo is the target every tone-map filter is given: the BT.2390
// curve, to a 100-nit BT.709 picture.
const toneMapTo = "tonemap=bt2390:desat=0:peak=100:t=bt709:m=bt709:p=bt709"

// The backend table. Some filters exist only in the patched ffmpeg build
// that the Dockerfile ships: tonemapx, tonemap_cuda, tonemap_videotoolbox,
// the bt2390 curve and alphasrc. They are the reason Kivo ships that build.
var (
	software = &backend{
		kind: media.Software,
		scale: func(w, h int, _ bool) string {
			if w == 0 {
				return ""
			}
			return fmt.Sprintf("scale=w=%d:h=%d", w, h)
		},
		toneMap:  "tonemapx=" + toneMapTo + ":format=yuv420p",
		overlay:  "overlay=eof_action=pass:repeatlast=0",
		upload:   "format=yuv420p",
		encoders: map[media.Codec]string{media.H264: "libx264", media.HEVC: "libx265"},
		vbr:      []string{"-preset", "veryfast"},
		// A stream runs one preset slower than a download, which gives the
		// accelerators' picture at the cap and still encodes 1080p at more
		// than twice real time on a six-core desktop CPU.
		fillVBR: []string{"-preset", "faster"},
		gop: func(c media.Codec) []string {
			if c == media.HEVC {
				return []string{"-x265-params", "log-level=error:scenecut=0:open-gop=0"}
			}
			return []string{"-sc_threshold", "0", "-flags:v", "+cgop"}
		},
		// For a download, a CRF under a VBV cap at the target average
		// honors both.
		aim: func(_ media.Codec, bitrate, q int) []string {
			return append([]string{"-crf", strconv.Itoa(q)}, vbv(target(bitrate))...)
		},
		quality: map[media.Fidelity]map[media.Codec]int{
			media.FidelityHigh:     {media.H264: 18, media.HEVC: 22},
			media.FidelityStandard: {media.H264: 21, media.HEVC: 25},
		},
		forcedIDR: []string{"-forced-idr", "1"},
	}

	// vaapi decodes, scales and draws on the device. It tone-maps through
	// OpenCL mapped from the device, and encodes with VA-API.
	vaapi = &backend{
		kind: media.VAAPI,
		init: func(node string) []string {
			return []string{"-init_hw_device", "vaapi=va:" + node, "-filter_hw_device", "va"}
		},
		decode:    []string{"-hwaccel", "vaapi", "-hwaccel_device", "va", "-hwaccel_output_format", "vaapi"},
		scale:     scaleOnDevice("scale_vaapi"),
		toneMap:   "hwmap=derive_device=opencl:mode=read,tonemap_opencl=" + toneMapTo + ":format=nv12,hwmap=derive_device=vaapi:mode=write:reverse=1,format=vaapi",
		overlay:   "overlay_vaapi=eof_action=pass:repeatlast=0",
		subUpload: "format=bgra,hwupload",
		upload:    "format=nv12,hwupload",
		encoders:  map[media.Codec]string{media.H264: "h264_vaapi", media.HEVC: "hevc_vaapi"},
		// Downloads fill up to the cap too, because which drivers honor
		// QVBR's or ICQ's quality value varies by GPU.
		vbr: []string{"-rc_mode", "VBR"},
		gop: closedGOP,
	}

	// qsv decodes and filters as vaapi does, on the same render node. It
	// then maps the frames to Quick Sync for its encoder.
	qsv = &backend{
		kind: media.QSV,
		init: func(node string) []string {
			return []string{"-init_hw_device", "vaapi=va:" + node, "-init_hw_device", "qsv=qs@va", "-filter_hw_device", "va"}
		},
		decode:    vaapi.decode,
		scale:     vaapi.scale,
		toneMap:   vaapi.toneMap,
		overlay:   vaapi.overlay,
		subUpload: vaapi.subUpload,
		upload:    "format=nv12,hwupload=extra_hw_frames=64",
		tail:      "hwmap=derive_device=qsv,format=qsv",
		encoders:  map[media.Codec]string{media.H264: "h264_qsv", media.HEVC: "hevc_qsv"},
		vbr:       []string{"-preset", "veryfast"},
		gop:       closedGOP,
		// An H.264 download runs QVBR, which aims at the quality value
		// under the cap. Intel's HEVC QVBR ignores the quality value, so an
		// HEVC download runs ICQ, which aims at the quality alone, with no
		// cap: a hard title can average more than the cap.
		aim: func(c media.Codec, bitrate, q int) []string {
			if c == media.HEVC {
				return []string{"-global_quality:v", strconv.Itoa(q)}
			}
			return append([]string{"-global_quality:v", strconv.Itoa(q)}, capped(bitrate)...)
		},
		quality: map[media.Fidelity]map[media.Codec]int{
			media.FidelityHigh:     {media.H264: 20, media.HEVC: 20},
			media.FidelityStandard: {media.H264: 23, media.HEVC: 23},
		},
		forcedIDR: []string{"-forced_idr", "1"},
	}

	// nvenc decodes and filters with CUDA. Frames from the CPU are uploaded
	// too, so the encoder runs on the frames' GPU.
	nvenc = &backend{
		kind: media.NVENC,
		init: func(node string) []string {
			return []string{"-init_hw_device", "cuda=cu:" + node, "-filter_hw_device", "cu"}
		},
		decode:    []string{"-hwaccel", "cuda", "-hwaccel_device", "cu", "-hwaccel_output_format", "cuda"},
		scale:     scaleOnDevice("scale_cuda"),
		toneMap:   "tonemap_cuda=" + toneMapTo + ":format=nv12",
		overlay:   "overlay_cuda=eof_action=pass:repeatlast=0",
		subUpload: "format=yuva420p,hwupload",
		upload:    "format=nv12,hwupload",
		encoders:  map[media.Codec]string{media.H264: "h264_nvenc", media.HEVC: "hevc_nvenc"},
		vbr:       []string{"-preset", "p4", "-tune", "hq", "-rc", "vbr"},
		gop:       func(media.Codec) []string { return []string{"-no-scenecut", "1"} },
		// For a download, a target quality and no average aims at the
		// quality under a cap at the target average.
		aim: func(_ media.Codec, bitrate, q int) []string {
			return append([]string{"-cq:v", strconv.Itoa(q), "-b:v", "0"}, vbv(target(bitrate))...)
		},
		quality: map[media.Fidelity]map[media.Codec]int{
			media.FidelityHigh:     {media.H264: 20, media.HEVC: 22},
			media.FidelityStandard: {media.H264: 23, media.HEVC: 25},
		},
		forcedIDR: []string{"-forced-idr", "1"},
	}

	// videoToolbox is best effort, because there is no test machine for it.
	videoToolbox = &backend{
		kind: media.VideoToolbox,
		init: func(string) []string {
			return []string{"-init_hw_device", "videotoolbox=vt", "-filter_hw_device", "vt"}
		},
		decode: []string{"-hwaccel", "videotoolbox", "-hwaccel_output_format", "videotoolbox_vld"},
		scale: func(w, h int, _ bool) string {
			if w == 0 {
				return ""
			}
			return fmt.Sprintf("scale_vt=w=%d:h=%d", w, h)
		},
		toneMap:   "tonemap_videotoolbox=" + toneMapTo + ":format=nv12",
		overlay:   "overlay_videotoolbox=eof_action=pass:repeatlast=0",
		subUpload: "format=bgra,hwupload",
		upload:    "format=nv12",
		encoders:  map[media.Codec]string{media.H264: "h264_videotoolbox", media.HEVC: "hevc_videotoolbox"},
		// Downloads fill up to the cap too, because its constant quality
		// mode is on Apple silicon only.
		vbr: []string{"-realtime", "1", "-prio_speed", "1"},
		gop: func(media.Codec) []string { return nil },
	}

	backends = map[media.Backend]*backend{
		media.Software: software, media.VAAPI: vaapi, media.QSV: qsv, media.NVENC: nvenc, media.VideoToolbox: videoToolbox,
	}
)

// scaleOnDevice builds a device's scale filter, which takes a size and a
// format.
func scaleOnDevice(filter string) func(w, h int, to8 bool) string {
	return func(w, h int, to8 bool) string {
		var opts []string
		if w > 0 {
			opts = append(opts, "w="+strconv.Itoa(w), "h="+strconv.Itoa(h))
		}
		if to8 {
			opts = append(opts, "format=nv12")
		}
		if len(opts) == 0 {
			return ""
		}
		return filter + "=" + strings.Join(opts, ":")
	}
}

// closedGOP closes GOPs, for an encoder that cuts no keyframes at scene
// changes while keyframes are forced.
func closedGOP(media.Codec) []string { return []string{"-flags:v", "+cgop"} }

// rateMode is how an encode spends its cap.
type rateMode int

const (
	// fill is a stream's: it fills up to the cap, the same way on every
	// backend (capped).
	fill rateMode = iota
	// aimQuality is a download's: the encoder's quality value (aim), and
	// fill where the encoder has no quality mode.
	aimQuality
)

// rateControl is the encoder's rate control for c capped at bitrate, in
// mode.
func (b *backend) rateControl(mode rateMode, c media.Codec, bitrate, q int) []string {
	var mid []string
	if mode == aimQuality && b.aim != nil {
		mid = b.aim(c, bitrate, q)
	}
	if mid == nil {
		mid = capped(bitrate)
	}
	vbr := b.vbr
	if mode == fill && b.fillVBR != nil {
		vbr = b.fillVBR
	}
	return append(append(slices.Clone(vbr), mid...), b.gop(c)...)
}

// vbv caps video at bitrate, with a buffer of two seconds.
func vbv(bitrate int) []string {
	return []string{"-maxrate:v", strconv.Itoa(bitrate), "-bufsize:v", strconv.Itoa(2 * bitrate)}
}

// capped fills up to the cap, which is how every stream is encoded: VBR
// that averages target(bitrate), with bitrate as the peak rate. The
// picture is about what a constant bitrate at the cap gives, but the cap
// is never exceeded. An average at the cap would be CBR on QSV, which
// overshoots it.
func capped(bitrate int) []string {
	return append([]string{"-b:v", strconv.Itoa(target(bitrate))}, vbv(bitrate)...)
}

// target is the most a capped encode averages: 95% of the cap, which
// leaves room for the muxer's overhead and an audio encoder's overshoot
// under a size worked out from the caps.
func target(bitrate int) int { return bitrate / 20 * 19 }

// parseAccelerator splits an accelerator into its backend and node. The CPU
// is the software backend.
func parseAccelerator(d media.Accelerator) (*backend, string, error) {
	if d == "" {
		return software, "", nil
	}
	kind, node, _ := strings.Cut(string(d), ":")
	b, ok := backends[media.Backend(kind)]
	if !ok || b == software || (node == "") != (b == videoToolbox) {
		return nil, "", unsupported("no such accelerator %q", d)
	}
	return b, node, nil
}

// acceleratorOf returns the accelerator on b at node, in the form requests
// carry. For the software backend it returns the CPU.
func acceleratorOf(b *backend, node string) media.Accelerator {
	if b == software {
		return ""
	}
	if node == "" {
		return media.Accelerator(b.kind)
	}
	return media.Accelerator(string(b.kind) + ":" + node)
}
