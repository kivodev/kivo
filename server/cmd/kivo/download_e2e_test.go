package main

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mmcdole/kivo/internal/config"
)

// download downloads the short film at 480p with its SubRip track, for an
// iPhone, which plays MP4 but not MKV. The server converts it with real
// ffmpeg, in the background, into an MP4. The download gives the MP4's size
// and SHA-256, and the client fetches it by range, with the subtitle as
// WebVTT beside it. Once the device says it has the file, the server
// deletes the conversion. For a device that draws no subtitle files, the
// subtitle is inside the MP4.
func download(t *testing.T, c *client, boot config.Bootstrap, work, subtitle string) {
	t.Helper()
	caps := capsFor([]string{"mp4"}, true, h264("Baseline"), []string{"aac"}, "webvtt")
	type part struct {
		URL, Etag, Sha256 string
		SizeBytes         int64
	}
	var d struct {
		played
		Parts []part
	}
	c.call("POST", "/api/v1/downloads", map[string]any{"workId": work, "capabilities": caps, "quality": "480p", "subtitleTrackId": subtitle}, http.StatusCreated, &d)
	if d.Delivery != "conversion" {
		t.Fatalf("a download for an iPhone: %+v", d)
	}
	for deadline := time.Now().Add(60 * time.Second); d.State != "ready"; time.Sleep(200 * time.Millisecond) {
		if d.State == "failed" || time.Now().After(deadline) {
			t.Fatalf("download: %+v", d)
		}
		c.call("GET", "/api/v1/downloads/"+d.ID, nil, http.StatusOK, &d)
	}
	p := d.Parts[0]
	first := c.authed("/api/v1/"+p.URL, "bytes=0-99", "")
	rest := c.authed("/api/v1/"+p.URL, "bytes=100-", p.Etag)
	whole := first + rest
	sum := sha256.Sum256([]byte(whole))
	if len(first) != 100 || int64(len(whole)) != p.SizeBytes || hex.EncodeToString(sum[:]) != p.Sha256 {
		t.Fatalf("fetched %d bytes in two ranges, want %d with SHA-256 %s", len(whole), p.SizeBytes, p.Sha256)
	}
	file := filepath.Join(t.TempDir(), "download.mp4")
	if err := os.WriteFile(file, []byte(whole), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command(boot.FFprobe, "-v", "error", "-show_entries", "stream=codec_name:format=format_name", "-of", "csv=p=0", file).Output()
	if err != nil || !strings.Contains(string(out), "h264") || !strings.Contains(string(out), "aac") || !strings.Contains(string(out), "mp4") {
		t.Fatalf("the converted file: %s %v", out, err)
	}
	if s := d.externals(); len(s) == 0 || !strings.HasPrefix(c.authed("/api/v1/"+s[0].URL, "", ""), "WEBVTT") {
		t.Fatalf("its subtitle: %+v", d.Tracks)
	}

	c.call("DELETE", "/api/v1/downloads/"+d.ID, nil, http.StatusNoContent, nil)
	if entries, _ := os.ReadDir(filepath.Join(boot.CacheDir, "downloads")); len(entries) != 0 {
		t.Fatalf("a fetched download's conversion is still kept: %v", entries)
	}

	// A device that draws no subtitle files gets the subtitle inside the
	// file as its own text track, with the video and audio copied.
	caps = capsFor([]string{"mp4"}, true, h264(), []string{"aac"})
	c.call("POST", "/api/v1/downloads", map[string]any{"workId": work, "capabilities": caps, "quality": "original", "subtitleTrackId": subtitle}, http.StatusCreated, &d)
	for deadline := time.Now().Add(60 * time.Second); d.State != "ready"; time.Sleep(200 * time.Millisecond) {
		if d.State == "failed" || time.Now().After(deadline) {
			t.Fatalf("download: %+v", d)
		}
		c.call("GET", "/api/v1/downloads/"+d.ID, nil, http.StatusOK, &d)
	}
	if err := os.WriteFile(file, []byte(c.authed("/api/v1/"+d.Parts[0].URL, "", "")), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err = exec.Command(boot.FFprobe, "-v", "error", "-show_entries", "stream=codec_name", "-of", "csv=p=0", file).Output()
	if sub := d.selected("subtitle"); err != nil || strings.Fields(string(out))[2] != "mov_text" || len(d.externals()) != 0 || sub.Delivery != "file" || sub.Codec != "mov_text" {
		t.Fatalf("the subtitle in the file: %s %v %+v", out, err, d.Tracks)
	}
	c.call("DELETE", "/api/v1/downloads/"+d.ID, nil, http.StatusNoContent, nil)
}

// authed GETs a URL as the signed-in client, using Range and If-Range when
// given, and returns what came back.
func (c *client) authed(path, rng, ifRange string) string {
	c.t.Helper()
	req, _ := http.NewRequest("GET", c.base+path, nil)
	if rng != "" {
		req.Header.Set("Range", rng)
	}
	if ifRange != "" {
		req.Header.Set("If-Range", ifRange)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if want := map[bool]int{true: http.StatusPartialContent, false: http.StatusOK}[rng != ""]; resp.StatusCode != want {
		c.t.Fatalf("GET %s: %d, want %d", path, resp.StatusCode, want)
	}
	return string(b)
}
