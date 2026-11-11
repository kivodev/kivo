package playback

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mmcdole/kivo/internal/media"
)

var update = flag.Bool("update", false, "rewrite golden files")

func ptrBool(b bool) *bool { return &b }

// dolbyVision is a Dolby Vision video of a profile, with its base layer's
// compatibility ID and the range its transfer gives.
func dolbyVision(profile, compat int, base media.DynamicRange, el bool) *SourceTrack {
	v := hevc10()
	v.DolbyVisionProfile, v.DolbyVisionCompat, v.Range, v.DolbyVisionEL = profile, compat, base, el
	return v
}

// TestDolbyVision checks the Dolby Vision match: a listed profile, with
// its compatibility ID among compat when that's given and no enhancement
// layer when el is false; or with doviFallback, a base layer whose range
// the entry shows. SDR always shows, and the delivered HDR says which.
func TestDolbyVision(t *testing.T) {
	entry := func(e VideoEntry) Capabilities {
		e.Codec, e.MaxBitDepth = "hevc", 10
		return alike([]string{"mkv"}, true, []VideoEntry{e}, []AudioEntry{{Codec: "aac", MaxChannels: 2}})
	}
	hdr10 := []media.DynamicRange{media.HDR10}
	for _, c := range []struct {
		name  string
		caps  Capabilities
		video *SourceTrack
		hdr   string // as delivered, when it plays from the file
		unmet *Unmet
	}{
		{"8.1, profile 8 listed", entry(VideoEntry{DolbyVision: []DolbyVisionEntry{{Profile: 8}}}), dolbyVision(8, 1, media.HDR10, false), "dolbyVision", nil},
		{"8.1, profile 8 listed for compat 1", entry(VideoEntry{DolbyVision: []DolbyVisionEntry{{Profile: 8, Compat: []int{1}}}}),
			dolbyVision(8, 1, media.HDR10, false), "dolbyVision", nil},
		{"8.4, profile 8 listed only for compat 1", entry(VideoEntry{DolbyVision: []DolbyVisionEntry{{Profile: 8, Compat: []int{1}}}}),
			dolbyVision(8, 4, media.HLG, false), "", &Unmet{Field: "dolbyVision", Limit: "8", Actual: "8.4"}},
		{"8.1 by its HDR10 base layer", entry(VideoEntry{HDR: hdr10, DoviFallback: true}), dolbyVision(8, 1, media.HDR10, false), "hdr10", nil},
		{"8.1 without a fallback", entry(VideoEntry{HDR: hdr10}), dolbyVision(8, 1, media.HDR10, false), "", &Unmet{Field: "dolbyVision", Actual: "8.1"}},
		{"8.4 needs HLG for its fallback", entry(VideoEntry{HDR: hdr10, DoviFallback: true}), dolbyVision(8, 4, media.HLG, false), "",
			&Unmet{Field: "dolbyVision", Actual: "8.4"}},
		{"8.4 by its HLG base layer", entry(VideoEntry{HDR: []media.DynamicRange{media.HLG}, DoviFallback: true}), dolbyVision(8, 4, media.HLG, false), "hlg", nil},
		{"8.2 by its SDR base layer, which always shows", entry(VideoEntry{DoviFallback: true}), dolbyVision(8, 2, media.SDR, false), "", nil},
		{"7 by its HDR10 base layer", entry(VideoEntry{HDR: hdr10, DoviFallback: true}), dolbyVision(7, 6, media.HDR10, true), "hdr10", nil},
		{"7 with an enhancement layer, el false", entry(VideoEntry{DolbyVision: []DolbyVisionEntry{{Profile: 7, EL: ptrBool(false)}}}),
			dolbyVision(7, 6, media.HDR10, true), "", &Unmet{Field: "dolbyVision", Limit: "7", Actual: "7.6"}},
		{"7 with an enhancement layer, el absent", entry(VideoEntry{DolbyVision: []DolbyVisionEntry{{Profile: 7}}}),
			dolbyVision(7, 6, media.HDR10, true), "dolbyVision", nil},
		{"5 has no base layer to fall back to", entry(VideoEntry{HDR: hdr10, DoviFallback: true}), dolbyVision(5, 0, media.HDR10, false), "",
			&Unmet{Field: "dolbyVision", Actual: "5"}},
		{"10.0 has none either", entry(VideoEntry{HDR: hdr10, DoviFallback: true}), dolbyVision(10, 0, media.SDR, false), "",
			&Unmet{Field: "dolbyVision", Actual: "10"}},
		{"10.1 by its compatibility ID", entry(VideoEntry{HDR: hdr10, DoviFallback: true}), dolbyVision(10, 1, media.HDR10, false), "hdr10", nil},
		{"8 probed before the ID was read falls back by its transfer", entry(VideoEntry{HDR: hdr10, DoviFallback: true}),
			dolbyVision(8, 0, media.HDR10, false), "hdr10", nil},
		{"HDR10 shows on an HDR10 entry", entry(VideoEntry{HDR: hdr10}), hevc10(), "hdr10", nil},
		{"HDR10 on an SDR entry", entry(VideoEntry{}), hevc10(), "", &Unmet{Field: "hdr", Actual: "hdr10"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			d := decided(Input{Caps: c.caps, Source: Source{Container: "matroska,webm", Video: c.video, Audio: audio("aac", 2), DefaultAudio: true, Remuxable: true}})
			v, _ := d.Selected(media.Video)
			if c.unmet == nil {
				if v.Delivery != DeliverFile || v.HDR != c.hdr {
					t.Fatalf("got %+v, want it from the file as %q", v, c.hdr)
				}
				return
			}
			if v.Delivery == DeliverFile || v.Reason != ReasonVideoHdrUnsupported || v.Unmet == nil || *v.Unmet != *c.unmet {
				t.Fatalf("got %+v (%+v), want videoHdrUnsupported %+v", v, v.Unmet, *c.unmet)
			}
		})
	}
}

