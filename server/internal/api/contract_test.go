package api

import (
	"net/http"
	"os"
	"reflect"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/mmcdole/kivo/internal/api/internal/gen"
	"github.com/mmcdole/kivo/internal/id"
	"github.com/mmcdole/kivo/internal/media"
	"github.com/mmcdole/kivo/internal/playback"
	"github.com/mmcdole/kivo/internal/query"
)

// TestQueryAcceptsEveryContractValue checks that a rule takes every value
// the contract gives a resolution.
func TestQueryAcceptsEveryContractValue(t *testing.T) {
	raw, err := os.ReadFile("openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var spec struct {
		Components struct {
			Schemas map[string]struct{ Enum []string }
		}
	}
	if err := yaml.Unmarshal(raw, &spec); err != nil {
		t.Fatal(err)
	}
	e := newEnv(t, serving{})
	for field, schema := range map[string]string{"resolution": "Resolution"} {
		values := spec.Components.Schemas[schema].Enum
		if len(values) == 0 {
			t.Fatalf("%s has no values", schema)
		}
		for _, v := range values {
			if w := e.grid(e.adminToken, e.lib, "movie", map[string]any{"where": allOf([]string{field, "anyOf", v})}); w.Code != http.StatusOK {
				t.Errorf("%s=%s: %d %s", field, v, w.Code, w.Body.String())
			}
		}
	}
}

// TestPlayStarted checks a play's answer: its delivery, URL and every
// choice in tracks, and nothing else about its tracks.
func TestPlayStarted(t *testing.T) {
	e := newEnv(t, serving{})
	w := e.do("POST", "/api/v1/plays", e.adminToken, map[string]any{"workId": e.heat.Work, "capabilities": clientCaps})
	if w.Code != http.StatusCreated {
		t.Fatalf("start: %d %s", w.Code, w.Body.String())
	}
	ps := decode[map[string]any](t, w)
	for _, gone := range []string{"decision", "subtitles", "subtitleTracks", "audioTrackId", "subtitleTrackId"} {
		if _, ok := ps[gone]; ok {
			t.Errorf("the play has %s", gone)
		}
	}
	if ps["delivery"] != "file" || ps["url"] == nil || len(ps["tracks"].([]any)) != 3 {
		t.Fatalf("play %v", ps)
	}
}

// TestPlayTracksOnTheWire checks a decision's choices on the wire: off,
// the transcode block, the failed limit, a file's index and an external subtitle's
// URL and fonts.
func TestPlayTracksOnTheWire(t *testing.T) {
	play, video, ass := id.New(), id.New(), id.New()
	d := playback.Decision{Tracks: []playback.TrackDecision{
		{Type: media.Video, Track: video, Index: 0, Selected: true, Delivery: playback.DeliverStream, Codec: media.H264,
			Reason: playback.ReasonVideoLevelUnsupported, Unmet: &playback.Unmet{Field: "maxLevel", Limit: "4.1", Actual: "5.1"},
			Video: &media.VideoRendition{Codec: media.H264, Width: 1920, Height: 1080, Bitrate: 8_000_000}, Encoder: "CPU"},
		{Type: media.Subtitle, Track: ass, Index: 4, Selected: true, Delivery: playback.DeliverExternal, Codec: "ass"},
		{Type: media.Subtitle, Index: -1, Delivery: playback.DeliverNone},
	}}
	externals := []playback.ExternalSubtitle{{Track: ass, Format: media.ASS, Name: ass.String() + ".ass", Fonts: []string{"ab"}, Ready: true}}
	got := playTracks(d, externals, playMedia, play)
	want := []gen.PlayTrack{
		{TrackID: video.String(), Type: gen.TrackTypeVideo, Selected: ptr(true), Delivery: ptr(gen.TrackDeliveryStream), Codec: ptr("h264"),
			Reason: ptr(gen.TrackReasonVideoLevelUnsupported), Mismatch: &gen.CapabilityMismatch{Field: "maxLevel", Declared: ptr("4.1"), Actual: "5.1"},
			Transcode: &gen.TrackTranscode{Width: ptr(1920), Height: ptr(1080), BitrateBps: ptr(int64(8_000_000)), Encoder: ptr("CPU")}},
		{TrackID: ass.String(), Type: gen.TrackTypeSubtitle, Selected: ptr(true), Delivery: ptr(gen.TrackDeliveryExternal), Codec: ptr("ass"),
			URL: ptr(mediaURL(playMedia, play, "subtitles", ass.String()+".ass")), Fonts: &[]string{fontURL("ab")}},
		{TrackID: "off", Type: gen.TrackTypeSubtitle, Delivery: ptr(gen.TrackDeliveryNone)},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got  %+v\nwant %+v", got, want)
	}
}

// TestPlayVideoProcessing checks where Now playing says a play's video is
// transcoded: on its accelerator, on the CPU, or nowhere when it isn't.
func TestPlayVideoProcessing(t *testing.T) {
	video := func(codec media.Codec, accel media.Accelerator) playback.Decision {
		return playback.Decision{Tracks: []playback.TrackDecision{
			{Type: media.Video, Track: id.New(), Selected: true, Delivery: playback.DeliverStream,
				Video: &media.VideoRendition{Codec: codec}, Accelerator: accel},
			{Type: media.Audio, Track: id.New(), Selected: true, Delivery: playback.DeliverStream,
				Audio: &media.AudioRendition{Codec: media.AAC}},
		}}
	}
	file := playback.Decision{Tracks: []playback.TrackDecision{{Type: media.Video, Track: id.New(), Selected: true, Delivery: playback.DeliverFile}}}
	for _, tc := range []struct {
		name string
		d    playback.Decision
		want *gen.ProcessedOn
	}{
		{"accelerator", video(media.H264, "qsv:/dev/dri/renderD128"), ptr(gen.ProcessedOnAccelerator)},
		{"cpu", video(media.H264, ""), ptr(gen.ProcessedOnCPU)},
		{"video copied, audio transcoded", video(media.Copy, ""), nil},
		{"direct play", file, nil},
	} {
		got := playInfo(playback.Play{Decision: tc.d}, query.WorkSummary{}).VideoProcessing
		if !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%s: got %v, want %v", tc.name, deref(got), deref(tc.want))
		}
	}
}
