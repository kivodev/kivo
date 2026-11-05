//go:build linux

package ffmpeg

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/mmcdole/kivo/internal/media"
)

// machine lays out a machine's device nodes and sysfs under a temporary
// root. The machine has an Intel GPU the server can use, an AMD GPU it may
// not open, another AMD GPU whose node wasn't passed into the container,
// and an NVIDIA GPU.
func machine(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	files := map[string]string{
		"sys/class/drm/renderD128/device/vendor":           "0x8086\n",
		"dev/dri/renderD128":                               "",
		"sys/class/drm/renderD129/device/vendor":           "0x1002\n",
		"dev/dri/renderD129":                               "",
		"sys/class/drm/renderD130/device/vendor":           "0x1002\n",
		"dev/nvidia0":                                      "",
		"dev/nvidiactl":                                    "",
		"proc/driver/nvidia/gpus/0000:01:00.0/information": "Model: \t\t NVIDIA GeForce RTX 3060\nIRQ: 145\n",
		"sys/bus/pci/devices/0000:01:00.0/vendor":          "0x10de\n",
		"sys/bus/pci/devices/0000:01:00.0/class":           "0x030000\n",
	}
	for name, body := range files {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Chmod(filepath.Join(root, "dev/dri/renderD129"), 0); err != nil {
		t.Fatal(err)
	}
	return root
}

// TestFakeAcceleratorTest runs the accelerator test against a fake ffmpeg
// on a fake machine. Quick Sync passes everything except OpenCL tone
// mapping. VA-API's encoder ignores forced keyframes and has no HEVC.
// CUDA's libraries are missing. This ffmpeg can't make AV1.
func TestFakeAcceleratorTest(t *testing.T) {
	stderr := func(name string) string { return fixture(t, "stderr/"+name) }
	r := useFake(t, "hardware", fixture(t, "hw-grid.mp4"))
	r.root = machine(t)
	t.Setenv(fakeRules, strings.Join([]string{
		"libsvtav1=" + stderr("unsupported-no-filter.txt"),
		"h264_vaapi+force_key_frames=" + fixture(t, "hw-offgrid.mp4"),
		"hevc_vaapi=" + stderr("busy-vaapi-allocation.txt"),
		"tonemap_opencl=" + stderr("failed-open-encoder.txt"),
		"cuda=" + stderr("failed-cuda-runtime.txt"),
	}, ";"))

	caps := r.DetectAccelerators(t.Context(), t.TempDir())

	grid := func(c media.Codec) media.EncoderCaps { return media.EncoderCaps{Codec: c, GOP: media.GridGOP} }
	decoded := []media.VideoFormat{{Codec: media.H264, BitDepth: 8}, {Codec: media.HEVC, BitDepth: 8}, {Codec: media.HEVC, BitDepth: 10}}
	want := []media.Caps{
		{Accelerator: "qsv:/dev/dri/renderD128", Backend: media.QSV, Name: "Intel Quick Sync (QSV)",
			Encoders: []media.EncoderCaps{grid(media.H264), grid(media.HEVC)}, Decoders: decoded,
			ToneMap: media.OnCPU, BurnIn: media.OnAccelerator},
		{Accelerator: "vaapi:/dev/dri/renderD128", Backend: media.VAAPI, Name: "Intel VA-API",
			Encoders: []media.EncoderCaps{{Codec: media.H264, GOP: media.FixedGOP}}, Decoders: decoded,
			ToneMap: media.OnCPU, BurnIn: media.OnAccelerator},
	}
	if !caps.Tested || len(caps.Accelerators) != len(want) {
		t.Fatalf("accelerators %+v, want %+v", caps.Accelerators, want)
	}
	for i := range want {
		if got := caps.Accelerators[i]; !equalCaps(got, want[i]) {
			t.Errorf("accelerator %d:\ngot  %+v\nwant %+v", i, got, want[i])
		}
	}
	cpu := media.Caps{Backend: media.Software, Name: "CPU",
		Encoders: []media.EncoderCaps{grid(media.H264), grid(media.HEVC)}, ToneMap: media.OnCPU, BurnIn: media.OnCPU}
	if !equalCaps(caps.CPU, cpu) {
		t.Errorf("CPU:\ngot  %+v\nwant %+v", caps.CPU, cpu)
	}

	reasons := map[media.Accelerator]media.ProblemReason{}
	for _, p := range caps.Problems {
		reasons[p.Accelerator] = p.Reason
		if p.Detail == "" {
			t.Errorf("%s: no detail", p.Accelerator)
		}
	}
	wantReasons := map[media.Accelerator]media.ProblemReason{
		"vaapi:/dev/dri/renderD130": media.NoDeviceNode,
		"nvenc:0":                   media.NoDriver,
	}
	if os.Geteuid() != 0 { // root opens anything
		wantReasons["vaapi:/dev/dri/renderD129"] = media.NoPermission
	}
	for d, reason := range wantReasons {
		if reasons[d] != reason {
			t.Errorf("%s: reason %q, want %q (problems %+v)", d, reasons[d], reason, caps.Problems)
		}
	}
	if got := r.Capabilities(); len(got.Accelerators) != len(caps.Accelerators) {
		t.Errorf("Capabilities() has %d accelerators, the test found %d", len(got.Accelerators), len(caps.Accelerators))
	}

	// Each job's command line follows what its device passed.
	commands := map[string]media.TranscodeRequest{}
	req := func(d media.Accelerator, src media.Track, hdr media.HDRMode) media.TranscodeRequest {
		return media.TranscodeRequest{Path: "/m/a.mkv", Start: 6 * time.Second, SegmentDuration: 6 * time.Second, Source: src, Accelerator: d,
			Video: media.VideoRendition{Codec: media.H264, Width: 1280, Height: 720, Bitrate: 4_000_000, HDR: hdr}}
	}
	commands["grid on qsv"] = req("qsv:/dev/dri/renderD128", sdr1080, media.PassThrough)
	commands["fixed GOP on vaapi"] = req("vaapi:/dev/dri/renderD128", sdr1080, media.PassThrough)
	commands["tone-mapped on the CPU for qsv"] = req("qsv:/dev/dri/renderD128", hdr2160, media.ToneMap)
	checks := map[string]func(string) bool{
		"grid on qsv": func(c string) bool {
			return strings.Contains(c, "-hwaccel vaapi") && strings.Contains(c, "-force_key_frames:v") && strings.Contains(c, "h264_qsv")
		},
		"fixed GOP on vaapi": func(c string) bool {
			return !strings.Contains(c, "-force_key_frames") && strings.Contains(c, "-g:v 144") && strings.Contains(c, "h264_vaapi")
		},
		"tone-mapped on the CPU for qsv": func(c string) bool {
			return !strings.Contains(c, "-hwaccel") && strings.Contains(c, "tonemapx") && strings.Contains(c, "hwupload")
		},
	}
	for name, req := range commands {
		out, err := r.Transcode(t.Context(), req)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if c := out.Diagnostics().Command; !checks[name](c) {
			t.Errorf("%s: command %s", name, c)
		}
		out.Close()
	}
	for _, d := range []media.Accelerator{"nvenc:0", "vaapi:/dev/dri/renderD129", "amf:0"} {
		if _, err := r.Transcode(t.Context(), req(d, sdr1080, media.PassThrough)); !errors.Is(err, media.ErrUnsupported) {
			t.Errorf("transcode on %s: %v, want ErrUnsupported", d, err)
		}
	}
	hevc := req("vaapi:/dev/dri/renderD128", sdr1080, media.PassThrough)
	hevc.Video.Codec = media.HEVC
	if _, err := r.Transcode(t.Context(), hevc); !errors.Is(err, media.ErrUnsupported) {
		t.Errorf("HEVC on vaapi, which failed it: %v, want ErrUnsupported", err)
	}
}

func equalCaps(a, b media.Caps) bool {
	return a.Accelerator == b.Accelerator && a.Backend == b.Backend && a.Name == b.Name && slices.Equal(a.Encoders, b.Encoders) &&
		slices.Equal(a.Decoders, b.Decoders) && a.ToneMap == b.ToneMap && a.BurnIn == b.BurnIn
}

// TestFakeAcceleratorTestNothingPasses checks that a device on which no
// encoder passes is reported as a problem, not a device, and that a CPU
// without encoders leaves nothing.
func TestFakeAcceleratorTestNothingPasses(t *testing.T) {
	r := useFake(t, "hardware", fixture(t, "hw-grid.mp4"))
	r.root = t.TempDir()
	t.Setenv(fakeRules, "libx264+pipe:1="+fixture(t, "stderr/unsupported-no-filter.txt")+
		";libx265="+fixture(t, "hw-offgrid.mp4"))
	caps := r.DetectAccelerators(t.Context(), t.TempDir())
	if !caps.Tested || len(caps.Accelerators) != 0 || len(caps.Problems) != 0 || len(caps.CPU.Encoders) != 0 {
		t.Fatalf("got %+v, want nothing that encodes", caps)
	}
}

// TestRealAcceleratorTest runs the accelerator test on this machine. Its
// CPU must pass both encoders on the grid. Any GPU it offers must encode
// something.
func TestRealAcceleratorTest(t *testing.T) {
	r := New(realTools(t))
	caps := r.DetectAccelerators(t.Context(), t.TempDir())
	cpu := caps.CPU
	if cpu.Backend != media.Software || len(cpu.Encoders) != 2 ||
		cpu.Encoders[0].GOP != media.GridGOP || cpu.Encoders[1].GOP != media.GridGOP {
		t.Errorf("CPU %+v, want H.264 and HEVC with keyframes on the grid", cpu)
	}
	if (cpu.ToneMap == media.OnCPU) != hasFilter(t, "tonemapx") || (cpu.BurnIn == media.OnCPU) != hasFilter(t, "subtitles") {
		t.Errorf("CPU tone mapping %q and burn-in %q don't match this ffmpeg's filters", cpu.ToneMap, cpu.BurnIn)
	}
	for _, d := range caps.Accelerators {
		t.Logf("%s (%s): encoders %+v, decoders %v, tone map %q, burn-in %q", label(d.Accelerator), d.Name, d.Encoders, d.Decoders, d.ToneMap, d.BurnIn)
		if len(d.Encoders) == 0 {
			t.Errorf("%s offered without an encoder", label(d.Accelerator))
		}
	}
	for _, p := range caps.Problems {
		t.Logf("%s (%s): %s: %s", label(p.Accelerator), p.Name, p.Reason, p.Detail)
	}
}
