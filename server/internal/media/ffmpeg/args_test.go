package ffmpeg

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/mmcdole/kivo/internal/media"
)

var update = flag.Bool("update", false, "rewrite golden files")

func copyAudio(track int) media.AudioOut {
	return media.AudioOut{Track: track, Out: media.AudioRendition{Codec: media.Copy}}
}

// TestGoldenArgv pins the command line of every request shape. Each golden
// file holds the quoted command, as Diagnostics shows it.
func TestGoldenArgv(t *testing.T) {
	video := media.VideoRendition{Codec: media.Copy}
	tests := []struct {
		name string
		args func() ([]string, error)
	}{
		{"probe", func() ([]string, error) { return probeArgs("/media/Movies/Alien (1979)/Alien.mkv"), nil }},
		{"remux", func() ([]string, error) {
			return transcodeArgs(media.TranscodeRequest{Path: "/media/a.mkv", SegmentDuration: 6 * time.Second,
				Video: video, Audio: []media.AudioOut{copyAudio(1)}}, nil)
		}},
		{"remux-seek", func() ([]string, error) {
			return transcodeArgs(media.TranscodeRequest{Path: "/media/a.mkv", Start: 754208 * time.Millisecond,
				SegmentDuration: 6 * time.Second, VideoTrack: 1, Video: video,
				Audio: []media.AudioOut{copyAudio(2), copyAudio(4)}}, nil)
		}},
		{"audio-transcode", func() ([]string, error) {
			return transcodeArgs(media.TranscodeRequest{Path: "/media/a.mkv", Start: 90 * time.Second,
				SegmentDuration: 6 * time.Second, Video: video, Audio: []media.AudioOut{
					{Track: 1, SourceChannels: 6, Out: media.AudioRendition{Codec: media.AAC, Channels: 2, Bitrate: 192000, SampleRate: 48000}},
					{Track: 2, Out: media.AudioRendition{Codec: media.AAC}},
					copyAudio(3),
					{Track: 4, SourceChannels: 8, Out: media.AudioRendition{Codec: media.AAC, Channels: 6}},
				}}, nil)
		}},
		// A dialogue boost mixes surround down to stereo, keeps 5.1 with
		// its centre raised, and only levels stereo and mono.
		{"dialogue-boost", func() ([]string, error) {
			boost := func(track, from, to int) media.AudioOut {
				return media.AudioOut{Track: track, Source: "truehd", SourceChannels: from,
					Out: media.AudioRendition{Codec: media.AAC, Channels: to, Bitrate: 64000 * to, SampleRate: 48000, DialogueBoost: true}}
			}
			return transcodeArgs(media.TranscodeRequest{Path: "/media/a.mkv", SegmentDuration: 6 * time.Second, Video: video,
				Audio: []media.AudioOut{boost(1, 8, 2), boost(2, 8, 6), boost(3, 2, 2), boost(4, 1, 1)}}, nil)
		}},
		{"remux-text", func() ([]string, error) {
			return transcodeArgs(media.TranscodeRequest{Path: "/media/a.mkv", Start: 754208 * time.Millisecond,
				SegmentDuration: 6 * time.Second, Video: video, Audio: []media.AudioOut{copyAudio(1)},
				Text: []media.CuesOut{{Track: 3, Path: "/cache/transcode/s/text-2-0.srt"}, {Track: 4, Path: "/cache/transcode/s/text-2-1.srt"}}}, nil)
		}},
		{"video-only", func() ([]string, error) {
			return transcodeArgs(media.TranscodeRequest{Path: "/media/a.mp4", SegmentDuration: 6 * time.Second, Video: video}, nil)
		}},
		{"remux-strip-dolby-vision", func() ([]string, error) {
			return transcodeArgs(media.TranscodeRequest{Path: "/media/dv7.mkv", SegmentDuration: 6 * time.Second,
				Video: media.VideoRendition{Codec: media.Copy, HDR: media.StripDolbyVision}, Audio: []media.AudioOut{copyAudio(1)}}, nil)
		}},
		// A name ffmpeg would otherwise read as a protocol and an option.
		{"hostile-path", func() ([]string, error) {
			return transcodeArgs(media.TranscodeRequest{Path: "concat:/etc/passwd|-y 'x'.mkv", Video: video}, nil)
		}},
		{"convert-remux", func() ([]string, error) {
			return convertArgs(media.ConvertRequest{Path: "/media/a.mkv", Source: hdr2160, Video: video,
				Audio: []media.AudioOut{copyAudio(1)}, Dst: "/cache/downloads/session-1/download-2.tmp"}, nil)
		}},
		{"convert-text", func() ([]string, error) {
			return convertArgs(media.ConvertRequest{Path: "/media/a.mp4", Source: sdr1080, Video: video,
				Audio: []media.AudioOut{copyAudio(1)}, Text: []media.TextOut{{Path: "/media/a.en.srt", Title: "English", Language: "en"}},
				Dst: "/cache/downloads/d1/0.mp4.tmp"}, nil)
		}},
		// Every text track is carried with its language. The audio keeps
		// its language too, even though the source's tags are dropped.
		{"convert-texts", func() ([]string, error) {
			jpn := copyAudio(1)
			jpn.Language = "ja"
			return convertArgs(media.ConvertRequest{Path: "/media/a.mkv", Source: sdr1080, Video: video,
				Audio: []media.AudioOut{jpn}, Text: []media.TextOut{
					{Track: 3, Title: "English", Language: "eng"},
					{Path: "/media/a.fr.srt", Language: "fr"},
					{Track: 4, Language: "und"},
					{Path: "/subtitles/b.pt-BR.srt", Title: "Português (Brasil)", Language: "pt-BR"},
				},
				Dst: "/cache/downloads/d1/0.mp4.tmp"}, nil)
		}},
		{"convert-video", func() ([]string, error) {
			return convertArgs(media.ConvertRequest{Path: "/media/a.mkv", Source: sdr1080,
				Video:  media.VideoRendition{Codec: media.HEVC, Width: 1920, Height: 1080, Bitrate: 8_000_000},
				Audio:  []media.AudioOut{{Track: 1, SourceChannels: 6, Out: media.AudioRendition{Codec: media.AAC, Channels: 2}}},
				BurnIn: &media.SubtitleRef{Track: 3}, Accelerator: "qsv:/dev/dri/renderD128",
				Dst: "/cache/downloads/session-1/download-2.tmp"}, testDevice(qsv, "/dev/dri/renderD128", media.GridGOP))
		}},
		{"convert-standard", func() ([]string, error) {
			return convertArgs(media.ConvertRequest{Path: "/media/a.mkv", Source: sdr1080,
				Video: media.VideoRendition{Codec: media.H264, Width: 1920, Height: 1080, Bitrate: 3_000_000, Fidelity: media.FidelityStandard},
				Audio: []media.AudioOut{{Track: 1, SourceChannels: 6, Out: media.AudioRendition{Codec: media.AAC, Channels: 2, Bitrate: 128_000}}},
				Dst:   "/cache/downloads/session-1/download-2.tmp"}, testDevice(qsv, "/dev/dri/renderD128", media.GridGOP))
		}},
		{"image", func() ([]string, error) {
			return imageArgs("/data/cache/images/tmp/poster.jpg", []media.ImageOut{
				{Width: 240, Path: "/data/cache/images/p240.webp.tmp"},
				{Width: 480, Path: "/data/cache/images/p480.webp.tmp"},
				{Width: 960, Path: "/data/cache/images/p960.webp.tmp"},
			})
		}},
		{"subtitles", func() ([]string, error) {
			return subtitlesArgs(media.SubtitlesRequest{Path: "/media/Anime/Show/01.mkv", Tracks: []media.SubtitleOut{
				{Track: 2, Format: media.ASS, Path: "/cache/2.ass"},
				{Track: 3, Format: media.SRT, Path: "/cache/3.srt"},
				{Track: 4, Format: media.PGS, Path: "/cache/4.sup"},
			}})
		}},
		{"previews", func() ([]string, error) {
			return previewsArgs(media.PreviewsRequest{Path: "/media/a.mkv", Source: sdr1080, Interval: 10 * time.Second,
				Width: 320, Columns: 10, Rows: 10, Dir: "/cache/previews"}, 320, 180), nil
		}},
		{"previews-dolby-vision", func() ([]string, error) {
			return previewsArgs(media.PreviewsRequest{Path: "/media/dv7.mkv", Source: dv7, Interval: 10 * time.Second,
				Width: 320, Columns: 10, Rows: 10, Dir: "/cache/previews"}, 320, 134), nil
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args, err := tt.args()
			if err != nil {
				t.Fatal(err)
			}
			tool := "ffmpeg"
			if slices.Contains([]string{"probe", "attachments"}, tt.name) {
				tool = "ffprobe"
			}
			checkGolden(t, tt.name, quote(append([]string{tool}, slashed(args)...))+"\n")
			again, _ := tt.args()
			if quote(again) != quote(args) {
				t.Error("the same request gave a different argv")
			}
		})
	}
}

