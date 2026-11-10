package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/mmcdole/kivo/internal/store"

	"github.com/mmcdole/kivo/internal/api/internal/gen"
	"github.com/mmcdole/kivo/internal/auth"
	"github.com/mmcdole/kivo/internal/catalog"
	"github.com/mmcdole/kivo/internal/id"
	"github.com/mmcdole/kivo/internal/media"
	"github.com/mmcdole/kivo/internal/playback"
	"github.com/mmcdole/kivo/internal/query"
	"github.com/mmcdole/kivo/internal/users"
)

func (e *env) startPlayback() map[string]any {
	e.t.Helper()
	w := e.do("POST", "/api/v1/plays", e.adminToken, map[string]any{"workId": e.heat.Work, "capabilities": clientCaps})
	if w.Code != http.StatusCreated {
		e.t.Fatalf("start: %d %s", w.Code, w.Body.String())
	}
	return decode[map[string]any](e.t, w)
}

// TestMediaCrossOrigin fetches a direct-play file the way Google Cast
// would, from another origin without cookies or a token.
func TestMediaCrossOrigin(t *testing.T) {
	e := newEnv(t, serving{})
	ps := e.startPlayback()
	if ps["delivery"] != "file" {
		t.Fatalf("play %v", ps)
	}
	url := "/api/v1/" + ps["url"].(string)

	w := e.do("GET", url, "", nil, "Origin", "https://cast.example")
	if w.Code != http.StatusOK || w.Body.String() != heatBytes {
		t.Fatalf("cross-origin GET: %d %q", w.Code, w.Body.String())
	}
	if w.Header().Get("Access-Control-Allow-Origin") != "*" || w.Header().Get("Access-Control-Allow-Credentials") != "" {
		t.Errorf("CORS headers %v", w.Header())
	}
	if w := e.do("HEAD", url, "", nil, "Origin", "https://cast.example"); w.Code != http.StatusOK {
		t.Errorf("HEAD: %d", w.Code)
	}
	w = e.do("GET", url, "", nil, "Range", "bytes=10-19")
	if w.Code != http.StatusPartialContent || w.Body.String() != heatBytes[10:20] {
		t.Errorf("range: %d %q", w.Code, w.Body.String())
	}
	w = e.do("GET", url, "", nil, "Range", "bytes=1000-")
	if w.Code != http.StatusRequestedRangeNotSatisfiable || problemCode(w) != codeRangeNotSatisfiable {
		t.Errorf("unsatisfiable range: %d %s %q", w.Code, problemCode(w), w.Body.String())
	}
	if w := e.do("GET", "/api/v1/plays/"+ps["id"].(string)+"/files/1", "", nil); w.Code != http.StatusNotFound {
		t.Errorf("a part past the end: %d", w.Code)
	}
	if w := e.do("GET", "/api/v1/plays/"+id.New().String()+"/files/0", "", nil); w.Code != http.StatusNotFound {
		t.Errorf("unknown play: %d", w.Code)
	}
	if w := e.do("GET", "/api/v1/plays/"+ps["id"].(string)+"/subtitles/0.vtt", "", nil); w.Code != http.StatusNotFound {
		t.Errorf("subtitles: %d", w.Code)
	}

	// Signing out the session it was started by closes its media at once.
	if err := e.d.Auth.RevokeSession(ctx, auth.Caller{User: e.admin}, e.adminSession); err != nil {
		t.Fatal(err)
	}
	if w := e.do("GET", url, "", nil); w.Code != http.StatusNotFound {
		t.Errorf("after revoking: %d", w.Code)
	}
}

