package api

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"regexp"
	"strconv"
	"testing"
	"time"

	"github.com/mmcdole/kivo/internal/id"
)

func TestDownloads(t *testing.T) {
	e := newEnv(t, serving{})
	e.run()
	create := func(caps map[string]any, quality string) map[string]any {
		t.Helper()
		w := e.do("POST", "/api/v1/downloads", e.adminToken, map[string]any{"workId": e.heat.Work, "capabilities": caps, "quality": quality})
		if w.Code != http.StatusCreated {
			t.Fatalf("create: %d %s", w.Code, w.Body.String())
		}
		return decode[map[string]any](t, w)
	}
	part := func(d map[string]any) map[string]any { return d["parts"].([]any)[0].(map[string]any) }

	// A device that plays the MP4 gets the original. It is ready at once,
	// and its part's size and validator are known.
	d := create(clientCaps, "original")
	p := part(d)
	if d["state"] != "ready" || d["delivery"] != "file" || d["quality"] != "original" || p["sizeBytes"] != float64(len(heatBytes)) || p["etag"] == nil || p["contentType"] != "video/mp4" {
		t.Fatalf("the original: %v", d)
	}
	file, etag := "/api/v1/"+p["url"].(string), p["etag"].(string)
	w := e.do("GET", file, e.adminToken, nil, "Range", "bytes=10-", "If-Range", etag)
	if w.Code != http.StatusPartialContent || w.Body.String() != heatBytes[10:] || w.Header().Get("ETag") != etag {
		t.Fatalf("resumed: %d %v %q", w.Code, w.Header(), w.Body.String())
	}
	if w := e.do("GET", file, e.adminToken, nil, "Range", "bytes=10-", "If-Range", `"changed"`); w.Code != http.StatusOK || w.Body.String() != heatBytes {
		t.Fatalf("resumed on a stale validator: %d %q", w.Code, w.Body.String())
	}
	if w := e.do("GET", file, "", nil); w.Code != http.StatusUnauthorized {
		t.Fatalf("without a token: %d", w.Code)
	}
	if w := e.do("GET", file, e.nobodyToken, nil); w.Code != http.StatusNotFound {
		t.Fatalf("someone else's download: %d", w.Code)
	}
	if w := e.do("GET", "/api/v1/downloads/"+d["id"].(string), e.nobodyToken, nil); w.Code != http.StatusNotFound {
		t.Fatalf("someone else's download: %d", w.Code)
	}

	// A device that plays only E-AC-3 gets a conversion. It is prepared in
	// the background, then kept with its size and hash.
	d = create(map[string]any{
		"file": map[string]any{"containers": []string{"mp4"}}, "subtitles": []map[string]any{{"codec": "webvtt", "delivery": []string{"external"}}},
		"video": []map[string]any{{"codec": "h264", "delivery": []string{"file"}}},
		"audio": []map[string]any{{"codec": "eac3", "delivery": []string{"file"}, "maxChannels": 6}},
	}, "1080p")
	if d["delivery"] != "conversion" || d["state"] == "ready" || d["sizeBytes"].(float64) <= 0 {
		t.Fatalf("a conversion: %v", d)
	}
	status := "/api/v1/downloads/" + d["id"].(string)
	for deadline := time.Now().Add(5 * time.Second); d["state"] != "ready"; time.Sleep(time.Millisecond) {
		w := e.do("GET", status, e.adminToken, nil)
		if w.Code != http.StatusOK || time.Now().After(deadline) || d["state"] == "failed" {
			t.Fatalf("download: %d %s", w.Code, w.Body.String())
		}
		d = decode[map[string]any](t, w)
	}
	p = part(d)
	file, etag = "/api/v1/"+p["url"].(string), p["etag"].(string)
	w = e.do("GET", file, e.adminToken, nil)
	sum := sha256.Sum256(w.Body.Bytes())
	if w.Code != http.StatusOK || w.Header().Get("Content-Type") != "video/mp4" || hex.EncodeToString(sum[:]) != p["sha256"] ||
		strconv.Itoa(w.Body.Len()) != w.Header().Get("Content-Length") || float64(w.Body.Len()) != p["sizeBytes"] || d["expiresAt"] == nil {
		t.Fatalf("the converted file: %d %v %v", w.Code, w.Header(), d)
	}
	whole := w.Body.String()
	if w := e.do("GET", file, e.adminToken, nil, "Range", "bytes=5-", "If-Range", etag); w.Code != http.StatusPartialContent || w.Body.String() != whole[5:] {
		t.Fatalf("resumed: %d %q", w.Code, w.Body.String())
	}
	// The device's list has both downloads, newest first. Another device
	// of the same user has none.
	list := decode[map[string]any](t, e.do("GET", "/api/v1/downloads", e.adminToken, nil))["items"].([]any)
	if len(list) != 2 || list[0].(map[string]any)["id"] != d["id"] {
		t.Fatalf("the device's downloads: %v", list)
	}
	me, err := id.Parse(decode[map[string]any](t, e.do("GET", "/api/v1/users/me", e.adminToken, nil))["id"].(string))
	if err != nil {
		t.Fatal(err)
	}
	other := decode[map[string]any](t, e.do("GET", "/api/v1/downloads", e.session(me), nil))["items"].([]any)
	if len(other) != 0 {
		t.Fatalf("another device's downloads: %v", other)
	}
	if w := e.do("POST", status+"/retry", e.adminToken, nil); w.Code != http.StatusOK || decode[map[string]any](t, w)["state"] != "ready" {
		t.Fatalf("retrying one that didn't fail: %d", w.Code)
	}

	// Once the device has it, the server deletes it.
	if w := e.do("DELETE", status, e.adminToken, nil); w.Code != http.StatusNoContent {
		t.Fatalf("delete: %d", w.Code)
	}
	if w := e.do("GET", status, e.adminToken, nil); w.Code != http.StatusNotFound {
		t.Fatalf("forgotten: %d", w.Code)
	}
	if w := e.do("GET", file, e.adminToken, nil); w.Code != http.StatusNotFound {
		t.Fatalf("a forgotten download's file: %d", w.Code)
	}

	if w := e.do("POST", "/api/v1/downloads", e.adminToken, map[string]any{"workId": e.heat.Work, "capabilities": clientCaps, "quality": "4k"}); w.Code != http.StatusBadRequest {
		t.Fatalf("an unknown quality: %d", w.Code)
	}
}