// checkGolden compares got with testdata/name.golden, or rewrites it.
func checkGolden(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join("testdata", name+".golden")
	if *update {
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got != string(want) {
		t.Errorf("argv changed (go test -update to accept):\ngot  %s\nwant %s", got, want)
	}
}

// The sources the video goldens transcode, as Probe would describe them.
var (
	sdr1080 = media.Track{Index: 0, Kind: media.Video, Codec: media.H264, BitDepth: 8, Width: 1920, Height: 1080,
		FrameRate: 24000.0 / 1001, Range: media.SDR}
	hdr2160 = media.Track{Index: 0, Kind: media.Video, Codec: media.HEVC, BitDepth: 10, Width: 3840, Height: 2160,
		FrameRate: 24000.0 / 1001, Range: media.HDR10}
	dv7 = media.Track{Index: 0, Kind: media.Video, Codec: media.HEVC, BitDepth: 10, Width: 3840, Height: 1604,
		FrameRate: 24, Range: media.HDR10, DolbyVisionProfile: 7}
	av1Source = media.Track{Index: 0, Kind: media.Video, Codec: "av1", BitDepth: 10, Width: 1920, Height: 800,
		FrameRate: 25, Range: media.SDR}
	sdr60 = media.Track{Index: 0, Kind: media.Video, Codec: media.H264, BitDepth: 8, Width: 1280, Height: 720,
		FrameRate: 60, Range: media.SDR}
)

// testDevice is a device whose accelerator test passed everything except
// AV1.
func testDevice(b *backend, node string, mode media.GOPMode) *device {
	return &device{b: b, node: node,
		encoders: map[media.Codec]media.GOPMode{media.H264: mode, media.HEVC: mode},
		decoders: map[media.VideoFormat]bool{{Codec: media.H264, BitDepth: 8}: true, {Codec: media.HEVC, BitDepth: 8}: true,
			{Codec: media.HEVC, BitDepth: 10}: true},
		toneMap: true, burnIn: true}
}

// videoCase is one video request the goldens pin on every backend.
type videoCase struct {
	name  string
	req   media.TranscodeRequest
	fixed bool // runs on a device whose encoder ignores forced keyframes
}

func videoCases() []videoCase {
	stereo := []media.AudioOut{{Track: 1, Out: media.AudioRendition{Codec: media.AAC, Channels: 2, Bitrate: 192000}}}
	req := func(src media.Track, v media.VideoRendition) media.TranscodeRequest {
		return media.TranscodeRequest{Path: "/media/Movies/Film.mkv", Start: 756 * time.Second, SegmentDuration: 6 * time.Second,
			Source: src, Video: v, Audio: stereo}
	}
	h264 := media.VideoRendition{Codec: media.H264, Width: 1280, Height: 720, Bitrate: 4_000_000}
	hevc := media.VideoRendition{Codec: media.HEVC, Width: 1920, Height: 1080, Bitrate: 8_000_000, HDR: media.ToneMap}
	full := media.VideoRendition{Codec: media.H264, Bitrate: 10_000_000}
	cases := []videoCase{
		{name: "h264-720p", req: req(sdr1080, h264)},
		{name: "hevc-tone-mapped", req: req(hdr2160, hevc)},
		{name: "dolby-vision-7-tone-mapped", req: req(dv7, media.VideoRendition{Codec: media.H264, Width: 1920, Height: 1080,
			Bitrate: 8_000_000, HDR: media.ToneMap})},
		{name: "burn-in-image", req: req(sdr1080, h264)},
		{name: "burn-in-text", req: req(sdr1080, full)},
		{name: "cpu-decode", req: req(av1Source, h264)},
		{name: "frame-rate-cap", req: req(sdr60, media.VideoRendition{Codec: media.H264, FrameRate: 30, Bitrate: 3_000_000})},
		{name: "fixed-gop", req: req(sdr1080, h264), fixed: true},
	}
	cases[3].req.BurnIn = &media.SubtitleRef{Track: 4}
	cases[4].req.BurnIn = &media.SubtitleRef{Path: "/data/cache/subtitles/01J/3.ass", Fonts: "/data/cache/subtitles/01J/fonts"}
	cases[4].req.Start = 0
	return cases
}

// TestGoldenVideoArgv pins every video request on every backend, one golden
// file per backend.
func TestGoldenVideoArgv(t *testing.T) {
	for _, tt := range []struct {
		name string
		b    *backend
		node string
	}{
		{"software", software, ""}, {"vaapi", vaapi, "/dev/dri/renderD128"}, {"qsv", qsv, "/dev/dri/renderD128"},
		{"qsv-d3d11", qsvD3D11, "1"}, {"nvenc", nvenc, "0"}, {"videotoolbox", videoToolbox, ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var got strings.Builder
			for _, c := range videoCases() {
				if c.fixed && tt.b == software {
					continue
				}
				mode := media.GridGOP
				if c.fixed {
					mode = media.FixedGOP
				}
				c.req.Accelerator = acceleratorOf(tt.b, tt.node)
				args, err := transcodeArgs(c.req, testDevice(tt.b, tt.node, mode))
				if err != nil {
					t.Fatalf("%s: %v", c.name, err)
				}
				fmt.Fprintf(&got, "# %s\n%s\n", c.name, quote(append([]string{"ffmpeg"}, args...)))
			}
			checkGolden(t, "video-"+tt.name, got.String())
		})
	}
}