func TestMediaFollowsGrants(t *testing.T) {
	e := newEnv(t, serving{})
	ps := e.startPlayback()
	url := "/api/v1/" + ps["url"].(string)
	if w := e.do("GET", url, "", nil); w.Code != http.StatusOK {
		t.Fatalf("before: %d", w.Code)
	}
	// Take the library away from the admin.
	var admin id.ID
	e.b.Write(func(ctx context.Context, tx *store.Tx) error {
		return tx.QueryRowContext(ctx, `SELECT id FROM user WHERE username = 'admin'`).Scan(&admin)
	})
	other, _ := e.b.Library(catalog.Movies)
	if err := e.d.Users.Update(ctx, id.ID{}, admin, users.Change{Grants: &users.Grants{Libraries: []id.ID{other}}}); err != nil {
		t.Fatal(err)
	}
	if w := e.do("GET", url, "", nil); w.Code != http.StatusNotFound {
		t.Errorf("after removing the grant: %d", w.Code)
	}
}

func TestScrubPreviews(t *testing.T) {
	e := newEnv(t, serving{})
	ps := e.startPlayback()
	base := "/api/v1/plays/" + ps["id"].(string) + "/previews/"
	if p := ps["previews"].(map[string]any); p["ready"] != false || p["url"] != nil {
		t.Errorf("before they're made: %v", p)
	}
	if w := e.do("GET", base+"manifest.json", "", nil); w.Code != http.StatusNotFound {
		t.Errorf("manifest before they're made: %d", w.Code)
	}

	if err := os.WriteFile(filepath.Join(e.makePreviews(), "sheet-0.webp"), []byte("RIFF....WEBP"), 0o644); err != nil {
		t.Fatal(err)
	}
	ps = e.startPlayback()
	base = "/api/v1/plays/" + ps["id"].(string) + "/previews/"
	p := ps["previews"].(map[string]any)
	if p["ready"] != true || p["url"] != "plays/"+ps["id"].(string)+"/previews/manifest.json" {
		t.Fatalf("previews %v", p)
	}
	w := e.do("GET", base+"manifest.json", "", nil, "Origin", "https://cast.example")
	m := decode[map[string]any](t, w)
	if w.Code != http.StatusOK || m["intervalMs"] != 10000.0 || m["width"] != 320.0 || m["columns"] != 10.0 ||
		len(m["sheets"].([]any)) != 1 || m["sheets"].([]any)[0] != "sheet-0.webp" {
		t.Fatalf("manifest: %d %v", w.Code, m)
	}
	w = e.do("GET", base+"sheet-0.webp", "", nil)
	if w.Code != http.StatusOK || w.Header().Get("Content-Type") != "image/webp" || w.Body.String() != "RIFF....WEBP" {
		t.Errorf("sheet: %d %v %q", w.Code, w.Header(), w.Body.String())
	}
	for _, name := range []string{"sheet-1.webp", "..%2Fsheet-0.webp", "sheet-0.jpg"} {
		if w := e.do("GET", base+name, "", nil); w.Code != http.StatusNotFound {
			t.Errorf("%s: %d", name, w.Code)
		}
	}
}

