//go:build linux

package ffmpeg

import (
	"context"
	"errors"
	"iter"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/mmcdole/kivo/internal/media"
)

func remux(start time.Duration) media.TranscodeRequest {
	return media.TranscodeRequest{Path: "/m/a.mkv", Start: start, Video: media.VideoRendition{Codec: media.Copy},
		Audio: []media.AudioOut{{Track: 1, Out: media.AudioRendition{Codec: media.Copy}}}}
}

// collect ranges over out's segments, returning their starts and the error
// that ended them.
func collect(out media.Output) ([]time.Duration, error) {
	var starts []time.Duration
	for seg, err := range out.Segments() {
		if err != nil {
			return starts, err
		}
		starts = append(starts, seg.Start)
	}
	return starts, nil
}

func pid(out media.Output) int { return out.(*output).p.cmd.Process.Pid }

// gone reports whether pid has exited, polling briefly. A process killed
// with its group may take a moment to be reaped by init.
func gone(pid int) bool {
	for range 100 {
		b, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
		if err != nil {
			return true
		}
		// The state follows the parenthesized command name.
		if s := string(b); strings.HasPrefix(s[strings.LastIndexByte(s, ')')+2:], "Z") {
			return true
		}
		time.Sleep(20 * time.Millisecond)
	}
	return false
}

func TestFakeRemux(t *testing.T) {
	r := useFake(t, "output", fixture(t, "remux-h264.mp4"))
	out, err := r.Transcode(t.Context(), remux(2400*time.Millisecond))
	if err != nil {
		t.Fatal(err)
	}
	defer out.Close()
	init, err := out.Init()
	if err != nil || len(init) == 0 {
		t.Fatalf("Init: %d bytes, %v", len(init), err)
	}
	starts, err := collect(out)
	if err != nil {
		t.Fatal(err)
	}
	// The fixture's seek landed on the keyframe before 2.4 s.
	want := []time.Duration{1600 * time.Millisecond, 2400 * time.Millisecond, 4 * time.Second, 6200 * time.Millisecond}
	if len(starts) != len(want) {
		t.Fatalf("segments start at %v, want %v", starts, want)
	}
	if d := out.Diagnostics(); d.ExitCode != 0 || !strings.Contains(d.Command, "file:/m/a.mkv") {
		t.Errorf("diagnostics %+v", d)
	}
}

func TestFakeErrorClasses(t *testing.T) {
	files, err := filepath.Glob(fixture(t, "stderr/*.txt"))
	if err != nil || len(files) == 0 {
		t.Fatal("no stderr fixtures", err)
	}
	for _, f := range files {
		name := filepath.Base(f)
		t.Run(name, func(t *testing.T) {
			want := kindFromName(t, name)
			r := useFake(t, "fail", f)
			_, err := r.Probe(t.Context(), "/m/a.mkv")
			var me *media.Error
			if !errors.As(err, &me) || me.Kind != want {
				t.Fatalf("Probe: %v, want %v", err, want)
			}
			if me.Diagnostics.ExitCode != 1 || me.Diagnostics.Stderr == "" || me.Detail == "" {
				t.Errorf("diagnostics %+v, detail %q", me.Diagnostics, me.Detail)
			}
			out, err := r.Transcode(t.Context(), remux(0))
			if err != nil {
				t.Fatal(err)
			}
			defer out.Close()
			if _, err := out.Init(); !errors.Is(err, want) {
				t.Fatalf("Init: %v, want %v", err, want)
			}
		})
	}
}

func TestFakeUnrecognizedFailure(t *testing.T) {
	r := useFake(t, "fail", "")
	if _, err := r.Probe(t.Context(), "/m/a.mkv"); !errors.Is(err, media.ErrSourceUnreadable) {
		t.Errorf("Probe: %v, want ErrSourceUnreadable", err)
	}
	out, err := r.Transcode(t.Context(), remux(0))
	if err != nil {
		t.Fatal(err)
	}
	defer out.Close()
	if _, err := collect(out); !errors.Is(err, media.ErrEncoderFailed) {
		t.Errorf("Segments: %v, want ErrEncoderFailed", err)
	}
}