// TestStreamsFillUpToTheCap checks that a stream on every backend runs the
// same rate control, whatever its Fidelity: VBR averaging 95% of the cap,
// the cap as peak with a two-second buffer, and no quality value. An
// average at the peak would be CBR.
func TestStreamsFillUpToTheCap(t *testing.T) {
	const capBitrate = 8_000_000
	flag := func(args []string, name string) string {
		if i := slices.Index(args, name); i >= 0 && i+1 < len(args) {
			return args[i+1]
		}
		return ""
	}
	for _, b := range []*backend{software, vaapi, qsv, qsvD3D11, nvenc, videoToolbox} {
		for _, c := range []media.Codec{media.H264, media.HEVC} {
			for _, f := range []media.Fidelity{media.FidelityHigh, media.FidelityStandard} {
				req := media.TranscodeRequest{Path: "/m/a.mkv", SegmentDuration: 6 * time.Second, Source: sdr1080,
					Video: media.VideoRendition{Codec: c, Bitrate: capBitrate, Fidelity: f}}
				args, err := transcodeArgs(req, testDevice(b, "0", media.GridGOP))
				if err != nil {
					t.Fatal(err)
				}
				avg, peak, buf := flag(args, "-b:v"), flag(args, "-maxrate:v"), flag(args, "-bufsize:v")
				if avg != "7600000" || peak != "8000000" || buf != "16000000" || avg == peak {
					t.Errorf("%s %s %q: -b:v %s -maxrate:v %s -bufsize:v %s, want 7600000, 8000000, 16000000",
						b.kind, c, f, avg, peak, buf)
				}
				for _, q := range []string{"-crf", "-cq:v", "-global_quality:v", "-qp"} {
					if slices.Contains(args, q) {
						t.Errorf("%s %s %q: a stream sets %s", b.kind, c, f, q)
					}
				}
			}
		}
	}
	if args := nvenc.rateControl(fill, media.H264, capBitrate, 20); flag(args, "-rc") != "vbr" {
		t.Errorf("NVENC stream: %q, want -rc vbr", args)
	}
	if args := software.rateControl(fill, media.H264, capBitrate, 18); flag(args, "-preset") != "faster" {
		t.Errorf("x264 stream: %q, want -preset faster", args)
	}
	if args := vaapi.rateControl(fill, media.H264, capBitrate, 0); flag(args, "-rc_mode") != "VBR" {
		t.Errorf("VA-API stream: %q, want -rc_mode VBR", args)
	}
}