func TestPlayControl(t *testing.T) {
	e := newEnv(t, serving{})
	ps := e.startPlayback()
	sid := ps["id"].(string)
	if parts := ps["parts"].([]any); len(parts) != 1 {
		t.Errorf("parts %v", parts)
	}
	// A movie's credits are its only marker.
	if chapters := ps["chapters"].([]any); len(chapters) != 3 || chapters[2].(map[string]any)["title"] != "End Credits" {
		t.Errorf("chapters %v", chapters)
	}
	want := []any{map[string]any{"kind": "credits", "startMs": 6_600_000.0, "endMs": 7_200_000.0, "source": "chapter", "final": true}}
	if markers := ps["markers"]; !reflect.DeepEqual(markers, want) {
		t.Errorf("markers %v", markers)
	}
	hb := "/api/v1/plays/" + sid
	if w := e.do("PATCH", hb, e.adminToken, map[string]any{"positionMs": 60000, "state": "paused"}); w.Code != http.StatusNoContent {
		t.Errorf("heartbeat: %d %s", w.Code, w.Body.String())
	}
	// Another user can't control it, because to them it's not found.
	if w := e.do("PATCH", hb, e.nobodyToken, map[string]any{"positionMs": 0, "state": "playing"}); w.Code != http.StatusNotFound {
		t.Errorf("someone else's heartbeat: %d", w.Code)
	}
	// A stop may say playback failed, with the position and the player's
	// detail for the log. Only the play's own player says so.
	stop := "/api/v1/plays/" + sid + "/stop"
	for name, c := range map[string]struct {
		token string
		body  map[string]any
		want  int
	}{
		"too long a detail":      {e.adminToken, map[string]any{"positionMs": 61000, "code": "mediaDecodeError", "detail": strings.Repeat("x", 1001)}, http.StatusBadRequest},
		"a code, no position":    {e.adminToken, map[string]any{"code": "mediaDecodeError"}, http.StatusBadRequest},
		"a detail, no code":      {e.adminToken, map[string]any{"positionMs": 61000, "detail": "decode"}, http.StatusBadRequest},
		"from the Cast receiver": {e.castToken(e.adminToken)["token"].(string), map[string]any{"positionMs": 61000, "code": "mediaDecodeError"}, http.StatusForbidden},
	} {
		if w := e.do("POST", stop, c.token, c.body); w.Code != c.want {
			t.Errorf("stop with %s: %d, want %d", name, w.Code, c.want)
		}
	}
	if w := e.do("POST", "/api/v1/plays/"+sid+"/stop", e.adminToken, map[string]any{"positionMs": 61000, "code": "mediaDecodeError", "detail": "decode"}); w.Code != http.StatusNoContent {
		t.Errorf("stop: %d %s", w.Code, w.Body.String())
	}
	if w := e.do("PATCH", hb, e.adminToken, map[string]any{"positionMs": 0, "state": "playing"}); w.Code != http.StatusNotFound {
		t.Errorf("heartbeat after stop: %d", w.Code)
	}
}

// TestBandwidthTest sends a remote client the bytes it asks for, and a
// local one nothing to time.
func TestBandwidthTest(t *testing.T) {
	e := newEnv(t, serving{})
	// httptest's client address, 192.0.2.1, is remote.
	w := e.do("GET", "/api/v1/bandwidth-test?size=100000", e.nobodyToken, nil)
	if w.Code != http.StatusOK || w.Body.Len() != 100000 || w.Header().Get("Content-Length") != "100000" {
		t.Errorf("remote: %d, %d bytes, Content-Length %q", w.Code, w.Body.Len(), w.Header().Get("Content-Length"))
	}
	r := httptest.NewRequest("GET", "/api/v1/bandwidth-test?size=100000", nil)
	r.RemoteAddr = "192.168.1.20:50000"
	r.Header.Set("Authorization", "Bearer "+e.nobodyToken)
	local := httptest.NewRecorder()
	e.srv.ServeHTTP(local, r)
	if local.Code != http.StatusNoContent || local.Body.Len() != 0 {
		t.Errorf("local: %d, %d bytes", local.Code, local.Body.Len())
	}
	for _, size := range []string{"0", "10000001"} {
		if w := e.do("GET", "/api/v1/bandwidth-test?size="+size, e.nobodyToken, nil); w.Code != http.StatusBadRequest {
			t.Errorf("size %s: %d", size, w.Code)
		}
	}
	if w := e.do("GET", "/api/v1/bandwidth-test?size=1", "", nil); w.Code != http.StatusUnauthorized {
		t.Errorf("signed out: %d", w.Code)
	}
}