// TestVideoLimits checks every limit of a video entry, each failing with
// its reason and the field, the declared value and the video's.
func TestVideoLimits(t *testing.T) {
	video := func() *SourceTrack {
		v := h264()
		v.Level, v.FrameRate, v.References, v.Bitrate, v.Tag = 51, 59.94, 5, 30_000_000, "avc1"
		return v
	}
	for _, c := range []struct {
		name  string
		entry VideoEntry
		video *SourceTrack
		why   Reason
		unmet Unmet
	}{
		{"a level above the entry's, as specification decimals", VideoEntry{MaxLevel: 4.1}, video(), ReasonVideoLevelUnsupported, Unmet{"maxLevel", "4.1", "5.1"}},
		{"a frame rate", VideoEntry{MaxFramerate: 30}, video(), ReasonVideoFramerateUnsupported, Unmet{"maxFramerate", "30", "59.94"}},
		{"a bitrate", VideoEntry{MaxBitrate: 20_000_000}, video(), ReasonVideoBitrateUnsupported, Unmet{"maxBitrate", "20000000", "30000000"}},
		{"reference frames", VideoEntry{MaxRefFrames: 4}, video(), ReasonVideoRefFramesUnsupported, Unmet{"maxRefFrames", "4", "5"}},
		{"a profile", VideoEntry{Profiles: []string{"Main"}}, video(), ReasonVideoProfileUnsupported, Unmet{"profiles", "Main", "High"}},
		{"a bit depth", VideoEntry{MaxBitDepth: 8}, hevcAs("h264"), ReasonVideoBitDepthUnsupported, Unmet{"maxBitDepth", "8", "10"}},
		{"a width", VideoEntry{MaxWidth: 1280}, video(), ReasonVideoResolutionUnsupported, Unmet{"maxWidth", "1280", "1920"}},
		{"a height", VideoEntry{MaxHeight: 720}, video(), ReasonVideoResolutionUnsupported, Unmet{"maxHeight", "720", "1080"}},
		{"interlaced video", VideoEntry{Interlaced: ptrBool(false)}, interlaced(video()), ReasonVideoInterlacedUnsupported, Unmet{"interlaced", "false", "true"}},
		{"non-square pixels", VideoEntry{Anamorphic: ptrBool(false)}, anamorphic(video()), ReasonVideoAnamorphicUnsupported, Unmet{"anamorphic", "false", "true"}},
		{"a codec tag, from a file", VideoEntry{Tags: []string{"avc3"}}, video(), ReasonVideoCodecTagUnsupported, Unmet{"tags", "avc3", "avc1"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			e := c.entry
			e.Codec = "h264"
			caps := alike([]string{"mkv"}, false, []VideoEntry{e}, []AudioEntry{{Codec: "aac", MaxChannels: 2}})
			d := decided(Input{Caps: caps, Source: Source{Container: "matroska,webm", Video: c.video, Audio: audio("aac", 2), DefaultAudio: true}})
			if d.Playable || d.Refusal != RefuseFileUnplayable || d.Unmet == nil || *d.Unmet != c.unmet {
				t.Fatalf("got %+v (%+v), want %+v", d, d.Unmet, c.unmet)
			}
			caps = alike([]string{"mkv"}, true, []VideoEntry{e}, []AudioEntry{{Codec: "aac", MaxChannels: 2}})
			d = decided(Input{Caps: caps, Source: Source{Container: "matroska,webm", Video: c.video, Audio: audio("aac", 2), DefaultAudio: true, Remuxable: true}})
			if v, _ := d.Selected(media.Video); v.Reason != c.why || v.Unmet == nil || *v.Unmet != c.unmet {
				t.Fatalf("in a stream: %+v (%+v), want %s", v, v.Unmet, c.why)
			}
		})
	}
	// Without a limit, everything plays.
	caps := alike([]string{"mkv"}, false, []VideoEntry{{Codec: "h264"}}, []AudioEntry{{Codec: "aac", MaxChannels: 2}})
	if d := decided(Input{Caps: caps, Source: Source{Container: "matroska,webm", Video: interlaced(anamorphic(video())), Audio: audio("aac", 2), DefaultAudio: true}}); d.Delivery != DeliverFile {
		t.Fatalf("no limits: %+v", d)
	}
	// The entry that gets furthest gives the reason: here the 4.1 entry
	// fails only its level, after the other fails its profile.
	caps = alike([]string{"mkv"}, false, []VideoEntry{{Codec: "h264", Profiles: []string{"Main"}}, {Codec: "h264", MaxLevel: 4.1}},
		[]AudioEntry{{Codec: "aac", MaxChannels: 2}})
	if d := decided(Input{Caps: caps, Source: Source{Container: "matroska,webm", Video: video(), Audio: audio("aac", 2), DefaultAudio: true}}); d.Unmet == nil || d.Unmet.Field != "maxLevel" {
		t.Fatalf("the furthest entry: %+v", d.Unmet)
	}
}