func TestFakeFailureAfterFirstSegment(t *testing.T) {
	r := useFake(t, "fail-after-segment", fixture(t, "remux-hevc.mp4"))
	out, err := r.Transcode(t.Context(), remux(0))
	if err != nil {
		t.Fatal(err)
	}
	defer out.Close()
	starts, err := collect(out)
	if len(starts) != 1 || !errors.Is(err, media.ErrSourceCorrupt) {
		t.Fatalf("got segments %v and %v, want one segment then ErrSourceCorrupt", starts, err)
	}
}

func TestFakeMissingBinary(t *testing.T) {
	r := New(filepath.Join(t.TempDir(), "ffmpeg"), filepath.Join(t.TempDir(), "ffprobe"))
	if _, err := r.Probe(t.Context(), "/m/a.mkv"); !errors.Is(err, media.ErrEncoderFailed) {
		t.Errorf("Probe: %v, want ErrEncoderFailed", err)
	}
	if _, err := r.Transcode(t.Context(), remux(0)); !errors.Is(err, media.ErrEncoderFailed) {
		t.Errorf("Transcode: %v, want ErrEncoderFailed", err)
	}
}

func TestFakeBackgroundPriority(t *testing.T) {
	useFake(t, "sleep", "")
	// nice is the 17th field after the parenthesized command name.
	nice := func(pid string) string {
		b, err := os.ReadFile("/proc/" + pid + "/stat")
		if err != nil {
			t.Fatal(err)
		}
		return strings.Fields(string(b[strings.LastIndexByte(string(b), ')')+2:]))[16]
	}
	for _, how := range []spawn{{}, {background: true}} {
		p, err := start(t.Context(), "test", os.Args[0], nil, how)
		if err != nil {
			t.Fatal(err)
		}
		got := nice(strconv.Itoa(p.cmd.Process.Pid))
		p.close()
		want := nice("self")
		if how.background {
			want = strconv.Itoa(backgroundNice)
		}
		if got != want {
			t.Errorf("background %v: nice %s, want %s", how.background, got, want)
		}
	}
}

func TestFakeCancelAndTimeout(t *testing.T) {
	for _, mode := range []string{"hang", "ignore-term"} {
		t.Run(mode+" cancelled", func(t *testing.T) {
			r := useFake(t, mode, fixture(t, "remux-h264.mp4"))
			ctx, cancel := context.WithCancel(t.Context())
			out, err := r.Transcode(ctx, remux(0))
			if err != nil {
				t.Fatal(err)
			}
			defer out.Close()
			if _, err := out.Init(); err != nil {
				t.Fatal(err)
			}
			time.AfterFunc(50*time.Millisecond, cancel)
			if _, err := collect(out); !errors.Is(err, media.ErrCancelled) {
				t.Fatalf("Segments: %v, want ErrCancelled", err)
			}
			if !gone(pid(out)) {
				t.Error("ffmpeg outlived its context")
			}
		})
		t.Run(mode+" timed out", func(t *testing.T) {
			r := useFake(t, mode, fixture(t, "remux-h264.mp4"))
			ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
			defer cancel()
			out, err := r.Transcode(ctx, remux(0))
			if err != nil {
				t.Fatal(err)
			}
			defer out.Close()
			if _, err := collect(out); !errors.Is(err, media.ErrTimeout) {
				t.Fatalf("Segments: %v, want ErrTimeout", err)
			}
		})
	}
}