func TestAdminStopAndWatchHistory(t *testing.T) {
	e := newEnv(t, serving{})
	var friend users.User
	e.b.Write(func(ctx context.Context, tx *store.Tx) error {
		var err error
		friend, err = e.d.Users.CreateUser(ctx, tx, users.NewUser{Username: "friend", Grants: users.Grants{All: true}})
		return err
	})
	token := e.session(friend.ID)
	w := e.do("POST", "/api/v1/plays", token, map[string]any{"workId": e.heat.Work, "capabilities": clientCaps})
	if w.Code != http.StatusCreated {
		t.Fatalf("start: %d %s", w.Code, w.Body.String())
	}
	sid := decode[map[string]any](t, w)["id"].(string)
	hb := "/api/v1/plays/" + sid
	if w := e.do("PATCH", hb, token, map[string]any{"positionMs": 60000, "state": "playing"}); w.Code != http.StatusNoContent {
		t.Fatalf("heartbeat: %d", w.Code)
	}

	// An admin reads it for diagnostics; its owner can't.
	one := "/api/v1/plays/" + sid
	if w := e.do("GET", one, e.adminToken, nil); w.Code != http.StatusOK || decode[map[string]any](t, w)["username"] != "friend" {
		t.Fatalf("an admin's read: %d %s", w.Code, w.Body.String())
	}
	if w := e.do("GET", one, token, nil); w.Code != http.StatusForbidden {
		t.Fatalf("the owner's read: %d", w.Code)
	}

	// Someone else can't stop it. An admin can, without knowing where it
	// is playing.
	stop := "/api/v1/plays/" + sid + "/stop"
	if w := e.do("POST", stop, e.nobodyToken, map[string]any{}); w.Code != http.StatusNotFound {
		t.Fatalf("a stranger's stop: %d", w.Code)
	}
	if w := e.do("POST", stop, e.adminToken, map[string]any{"message": strings.Repeat("x", 501)}); w.Code != http.StatusBadRequest {
		t.Fatalf("an admin's stop with too long a message: %d", w.Code)
	}
	if w := e.do("POST", stop, e.adminToken, map[string]any{"message": "Server maintenance"}); w.Code != http.StatusNoContent {
		t.Fatalf("an admin's stop: %d %s", w.Code, w.Body.String())
	}
	// Its player learns of the stop, with the admin's words, rather than
	// starting it again.
	w = e.do("PATCH", hb, token, map[string]any{"positionMs": 0, "state": "playing"})
	if p := decode[map[string]any](t, w); w.Code != http.StatusGone || p["code"] != "playEnded" || p["message"] != "Server maintenance" {
		t.Fatalf("heartbeat after an admin's stop: %d %v", w.Code, p)
	}
	if w := e.do("PATCH", hb, e.nobodyToken, map[string]any{"positionMs": 0, "state": "playing"}); w.Code != http.StatusNotFound {
		t.Fatalf("a stranger's heartbeat after an admin's stop: %d", w.Code)
	}
	if w := e.do("GET", one, e.adminToken, nil); w.Code != http.StatusNotFound {
		t.Fatalf("an admin's read after the stop: %d", w.Code)
	}

	// The play is in the friend's history, where its last heartbeat left it,
	// and in the server-wide history with who played it.
	page := decode[map[string]any](t, e.do("GET", "/api/v1/users/me/play-events", token, nil))
	items := page["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("history %v", page)
	}
	play := items[0].(map[string]any)
	if play["positionMs"] != 60000.0 || play["work"].(map[string]any)["title"] != "Heatwave Seeded" || play["username"] != "friend" {
		t.Fatalf("play %v", play)
	}
	if mine := decode[map[string]any](t, e.do("GET", "/api/v1/users/me/play-events", e.adminToken, nil)); len(mine["items"].([]any)) != 0 {
		t.Fatalf("the admin's own history: %v", mine)
	}
	all := decode[map[string]any](t, e.do("GET", "/api/v1/play-events", e.adminToken, nil))
	if items := all["items"].([]any); len(items) != 1 || items[0].(map[string]any)["username"] != "friend" {
		t.Fatalf("server-wide history: %v", all)
	}
	for user, want := range map[string]int{friend.ID.String(): 1, e.admin.String(): 0} {
		page := decode[map[string]any](t, e.do("GET", "/api/v1/play-events?userId="+user, e.adminToken, nil))
		if n := len(page["items"].([]any)); n != want {
			t.Errorf("one user's history: %d plays, want %d", n, want)
		}
	}
	if w := e.do("GET", "/api/v1/play-events", e.nobodyToken, nil); w.Code != http.StatusForbidden {
		t.Fatalf("a non-admin: %d", w.Code)
	}
	if w := e.do("GET", "/api/v1/users/me/play-events?after=bogus", token, nil); w.Code != http.StatusBadRequest {
		t.Fatalf("a bad cursor: %d", w.Code)
	}
}