// TestDownloadsAimAtQuality checks that a download aims at its encoder's
// quality value where the encoder has a quality mode, under the cap where
// that mode allows one.
func TestDownloadsAimAtQuality(t *testing.T) {
	const capBitrate = 8_000_000
	for _, tt := range []struct {
		b    *backend
		c    media.Codec
		want string
	}{
		{software, media.H264, "-preset veryfast -crf 18 -maxrate:v 7600000 -bufsize:v 15200000 -sc_threshold 0 -flags:v +cgop"},
		{qsv, media.H264, "-preset veryfast -global_quality:v 20 -b:v 7600000 -maxrate:v 8000000 -bufsize:v 16000000 -flags:v +cgop"},
		{qsv, media.HEVC, "-preset veryfast -global_quality:v 20 -flags:v +cgop"},
		{nvenc, media.HEVC, "-preset p4 -tune hq -rc vbr -cq:v 22 -b:v 0 -maxrate:v 7600000 -bufsize:v 15200000 -no-scenecut 1"},
		{vaapi, media.H264, "-rc_mode VBR -b:v 7600000 -maxrate:v 8000000 -bufsize:v 16000000 -flags:v +cgop"},
	} {
		q := tt.b.quality[media.FidelityHigh][tt.c]
		if got := strings.Join(tt.b.rateControl(aimQuality, tt.c, capBitrate, q), " "); got != tt.want {
			t.Errorf("%s %s: %s, want %s", tt.b.kind, tt.c, got, tt.want)
		}
	}
}