func TestDownloadEstimates(t *testing.T) {
	e := newEnv(t, serving{})
	w := e.do("POST", "/api/v1/downloads/estimate", e.adminToken, map[string]any{
		"items": []map[string]any{{"workId": e.heat.Work}, {"workId": id.New()}}, "capabilities": clientCaps,
	})
	if w.Code != http.StatusOK {
		t.Fatalf("estimate: %d %s", w.Code, w.Body.String())
	}
	items := decode[map[string]any](t, w)["items"].([]any)
	heat, gone := items[0].(map[string]any), items[1].(map[string]any)
	options := heat["options"].([]any)
	first, small := options[0].(map[string]any), options[2].(map[string]any)
	// Two hours at 8 Mbps is larger than Standard's most at 3 Mbps. The
	// original's size is the one scanning recorded.
	if !heat["playable"].(bool) || len(options) != 3 ||
		first["quality"] != "original" || first["asIs"] != true || first["sizeBytes"] != float64(1<<30) ||
		small["quality"] != "standard" || small["asIs"] != false || small["delivery"] != "conversion" {
		t.Fatalf("a movie: %v", heat)
	}
	if gone["playable"] != false || len(gone["options"].([]any)) != 0 {
		t.Fatalf("a work that isn't there: %v", gone)
	}
}

func TestDownloadCheck(t *testing.T) {
	e := newEnv(t, serving{})
	body := map[string]any{"items": []map[string]any{
		{"workId": e.heat.Work, "versionId": e.heat.Version},
		{"workId": e.heat.Work, "versionId": id.New()},
	}}
	check := func(token string) []bool {
		t.Helper()
		w := e.do("POST", "/api/v1/downloads/check", token, body)
		if w.Code != http.StatusOK {
			t.Fatalf("check: %d %s", w.Code, w.Body.String())
		}
		var out []bool
		for _, it := range decode[map[string]any](t, w)["items"].([]any) {
			out = append(out, it.(map[string]any)["available"].(bool))
		}
		return out
	}
	if got := check(e.adminToken); len(got) != 2 || !got[0] || got[1] {
		t.Fatalf("the admin: %v", got)
	}
	if got := check(e.nobodyToken); got[0] || got[1] {
		t.Fatalf("a user that sees nothing: %v", got)
	}
	many := make([]map[string]any, 1001)
	for i := range many {
		many[i] = map[string]any{"workId": e.heat.Work, "versionId": e.heat.Version}
	}
	if w := e.do("POST", "/api/v1/downloads/check", e.adminToken, map[string]any{"items": many}); w.Code != http.StatusBadRequest {
		t.Fatalf("1001 downloads: %d", w.Code)
	}
}