func TestAcceleratorsAndNowPlaying(t *testing.T) {
	e := newEnv(t, serving{})
	// The CPU isn't an accelerator, so a server with none lists nothing.
	accel := decode[map[string]any](t, e.do("GET", "/api/v1/accelerators", e.adminToken, nil))
	if items := accel["items"].([]any); accel["tested"] != true || len(items) != 0 {
		t.Fatalf("accelerators %v", accel)
	}
	w := e.do("PATCH", "/api/v1/settings", e.adminToken, map[string]any{"accelerator": "none", "videoTranscodeLimit": 2})
	if st := decode[map[string]any](t, w); w.Code != http.StatusOK || st["accelerator"] != "none" || st["videoTranscodeLimit"] != 2.0 {
		t.Fatalf("settings: %d %v", w.Code, st)
	}
	if w := e.do("PATCH", "/api/v1/settings", e.adminToken, map[string]any{"accelerator": "gpu"}); w.Code != http.StatusBadRequest {
		t.Fatalf("a bad accelerator: %d", w.Code)
	}

	ps := e.startPlayback()
	hb := "/api/v1/plays/" + ps["id"].(string)
	if w := e.do("PATCH", hb, e.adminToken, map[string]any{"positionMs": 60000, "state": "playing", "seekToFirstFrameMs": 1200, "bandwidthBps": 40_000_000}); w.Code != http.StatusNoContent {
		t.Fatalf("heartbeat: %d", w.Code)
	}
	page := decode[map[string]any](t, e.do("GET", "/api/v1/plays", e.adminToken, nil))
	items := page["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("plays %v", page)
	}
	s := items[0].(map[string]any)
	if s["title"] != "Heatwave Seeded" || s["username"] != "admin" || s["clientName"] != "Test" || s["seekToFirstFrameMs"].([]any)[0] != 1200.0 ||
		s["positionMs"] != 60000.0 || s["bitrateBps"] != 8_000_000.0 || s["bandwidthBps"] != 40_000_000.0 {
		t.Fatalf("play %v", s)
	}
	// The summary's artwork and title parts, and titles for the source
	// tracks that the decision names.
	work, _ := s["work"].(map[string]any)
	tracks, _ := s["tracks"].([]any)
	if work["title"] != "Heatwave Seeded" || s["versionId"] == nil || s["durationMs"] == nil || s["address"] == nil || len(tracks) == 0 ||
		tracks[0].(map[string]any)["displayTitle"] == "" {
		t.Fatalf("play's summary and tracks %v", s)
	}
	if w := e.do("GET", "/api/v1/plays", e.nobodyToken, nil); w.Code != http.StatusForbidden {
		t.Fatalf("a non-admin: %d", w.Code)
	}
	metrics := e.do("GET", "/api/v1/metrics", e.adminToken, nil).Body.String()
	for _, want := range []string{`kivo_plays{summary="directPlay"} 1`, "kivo_seek_first_frame_seconds_count 1", "kivo_seek_first_frame_seconds_sum 1.2"} {
		if !strings.Contains(metrics, want) {
			t.Errorf("metrics lack %q:\n%s", want, metrics)
		}
	}
}