func TestUnsupportedRequests(t *testing.T) {
	copyVideo := media.VideoRendition{Codec: media.Copy}
	h264 := media.VideoRendition{Codec: media.H264, Bitrate: 4_000_000}
	video := func(src media.Track, v media.VideoRendition) media.TranscodeRequest {
		return media.TranscodeRequest{SegmentDuration: 6 * time.Second, Source: src, Video: v}
	}
	tests := []struct {
		name string
		req  media.TranscodeRequest
		dev  *device
	}{
		{"burn-in", media.TranscodeRequest{Video: copyVideo, BurnIn: &media.SubtitleRef{Track: 3}}, nil},
		{"audio codec", media.TranscodeRequest{Video: copyVideo,
			Audio: []media.AudioOut{{Track: 1, Out: media.AudioRendition{Codec: "opus"}}}}, nil},
		{"video codec", video(sdr1080, media.VideoRendition{Codec: "av1", Bitrate: 1}), untestedCPU()},
		{"no source", video(media.Track{}, h264), untestedCPU()},
		{"10-bit", video(sdr1080, media.VideoRendition{Codec: media.HEVC, BitDepth: 10, Bitrate: 1}), untestedCPU()},
		{"HEVC High", video(sdr1080, media.VideoRendition{Codec: media.HEVC, Profile: "high", Bitrate: 1}), untestedCPU()},
		{"no bitrate", video(sdr1080, media.VideoRendition{Codec: media.H264}), untestedCPU()},
		{"no grid", media.TranscodeRequest{Source: sdr1080, Video: h264}, untestedCPU()},
		{"HDR kept", video(hdr2160, h264), untestedCPU()},
		{"HDR10 kept", video(dv7, media.VideoRendition{Codec: media.H264, Bitrate: 1, HDR: media.StripDolbyVision}), untestedCPU()},
		{"encoder not passed", video(sdr1080, media.VideoRendition{Codec: media.HEVC, Bitrate: 1}),
			&device{b: nvenc, node: "0", encoders: map[media.Codec]media.GOPMode{media.H264: media.GridGOP}}},
	}
	for _, tt := range tests {
		if _, err := transcodeArgs(tt.req, tt.dev); !errors.Is(err, media.ErrUnsupported) {
			t.Errorf("%s: %v, want ErrUnsupported", tt.name, err)
		}
	}
	if _, err := imageArgs("/a.jpg", []media.ImageOut{{Width: 0, Path: "/b.webp"}}); !errors.Is(err, media.ErrUnsupported) {
		t.Errorf("image width 0: %v, want ErrUnsupported", err)
	}
}

