package ffmpeg

import (
	"slices"
	"testing"
	"time"

	"github.com/mmcdole/kivo/internal/media"
)

func TestHangsQSVAfterOpenCL(t *testing.T) {
	for _, tt := range []struct {
		release string
		want    bool
	}{
		{"5.17.15", false},
		{"5.18.0", true},
		{"5.19.0-50-generic", true},
		{"6.0.17", true},
		{"6.0.18", false},
		{"6.0.18-arch1", false},
		{"6.1.3", true},
		{"6.1.4", false},
		{"6.2", false},
		{"6.6.78-Unraid", false},
		{"garbage", false},
		{"", false},
	} {
		if got := parseKernelVersion(tt.release).hangsQSVAfterOpenCL(); got != tt.want {
			t.Errorf("%q: %v, want %v", tt.release, got, tt.want)
		}
	}
}

func TestParseKernelVersion(t *testing.T) {
	for release, want := range map[string]kernelVersion{
		"5.19.0-50-generic": {5, 19, 0},
		"6.0.18-arch1":      {6, 0, 18},
		"6.6.78-Unraid":     {6, 6, 78},
		"6.2":               {6, 2, 0},
		"6.1-rc3":           {6, 1, 0},
		"6":                 {},
		"x.1.2":             {},
	} {
		if got := parseKernelVersion(release); got != want {
			t.Errorf("%q: %v, want %v", release, got, want)
		}
	}
}

// TestAsyncDepthOnHangingKernels checks that a Quick Sync encode of video
// decoded and tone-mapped on the GPU sets an async depth of 1 on a kernel
// whose i915 driver can hang, and that nothing else does.
func TestAsyncDepthOnHangingKernels(t *testing.T) {
	affected, fixed := parseKernelVersion("5.19.0-50-generic"), parseKernelVersion("6.6.78-Unraid")
	toneMapped := media.VideoRendition{Codec: media.H264, Width: 1920, Height: 1080, Bitrate: 8_000_000, HDR: media.ToneMap}
	for _, tt := range []struct {
		name   string
		b      *backend
		kernel kernelVersion
		src    media.Track
		cpuMap bool // the device's tone map failed, so the CPU tone-maps
		want   bool
	}{
		{"qsv, OpenCL tone map, affected kernel", qsv, affected, hdr2160, false, true},
		{"qsv, OpenCL tone map, fixed kernel", qsv, fixed, hdr2160, false, false},
		{"qsv, OpenCL tone map, unknown kernel", qsv, kernelVersion{}, hdr2160, false, false},
		{"qsv, CPU tone map, affected kernel", qsv, affected, hdr2160, true, false},
		{"qsv, SDR, affected kernel", qsv, affected, sdr1080, false, false},
		{"vaapi, OpenCL tone map, affected kernel", vaapi, affected, hdr2160, false, false},
	} {
		dev := testDevice(tt.b, "/dev/dri/renderD128", media.GridGOP)
		dev.kernel = tt.kernel
		dev.toneMap = !tt.cpuMap
		req := media.TranscodeRequest{Path: "/m/a.mkv", SegmentDuration: 6 * time.Second, Source: tt.src, Video: toneMapped,
			Accelerator: acceleratorOf(tt.b, "/dev/dri/renderD128")}
		args, err := transcodeArgs(req, dev)
		if err != nil {
			t.Fatalf("%s: %v", tt.name, err)
		}
		i := slices.Index(args, "-async_depth")
		if got := i >= 0 && args[i+1] == "1"; got != tt.want || (i >= 0) != tt.want {
			t.Errorf("%s: async depth 1 %v, want %v: %q", tt.name, got, tt.want, args)
		}
	}
}