// TestPlayDialogueBoost checks that a play says whether it boosts dialogue
// and what the boost costs, and that a boosted play is a stream. The
// seeded file plays directly but has no keyframe index, so a stream would
// also encode its video, which this server's media can't do.
func TestPlayDialogueBoost(t *testing.T) {
	e := newEnv(t, serving{})
	start := func(body map[string]any) *httptest.ResponseRecorder {
		body["workId"], body["capabilities"] = e.heat.Work, clientCaps
		return e.do("POST", "/api/v1/plays", e.adminToken, body)
	}
	w := start(map[string]any{})
	if w.Code != http.StatusCreated {
		t.Fatalf("start: %d %s", w.Code, w.Body.String())
	}
	plain := decode[map[string]any](t, w)
	if b, _ := plain["dialogueBoost"].(map[string]any); b["enabled"] != false || b["cost"] != "videoTranscode" {
		t.Fatalf("a play without the boost: %v", plain["dialogueBoost"])
	}
	if w := start(map[string]any{"dialogueBoost": true}); w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), `"transcodeFailed"`) {
		t.Fatalf("a boosted play: %d %s", w.Code, w.Body.String())
	}
}

// TestPlayStartedDialogueBoost checks a play's dialogue boost on the wire,
// and that the request's switch is read into the start.
func TestPlayStartedDialogueBoost(t *testing.T) {
	boost := true
	r, err := parseStartRequest(&gen.StartPlayJSONRequestBody{WorkID: id.New(), DialogueBoost: &boost}, &request{})
	if err != nil || !r.DialogueBoost {
		t.Fatalf("the request's switch: %+v %v", r, err)
	}
	for _, c := range []struct {
		in   *playback.DialogueBoost
		want *gen.DialogueBoost
	}{
		{nil, nil},
		{&playback.DialogueBoost{}, &gen.DialogueBoost{}},
		{&playback.DialogueBoost{Enabled: true, Cost: playback.CostAudioAsAAC},
			&gen.DialogueBoost{Enabled: true, Cost: ptr(gen.DialogueBoostCostAudioAsAac)}},
	} {
		got := playStarted(playback.Started{Decision: playback.Decision{DialogueBoost: c.in}}).DialogueBoost
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("%+v on the wire: %+v, want %+v", c.in, got, c.want)
		}
	}
}

