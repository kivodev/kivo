package playback

import (
	"testing"

	"github.com/mmcdole/kivo/internal/media"
)

func TestCapsSummary(t *testing.T) {
	both := []Delivery{DeliverFile, DeliverStream}
	c := Capabilities{
		File: &FileCaps{Containers: []string{"mkv"}, TrackSelection: true},
		Video: []VideoEntry{
			{Codec: media.H264, Delivery: both, MaxWidth: 1920, MaxHeight: 1080},
			{Codec: media.HEVC, Delivery: []Delivery{DeliverFile}, HDR: []media.DynamicRange{media.HDR10}, MaxWidth: 3840, MaxHeight: 2160,
				DolbyVision: []DolbyVisionEntry{{Profile: 5}, {Profile: 8}}},
			{Codec: media.HEVC, Delivery: []Delivery{DeliverStream}, HDR: []media.DynamicRange{media.HDR10, media.HLG}},
		},
		Audio:     []AudioEntry{{Codec: "aac", Delivery: both, MaxChannels: 6}, {Codec: "eac3", Delivery: []Delivery{DeliverFile}, MaxChannels: 8}},
		Subtitles: []SubtitleEntry{{Codec: webVTT, Delivery: []Delivery{DeliverStream, DeliverExternal}}},
	}
	want := "hevc=file,stream hdr=dv5,8,hdr10,hdr10,hlg audio=aac,eac3 subtitles=webvtt:stream,external max=3840x2160 selection=true"
	if got := capsSummary(c); got != want {
		t.Errorf("got %q", got)
	}
	if got := capsSummary(Capabilities{}); got != "hevc=none hdr=none audio=none subtitles=none max=0x0 selection=false" {
		t.Errorf("empty: %q", got)
	}
}
