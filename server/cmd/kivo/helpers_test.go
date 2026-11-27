package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// capsFor builds a client's capabilities: video and audio it plays from
// files in containers and, with stream, in a stream too; WebVTT shown in a
// stream, and the subtitle codecs it draws as files.
func capsFor(containers []string, stream bool, video []map[string]any, audio []string, externals ...string) map[string]any {
	delivery := []string{"file"}
	var subtitles []map[string]any
	if stream {
		delivery = append(delivery, "stream")
		subtitles = append(subtitles, map[string]any{"codec": "webvtt", "delivery": []string{"stream"}})
	}
	for _, c := range externals {
		subtitles = append(subtitles, map[string]any{"codec": c, "delivery": []string{"external"}})
	}
	var v, a []map[string]any
	for _, e := range video {
		entry := map[string]any{"delivery": delivery}
		for k, x := range e {
			entry[k] = x
		}
		v = append(v, entry)
	}
	for _, c := range audio {
		a = append(a, map[string]any{"codec": c, "delivery": delivery, "maxChannels": 2})
	}
	return map[string]any{"file": map[string]any{"containers": containers}, "video": v, "audio": a, "subtitles": subtitles}
}

// h264 is a video entry for H.264, of these profiles when given.
func h264(profiles ...string) []map[string]any {
	e := map[string]any{"codec": "h264"}
	if len(profiles) > 0 {
		e["profiles"] = profiles
	}
	return []map[string]any{e}
}

// played is the part of a play's or a download's answer the checks read.
type played struct {
	ID, State, Delivery, URL string
	StartMs                  int
	Tracks                   []playedTrack
	Previews                 struct {
		Ready bool
		URL   string
	}
}

type playedTrack struct {
	TrackID, Type, Delivery, Codec, Reason, URL string
	Selected                                    bool
	Transcode                                   *struct{}
	Fonts                                       []string
}

// selected is the selected track of a type.
func (p played) selected(kind string) playedTrack {
	for _, t := range p.Tracks {
		if t.Type == kind && t.Selected && t.TrackID != "off" {
			return t
		}
	}
	return playedTrack{}
}

// summary is the play's label: its video's.
func (p played) summary() string {
	v := p.selected("video")
	switch {
	case v.Transcode != nil:
		return "transcode"
	case v.Delivery == "file":
		return "directPlay"
	}
	return "directStream"
}

// externals are the subtitle tracks delivered external, as files.
func (p played) externals() []playedTrack {
	var out []playedTrack
	for _, t := range p.Tracks {
		if t.Delivery == "external" {
			out = append(out, t)
		}
	}
	return out
}

// makeMovie writes an H.264 and AAC test file with a keyframe every 2 s. It
// is dated an hour ago so the scan treats it as settled. An MKV also gets a
// SubRip track. A "Styled" MKV gets an ASS track instead, with a font
// attached.
func makeMovie(t *testing.T, ffmpeg, path string, d time.Duration) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	args := []string{"-nostdin", "-loglevel", "error", "-f", "lavfi", "-i", "testsrc2=size=320x180:rate=10",
		"-f", "lavfi", "-i", "sine=frequency=440:sample_rate=48000"}
	if strings.Contains(filepath.Base(path), "Styled") {
		ass, font := filepath.Join(t.TempDir(), "subtitles.ass"), filepath.Join(t.TempDir(), "styled.ttf")
		if err := os.WriteFile(ass, []byte(styledASS), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(font, styledFont, 0o644); err != nil {
			t.Fatal(err)
		}
		args = append(args, "-i", ass, "-map", "0", "-map", "1", "-map", "2", "-c:s", "ass", "-metadata:s:s:0", "title=Styled",
			"-attach", font, "-metadata:s:t:0", "mimetype=application/x-truetype-font")
	} else if filepath.Ext(path) == ".mkv" {
		srt := filepath.Join(t.TempDir(), "subtitles.srt")
		if err := os.WriteFile(srt, []byte("1\n00:00:01,000 --> 00:00:03,000\nHello\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		args = append(args, "-i", srt, "-map", "0", "-map", "1", "-map", "2", "-c:s", "srt")
	}
	cmd := exec.Command(ffmpeg, append(args, "-t", fmt.Sprint(d.Seconds()),
		"-c:v", "libx264", "-preset", "ultrafast", "-g", "20", "-keyint_min", "20", "-pix_fmt", "yuv420p",
		"-c:a", "aac", "-b:a", "64k", path)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("making %s: %v\n%s", path, err, out)
	}
	old := time.Now().Add(-time.Hour)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatal(err)
	}
}

func freeAddr(t *testing.T) string {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return l.Addr().String()
}

// client is a signed-in browser. It has a cookie jar, and sends a
// same-origin Origin header on unsafe requests.
type client struct {
	t    *testing.T
	base string
	http *http.Client
}

func newClient(t *testing.T, base string) *client {
	jar, _ := cookiejar.New(nil)
	return &client{t: t, base: base, http: &http.Client{Jar: jar, Timeout: 30 * time.Second}}
}

func (c *client) waitHealthy() {
	c.t.Helper()
	for range 100 {
		if resp, err := c.http.Get(c.base + "/healthz"); err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	c.t.Fatal("server never became healthy")
}

func (c *client) call(method, path string, body any, want int, out any) {
	c.t.Helper()
	var r io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		r = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, c.base+path, r)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", c.base)
	resp, err := c.http.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != want {
		c.t.Fatalf("%s %s: %d, want %d: %s", method, path, resp.StatusCode, want, b)
	}
	if out != nil {
		if err := json.Unmarshal(b, out); err != nil {
			c.t.Fatalf("%s %s: %v: %s", method, path, err, b)
		}
	}
}

// fetch GETs a media URL without the cookie, as a cast receiver would, and
// returns how many bytes came back.
func (c *client) fetch(path string) int {
	c.t.Helper()
	return len(c.get(path))
}

// get GETs a media URL without the cookie and returns what came back.
func (c *client) get(path string) string {
	c.t.Helper()
	resp, err := http.Get(c.base + path)
	if err != nil {
		c.t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		c.t.Fatalf("GET %s: %d", path, resp.StatusCode)
	}
	return string(b)
}