func hevcAs(codec media.Codec) *SourceTrack {
	v := hevc10()
	v.Codec, v.Profile = codec, ""
	return v
}

func interlaced(v *SourceTrack) *SourceTrack { v.Interlaced = true; return v }
func anamorphic(v *SourceTrack) *SourceTrack { v.Anamorphic = true; return v }

// TestAudioLimits checks an audio entry's limits.
func TestAudioLimits(t *testing.T) {
	truehd := func() *SourceTrack {
		a := audio("truehd", 8)
		a.SampleRate, a.BitDepth, a.Profile = 96_000, 24, "Dolby TrueHD + Dolby Atmos"
		return a
	}
	for _, c := range []struct {
		entry AudioEntry
		why   Reason
		unmet Unmet
	}{
		{AudioEntry{MaxChannels: 6}, ReasonAudioChannelsUnsupported, Unmet{"maxChannels", "6", "8"}},
		{AudioEntry{MaxChannels: 8, MaxSampleRate: 48_000}, ReasonAudioSampleRateUnsupported, Unmet{"maxSampleRate", "48000", "96000"}},
		{AudioEntry{MaxChannels: 8, MaxBitDepth: 16}, ReasonAudioBitDepthUnsupported, Unmet{"maxBitDepth", "16", "24"}},
		{AudioEntry{MaxChannels: 8, Profiles: []string{"Dolby TrueHD"}}, ReasonAudioProfileUnsupported, Unmet{"profiles", "Dolby TrueHD", "Dolby TrueHD + Dolby Atmos"}},
		{AudioEntry{MaxChannels: 8, Containers: []string{"mp4"}}, ReasonContainerUnsupported, Unmet{"containers", "mp4", "mkv"}},
	} {
		e := c.entry
		e.Codec = "truehd"
		caps := withAudio(alike([]string{"mkv", "mp4"}, true, []VideoEntry{{Codec: "h264"}}, []AudioEntry{{Codec: "aac", MaxChannels: 6}}), "aac", 6)
		caps.Audio = append(caps.Audio, AudioEntry{Codec: e.Codec, Delivery: []Delivery{DeliverFile}, Containers: e.Containers, MaxChannels: e.MaxChannels,
			MaxSampleRate: e.MaxSampleRate, MaxBitDepth: e.MaxBitDepth, Profiles: e.Profiles})
		d := decided(Input{Caps: caps, Source: Source{Container: "matroska,webm", Video: h264(), Audio: truehd(), DefaultAudio: true, Remuxable: true}})
		if v, _ := d.Selected(media.Video); d.Delivery != DeliverStream || v.Reason != c.why || v.Unmet == nil || *v.Unmet != c.unmet {
			t.Errorf("%+v: video %+v (%+v), want %s %+v", c.entry, v, v.Unmet, c.why, c.unmet)
		}
	}
}

