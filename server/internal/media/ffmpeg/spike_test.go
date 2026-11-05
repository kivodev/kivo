package ffmpeg

import (
	"fmt"
	"iter"
	"math/rand/v2"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/mmcdole/kivo/internal/media"
)

// Harnesses for timing far seeks and checking HEVC and Dolby Vision
// packaging on real hardware. They run only when KIVO_SPIKE_FILE names a source file,
// with a real ffmpeg in KIVO_FFMPEG, and are configured by environment:
//
//	KIVO_SPIKE_FILE         the source file
//	KIVO_SPIKE_ACCELERATOR  the accelerator, as Capabilities names it; empty for the CPU
//	KIVO_SPIKE_CODEC        h264 (default), hevc, or copy
//	KIVO_SPIKE_SIZE         the bounding size, such as 1920x1080 (default); empty keeps the source's
//	KIVO_SPIKE_BITRATE      bits per second (default 8000000)
//	KIVO_SPIKE_SEGMENT      the segment grid (default 6s)
//	KIVO_SPIKE_SEEKS        far seeks to time (default 20)
//	KIVO_SPIKE_HDR          tonemap (the default for HDR sources), strip, or keep
//	KIVO_SPIKE_OUT          where TestSpikePackage writes its HLS stream
//	KIVO_SPIKE_LENGTH   how much TestSpikePackage writes (default 2m)

// spikeRequest probes the source and builds the request the environment
// describes. It returns the request, the runner with its accelerators
// tested, and the source's duration.
func spikeRequest(t *testing.T) (*Runner, media.TranscodeRequest, time.Duration) {
	path := os.Getenv("KIVO_SPIKE_FILE")
	if path == "" {
		t.Skip("KIVO_SPIKE_FILE not set")
	}
	r := New(realTools(t))
	caps := r.DetectAccelerators(t.Context(), filepath.Join(t.TempDir(), "media"))
	for _, d := range append(caps.Accelerators, caps.CPU) {
		t.Logf("%s (%s): encoders %v, decoders %v, tone map %q, burn-in %q", label(d.Accelerator), d.Name, d.Encoders, d.Decoders, d.ToneMap, d.BurnIn)
	}
	for _, p := range caps.Problems {
		t.Logf("problem %s (%s): %s: %s", label(p.Accelerator), p.Name, p.Reason, p.Detail)
	}
	res, err := r.Probe(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	i := slices.IndexFunc(res.Tracks, func(tr media.Track) bool { return tr.Kind == media.Video && !tr.AttachedPicture })
	if i < 0 {
		t.Fatal("no video track")
	}
	src := res.Tracks[i]
	req := media.TranscodeRequest{Path: path, SegmentDuration: envDuration(t, "KIVO_SPIKE_SEGMENT", 6*time.Second),
		VideoTrack: src.Index, Source: src, Accelerator: media.Accelerator(os.Getenv("KIVO_SPIKE_ACCELERATOR")),
		Video: media.VideoRendition{Codec: media.Codec(cmpEnv("KIVO_SPIKE_CODEC", "h264")),
			Bitrate: envInt(t, "KIVO_SPIKE_BITRATE", 8_000_000)}}
	if size := cmpEnv("KIVO_SPIKE_SIZE", "1920x1080"); size != "" {
		w, h, _ := strings.Cut(size, "x")
		req.Video.Width, _ = strconv.Atoi(w)
		req.Video.Height, _ = strconv.Atoi(h)
	}
	hdr := src.Range != media.SDR || src.DolbyVisionProfile != 0
	switch os.Getenv("KIVO_SPIKE_HDR") {
	case "strip":
		req.Video.HDR = media.StripDolbyVision
	case "keep":
	default:
		if hdr && req.Video.Codec != media.Copy {
			req.Video.HDR = media.ToneMap
		}
	}
	if a := slices.IndexFunc(res.Tracks, func(tr media.Track) bool { return tr.Kind == media.Audio }); a >= 0 {
		req.Audio = []media.AudioOut{{Track: res.Tracks[a].Index, Out: media.AudioRendition{Codec: media.AAC, Channels: 2}}}
	}
	t.Logf("source: %s %d-bit %dx%d at %.3f fps, %s, Dolby Vision profile %d, %v long",
		src.Codec, src.BitDepth, src.Width, src.Height, src.FrameRate, src.Range, src.DolbyVisionProfile, res.Duration)
	return r, req, res.Duration
}

// TestSpikeFarSeek times far seeks. It starts jobs at grid points spread
// over the file. Each job is timed to its init segment, which needs the
// input opened and the device and encoder set up, and to its first segment.
func TestSpikeFarSeek(t *testing.T) {
	r, req, duration := spikeRequest(t)
	seeks := envInt(t, "KIVO_SPIKE_SEEKS", 20)
	steps := int(duration*8/10/req.SegmentDuration) - 1
	if steps < 1 {
		t.Fatal("the file is too short")
	}
	rng := rand.New(rand.NewPCG(1, 2)) // the same positions every run
	var inits, firsts []time.Duration
	t.Logf("%8s %10s %10s", "seek to", "init", "segment")
	for i := range seeks {
		req.Start = duration/10 + time.Duration(rng.IntN(steps))*req.SegmentDuration
		req.Start -= req.Start % req.SegmentDuration
		began := time.Now()
		out, err := r.Transcode(t.Context(), req)
		if err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			t.Logf("command: %s", out.Diagnostics().Command)
		}
		if _, err := out.Init(); err != nil {
			t.Fatal(err, out.Diagnostics().Stderr)
		}
		init := time.Since(began)
		next, stop := iter.Pull2(out.Segments())
		if _, err, ok := next(); !ok || err != nil {
			t.Fatal(err, out.Diagnostics().Stderr)
		}
		first := time.Since(began)
		stop()
		out.Close()
		inits, firsts = append(inits, init), append(firsts, first)
		t.Logf("%8v %10v %10v", req.Start, init.Round(time.Millisecond), first.Round(time.Millisecond))
	}
	t.Logf("init:          median %v, p95 %v", percentile(inits, 50), percentile(inits, 95))
	t.Logf("first segment: median %v, p95 %v", percentile(firsts, 50), percentile(firsts, 95))
}

