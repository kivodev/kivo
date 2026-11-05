package main

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/mmcdole/kivo/internal/config"
)

// styledASS is the styled test film's subtitle track. It uses a font that
// the file carries.
const styledASS = `[Script Info]
ScriptType: v4.00+
PlayResX: 320
PlayResY: 180

[V4+ Styles]
Format: Name, Fontname, Fontsize, PrimaryColour, SecondaryColour, OutlineColour, BackColour, Bold, Italic, Underline, StrikeOut, ScaleX, ScaleY, Spacing, Angle, BorderStyle, Outline, Shadow, Alignment, MarginL, MarginR, MarginV, Encoding
Style: Default,Kivo Test,20,&H00FFFFFF,&H000000FF,&H00000000,&H00000000,0,0,0,0,100,100,0,0,1,2,0,2,10,10,10,1

[Events]
Format: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text
Dialogue: 0,0:00:00.00,0:01:00.00,Default,,0,0,0,,{\i1}Styled{\i0}
`

// styledFont is the font the styled test film carries. It is TrueType's
// magic number plus enough bytes to hash, which is all that preparing,
// serving and burning in look at. libass skips a font it can't read and
// draws with another.
var styledFont = append([]byte("\x00\x01\x00\x00\x00\x0aOS/2"), bytes.Repeat([]byte("kivo-test-font "), 64)...)

type styledPlay = played

// subtitle is how a play shows its subtitle.
func (p played) subtitle() (delivery, reason string) {
	s := p.selected("subtitle")
	return s.Delivery, s.Reason
}