// TestContainerScoping checks that an entry's containers limit it to
// those containers of a file, and don't apply to a stream.
func TestContainerScoping(t *testing.T) {
	both := []Delivery{DeliverFile, DeliverStream}
	caps := Capabilities{
		File: &FileCaps{Containers: []string{"mp4", "mkv"}},
		Video: []VideoEntry{
			{Codec: "h264", Delivery: both},
			{Codec: "hevc", Delivery: both, Containers: []string{"mp4", "mov"}, MaxBitDepth: 10, HDR: []media.DynamicRange{media.HDR10}},
		},
		Audio:     []AudioEntry{{Codec: "aac", Delivery: both, MaxChannels: 2}},
		Subtitles: []SubtitleEntry{vttStream},
	}
	mp4 := decided(Input{Caps: caps, Source: Source{Container: "mov,mp4,m4a,3gp,3g2,mj2", Video: hevc10(), Audio: audio("aac", 2), DefaultAudio: true, Remuxable: true}})
	if mp4.Delivery != DeliverFile {
		t.Fatalf("HEVC in an MP4: %+v", mp4)
	}
	mkv := decided(Input{Caps: caps, Source: Source{Container: "matroska,webm", Video: hevc10(), Audio: audio("aac", 2), DefaultAudio: true, Remuxable: true}})
	v, _ := mkv.Selected(media.Video)
	if mkv.Delivery != DeliverStream || v.Method() != DirectStream || v.Reason != ReasonContainerUnsupported ||
		*v.Unmet != (Unmet{Field: "containers", Limit: "mp4,mov", Actual: "mkv"}) {
		t.Fatalf("HEVC in an MKV, copied into the stream: %+v (%+v)", v, v.Unmet)
	}
	mov := decided(Input{Caps: caps, Source: Source{Container: "mov,mp4,m4a,3gp,3g2,mj2", Ext: ".mov", Video: hevc10(), Audio: audio("aac", 2), DefaultAudio: true}})
	if mov.Delivery != DeliverFile {
		t.Fatalf("HEVC in a MOV: %+v", mov)
	}
	// The file's own containers apply too.
	ts := decided(Input{Caps: caps, Source: Source{Container: "mpegts", Ext: ".m2ts", Video: h264(), Audio: audio("aac", 2), DefaultAudio: true, Remuxable: true}})
	if v, _ := ts.Selected(media.Video); v.Reason != ReasonContainerUnsupported || *v.Unmet != (Unmet{Field: "containers", Limit: "mp4,mkv", Actual: "m2ts"}) {
		t.Fatalf("an M2TS file: %+v (%+v)", v, v.Unmet)
	}
}