func TestFakeCloseKillsTheGroup(t *testing.T) {
	for _, mode := range []string{"hang", "ignore-term", "grandchild"} {
		t.Run(mode, func(t *testing.T) {
			pidFile := filepath.Join(t.TempDir(), "pid")
			t.Setenv(fakePidFile, pidFile)
			r := useFake(t, mode, fixture(t, "remux-h264.mp4"))
			out, err := r.Transcode(t.Context(), remux(0))
			if err != nil {
				t.Fatal(err)
			}
			if _, err := out.Init(); err != nil {
				t.Fatal(err)
			}
			done := make(chan error, 1)
			go func() {
				_, err := collect(out)
				done <- err
			}()
			began := time.Now()
			if err := out.Close(); err != nil {
				t.Fatal(err)
			}
			// The grandchild holds stderr open. Only killing the group lets
			// Close return before exec's WaitDelay gives up on it.
			if d := time.Since(began); d > waitDelay/2 {
				t.Errorf("Close took %v", d)
			}
			if err := <-done; !errors.Is(err, media.ErrCancelled) {
				t.Errorf("Segments: %v, want ErrCancelled", err)
			}
			if !gone(pid(out)) {
				t.Error("ffmpeg outlived Close")
			}
			if mode == "grandchild" {
				b, err := os.ReadFile(pidFile)
				if err != nil {
					t.Fatal(err)
				}
				child, _ := strconv.Atoi(string(b))
				if !gone(child) {
					t.Error("ffmpeg's child outlived Close")
				}
			}
		})
	}
}

// TestFakePauseResume pauses and resumes a remux and a video transcode, on
// the CPU and on a device.
func TestFakePauseResume(t *testing.T) {
	video := media.TranscodeRequest{Path: "/m/a.mkv", SegmentDuration: 6 * time.Second, Source: sdr1080,
		Video: media.VideoRendition{Codec: media.HEVC, Width: 1280, Height: 720, Bitrate: 4_000_000}}
	accelerated := video
	accelerated.Accelerator = "nvenc:0"
	for name, req := range map[string]media.TranscodeRequest{"remux": remux(0), "video": video, "video on an accelerator": accelerated} {
		t.Run(name, func(t *testing.T) {
			r := useFake(t, "keys", fixture(t, "remux-hevc.mp4"))
			r.hw.Store(&accelerators{devices: map[media.Accelerator]*device{"nvenc:0": testDevice(nvenc, "0", media.GridGOP)}})
			pauseResume(t, r, req)
		})
	}
}

func pauseResume(t *testing.T, r *Runner, req media.TranscodeRequest) {
	out, err := r.Transcode(t.Context(), req)
	if err != nil {
		t.Fatal(err)
	}
	defer out.Close()
	if _, err := out.Init(); err != nil {
		t.Fatal(err)
	}
	if err := out.Pause(); err != nil {
		t.Fatal(err)
	}
	next, stop := iter.Pull2(out.Segments())
	defer stop()
	if err := out.Resume(); err != nil {
		t.Fatal(err)
	}
	if seg, err, ok := next(); !ok || err != nil || seg.Start != 0 {
		t.Fatalf("after resume: segment at %v, %v", seg.Start, err)
	}
	// Stderr is drained by its own goroutine, so the line can land just
	// after the segment does. The diagnostics leave it out.
	raw := out.(*output).p.stderr
	deadline := time.Now().Add(5 * time.Second)
	for !strings.Contains(raw.String(), "paused") && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if !strings.Contains(raw.String(), "paused") {
		t.Errorf("ffmpeg didn't see the pause: %q", raw.String())
	}
	if got := out.Diagnostics().Stderr; got != "" {
		t.Errorf("diagnostics show the pause: %q", got)
	}
	out.Close()
	if err := out.Pause(); !errors.Is(err, media.ErrCancelled) {
		t.Errorf("Pause after Close: %v, want ErrCancelled", err)
	}
}

func TestFakeProbe(t *testing.T) {
	r := useFake(t, "output", fixture(t, "h264.mkv.probe.json"))
	res, err := r.Probe(t.Context(), "/m/a.mkv")
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Tracks) != 2 || len(res.Chapters) != 2 {
		t.Errorf("got %d tracks and %d chapters, want 2 and 2", len(res.Tracks), len(res.Chapters))
	}
}

func TestFakeImage(t *testing.T) {
	r := useFake(t, "output", fixture(t, "h264.mkv.keyframes"))
	got, err := r.Image(t.Context(), "/m/poster.jpg", []media.ImageOut{{Width: 240, Path: "/c/p240.webp"}})
	want, _ := os.ReadFile(fixture(t, "h264.mkv.keyframes"))
	if err != nil || string(got) != string(want) {
		t.Errorf("Image returned %q, %v; want the tool's output", got, err)
	}
}
