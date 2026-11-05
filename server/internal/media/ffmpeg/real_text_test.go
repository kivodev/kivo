package ffmpeg

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/mmcdole/kivo/internal/media"
)

// withText copies the corpus's h264.mkv, adding a text track in the given
// codec (srt, ass or webvtt) as track 2. The track has a cue every second,
// starting half a second in.
func withText(t *testing.T, codec string) string {
	t.Helper()
	ffmpegBin, _ := realTools(t)
	dir := t.TempDir()
	var srt strings.Builder
	for i := range 30 {
		fmt.Fprintf(&srt, "%d\n00:00:%02d,500 --> 00:00:%02d,900\nCue %d\n\n", i+1, i, i, i)
	}
	subs := filepath.Join(dir, "cues.srt")
	if err := os.WriteFile(subs, []byte(srt.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "text-"+codec+".mkv")
	out, err := exec.Command(ffmpegBin, "-hide_banner", "-loglevel", "error", "-i", filepath.Join(corpusDir(t), "h264.mkv"),
		"-i", subs, "-map", "0", "-map", "1", "-c", "copy", "-c:s", codec, path).CombinedOutput()
	if err != nil {
		t.Fatal(err, string(out))
	}
	return path
}

// TestRealTextTrackFromALateStart uses a source whose timestamps begin at
// 10 seconds, as an MPEG-TS recording's do. The probe reports that start.
// With copied timestamps, the cues come out on the source's timeline, and
// transcode cuts them into segments by spans shifted forward by that start.
func TestRealTextTrackFromALateStart(t *testing.T) {
	m := realMedia(t)
	ffmpegBin, _ := realTools(t)
	dir := t.TempDir()
	subs := filepath.Join(dir, "cues.srt")
	if err := os.WriteFile(subs, []byte("1\n00:00:03,500 --> 00:00:03,900\nCue 3\n\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "late.mkv")
	out, err := exec.Command(ffmpegBin, "-hide_banner", "-loglevel", "error", "-f", "lavfi", "-i", "testsrc=duration=6:size=160x90:rate=25",
		"-i", subs, "-map", "0", "-map", "1", "-c:v", "libx264", "-preset", "ultrafast", "-c:s", "srt", "-output_ts_offset", "10", path).CombinedOutput()
	if err != nil {
		t.Fatal(err, string(out))
	}
	res, err := m.Probe(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	if res.Start != 10*time.Second {
		t.Fatalf("start %v, want 10s", res.Start)
	}
	req := media.TranscodeRequest{Path: path, Start: 2 * time.Second, SegmentDuration: 6 * time.Second,
		Source: res.Tracks[0], Video: media.VideoRendition{Codec: media.Copy},
		Text: []media.CuesOut{{Track: 1, Path: filepath.Join(t.TempDir(), "text-1.srt")}}}
	o, err := m.Transcode(t.Context(), req)
	if err != nil {
		t.Fatal(err)
	}
	defer o.Close()
	if _, err := o.Init(); err != nil {
		t.Fatal(err)
	}
	for _, err := range o.Segments() {
		if err != nil {
			t.Fatal(err, o.Diagnostics().Stderr)
		}
	}
	cues, err := o.Cues(0)
	if err != nil {
		t.Fatal(err)
	}
	if len(cues) != 1 || cues[0].Start != 13500*time.Millisecond {
		t.Fatalf("cues %+v, want Cue 3 at 13.5s, on the source's timestamps", cues)
	}
}

// TestRealTextTrack reads a text track along with the video, both remuxed
// and encoded, from a seek. Once each segment is out, every cue that starts
// before its end must have been read, so a text segment can go out with
// it.
func TestRealTextTrack(t *testing.T) {
	m := realMedia(t)
	for _, codec := range []string{"srt", "ass", "webvtt"} {
		path := withText(t, codec)
		for _, video := range []media.VideoRendition{{Codec: media.Copy}, {Codec: media.H264, Bitrate: 300_000}} {
			t.Run(codec+" "+string(video.Codec), func(t *testing.T) {
				scratch := filepath.Join(t.TempDir(), "text-1.srt")
				req := media.TranscodeRequest{Path: path, Start: 12 * time.Second, SegmentDuration: 6 * time.Second,
					Source: videoTrack(t, m, path), Video: video,
					Audio: []media.AudioOut{{Track: 1, Out: media.AudioRendition{Codec: media.Copy}}},
					Text:  []media.CuesOut{{Track: 2, Path: scratch}}}
				out, err := m.Transcode(t.Context(), req)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := out.Init(); err != nil {
					t.Fatal(err)
				}
				var cues []media.Cue
				for seg, err := range out.Segments() {
					if err != nil {
						t.Fatal(err, out.Diagnostics().Stderr)
					}
					got, err := out.Cues(0)
					if err != nil {
						t.Fatal(err)
					}
					cues = append(cues, got...)
					// Every cue starting before this segment's start has been
					// read, so the segment before it can go out complete.
					for c := 12 * time.Second; c+500*time.Millisecond < seg.Start; c += time.Second {
						if !slices.ContainsFunc(cues, func(x media.Cue) bool { return x.Start == c+500*time.Millisecond }) {
							t.Fatalf("by the segment at %v, the cue at %v isn't read", seg.Start, c+500*time.Millisecond)
						}
					}
				}
				got, err := out.Cues(0)
				if err != nil {
					t.Fatal(err)
				}
				cues = append(cues, got...)
				if err := out.Close(); err != nil {
					t.Fatal(err)
				}
				// The input seek lands on a keyframe at or before the start,
				// so cues before it may come too.
				i := slices.IndexFunc(cues, func(c media.Cue) bool { return c.Text == "Cue 12" })
				if i < 0 || len(cues)-i != 18 || cues[i].Start != 12500*time.Millisecond || cues[i].End != 12900*time.Millisecond ||
					cues[len(cues)-1].Text != "Cue 29" {
					t.Fatalf("cues %+v", cues)
				}
				if _, err := os.Stat(scratch); !errors.Is(err, fs.ErrNotExist) {
					t.Fatalf("the scratch file is left: %v", err)
				}
			})
		}
	}
}