// TestSubtitleContainerScoping checks that a subtitle entry listing
// containers reads its codec only from a file in one of them, and that a
// track it doesn't read names the containers.
func TestSubtitleContainerScoping(t *testing.T) {
	caps := noExternal(androidTV)
	caps.Subtitles = append(cloned(caps.Subtitles), SubtitleEntry{Codec: pgsCodec, Delivery: []Delivery{DeliverFile}, Containers: []string{"mkv"}},
		SubtitleEntry{Codec: "subrip", Delivery: []Delivery{DeliverFile}, Containers: []string{"mkv"}})
	pgs, srt := sub(2, pgsCodec), sub(3, "subrip")
	src := func(container string, selected *SubtitleSource) Source {
		return Source{Container: container, Video: h264(), Audio: audio("aac", 2), DefaultAudio: true, Remuxable: true,
			Subtitle: selected, Subtitles: []SubtitleSource{*pgs, *srt}}
	}
	mkv := decided(Input{Caps: caps, Source: src("matroska,webm", pgs)})
	if s, _ := mkv.Selected(media.Subtitle); mkv.Delivery != DeliverFile || s.Delivery != DeliverFile || choiceIn(mkv, srt.Track).Delivery != DeliverFile {
		t.Fatalf("subtitles in an MKV: %+v", mkv.Tracks)
	}

	const mp4 = "mov,mp4,m4a,3gp,3g2,mj2"
	want := Unmet{Field: "containers", Limit: "mkv", Actual: "mp4"}
	d := decided(Input{Caps: caps, Source: src(mp4, nil)})
	if c := choiceIn(d, pgs.Track); d.Delivery != DeliverFile || c.Delivery != "" || c.Reason != ReasonContainerUnsupported || c.Unmet == nil || *c.Unmet != want {
		t.Fatalf("PGS in an MP4 during its play: %+v (%+v)", c, c.Unmet)
	}
	// The selected SubRip goes in the stream, and the video copied
	// beside it names the containers.
	d = decided(Input{Caps: caps, Source: src(mp4, srt)})
	s, _ := d.Selected(media.Subtitle)
	v, _ := d.Selected(media.Video)
	if d.Delivery != DeliverStream || s.Delivery != DeliverStream || v.Method() != DirectStream || v.Reason != ReasonContainerUnsupported ||
		v.Unmet == nil || *v.Unmet != want {
		t.Fatalf("SubRip selected in an MP4: %+v, video %+v (%+v)", s, v, v.Unmet)
	}
	// Without containers, any container the client plays qualifies.
	caps.Subtitles = readsSubtitles(noExternal(androidTV)).Subtitles
	if d = decided(Input{Caps: caps, Source: src(mp4, srt)}); d.Delivery != DeliverFile {
		t.Fatalf("SubRip in an MP4 for an entry without containers: %+v", d.Tracks)
	}
}

// TestExternalTextCodec checks that an external text subtitle comes as
// WebVTT, whatever its own codec, and an ASS one as ASS where the client
// draws it. The stream carries WebVTT too.
func TestExternalTextCodec(t *testing.T) {
	srt, vtt, movText, ass := prepared(sub(2, "subrip")), prepared(sub(3, "webvtt")), prepared(sub(4, "mov_text")), external(sub(5, "ass"))
	all := []SubtitleSource{*srt, *vtt, *movText, *ass}
	type codecs map[*SubtitleSource]media.Codec
	for _, c := range []struct {
		name  string
		files []SubtitleEntry
		want  codecs
	}{
		{"WebVTT", []SubtitleEntry{vttFile}, codecs{srt: webVTT, vtt: webVTT, movText: webVTT, ass: webVTT}},
		{"WebVTT and ASS", []SubtitleEntry{vttFile, assFile}, codecs{srt: webVTT, vtt: webVTT, movText: webVTT, ass: "ass"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			caps := noExternal(androidTV)
			caps.Subtitles = append(cloned(caps.Subtitles), c.files...)
			src := func(selected *SubtitleSource) Source {
				return Source{Container: "matroska,webm", Video: h264(), Audio: audio("aac", 2), DefaultAudio: true, Remuxable: true,
					Subtitle: selected, Subtitles: all}
			}
			for s, want := range c.want {
				d := decided(Input{Caps: caps, Source: src(s)})
				got, _ := d.Selected(media.Subtitle)
				if d.Delivery != DeliverFile || got.Delivery != DeliverExternal || got.Codec != want {
					t.Errorf("track %d (%s): %+v, want %s", s.Index, s.Codec, got, want)
				}
				if o := choiceIn(decided(Input{Caps: caps, Source: src(nil)}), s.Track); o.Delivery != DeliverExternal || o.Codec != want {
					t.Errorf("track %d (%s) not selected: %+v, want %s", s.Index, s.Codec, o, want)
				}
			}
			d := decided(Input{Caps: caps, Settings: Settings{DisableDirectPlay: true}, Source: src(srt)})
			for _, tr := range d.Tracks {
				if tr.Delivery == DeliverStream && tr.Type == media.Subtitle && tr.Codec != webVTT {
					t.Errorf("track %d in the stream as %s", tr.Index, tr.Codec)
				}
			}
			if s, _ := d.Selected(media.Subtitle); d.Delivery != DeliverStream || s.Delivery != DeliverStream {
				t.Errorf("SubRip selected in a stream: %+v", s)
			}
		})
	}
}