func TestFit(t *testing.T) {
	tests := []struct{ w, h, maxW, maxH, wantW, wantH int }{
		{1920, 1080, 1280, 720, 1280, 720},
		{1920, 800, 1280, 720, 1280, 532},  // scope, where the width binds
		{1440, 1080, 1280, 720, 960, 720},  // 4:3, where the height binds
		{1280, 720, 1920, 1080, 1280, 720}, // never enlarged
		{1920, 803, 0, 0, 1920, 802},       // odd heights made even
		{3840, 2160, 1920, 0, 1920, 1080},  // one bound
		{720, 480, 640, 360, 540, 360},
	}
	for _, tt := range tests {
		if w, h := fit(tt.w, tt.h, tt.maxW, tt.maxH); w != tt.wantW || h != tt.wantH {
			t.Errorf("fit(%d×%d in %d×%d) = %d×%d, want %d×%d", tt.w, tt.h, tt.maxW, tt.maxH, w, h, tt.wantW, tt.wantH)
		}
	}
}

func TestFrameRate(t *testing.T) {
	for r, want := range map[float64]string{24: "24", 24000.0 / 1001: "24000/1001", 30000.0 / 1001: "30000/1001",
		60000.0 / 1001: "60000/1001", 12.5: "12.500"} {
		if got := frameRate(r); got != want {
			t.Errorf("frameRate(%v) = %s, want %s", r, got, want)
		}
	}
}

func TestFilterValue(t *testing.T) {
	got := filterValue(`/subs/Don't panic: [part 1], a;b\c.ass`)
	want := `/subs/Don\\\'t panic\\: \[part 1\]\, a\;b\\\\c.ass`
	if got != want {
		t.Errorf("got  %s\nwant %s", got, want)
	}
}

func TestDelayMoovForDolbyAudio(t *testing.T) {
	for _, c := range []struct {
		source media.Codec
		out    media.Codec
		delay  bool
	}{
		{"eac3", media.Copy, true},
		{"ac3", media.Copy, true},
		{"aac", media.Copy, false},
		{"eac3", media.AAC, false}, // converted, and AAC needs no delay
		{"truehd", media.EAC3, true},
		{"dts", media.AC3, true},
	} {
		args := strings.Join(appendMuxer(nil, []media.AudioOut{{Track: 1, Source: c.source, Out: media.AudioRendition{Codec: c.out}}}), " ")
		if got := strings.Contains(args, "delay_moov"); got != c.delay {
			t.Errorf("%s %s: delay_moov %v, want %v", c.source, c.out, got, c.delay)
		}
	}
}

// TestStereoDownmix checks that the dialogue-keeping mix is used only to
// bring surround down to stereo. Mono and stereo are never remixed, and
// surround to 5.1 uses ffmpeg's standard downmix.
func TestStereoDownmix(t *testing.T) {
	for _, c := range []struct {
		source, out int
		mixed       bool
	}{
		{6, 2, true},
		{8, 2, true},
		{3, 2, true},
		{2, 2, false},
		{1, 1, false},
		{0, 2, false}, // unknown channels get the plain -ac 2
		{8, 6, false},
		{6, 6, false},
	} {
		args, err := appendAudio(nil, 0, media.AudioOut{Track: 1, SourceChannels: c.source, Out: media.AudioRendition{Codec: media.AAC, Channels: c.out}})
		if err != nil {
			t.Fatal(err)
		}
		if got := slices.Contains(args, stereoDownmix); got != c.mixed {
			t.Errorf("%d to %d channels: dialogue downmix %v, want %v: %q", c.source, c.out, got, c.mixed, args)
		}
	}
}

// slashed writes the file: paths in args with /, as the golden files have
// them, whatever separator the OS joined them with.
func slashed(args []string) []string {
	out := slices.Clone(args)
	for i, a := range out {
		if p, ok := strings.CutPrefix(a, "file:"); ok {
			out[i] = "file:" + filepath.ToSlash(p)
		}
	}
	return out
}