// TestSpikePackage writes the request's output from the start as an HLS
// stream, one segment per fragment, for players to try.
func TestSpikePackage(t *testing.T) {
	r, req, _ := spikeRequest(t)
	dir := os.Getenv("KIVO_SPIKE_OUT")
	if dir == "" {
		t.Fatal("KIVO_SPIKE_OUT not set")
	}
	length := envDuration(t, "KIVO_SPIKE_LENGTH", 2*time.Minute)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	out, err := r.Transcode(t.Context(), req)
	if err != nil {
		t.Fatal(err)
	}
	defer out.Close()
	t.Logf("command: %s", out.Diagnostics().Command)
	init, err := out.Init()
	if err != nil {
		t.Fatal(err, out.Diagnostics().Stderr)
	}
	if err := os.WriteFile(filepath.Join(dir, "init.mp4"), init, 0o644); err != nil {
		t.Fatal(err)
	}
	var segs []media.Segment
	for seg, err := range out.Segments() {
		if err != nil {
			t.Fatal(err, out.Diagnostics().Stderr)
		}
		if seg.Start >= length {
			break
		}
		name := fmt.Sprintf("seg-%04d.m4s", len(segs))
		if err := os.WriteFile(filepath.Join(dir, name), seg.Data, 0o644); err != nil {
			t.Fatal(err)
		}
		segs = append(segs, media.Segment{Start: seg.Start})
	}
	if len(segs) < 2 {
		t.Fatal("fewer than two segments")
	}
	var list strings.Builder
	var longest time.Duration
	for i := range segs[:len(segs)-1] {
		longest = max(longest, segs[i+1].Start-segs[i].Start)
	}
	fmt.Fprintf(&list, "#EXTM3U\n#EXT-X-VERSION:7\n#EXT-X-TARGETDURATION:%d\n#EXT-X-PLAYLIST-TYPE:VOD\n#EXT-X-INDEPENDENT-SEGMENTS\n#EXT-X-MAP:URI=\"init.mp4\"\n",
		int(longest.Round(time.Second)/time.Second))
	for i := range segs[:len(segs)-1] { // the last one's length is unknown
		fmt.Fprintf(&list, "#EXTINF:%.3f,\nseg-%04d.m4s\n", (segs[i+1].Start - segs[i].Start).Seconds(), i)
	}
	list.WriteString("#EXT-X-ENDLIST\n")
	if err := os.WriteFile(filepath.Join(dir, "index.m3u8"), []byte(list.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Logf("wrote %d segments to %s", len(segs)-1, dir)
}

func percentile(ds []time.Duration, p int) time.Duration {
	s := slices.Sorted(slices.Values(ds))
	return s[(len(s)-1)*p/100].Round(time.Millisecond)
}

func cmpEnv(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok {
		return v
	}
	return fallback
}

func envInt(t *testing.T, key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		t.Fatalf("%s: %v", key, err)
	}
	return n
}

func envDuration(t *testing.T, key string, fallback time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		t.Fatalf("%s: %v", key, err)
	}
	return d
}