// styledSubtitles checks styled subtitles end to end on the styled film:
//   - They are prepared in the background as the file is added.
//   - Until then, a remux carries them plain in its stream. The same
//     happens after the font store is deleted, until the next preparation
//     heals it.
//   - The client draws them, fetching the font by hash.
//   - For a client that doesn't draw ASS, they are burned in from the
//     prepared file.
func styledSubtitles(t *testing.T, c *client, boot config.Bootstrap, work string) {
	t.Helper()
	var detail struct {
		Versions []struct {
			Tracks []struct{ ID, Type, Title string }
		}
	}
	c.call("GET", "/api/v1/works/"+work, nil, http.StatusOK, &detail)
	track := ""
	for _, tr := range detail.Versions[0].Tracks {
		if tr.Type == "subtitle" && tr.Title == "Styled" {
			track = tr.ID
		}
	}
	if track == "" {
		t.Fatalf("no styled track: %+v", detail)
	}

	// Probing asked for the file's preparation, which finishes before any
	// play asks for it.
	for deadline := time.Now().Add(60 * time.Second); ; time.Sleep(100 * time.Millisecond) {
		var tasks struct {
			Items []struct {
				ID      string
				Running bool
			}
		}
		c.call("GET", "/api/v1/tasks", nil, http.StatusOK, &tasks)
		i := slices.IndexFunc(tasks.Items, func(t struct {
			ID      string
			Running bool
		}) bool {
			return t.ID == "extract-subtitles"
		})
		if i < 0 {
			t.Fatalf("no extract-subtitles task: %+v", tasks.Items)
		}
		if !tasks.Items[i].Running {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("subtitles are still being extracted")
		}
	}

	draws := capsFor([]string{"mp4", "mkv"}, true, h264(), []string{"aac"}, "webvtt", "ass")
	start := func(caps map[string]any) styledPlay {
		t.Helper()
		var s styledPlay
		c.call("POST", "/api/v1/plays", map[string]any{"workId": work, "capabilities": caps, "subtitleTrackId": track, "positionMs": 0}, http.StatusCreated, &s)
		return s
	}
	stop := func(s styledPlay) {
		t.Helper()
		c.call("POST", "/api/v1/plays/"+s.ID+"/stop", map[string]any{"positionMs": 0}, http.StatusNoContent, nil)
	}
	// prepared waits for a play that draws the track styled. Until then,
	// each play is a remux with the track plain in the stream, and says
	// why.
	prepared := func(what string) styledPlay {
		t.Helper()
		deadline := time.Now().Add(60 * time.Second)
		for {
			s := start(draws)
			method, reason := s.subtitle()
			if method == "external" {
				return s
			}
			if method != "stream" || reason != "subtitlePreparing" || s.summary() != "directStream" {
				t.Fatalf("%s: not prepared, but %+v", what, s.Tracks)
			}
			stop(s)
			if time.Now().After(deadline) {
				t.Fatalf("%s: never prepared", what)
			}
			time.Sleep(200 * time.Millisecond)
		}
	}
	notPrepared := func(what string) {
		t.Helper()
		s := start(draws)
		if method, reason := s.subtitle(); method != "stream" || reason != "subtitlePreparing" || s.summary() != "directStream" {
			t.Fatalf("%s: %+v", what, s.Tracks)
		}
		stop(s)
	}

	sum := sha256.Sum256(styledFont)
	hash := hex.EncodeToString(sum[:])
	s := start(draws)
	if method, _ := s.subtitle(); method != "external" {
		t.Fatalf("not prepared as added: %+v", s.Tracks)
	}
	sub := s.selected("subtitle")
	if s.summary() != "directPlay" || sub.TrackID != track || sub.Codec != "ass" || sub.URL == "" || !slices.Equal(sub.Fonts, []string{"fonts/" + hash}) {
		t.Fatalf("prepared: %+v", s)
	}
	stop(s)

	// A signed-in viewer can fetch the font by its hash. It is cacheable
	// forever, and gzipped when asked.
	font := func(header ...string) *http.Response {
		t.Helper()
		req, _ := http.NewRequest("GET", c.base+"/api/v1/fonts/"+hash, nil)
		for i := 0; i+1 < len(header); i += 2 {
			req.Header.Set(header[i], header[i+1])
		}
		resp, err := c.http.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		return resp
	}
	resp := font("Accept-Encoding", "gzip")
	zr, err := gzip.NewReader(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(zr)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK || !bytes.Equal(b, styledFont) || resp.Header.Get("Content-Type") != "font/ttf" ||
		resp.Header.Get("Cache-Control") != "private, max-age=31536000, immutable" || resp.Header.Get("ETag") != `W/"`+hash+`"` {
		t.Fatalf("font: %d %v", resp.StatusCode, resp.Header)
	}
	resp = font("If-None-Match", `W/"`+hash+`"`)
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotModified {
		t.Fatalf("font revalidated: %d", resp.StatusCode)
	}
	if resp, err := http.Get(c.base + "/api/v1/fonts/" + hash); err != nil || resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("font without signing in: %v %v", resp, err)
	}

	// With the font store deleted, the track is plain again. A download
	// that keeps the track as a file prepares it again.
	if err := os.RemoveAll(filepath.Join(boot.CacheDir, "fonts")); err != nil {
		t.Fatal(err)
	}
	notPrepared("fonts deleted")
	var d struct{ ID, State, Delivery string }
	c.call("POST", "/api/v1/downloads", map[string]any{"workId": work, "capabilities": draws, "quality": "original", "subtitleTrackId": track}, http.StatusCreated, &d)
	for deadline := time.Now().Add(60 * time.Second); d.State != "ready"; time.Sleep(100 * time.Millisecond) {
		if d.State == "failed" || time.Now().After(deadline) {
			t.Fatalf("download: %+v", d)
		}
		c.call("GET", "/api/v1/downloads/"+d.ID, nil, http.StatusOK, &d)
	}
	if d.Delivery != "file" {
		t.Fatalf("a download of the original: %+v", d)
	}
	c.call("DELETE", "/api/v1/downloads/"+d.ID, nil, http.StatusNoContent, nil)
	stop(prepared("fonts deleted"))
	if _, err := os.Stat(filepath.Join(boot.CacheDir, "fonts", hash[:2], hash+".ttf")); err != nil {
		t.Fatalf("the font isn't stored again: %v", err)
	}

	// For a client that doesn't draw ASS, the track is burned in from the
	// prepared file and its font.
	phone := capsFor([]string{}, true, h264("Baseline"), []string{"aac"}, "webvtt")
	s = start(phone)
	if method, _ := s.subtitle(); method != "burn" || s.summary() != "transcode" {
		t.Fatalf("burned in: %+v", s.Tracks)
	}
	if n := c.fetch("/api/v1/" + filepath.Dir(s.URL) + "/0.m4s"); n == 0 {
		t.Fatal("the burned-in segment is empty")
	}
	stop(s)
}