// TestTrackSelection checks that a client that selects audio tracks
// inside a file keeps the file for another track, and that its other
// audio tracks play there without a new play when it plays them.
func TestTrackSelection(t *testing.T) {
	jpn := &SourceTrack{ID: tid(3), Track: media.Track{Index: 2, Kind: media.Audio, Codec: "aac", Channels: 2}}
	dts := &SourceTrack{ID: tid(4), Track: media.Track{Index: 3, Kind: media.Audio, Codec: "dts", Channels: 6}}
	src := func(selected *SourceTrack, def bool) Source {
		return Source{Container: "matroska,webm", Video: h264(), Audio: selected, DefaultAudio: def, Remuxable: true,
			Tracks: []SourceTrack{*h264(), *audio("aac", 2), *jpn, *dts}}
	}
	selecting := androidTV
	selecting.File = &FileCaps{Containers: androidTV.File.Containers, TrackSelection: true}

	d := decided(Input{Caps: selecting, Source: src(jpn, false)})
	if a, _ := d.Selected(media.Audio); d.Delivery != DeliverFile || a.Track != jpn.ID || a.Index != 2 {
		t.Fatalf("another track, selected in the file: %+v", d)
	}
	want := map[*SourceTrack]TrackDecision{
		audio("aac", 2): {Delivery: DeliverFile, Codec: "aac", Index: 1},
		dts:             {Reason: ReasonAudioCodecUnsupported, Index: 3},
	}
	for track, w := range want {
		got := choiceIn(d, track.ID)
		if got.Delivery != w.Delivery || got.Codec != w.Codec || got.Index != w.Index || got.Reason != w.Reason {
			t.Errorf("track %d: %+v, want %+v", track.Index, got, w)
		}
	}

	d = decided(Input{Caps: androidTV, Source: src(jpn, false)})
	if v, _ := d.Selected(media.Video); d.Delivery != DeliverStream || v.Reason != ReasonTrackSelectionUnsupported {
		t.Fatalf("another track, without track selection: %+v", d)
	}
	d = decided(Input{Caps: androidTV, Source: src(audio("aac", 2), true)})
	if c := choiceIn(d, jpn.ID); d.Delivery != DeliverFile || c.Delivery != "" || c.Reason != ReasonTrackSelectionUnsupported {
		t.Fatalf("the default track without track selection, then another: %+v", c)
	}
	// In a stream, only the selected track is carried.
	d = decided(Input{Caps: selecting, Settings: Settings{DisableDirectPlay: true}, Source: src(audio("aac", 2), true)})
	if c := choiceIn(d, jpn.ID); d.Delivery != DeliverStream || c.Delivery != "" {
		t.Fatalf("another track in a stream: %+v", c)
	}
}

// choiceIn is how a decision shows a track.
func choiceIn(d Decision, track [16]byte) TrackDecision {
	for _, t := range d.Tracks {
		if t.Track == track {
			return t
		}
	}
	return TrackDecision{}
}

// TestBurnedInMakesEveryChoiceANewPlay checks that while a subtitle is
// burned in, every other subtitle choice, off included, has no delivery.
func TestBurnedInMakesEveryChoiceANewPlay(t *testing.T) {
	pgs, srt, ext := sub(2, pgsCodec), prepared(sub(3, "subrip")), external(sub(9, "subrip"))
	for _, c := range []struct {
		name string
		in   Input
	}{
		{"PGS the client can't draw", Input{Caps: browser}},
		{"burn subtitles turned on", Input{Caps: androidTV, Settings: Settings{BurnSubtitles: true}}},
		{"a fallback", Input{Caps: androidTV, Fallback: true}},
	} {
		t.Run(c.name, func(t *testing.T) {
			in := c.in
			in.Source = Source{Container: "matroska,webm", Video: h264(), Audio: audio("aac", 2), DefaultAudio: true, Remuxable: true,
				Subtitle: prepared(pgs), Subtitles: []SubtitleSource{*pgs, *srt, *ext}}
			d := decided(in)
			if s, _ := d.Selected(media.Subtitle); s.Delivery != DeliverBurn {
				t.Fatalf("not burned in: %+v", s)
			}
			for _, tr := range d.Tracks {
				if tr.Type == media.Subtitle && !tr.Selected && tr.Delivery != "" {
					t.Errorf("%v has delivery %s while a subtitle is burned in", tr.Track, tr.Delivery)
				}
			}
			if off := choiceIn(d, [16]byte{}); off.Type != media.Subtitle || off.Delivery != "" {
				t.Errorf("off: %+v", off)
			}
		})
	}
}