// The current form's capabilities are read entry by entry. A value the
// server doesn't know is skipped, an entry left with no delivery is
// dropped, as is one without a codec, or audio without its channels, and a
// video or audio entry's external delivery means nothing.
func TestParseCapabilities(t *testing.T) {
	raw := json.RawMessage(`{
		"file": {"containers": ["mkv", "mp4"], "trackSelection": true, "later": 1},
		"video": [
			{"codec": "hevc", "delivery": ["file", "stream"], "containers": ["mp4"], "tags": ["hvc1"], "profiles": ["Main 10"],
			 "maxLevel": 5.1, "maxBitDepth": 10, "maxWidth": 3840, "maxHeight": 2160, "maxFrameRate": 60, "maxBitrateBps": 100000000,
			 "maxRefFrames": 6, "interlaced": false, "anamorphic": false, "hdr": ["hdr10", "hdr10plus", "hlg"],
			 "dolbyVision": [{"profile": 8, "compat": [1, 4], "el": false}], "doviFallback": true, "someLaterLimit": 3},
			{"codec": "av1", "delivery": ["file", "teleport"]},
			{"codec": "vp9", "delivery": ["stream"], "hdr": ["hdr12"]},
			{"codec": "h264", "delivery": ["file"], "dolbyVision": [{"profile": 9}]},
			{"codec": "", "delivery": ["file"]},
			{"codec": "mpeg2video", "delivery": ["external"]},
			{"codec": "vc1", "delivery": ["file", "external"]}
		],
		"audio": [
			{"codec": "eac3", "delivery": ["file"], "maxChannels": 8, "maxSampleRate": 48000, "maxBitDepth": 24, "profiles": ["Dolby Digital Plus + Dolby Atmos"]},
			{"codec": "aac", "delivery": ["stream"]},
			{"codec": "opus", "delivery": ["file", "beam"], "maxChannels": 2}
		],
		"subtitles": [
			{"codec": "webvtt", "delivery": ["stream", "external"]},
			{"codec": "ass", "delivery": ["holo"]},
			{"codec": "subrip", "delivery": ["file"]},
			{"codec": "hdmv_pgs_subtitle", "delivery": ["file"], "containers": ["mkv"]}
		]
	}`)
	var c gen.Capabilities
	if err := json.Unmarshal(raw, &c); err != nil {
		t.Fatal(err)
	}
	caps := parseCapabilities(c)
	both := []playback.Delivery{playback.DeliverFile, playback.DeliverStream}
	want := playback.Capabilities{
		File: &playback.FileCaps{Containers: []string{"mkv", "mp4"}, TrackSelection: true},
		Video: []playback.VideoEntry{
			{Codec: "hevc", Delivery: both, Containers: []string{"mp4"}, Tags: []string{"hvc1"}, Profiles: []string{"Main 10"},
				MaxLevel: 5.1, MaxBitDepth: 10, MaxWidth: 3840, MaxHeight: 2160, MaxFramerate: 60, MaxBitrate: 100_000_000, MaxRefFrames: 6,
				Interlaced: ptr(false), Anamorphic: ptr(false), HDR: []media.DynamicRange{media.HDR10, playback.HDR10Plus, media.HLG},
				DolbyVision: []playback.DolbyVisionEntry{{Profile: 8, Compat: []int{1, 4}, EL: ptr(false)}}, DoviFallback: true},
			{Codec: "av1", Delivery: []playback.Delivery{playback.DeliverFile}},
			{Codec: "vp9", Delivery: []playback.Delivery{playback.DeliverStream}},
			{Codec: "h264", Delivery: []playback.Delivery{playback.DeliverFile}, DolbyVision: []playback.DolbyVisionEntry{{Profile: 9}}},
			{Codec: "vc1", Delivery: []playback.Delivery{playback.DeliverFile}},
		},
		Audio: []playback.AudioEntry{
			{Codec: "eac3", Delivery: []playback.Delivery{playback.DeliverFile}, MaxChannels: 8, MaxSampleRate: 48000,
				MaxBitDepth: 24, Profiles: []string{"Dolby Digital Plus + Dolby Atmos"}},
			{Codec: "opus", Delivery: []playback.Delivery{playback.DeliverFile}, MaxChannels: 2},
		},
		Subtitles: []playback.SubtitleEntry{
			{Codec: "webvtt", Delivery: []playback.Delivery{playback.DeliverStream, playback.DeliverExternal}},
			{Codec: "subrip", Delivery: []playback.Delivery{playback.DeliverFile}},
			{Codec: "hdmv_pgs_subtitle", Delivery: []playback.Delivery{playback.DeliverFile}, Containers: []string{"mkv"}},
		},
	}
	if !reflect.DeepEqual(caps, want) {
		t.Errorf("got  %+v\nwant %+v", caps, want)
	}
	if s := parseSettings(&gen.PlaybackSettings{DirectPlay: ptr(false), BurnSubtitles: ptr(true)}); s != (playback.Settings{DisableDirectPlay: true, BurnSubtitles: true}) {
		t.Errorf("settings %+v", s)
	}
}

