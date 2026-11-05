package main

import (
	"bytes"
	"context"
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

	"github.com/mmcdole/kivo/internal/config"
	"github.com/mmcdole/kivo/internal/events"
	"github.com/mmcdole/kivo/internal/health"
)

// TestEndToEnd runs the real server with real ffmpeg. It covers setup, a
// library scanned and identified, browsing, direct play and a remux,
// progress and resume, subtitles extracted when a play chooses them,
// styled subtitles prepared in the background, previews made as files are
// added, and a clean shutdown. It needs ffmpeg and ffprobe on the PATH.
func TestEndToEnd(t *testing.T) {
	if testing.Short() {
		t.Skip("end to end")
	}
	ffmpeg, err1 := exec.LookPath("ffmpeg")
	ffprobe, err2 := exec.LookPath("ffprobe")
	if err1 != nil || err2 != nil {
		t.Skip("needs ffmpeg and ffprobe")
	}
	dir := t.TempDir()
	media := filepath.Join(dir, "media")
	makeMovie(t, ffmpeg, filepath.Join(media, "Long Film (2001)", "Long Film (2001).mp4"), 10*time.Minute)
	makeMovie(t, ffmpeg, filepath.Join(media, "Short Film (1999)", "Short Film (1999).mkv"), time.Minute)
	makeMovie(t, ffmpeg, filepath.Join(media, "Styled Film (2004)", "Styled Film (2004).mkv"), time.Minute)

	boot := config.DefaultBootstrap()
	boot.Listen = []string{freeAddr(t)}
	boot.DataDir, boot.CacheDir, boot.BackupDir = filepath.Join(dir, "data"), filepath.Join(dir, "cache"), filepath.Join(dir, "backups")
	boot.FFmpeg, boot.FFprobe = ffmpeg, ffprobe
	ctx, stop := context.WithCancel(context.Background())
	done := make(chan error, 1)
	bus := events.NewBus()
	go func() { done <- run(ctx, boot, health.NewLogs(100, bus), bus) }()
	defer func() {
		stop()
		select {
		case err := <-done:
			if err != nil {
				t.Errorf("shutdown: %v", err)
			}
		case <-time.After(30 * time.Second):
			t.Error("shutdown took over 30 s")
		}
		if entries, _ := os.ReadDir(filepath.Join(boot.CacheDir, "transcode")); len(entries) != 0 {
			t.Errorf("transcode folders left after shutdown: %v", entries)
		}
	}()

	c := newClient(t, "http://"+boot.Listen[0])
	c.waitHealthy()
	c.call("POST", "/api/v1/setup", map[string]any{"username": "admin", "password": "correct horse battery"}, http.StatusCreated, nil)
	var lib struct{ ID string }
	c.call("POST", "/api/v1/libraries", map[string]any{"name": "Movies", "kind": "movies", "folders": []string{media}, "previews": "nightlyAndAdded"}, http.StatusCreated, &lib)

	// The scan, the probe and identify's first pass put both films in the
	// library.
	works := map[string]string{}
	deadline := time.Now().Add(60 * time.Second)
	for len(works) < 3 && time.Now().Before(deadline) {
		var page struct {
			Items []struct{ ID, Title string }
		}
		c.call("GET", "/api/v1/libraries/"+lib.ID+"/works", nil, http.StatusOK, &page)
		for _, w := range page.Items {
			works[w.Title] = w.ID
		}
		time.Sleep(200 * time.Millisecond)
	}
	if len(works) != 3 {
		t.Fatalf("after scanning: %v", works)
	}

	caps := capsFor([]string{"mp4"}, true, h264(), []string{"aac"}, "webvtt")
	type play = played

	// Direct play, with progress saved on stop and resumed by the next play.
	var long play
	c.call("POST", "/api/v1/plays", map[string]any{"workId": works["Long Film"], "capabilities": caps}, http.StatusCreated, &long)
	if long.Delivery != "file" || long.summary() != "directPlay" {
		t.Fatalf("mp4: %+v", long)
	}
	if n := c.fetch("/api/v1/" + long.URL); n == 0 {
		t.Fatal("direct play file is empty")
	}
	c.call("PATCH", "/api/v1/plays/"+long.ID, map[string]any{"positionMs": 200_000, "state": "playing"}, http.StatusNoContent, nil)
	c.call("POST", "/api/v1/plays/"+long.ID+"/stop", map[string]any{"positionMs": 240_000}, http.StatusNoContent, nil)
	var again play
	c.call("POST", "/api/v1/plays", map[string]any{"workId": works["Long Film"], "capabilities": caps}, http.StatusCreated, &again)
	if again.StartMs != 240_000 {
		t.Fatalf("resumed at %d ms, want 240000", again.StartMs)
	}
	var home struct {
		ContinueWatching []struct{ Title string }
	}
	c.call("GET", "/api/v1/home", nil, http.StatusOK, &home)
	// With no provider key nothing matches. Unmatched items stay out of
	// Continue Watching, though their position is kept.
	if len(home.ContinueWatching) != 0 {
		t.Fatalf("continue watching: %+v", home.ContinueWatching)
	}

	// The mkv remuxes to HLS, with a planned playlist and then real
	// segments.
	var short play
	c.call("POST", "/api/v1/plays", map[string]any{"workId": works["Short Film"], "capabilities": caps}, http.StatusCreated, &short)
	if short.Delivery != "stream" || short.summary() != "directStream" {
		t.Fatalf("mkv: %+v", short)
	}
	base := "/api/v1/" + filepath.Dir(short.URL) + "/"
	for _, name := range []string{"index.m3u8", "main.m3u8", "init.mp4", "0.m4s", "9.m4s"} {
		if n := c.fetch(base + name); n == 0 {
			t.Fatalf("%s is empty", name)
		}
	}
	c.call("POST", "/api/v1/plays/"+short.ID+"/stop", map[string]any{"positionMs": 0}, http.StatusNoContent, nil)

	// When chosen, its embedded SubRip track is in the stream. It is a
	// subtitle rendition whose segments arrive with the video's, starting
	// from the first.
	var streamed play
	var detail struct {
		Versions []struct {
			Tracks []struct{ ID, Type string }
		}
	}
	c.call("GET", "/api/v1/works/"+works["Short Film"], nil, http.StatusOK, &detail)
	subtitle := ""
	for _, tr := range detail.Versions[0].Tracks {
		if tr.Type == "subtitle" {
			subtitle = tr.ID
		}
	}
	c.call("POST", "/api/v1/plays", map[string]any{"workId": works["Short Film"], "capabilities": caps, "subtitleTrackId": subtitle}, http.StatusCreated, &streamed)
	if len(streamed.Tracks) != 4 || streamed.selected("subtitle").Delivery != "stream" {
		t.Fatalf("the subtitle track: %+v", streamed.Tracks)
	}
	base = "/api/v1/" + filepath.Dir(streamed.URL) + "/"
	if b := c.get(base + "index.m3u8"); !strings.Contains(b, `TYPE=SUBTITLES`) || !strings.Contains(b, `URI="text-0.m3u8"`) ||
		!strings.Contains(b, `STABLE-RENDITION-ID="`+subtitle+`"`) {
		t.Fatalf("multivariant playlist:\n%s", b)
	}
	if b := c.get(base + "text-0.m3u8"); !strings.Contains(b, "\nt0-0.vtt\n") {
		t.Fatalf("subtitle playlist:\n%s", b)
	}
	if n := c.fetch(base + "0.m4s"); n == 0 {
		t.Fatal("0.m4s is empty")
	}
	before := time.Now()
	vtt := c.get(base + "t0-0.vtt")
	if !strings.HasPrefix(vtt, "WEBVTT\nX-TIMESTAMP-MAP=MPEGTS:900000,LOCAL:00:00:00.000\n") || !strings.Contains(vtt, "00:00:01.000 --> 00:00:03.000\nHello") {
		t.Fatalf("t0-0.vtt:\n%s", vtt)
	}
	if waited := time.Since(before); waited > time.Second {
		t.Fatalf("t0-0.vtt took %v after its video", waited)
	}
	c.call("POST", "/api/v1/plays/"+streamed.ID+"/stop", map[string]any{"positionMs": 0}, http.StatusNoContent, nil)

	// With subtitles off, the track is a rendition too, so switching to it
	// needs no new play.
	var renditions play
	c.call("POST", "/api/v1/plays", map[string]any{"workId": works["Short Film"], "capabilities": caps, "subtitleTrackId": "off"}, http.StatusCreated, &renditions)
	if len(renditions.Tracks) != 4 || renditions.Tracks[2].TrackID != subtitle || renditions.Tracks[2].Delivery != "stream" {
		t.Fatalf("the tracks with subtitles off: %+v", renditions.Tracks)
	}
	base = "/api/v1/" + filepath.Dir(renditions.URL) + "/"
	if b := c.get(base + "index.m3u8"); !strings.Contains(b, `DEFAULT=NO`) || !strings.Contains(b, `STABLE-RENDITION-ID="`+subtitle+`"`) {
		t.Fatalf("multivariant playlist with subtitles off:\n%s", b)
	}
	if n := c.fetch(base + "0.m4s"); n == 0 {
		t.Fatal("0.m4s is empty")
	}
	if vtt := c.get(base + "t0-0.vtt"); !strings.Contains(vtt, "Hello") {
		t.Fatalf("t0-0.vtt with subtitles off:\n%s", vtt)
	}
	c.call("POST", "/api/v1/plays/"+renditions.ID+"/stop", map[string]any{"positionMs": 0}, http.StatusNoContent, nil)

	// A client that plays the mkv itself also gets the track in the stream,
	// because it isn't extracted yet. The file's previews are made, because
	// the library makes them as files are added.
	direct := capsFor([]string{"mp4", "mkv"}, true, h264(), []string{"aac"}, "webvtt")
	var analyzed play
	deadline = time.Now().Add(60 * time.Second)
	for {
		c.call("POST", "/api/v1/plays", map[string]any{"workId": works["Short Film"], "capabilities": direct, "subtitleTrackId": subtitle}, http.StatusCreated, &analyzed)
		c.call("POST", "/api/v1/plays/"+analyzed.ID+"/stop", map[string]any{"positionMs": 0}, http.StatusNoContent, nil)
		if analyzed.Previews.Ready {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("no previews: %+v", analyzed)
		}
		time.Sleep(200 * time.Millisecond)
	}
	if analyzed.selected("subtitle").Delivery != "stream" || len(analyzed.externals()) != 0 {
		t.Fatalf("the track not extracted: %+v", analyzed)
	}
	c.call("POST", "/api/v1/plays", map[string]any{"workId": works["Short Film"], "capabilities": direct, "subtitleTrackId": subtitle}, http.StatusCreated, &analyzed)
	var manifest struct{ Sheets []string }
	c.call("GET", "/api/v1/"+analyzed.Previews.URL, nil, http.StatusOK, &manifest)
	if len(manifest.Sheets) != 1 || c.fetch("/api/v1/"+filepath.Dir(analyzed.Previews.URL)+"/"+manifest.Sheets[0]) == 0 {
		t.Fatalf("previews %+v", manifest)
	}
	c.call("POST", "/api/v1/plays/"+analyzed.ID+"/stop", map[string]any{"positionMs": 0}, http.StatusNoContent, nil)

	styledSubtitles(t, c, boot, works["Styled Film"])
	download(t, c, boot, works["Short Film"], subtitle)

	// The download extracted the track as WebVTT, so a client that plays
	// the mkv itself now draws it from a file.
	c.call("POST", "/api/v1/plays", map[string]any{"workId": works["Short Film"], "capabilities": direct, "subtitleTrackId": subtitle}, http.StatusCreated, &analyzed)
	if s := analyzed.externals(); len(s) != 1 || !s[0].Selected || analyzed.Delivery != "file" {
		t.Fatalf("the track once extracted: %+v", analyzed)
	}
	if b := c.get("/api/v1/" + analyzed.externals()[0].URL); !strings.HasPrefix(b, "WEBVTT") || !strings.Contains(b, "Hello") {
		t.Fatalf("subtitle %q", b)
	}
	c.call("POST", "/api/v1/plays/"+analyzed.ID+"/stop", map[string]any{"positionMs": 0}, http.StatusNoContent, nil)

	// A client that can't play this H.264 as it is still gets it, encoded
	// on the segment grid, both from the start and after a far seek.
	transcode := capsFor([]string{}, true, h264("Baseline"), []string{"aac"}, "webvtt")
	var enc play
	c.call("POST", "/api/v1/plays", map[string]any{"workId": works["Long Film"], "capabilities": transcode, "positionMs": 0}, http.StatusCreated, &enc)
	if enc.Delivery != "stream" || enc.summary() != "transcode" {
		t.Fatalf("a video transcode: %+v", enc)
	}
	base = "/api/v1/" + filepath.Dir(enc.URL) + "/"
	for _, name := range []string{"index.m3u8", "main.m3u8", "init.mp4", "0.m4s", "1.m4s", "80.m4s", "81.m4s"} {

		if n := c.fetch(base + name); n == 0 {
			t.Fatalf("transcode %s is empty", name)
		}
	}
	var plays struct {
		Items []struct {
			Transcode struct{ State, Command string }
		}
	}
	c.call("GET", "/api/v1/plays", nil, http.StatusOK, &plays)
	running := 0
	for _, s := range plays.Items {
		if strings.Contains(s.Transcode.Command, "libx264") {
			running++
		}
	}
	if running != 1 {
		t.Fatalf("Now playing page: %+v", plays.Items)
	}
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