// TestOtherSubtitles checks how the subtitle tracks other than the
// selected one show during a play.
func TestOtherSubtitles(t *testing.T) {
	srt, srtReady, ass, assReady := sub(2, "subrip"), prepared(sub(3, "subrip")), sub(4, "ass"), prepared(sub(5, "ass"))
	pgs, pgsReady, ext, extSup := sub(6, pgsCodec), prepared(sub(7, pgsCodec)), external(sub(8, "subrip")), external(sub(9, pgsCodec))
	all := []SubtitleSource{*srt, *srtReady, *ass, *assReady, *pgs, *pgsReady, *ext, *extSup}
	type shows map[*SubtitleSource]Delivery
	for _, c := range []struct {
		name string
		in   Input
		want shows
	}{
		{"a stream: every text track a rendition but the prepared ASS the client draws",
			Input{Caps: browser, Settings: Settings{DisableDirectPlay: true}},
			shows{srt: DeliverStream, srtReady: DeliverStream, ass: DeliverStream, assReady: DeliverExternal, pgs: "", pgsReady: "", ext: DeliverStream, extSup: ""}},
		{"a stream on AVPlayer: prepared ASS is burned in by a new play",
			Input{Caps: safari, Settings: Settings{DisableDirectPlay: true}},
			shows{srt: DeliverStream, srtReady: DeliverStream, ass: DeliverStream, assReady: "", pgs: "", pgsReady: "", ext: DeliverStream, extSup: ""}},
		{"a file: ready subtitles external",
			Input{Caps: androidTV},
			shows{srt: "", srtReady: DeliverExternal, ass: "", assReady: "", pgs: "", pgsReady: DeliverExternal, ext: DeliverExternal, extSup: DeliverExternal}},
		{"a file on a client that reads subtitles from it",
			Input{Caps: readsSubtitles(androidTV)},
			shows{srt: DeliverFile, srtReady: DeliverFile, ass: DeliverFile, assReady: DeliverFile, pgs: DeliverFile, pgsReady: DeliverFile, ext: DeliverExternal, extSup: DeliverExternal}},
	} {
		t.Run(c.name, func(t *testing.T) {
			in := c.in
			in.Source = Source{Container: "matroska,webm", Video: h264(), Audio: audio("aac", 2), DefaultAudio: true, Remuxable: true, Subtitles: all}
			d := decided(in)
			for s, want := range c.want {
				if got := choiceIn(d, s.Track); got.Delivery != want {
					t.Errorf("track %d (%s): %+v, want %q", s.Index, s.Codec, got, want)
				}
			}
			if off := choiceIn(d, [16]byte{}); !off.Selected || off.Delivery != DeliverNone {
				t.Errorf("off: %+v", off)
			}
			for _, tr := range d.Tracks {
				if tr.Delivery == DeliverFile && tr.Type == media.Subtitle && tr.Index < 0 {
					t.Errorf("a subtitle file in the file: %+v", tr)
				}
			}
		})
	}
}

// readsSubtitles adds subtitle entries for what a player reads from the
// file itself.
func readsSubtitles(c Capabilities) Capabilities {
	c.Subtitles = append(cloned(c.Subtitles), SubtitleEntry{Codec: "subrip", Delivery: []Delivery{DeliverFile}},
		SubtitleEntry{Codec: "ass", Delivery: []Delivery{DeliverFile}}, SubtitleEntry{Codec: pgsCodec, Delivery: []Delivery{DeliverFile}})
	return c
}

func cloned[T any](xs []T) []T { return append([]T(nil), xs...) }