// TestPlayRequestChoosesTracksByID starts plays with the track IDs that
// the work's detail lists, and with subtitles off. It then switches
// subtitles in a heartbeat.
func TestPlayRequestChoosesTracksByID(t *testing.T) {
	e := newEnv(t, serving{})
	detail := decode[map[string]any](t, e.do("GET", "/api/v1/works/"+e.heat.Work.String(), e.adminToken, nil))
	var audio string
	for _, tr := range detail["versions"].([]any)[0].(map[string]any)["tracks"].([]any) {
		if tr := tr.(map[string]any); tr["type"] == "audio" {
			audio = tr["id"].(string)
		}
	}
	if v := detail["versions"].([]any)[0].(map[string]any); v["audioTrackId"] != audio || v["subtitleTrackId"] != nil {
		t.Fatalf("the tracks a play starts with: %v", v)
	}
	start := func(body map[string]any) *httptest.ResponseRecorder {
		body["workId"], body["capabilities"] = e.heat.Work, clientCaps
		return e.do("POST", "/api/v1/plays", e.adminToken, body)
	}
	w := start(map[string]any{"audioTrackId": audio, "subtitleTrackId": "off"})
	if w.Code != http.StatusCreated {
		t.Fatalf("start: %d %s", w.Code, w.Body.String())
	}
	ps := decode[map[string]any](t, w)
	tracks := ps["tracks"].([]any)
	a, off := tracks[1].(map[string]any), tracks[len(tracks)-1].(map[string]any)
	if a["trackId"] != audio || a["selected"] != true || off["trackId"] != "off" || off["selected"] != true {
		t.Fatalf("play %v", ps)
	}
	if w := start(map[string]any{"subtitleTrackId": "2"}); w.Code != http.StatusBadRequest {
		t.Errorf("a positional subtitle index: %d", w.Code)
	}
	beat := func(sub string) int {
		body := map[string]any{"positionMs": 1000, "state": "playing", "subtitleTrackId": sub}
		return e.do("PATCH", "/api/v1/plays/"+ps["id"].(string), e.adminToken, body).Code
	}
	if c := beat("off"); c != http.StatusNoContent {
		t.Errorf("a heartbeat with subtitles off: %d", c)
	}
	if c := beat("2"); c != http.StatusBadRequest {
		t.Errorf("a heartbeat with a positional subtitle index: %d", c)
	}
	if w := start(map[string]any{"audioTrackId": id.New()}); w.Code != http.StatusNotFound {
		t.Errorf("an unknown audio track: %d %s", w.Code, w.Body.String())
	}
}

func TestVideoInfo(t *testing.T) {
	for _, c := range []struct {
		track media.Track
		res   gen.Resolution
		hdr   *gen.HdrFormat
	}{
		{media.Track{Width: 3840, Height: 1600, Range: media.HDR10, DolbyVisionProfile: 8}, gen.ResolutionN4K, ptr(gen.HdrFormatDolbyVision)},
		{media.Track{Width: 1920, Height: 800, Range: media.HLG}, gen.ResolutionN1080P, ptr(gen.HdrFormatHlg)},
		{media.Track{Width: 1280, Height: 720, Range: media.SDR}, gen.ResolutionN720P, nil},
		{media.Track{Width: 720, Height: 480}, gen.ResolutionSd, nil},
	} {
		got := videoInfo(c.track)
		if *got.Resolution != c.res || !reflect.DeepEqual(got.Hdr, c.hdr) {
			t.Errorf("videoInfo(%dx%d %s) = %s %v, want %s %v", c.track.Width, c.track.Height, c.track.Range, *got.Resolution, got.Hdr, c.res, c.hdr)
		}
	}
}

// A version's tracks carry media's decisions. A commentary is found by its
// title, the original audio is marked beside a dub, and the video has its
// resolution and HDR format.
func TestVersionTracks(t *testing.T) {
	v := version(query.Version{Tracks: []query.Track{
		{Type: "video", Codec: "hevc", Width: 3840, Height: 2160, Range: "hdr10"},
		{Type: "audio", Codec: "truehd", Language: "jpn"},
		{Type: "audio", Codec: "eac3", Language: "eng"},
		{Type: "audio", Codec: "aac", Language: "jpn", Title: "Director's commentary"},
		{Type: "subtitle", Source: "external", Codec: "subrip", Language: "jpn"},
	}}, "ja")
	type facts struct{ original, commentary bool }
	got := each(v.Tracks, func(t gen.Track) facts { return facts{deref(t.Original), deref(t.Commentary)} })
	want := []facts{{false, false}, {true, false}, {false, false}, {false, true}, {false, false}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("tracks: %v, want %v", got, want)
	}
	if deref(v.Resolution) != gen.ResolutionN4K || deref(v.Hdr) != gen.HdrFormatHdr10 {
		t.Errorf("version: %v %v, want 4k hdr10", deref(v.Resolution), deref(v.Hdr))
	}
	if s := v.Tracks[4].Subtitle; s == nil || s.Source != gen.SubtitleSourceExternal {
		t.Errorf("subtitle: %+v, want an external file", s)
	}
}