func TestOfflineProgress(t *testing.T) {
	e := newEnv(t, serving{})
	state := "/api/v1/works/" + e.heat.Work.String() + "/user-state"
	at := time.Now().UTC().Add(-time.Minute)
	w := e.do("PATCH", state, e.adminToken, map[string]any{"positionMs": 1_800_000, "playedAt": at})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("without a version: %d", w.Code)
	}
	w = e.do("PATCH", state, e.adminToken, map[string]any{"positionMs": 1_800_000, "playedAt": at, "versionId": e.heat.Version})
	if w.Code != http.StatusOK || decode[map[string]any](t, w)["positionMs"] != 1_800_000.0 {
		t.Fatalf("an offline position: %d %s", w.Code, w.Body.String())
	}
	w = e.do("PATCH", state, e.adminToken, map[string]any{"positionMs": 3_600_000, "playedAt": at.Add(-time.Hour), "versionId": e.heat.Version})
	if w.Code != http.StatusOK || decode[map[string]any](t, w)["positionMs"] != 1_800_000.0 {
		t.Fatalf("an older one: %d %s", w.Code, w.Body.String())
	}
	w = e.do("PATCH", state, e.adminToken, map[string]any{"positionMs": 60_000, "playedAt": at, "versionId": e.episode.Version})
	if w.Code != http.StatusNotFound {
		t.Fatalf("another work's version: %d", w.Code)
	}
	page := decode[map[string]any](t, e.do("GET", "/api/v1/users/me/play-events", e.adminToken, nil))
	if items := page["items"].([]any); len(items) != 1 || items[0].(map[string]any)["positionMs"] != 1_800_000.0 {
		t.Fatalf("history %v", page)
	}
}

// A conversion's tracks read the same while it waits and once it's ready:
// each inside the file at its stream index there, with why it isn't the
// original's and what it's encoded to.
func TestADownloadsTracksOnTheWire(t *testing.T) {
	e := newEnv(t, serving{})
	e.run()
	w := e.do("POST", "/api/v1/downloads", e.adminToken, map[string]any{"workId": e.heat.Work, "quality": "high", "capabilities": map[string]any{
		"file": map[string]any{"containers": []string{"mp4"}}, "subtitles": []map[string]any{{"codec": "webvtt", "delivery": []string{"external"}}},
		"video": []map[string]any{{"codec": "h264", "delivery": []string{"file"}}},
		"audio": []map[string]any{{"codec": "eac3", "delivery": []string{"file"}, "maxChannels": 6}},
	}})
	if w.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", w.Code, w.Body.String())
	}
	const want = `[{"codec":"h264","delivery":"file","index":0,"mismatch":{"actual":"aac","declared":"eac3","field":"codec"},"reason":"audioCodecUnsupported","selected":true,"trackId":"ID","type":"video"},` +
		`{"codec":"eac3","delivery":"file","index":1,"mismatch":{"actual":"aac","declared":"eac3","field":"codec"},"reason":"audioCodecUnsupported","selected":true,"trackId":"ID","transcode":{"bitrateBps":384000,"channels":2},"type":"audio"},` +
		`{"delivery":"none","selected":true,"trackId":"off","type":"subtitle"}]`
	ids := regexp.MustCompile(`[0-9a-f]{8}(-[0-9a-f]{4}){3}-[0-9a-f]{12}`)
	d := decode[map[string]any](t, w)
	for deadline := time.Now().Add(5 * time.Second); ; time.Sleep(time.Millisecond) {
		tracks, err := json.Marshal(d["tracks"])
		if got := ids.ReplaceAllString(string(tracks), "ID"); err != nil || got != want {
			t.Fatalf("%s, its tracks:\n got %s\nwant %s", d["state"], got, want)
		}
		if d["state"] == "ready" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("download: %v", d)
		}
		d = decode[map[string]any](t, e.do("GET", "/api/v1/downloads/"+d["id"].(string), e.adminToken, nil))
	}
}