// TestSelectedSubtitleFromTheFile checks that a subtitle the client reads
// from the file keeps a direct play, though it isn't extracted.
func TestSelectedSubtitleFromTheFile(t *testing.T) {
	for _, codec := range []media.Codec{"subrip", "ass", pgsCodec} {
		d := decided(Input{Caps: readsSubtitles(androidTV), Source: Source{Container: "matroska,webm", Video: h264(), Audio: audio("aac", 2),
			DefaultAudio: true, Remuxable: true, Subtitle: sub(2, codec)}})
		if s, _ := d.Selected(media.Subtitle); d.Delivery != DeliverFile || s.Delivery != DeliverFile || s.Index != 2 || s.Codec != codec {
			t.Errorf("%s: %+v", codec, s)
		}
	}
}

// TestDecisionGolden pins every choice of a version with many tracks, as
// several clients see it: its delivery, codec, HDR, reason, failed limit
// and transcode.
func TestDecisionGolden(t *testing.T) {
	dv := dolbyVision(8, 1, media.HDR10, false)
	jpn := SourceTrack{ID: tid(3), Track: media.Track{Index: 2, Kind: media.Audio, Codec: "truehd", Channels: 8}}
	srt, ass, pgs, ext := sub(4, "subrip"), prepared(sub(5, "ass")), prepared(sub(6, pgsCodec)), external(sub(9, "subrip"))
	source := func(subtitle *SubtitleSource) Source {
		return Source{Container: "matroska,webm", Ext: ".mkv", Bitrate: 30_000_000, Video: dv, Audio: audio("eac3", 6), DefaultAudio: true, Remuxable: true,
			Tracks: []SourceTrack{*dv, *audio("eac3", 6), jpn}, Subtitle: subtitle, Subtitles: []SubtitleSource{*srt, *ass, *pgs, *ext}}
	}
	selecting := readsSubtitles(withAudio(androidTV, "truehd", 8))
	selecting.File = &FileCaps{Containers: androidTV.File.Containers, TrackSelection: true}
	var b strings.Builder
	for _, c := range []struct {
		name string
		in   Input
	}{
		{"browser, SubRip", Input{Caps: browser, Encoders: cpuOnly, Source: source(srt)}},
		{"AVPlayer, PGS", Input{Caps: safari, Source: source(pgs)}},
		{"AVPlayer in picture-in-picture, the external SubRip", Input{Caps: noExternal(safari), Source: source(ext)}},
		{"Android TV, styled ASS, reading from the file", Input{Caps: selecting, Source: source(ass)}},
		{"Android TV, off", Input{Caps: androidTV, Source: source(nil)}},
		{"Android TV, 4 Mbps remote", Input{Caps: androidTV, Limit: Limit{MaxBitrate: 4_000_000, Reason: ReasonRemoteQualityLimit}, Source: source(nil)}},
	} {
		d := decided(c.in)
		fmt.Fprintf(&b, "# %s: %s, %s\n", c.name, d.Delivery, d.Summary())
		for _, t := range d.Tracks {
			fmt.Fprintf(&b, "%-8s %-3d", t.Type, t.Track[15])
			if t.Selected {
				b.WriteString(" selected")
			}
			for _, f := range [][2]string{{"delivery", string(t.Delivery)}, {"codec", string(t.Codec)}, {"hdr", t.HDR}, {"reason", string(t.Reason)}} {
				if f[1] != "" {
					fmt.Fprintf(&b, " %s=%s", f[0], f[1])
				}
			}
			if t.Unmet != nil {
				fmt.Fprintf(&b, " limit=%s:%s:%s", t.Unmet.Field, t.Unmet.Limit, t.Unmet.Actual)
			}
			if t.Transcoded() && t.Video != nil {
				fmt.Fprintf(&b, " transcode=%dx%d@%d", t.Video.Width, t.Video.Height, t.Video.Bitrate)
			}
			if t.Transcoded() && t.Audio != nil {
				fmt.Fprintf(&b, " transcode=%dch@%d", t.Audio.Channels, t.Audio.Bitrate)
			}
			b.WriteString("\n")
		}
	}
	golden := filepath.Join("testdata", "decisions.golden")
	if *update {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(golden, []byte(b.String()), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatal(err)
	}
	if b.String() != string(want) {
		t.Fatalf("decisions (go test -update to accept):\n%s\nwant:\n%s", b.String(), want)
	}
}
