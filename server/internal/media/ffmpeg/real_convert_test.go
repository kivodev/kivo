package ffmpeg

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/mmcdole/kivo/internal/media"
)

// TestRealConvert makes MP4 files that Apple's players play from the
// corpus's MKVs. It remuxes H.264 and HEVC, tagged avc1 and hvc1, and
// encodes H.264 as HEVC, with AAC throughout.
func TestRealConvert(t *testing.T) {
	m, dir := realMedia(t), corpusDir(t)
	tests := []struct {
		name, src string
		video     media.VideoRendition
		tag       string
	}{
		{"h264 remux", "h264.mkv", media.VideoRendition{Codec: media.Copy}, "avc1"},
		{"hevc remux", "hevc.mkv", media.VideoRendition{Codec: media.Copy}, "hvc1"},
		{"h264 to hevc", "h264.mkv", media.VideoRendition{Codec: media.HEVC, Width: 160, Height: 120, Bitrate: 300_000}, "hvc1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := filepath.Join(dir, tt.src)
			dst := filepath.Join(t.TempDir(), "out.mp4")
			var last time.Duration
			err := m.Convert(t.Context(), media.ConvertRequest{
				Path: src, Source: videoTrack(t, m, src), Video: tt.video, Dst: dst,
				Audio:   []media.AudioOut{{Track: 1, Out: media.AudioRendition{Codec: media.Copy}}},
				Reached: func(d time.Duration) { last = d },
			})
			if err != nil {
				t.Fatal(err)
			}
			if last < 20*time.Second {
				t.Errorf("progress reached %v of 30s", last)
			}
			got := ffprobeStreams(t, dst)
			if got.Format.FormatName != "mov,mp4,m4a,3gp,3g2,mj2" || len(got.Streams) != 2 {
				t.Fatalf("output is %+v", got)
			}
			if v := got.Streams[0]; v.CodecTag != tt.tag {
				t.Errorf("video tagged %q, want %q", v.CodecTag, tt.tag)
			}
			if a := got.Streams[1]; a.CodecName != "aac" || a.CodecTag != "mp4a" {
				t.Errorf("audio is %s (%s), want AAC", a.CodecName, a.CodecTag)
			}
			if d, _ := strconv.ParseFloat(got.Format.Duration, 64); d < 29 || d > 31 {
				t.Errorf("duration %vs, want 30s", d)
			}
			b, err := os.ReadFile(dst)
			if err != nil {
				t.Fatal(err)
			}
			decodes(t, b)
		})
	}
}

// TestRealConvertKeepsLanguages converts a file with two audio tracks
// tagged by language, as an MKV's own tags have them. It also has two text
// subtitles, one embedded ASS and one SubRip file beside it. The MP4 must
// have each text track as mov_text, every audio and text track must keep
// its language, and the source's own title must be dropped.
func TestRealConvertKeepsLanguages(t *testing.T) {
	m, dir := realMedia(t), corpusDir(t)
	ffmpegBin, ffprobeBin := realTools(t)
	work := t.TempDir()
	srt := filepath.Join(work, "fr.srt")
	if err := os.WriteFile(srt, []byte("1\n00:00:01,000 --> 00:00:02,000\nBonjour\n\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(work, "dual.mkv")
	out, err := exec.Command(ffmpegBin, "-hide_banner", "-loglevel", "error", "-i", filepath.Join(dir, "h264.mkv"), "-i", srt,
		"-map", "0:v", "-map", "0:a", "-map", "0:a", "-map", "1", "-c", "copy", "-c:s", "ass",
		"-metadata:s:a:0", "language=eng", "-metadata:s:a:1", "language=jpn", "-metadata:s:s:0", "language=eng",
		"-metadata", "title=Some.Release.Group", src).CombinedOutput()
	if err != nil {
		t.Fatal(err, string(out))
	}
	dst := filepath.Join(work, "out.mp4")
	err = m.Convert(t.Context(), media.ConvertRequest{
		Path: src, Source: videoTrack(t, m, src), Video: media.VideoRendition{Codec: media.Copy}, Dst: dst,
		Audio: []media.AudioOut{
			{Track: 2, Language: "ja", Out: media.AudioRendition{Codec: media.Copy}},
			{Track: 1, Language: "en", Out: media.AudioRendition{Codec: media.Copy}},
		},
		Text: []media.TextOut{{Track: 3, Title: "English", Language: "en"}, {Path: srt, Language: "fr"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	b, err := exec.Command(ffprobeBin, "-v", "error", "-show_entries", "stream=codec_type,codec_name:stream_tags=language:format_tags=title",
		"-of", "json", dst).Output()
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Streams []struct {
			Type  string            `json:"codec_type"`
			Codec string            `json:"codec_name"`
			Tags  map[string]string `json:"tags"`
		} `json:"streams"`
		Format struct {
			Tags map[string]string `json:"tags"`
		} `json:"format"`
	}
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	var tracks []string
	for _, s := range got.Streams {
		tracks = append(tracks, s.Type+" "+s.Codec+" "+s.Tags["language"])
	}
	want := []string{"video h264 und", "audio aac jpn", "audio aac eng", "subtitle mov_text eng", "subtitle mov_text fra"}
	if !slices.Equal(tracks, want) {
		t.Errorf("tracks %q, want %q", tracks, want)
	}
	if title := got.Format.Tags["title"]; title != "" {
		t.Errorf("the source's title %q is kept", title)
	}
}

type probed struct {
	Streams []struct {
		CodecName string `json:"codec_name"`
		CodecTag  string `json:"codec_tag_string"`
	} `json:"streams"`
	Format struct {
		FormatName string `json:"format_name"`
		Duration   string `json:"duration"`
	} `json:"format"`
}

// ffprobeStreams asks ffprobe what a file holds.
func ffprobeStreams(t *testing.T, path string) probed {
	t.Helper()
	_, ffprobeBin := realTools(t)
	out, err := exec.Command(ffprobeBin, "-v", "error", "-show_entries", "stream=codec_name,codec_tag_string:format=format_name,duration",
		"-of", "json", path).Output()
	if err != nil {
		t.Fatal(err)
	}
	var p probed
	if err := json.Unmarshal(out, &p); err != nil {
		t.Fatal(err)
	}
	return p
}
