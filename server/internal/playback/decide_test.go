package playback

import (
	"math/rand/v2"
	"slices"
	"testing"

	"github.com/mmcdole/kivo/internal/id"
	"github.com/mmcdole/kivo/internal/media"
)

// The subtitle entries a client lists for its own files.
var (
	vttFile = SubtitleEntry{Codec: webVTT, Delivery: []Delivery{DeliverExternal}}
	assFile = SubtitleEntry{Codec: "ass", Delivery: []Delivery{DeliverExternal}}
	pgsFile = SubtitleEntry{Codec: pgsCodec, Delivery: []Delivery{DeliverExternal}}
	// vttStream is the WebVTT rendition every HLS player shows.
	vttStream = SubtitleEntry{Codec: webVTT, Delivery: []Delivery{DeliverStream}}
)

// alike returns capabilities that play the same codecs from a file and in
// a stream, as a native player does. Without hls, there's no stream. A
// player with a stream shows its WebVTT renditions.
func alike(containers []string, hls bool, video []VideoEntry, audio []AudioEntry, subtitles ...SubtitleEntry) Capabilities {
	d := []Delivery{DeliverFile}
	if hls {
		d = append(d, DeliverStream)
		subtitles = append([]SubtitleEntry{vttStream}, subtitles...)
	}
	c := Capabilities{File: &FileCaps{Containers: containers}, Subtitles: subtitles}
	for _, v := range video {
		v.Delivery = d
		c.Video = append(c.Video, v)
	}
	for _, a := range audio {
		a.Delivery = d
		c.Audio = append(c.Audio, a)
	}
	return c
}

// browser is a typical desktop browser: MP4 and WebM, H.264 and VP9, AAC
// and Opus, HLS through MSE, WebVTT and ASS files.
var browser = alike([]string{"mp4", "webm"}, true,
	[]VideoEntry{{Codec: "h264", Profiles: []string{"High", "Main", "Constrained Baseline"}, MaxBitDepth: 8}, {Codec: "vp9", MaxBitDepth: 8}},
	[]AudioEntry{{Codec: "aac", MaxChannels: 6}, {Codec: "opus", MaxChannels: 2}, {Codec: "mp3", MaxChannels: 2}},
	vttFile, assFile)

// allDolbyVision lists every Dolby Vision profile.
var allDolbyVision = []DolbyVisionEntry{{Profile: 5}, {Profile: 7}, {Profile: 8}, {Profile: 10}}

// safari is AVPlayer: HLS, HEVC Main 10 with HDR and Dolby Vision, AC-3 and
// E-AC-3 but not DTS, and only WebVTT files.
var safari = alike([]string{"mp4"}, true,
	[]VideoEntry{{Codec: "h264", MaxBitDepth: 8}, {Codec: "hevc", MaxBitDepth: 10, HDR: []media.DynamicRange{media.HDR10, media.HLG},
		DolbyVision: allDolbyVision, DoviFallback: true}},
	[]AudioEntry{{Codec: "aac", MaxChannels: 6}, {Codec: "ac3", MaxChannels: 6}, {Codec: "eac3", MaxChannels: 6}},
	vttFile)

// assTV plays Matroska directly and draws ASS itself.
var assTV = alike([]string{"mkv"}, true, []VideoEntry{{Codec: "h264", MaxBitDepth: 8}}, []AudioEntry{{Codec: "aac", MaxChannels: 6}}, vttFile, assFile)

// noHLS takes a client's stream away.
func noHLS(c Capabilities) Capabilities {
	out := Capabilities{File: c.File, Subtitles: slices.DeleteFunc(slices.Clone(c.Subtitles), func(e SubtitleEntry) bool { return slices.Equal(e.Delivery, vttStream.Delivery) })}
	for _, v := range c.Video {
		v.Delivery = []Delivery{DeliverFile}
		out.Video = append(out.Video, v)
	}
	for _, a := range c.Audio {
		a.Delivery = []Delivery{DeliverFile}
		out.Audio = append(out.Audio, a)
	}
	return out
}

// stereoPhone is an iPhone on its own speakers, which report two channels.
var stereoPhone = alike([]string{"mp4"}, true, []VideoEntry{safari.Video[0], safari.Video[1]},
	[]AudioEntry{{Codec: "aac", MaxChannels: 2}, {Codec: "ac3", MaxChannels: 2}, {Codec: "eac3", MaxChannels: 2}},
	vttFile, assFile)

// castReceiver is a Cast device on a TV that accepts Dolby audio. E-AC-3
// and AC-3 pass through from an MP4 file, while its HLS pipeline accepts
// only AAC.
var castReceiver = Capabilities{
	File: &FileCaps{Containers: []string{"mp4", "webm"}},
	Video: []VideoEntry{
		{Codec: "h264", Delivery: []Delivery{DeliverFile, DeliverStream}, MaxBitDepth: 8},
		{Codec: "hevc", Delivery: []Delivery{DeliverFile, DeliverStream}, MaxBitDepth: 10},
	},
	Audio: []AudioEntry{
		{Codec: "aac", Delivery: []Delivery{DeliverFile, DeliverStream}, MaxChannels: 6},
		{Codec: "ac3", Delivery: []Delivery{DeliverFile}, MaxChannels: 6},
		{Codec: "eac3", Delivery: []Delivery{DeliverFile}, MaxChannels: 6},
	},
	Subtitles: []SubtitleEntry{vttStream, vttFile},
}

// noExternal takes away the subtitle files a client draws, leaving what it
// shows in a stream.
func noExternal(c Capabilities) Capabilities {
	c.Subtitles = slices.DeleteFunc(slices.Clone(c.Subtitles), func(e SubtitleEntry) bool { return slices.Contains(e.Delivery, DeliverExternal) })
	return c
}

// withAudio adds an audio entry for a file and a stream, as a client lists
// a codec it passes through.
func withAudio(c Capabilities, codec media.Codec, channels int) Capabilities {
	c.Audio = append(slices.Clone(c.Audio), AudioEntry{Codec: codec, Delivery: []Delivery{DeliverFile, DeliverStream}, MaxChannels: channels})
	return c
}

func TestDownloadQualities(t *testing.T) {
	want := []DownloadQuality{
		{Name: "original"},
		{Name: "high", Height: 1080, Bitrate: 8_000_000},
		{Name: "standard", Height: 1080, Bitrate: 3_000_000, Fidelity: media.FidelityStandard, Stereo: true},
	}
	if !slices.Equal(DownloadQualities, want) {
		t.Fatalf("got %+v", DownloadQualities)
	}
	if q, ok := DownloadQualityNamed(DefaultDownloadQuality); !ok || q != want[1] {
		t.Fatalf("the default: %+v", q)
	}
}

// Migration (downloads): remove after every client has updated past this release.
func TestLegacyDownloadQualities(t *testing.T) {
	for name, want := range map[string]string{"1080p-high": "high", "1080p": "high", "720p": "standard", "480p": "standard"} {
		if q, ok := DownloadQualityNamed(name); !ok || q.Name != want {
			t.Errorf("%s: %+v, want %s", name, q, want)
		}
	}
}

// The download qualities by name.
var qHigh, qStandard = quality("high"), quality("standard")

func quality(name string) DownloadQuality {
	q, ok := DownloadQualityNamed(name)
	if !ok {
		panic("no download quality " + name)
	}
	return q
}

// androidTV decodes much more, including HEVC Main 10 HDR and passthrough
// audio to a receiver.
var androidTV = alike([]string{"mp4", "mkv", "webm"}, true,
	[]VideoEntry{{Codec: "h264", MaxBitDepth: 8}, {Codec: "hevc", MaxBitDepth: 10, HDR: []media.DynamicRange{media.HDR10, media.HLG}, DoviFallback: true}},
	[]AudioEntry{{Codec: "aac", MaxChannels: 8}, {Codec: "ac3", MaxChannels: 6}, {Codec: "eac3", MaxChannels: 8}},
	vttFile, pgsFile)

// tid is a test track's catalog ID.
func tid(n byte) id.ID { return id.ID{15: n} }

func h264() *SourceTrack {
	return &SourceTrack{ID: tid(1), Track: media.Track{Index: 0, Kind: media.Video, Codec: "h264", Profile: "High", Width: 1920, Height: 1080, BitDepth: 8}}
}
func hevc10() *SourceTrack {
	return &SourceTrack{ID: tid(1), Track: media.Track{Index: 0, Kind: media.Video, Codec: "hevc", Profile: "Main 10", Width: 3840, Height: 2160,
		BitDepth: 10, Range: media.HDR10}}
}
func audio(codec string, ch int) *SourceTrack {
	return &SourceTrack{ID: tid(2), Track: media.Track{Index: 1, Kind: media.Audio, Codec: media.Codec(codec), Channels: ch, Default: true}}
}

// sub is an embedded subtitle track.
func sub(index int, codec media.Codec) *SubtitleSource {
	return &SubtitleSource{Track: tid(byte(10 + index)), Index: index, Codec: codec}
}

// prepared marks a subtitle track's extraction as made.
func prepared(s *SubtitleSource) *SubtitleSource { s.Prepared = true; return s }

// external makes a subtitle track a file next to the video.
func external(s *SubtitleSource) *SubtitleSource { s.OwnFile, s.Index = true, -1; return s }

// decided completes a test's source as the service builds one, with the
// selected tracks among the version's tracks, and decides it.
func decided(in Input) Decision {
	src := &in.Source
	for _, t := range []*SourceTrack{src.Video, src.Audio} {
		if t != nil && !slices.ContainsFunc(src.Tracks, func(s SourceTrack) bool { return s.ID == t.ID }) {
			src.Tracks = append(src.Tracks, *t)
		}
	}
	if s := src.Subtitle; s != nil && !slices.ContainsFunc(src.Subtitles, func(x SubtitleSource) bool { return x.Track == s.Track }) {
		src.Subtitles = append(src.Subtitles, *s)
	}
	return Decide(in)
}

type want struct {
	delivery           Delivery
	video, audio       Method
	sub                Delivery
	videoWhy, audioWhy Reason
	playable           bool
}

func TestDecide(t *testing.T) {
	mkv := "matroska,webm"
	mp4 := "mov,mp4,m4a,3gp,3g2,mj2"
	for _, c := range []struct {
		name string
		in   Input
		want want
	}{
		{"mp4 H.264 AAC in a browser: direct play",
			Input{Caps: browser, Source: Source{Container: mp4, Video: h264(), Audio: audio("aac", 2), DefaultAudio: true, Remuxable: true}},
			want{delivery: DeliverFile, video: DirectPlay, audio: DirectPlay, playable: true}},
		{"mkv in a browser: remux for the container",
			Input{Caps: browser, Source: Source{Container: mkv, Video: h264(), Audio: audio("aac", 2), DefaultAudio: true, Remuxable: true}},
			want{delivery: DeliverStream, video: DirectStream, audio: DirectStream, videoWhy: ReasonContainerUnsupported, audioWhy: ReasonContainerUnsupported, playable: true}},
		{"DTS in a browser: video copied, audio to AAC",
			Input{Caps: browser, Source: Source{Container: mkv, Video: h264(), Audio: audio("dts", 6), DefaultAudio: true, Remuxable: true}},
			want{delivery: DeliverStream, video: DirectStream, audio: Transcode, videoWhy: ReasonAudioCodecUnsupported, audioWhy: ReasonAudioCodecUnsupported, playable: true}},
		{"7.1 AAC to a 5.1 browser: audio downmixed",
			Input{Caps: browser, Source: Source{Container: mp4, Video: h264(), Audio: audio("aac", 8), DefaultAudio: true, Remuxable: true}},
			want{delivery: DeliverStream, video: DirectStream, audio: Transcode, videoWhy: ReasonAudioChannelsUnsupported, audioWhy: ReasonAudioChannelsUnsupported, playable: true}},
		{"HEVC Main 10 HDR in a browser: needs a video transcode",
			Input{Caps: browser, Source: Source{Container: mkv, Video: hevc10(), Audio: audio("aac", 2), DefaultAudio: true, Remuxable: true}},
			want{delivery: DeliverStream, video: Transcode, audio: DirectStream, videoWhy: ReasonVideoCodecUnsupported, audioWhy: ReasonVideoCodecUnsupported, playable: true}},
		{"HEVC Main 10 HDR on Android TV: direct play",
			Input{Caps: androidTV, Source: Source{Container: mkv, Video: hevc10(), Audio: audio("eac3", 6), DefaultAudio: true, Remuxable: true}},
			want{delivery: DeliverFile, video: DirectPlay, audio: DirectPlay, playable: true}},
		{"E-AC-3 in an MP4 on a Cast receiver: direct play, passed through",
			Input{Caps: castReceiver, Source: Source{Container: mp4, Video: h264(), Audio: audio("eac3", 6), DefaultAudio: true, Remuxable: true}},
			want{delivery: DeliverFile, video: DirectPlay, audio: DirectPlay, playable: true}},
		{"E-AC-3 in an MKV on a Cast receiver: video copied, audio to AAC for its HLS",
			Input{Caps: castReceiver, Source: Source{Container: mkv, Video: h264(), Audio: audio("eac3", 6), DefaultAudio: true, Remuxable: true}},
			want{delivery: DeliverStream, video: DirectStream, audio: Transcode, videoWhy: ReasonContainerUnsupported, audioWhy: ReasonStreamCodecUnsupported, playable: true}},
		{"TrueHD passed through to a receiver",
			Input{Caps: withAudio(androidTV, "truehd", 8), Source: Source{Container: mkv, Video: h264(), Audio: audio("truehd", 8), DefaultAudio: true, Remuxable: true}},
			want{delivery: DeliverFile, video: DirectPlay, audio: DirectPlay, playable: true}},
		{"TrueHD without passthrough can't go in HLS",
			Input{Caps: androidTV, Source: Source{Container: mkv, Video: h264(), Audio: audio("truehd", 8), DefaultAudio: true, Remuxable: true}},
			want{delivery: DeliverStream, video: DirectStream, audio: Transcode, videoWhy: ReasonAudioCodecUnsupported, audioWhy: ReasonAudioCodecUnsupported, playable: true}},
		{"PGS on a client that can't draw it: burned in",
			Input{Caps: browser, Source: Source{Container: mp4, Video: h264(), Audio: audio("aac", 2), DefaultAudio: true, Remuxable: true, Subtitle: sub(2, "hdmv_pgs_subtitle")}},
			want{delivery: DeliverStream, video: Transcode, audio: DirectStream, sub: DeliverBurn, videoWhy: ReasonSubtitleCodecUnsupported, audioWhy: ReasonSubtitleCodecUnsupported, playable: true}},
		{"prepared PGS on Android TV: external",
			Input{Caps: androidTV, Source: Source{Container: mkv, Video: h264(), Audio: audio("aac", 2), DefaultAudio: true, Remuxable: true, Subtitle: prepared(sub(2, "hdmv_pgs_subtitle"))}},
			want{delivery: DeliverFile, video: DirectPlay, audio: DirectPlay, sub: DeliverExternal, playable: true}},
		{"PGS not extracted on Android TV: burned in",
			Input{Caps: androidTV, Source: Source{Container: mkv, Video: h264(), Audio: audio("aac", 2), DefaultAudio: true, Remuxable: true, Subtitle: sub(2, "hdmv_pgs_subtitle")}},
			want{delivery: DeliverStream, video: Transcode, audio: DirectStream, sub: DeliverBurn, videoWhy: ReasonSubtitlePreparing, audioWhy: ReasonSubtitlePreparing, playable: true}},
		{"embedded SubRip in a remux: in the stream",
			Input{Caps: browser, Source: Source{Container: mkv, Video: h264(), Audio: audio("aac", 2), DefaultAudio: true, Remuxable: true, Subtitle: sub(2, "subrip")}},
			want{delivery: DeliverStream, video: DirectStream, audio: DirectStream, sub: DeliverStream, videoWhy: ReasonContainerUnsupported, audioWhy: ReasonContainerUnsupported, playable: true}},
		{"extracted embedded SubRip in direct play: external",
			Input{Caps: androidTV, Source: Source{Container: mkv, Video: h264(), Audio: audio("aac", 2), DefaultAudio: true, Remuxable: true, Subtitle: prepared(sub(2, "subrip"))}},
			want{delivery: DeliverFile, video: DirectPlay, audio: DirectPlay, sub: DeliverExternal, playable: true}},
		{"embedded SubRip not extracted, where the file would play: a remux carries it",
			Input{Caps: androidTV, Source: Source{Container: mkv, Video: h264(), Audio: audio("aac", 2), DefaultAudio: true, Remuxable: true, Subtitle: sub(2, "subrip")}},
			want{delivery: DeliverStream, video: DirectStream, audio: DirectStream, sub: DeliverStream, videoWhy: ReasonSubtitlePreparing, audioWhy: ReasonSubtitlePreparing, playable: true}},
		{"a SubRip file next to the video in a remux: in the stream",
			Input{Caps: browser, Source: Source{Container: mkv, Video: h264(), Audio: audio("aac", 2), DefaultAudio: true, Remuxable: true, Subtitle: external(sub(4, "subrip"))}},
			want{delivery: DeliverStream, video: DirectStream, audio: DirectStream, sub: DeliverStream, videoWhy: ReasonContainerUnsupported, audioWhy: ReasonContainerUnsupported, playable: true}},
		{"prepared ASS in a remux, drawn by the client: styled, external",
			Input{Caps: browser, Source: Source{Container: mkv, Video: h264(), Audio: audio("aac", 2), DefaultAudio: true, Remuxable: true, Subtitle: prepared(sub(2, "ass"))}},
			want{delivery: DeliverStream, video: DirectStream, audio: DirectStream, sub: DeliverExternal, videoWhy: ReasonContainerUnsupported, audioWhy: ReasonContainerUnsupported, playable: true}},
		{"ASS not prepared in a remux: plain in the stream",
			Input{Caps: browser, Source: Source{Container: mkv, Video: h264(), Audio: audio("aac", 2), DefaultAudio: true, Remuxable: true, Subtitle: sub(2, "ass")}},
			want{delivery: DeliverStream, video: DirectStream, audio: DirectStream, sub: DeliverStream, videoWhy: ReasonContainerUnsupported, audioWhy: ReasonContainerUnsupported, playable: true}},
		{"prepared ASS in a direct play, drawn by the client: styled, external",
			Input{Caps: assTV, Source: Source{Container: mkv, Video: h264(), Audio: audio("aac", 2), DefaultAudio: true, Remuxable: true, Subtitle: prepared(sub(2, "ass"))}},
			want{delivery: DeliverFile, video: DirectPlay, audio: DirectPlay, sub: DeliverExternal, playable: true}},
		{"ASS not prepared in what would be a direct play: a remux carries it plain",
			Input{Caps: assTV, Source: Source{Container: mkv, Video: h264(), Audio: audio("aac", 2), DefaultAudio: true, Remuxable: true, Subtitle: sub(2, "ass")}},
			want{delivery: DeliverStream, video: DirectStream, audio: DirectStream, sub: DeliverStream, videoWhy: ReasonSubtitlePreparing, audioWhy: ReasonSubtitlePreparing, playable: true}},
		{"ASS not prepared on a client without HLS: a direct play, the extraction as a file",
			Input{Caps: noHLS(assTV), Source: Source{Container: mkv, Video: h264(), Audio: audio("aac", 2), DefaultAudio: true, Remuxable: true, Subtitle: sub(2, "ass")}},
			want{delivery: DeliverFile, video: DirectPlay, audio: DirectPlay, sub: DeliverExternal, playable: true}},
		{"prepared ASS on AVPlayer, which draws only WebVTT: burned in",
			Input{Caps: safari, Source: Source{Container: mkv, Video: h264(), Audio: audio("aac", 2), DefaultAudio: true, Remuxable: true, Subtitle: prepared(sub(2, "ass"))}},
			want{delivery: DeliverStream, video: Transcode, audio: DirectStream, sub: DeliverBurn, videoWhy: ReasonSubtitleCodecUnsupported, audioWhy: ReasonSubtitleCodecUnsupported, playable: true}},
		{"ASS not prepared on AVPlayer: plain in the stream",
			Input{Caps: safari, Source: Source{Container: mkv, Video: h264(), Audio: audio("aac", 2), DefaultAudio: true, Remuxable: true, Subtitle: sub(2, "ass")}},
			want{delivery: DeliverStream, video: DirectStream, audio: DirectStream, sub: DeliverStream, videoWhy: ReasonContainerUnsupported, audioWhy: ReasonContainerUnsupported, playable: true}},
		{"an ASS file next to the video on AVPlayer: WebVTT in the stream, needing no preparation",
			Input{Caps: safari, Source: Source{Container: mkv, Video: h264(), Audio: audio("aac", 2), DefaultAudio: true, Remuxable: true, Subtitle: external(sub(-1, "ass"))}},
			want{delivery: DeliverStream, video: DirectStream, audio: DirectStream, sub: DeliverStream, videoWhy: ReasonContainerUnsupported, audioWhy: ReasonContainerUnsupported, playable: true}},
		{"a downloaded SubRip on the iPhone app, which draws no subtitle files: in the stream",
			Input{Caps: noExternal(safari), Source: Source{Container: mkv, Video: h264(), Audio: audio("dts", 6), DefaultAudio: true, Remuxable: true, Subtitle: external(sub(-1, "subrip"))}},
			want{delivery: DeliverStream, video: DirectStream, audio: Transcode, sub: DeliverStream, videoWhy: ReasonAudioCodecUnsupported, audioWhy: ReasonAudioCodecUnsupported, playable: true}},
		{"a SubRip file next to a video the iPhone app would play: a remux carries it",
			Input{Caps: noExternal(safari), Source: Source{Container: mp4, Video: h264(), Audio: audio("aac", 2), DefaultAudio: true, Remuxable: true, Subtitle: external(sub(-1, "subrip"))}},
			want{delivery: DeliverStream, video: DirectStream, audio: DirectStream, sub: DeliverStream, videoWhy: ReasonSubtitleCodecUnsupported, audioWhy: ReasonSubtitleCodecUnsupported, playable: true}},
		{"extracted embedded SubRip where the iPhone app would play the file: a remux carries it",
			Input{Caps: noExternal(safari), Source: Source{Container: mp4, Video: h264(), Audio: audio("aac", 2), DefaultAudio: true, Remuxable: true, Subtitle: prepared(sub(2, "subrip"))}},
			want{delivery: DeliverStream, video: DirectStream, audio: DirectStream, sub: DeliverStream, videoWhy: ReasonSubtitleCodecUnsupported, audioWhy: ReasonSubtitleCodecUnsupported, playable: true}},
		{"embedded SubRip burned in on request, not extracted yet: in the stream",
			Input{Caps: androidTV, Settings: Settings{BurnSubtitles: true}, Source: Source{Container: mkv, Video: h264(), Audio: audio("aac", 2), DefaultAudio: true, Remuxable: true, Subtitle: sub(2, "subrip")}},
			want{delivery: DeliverStream, video: DirectStream, audio: DirectStream, sub: DeliverStream, videoWhy: ReasonSubtitlePreparing, audioWhy: ReasonSubtitlePreparing, playable: true}},
		{"embedded SubRip burned in on request once extracted",
			Input{Caps: androidTV, Settings: Settings{BurnSubtitles: true}, Source: Source{Container: mkv, Video: h264(), Audio: audio("aac", 2), DefaultAudio: true, Remuxable: true, Subtitle: prepared(sub(2, "subrip"))}},
			want{delivery: DeliverStream, video: Transcode, audio: DirectStream, sub: DeliverBurn, videoWhy: ReasonBurnSubtitlesEnabled, audioWhy: ReasonBurnSubtitlesEnabled, playable: true}},
		{"a long-GOP source with no index can't remux",
			Input{Caps: browser, Source: Source{Container: mkv, Video: h264(), Audio: audio("aac", 2), DefaultAudio: true, Remuxable: false}},
			want{delivery: DeliverStream, video: Transcode, audio: DirectStream, videoWhy: ReasonKeyframeIndexMissing, audioWhy: ReasonContainerUnsupported, playable: true}},
		{"a remote cap below the source's bitrate",
			Input{Caps: browser, Limit: Limit{MaxBitrate: 4_000_000, Reason: ReasonRemoteQualityLimit}, Source: Source{Container: mp4, Bitrate: 20_000_000, Video: h264(), Audio: audio("aac", 2), DefaultAudio: true, Remuxable: true}},
			want{delivery: DeliverStream, video: Transcode, audio: DirectStream, videoWhy: ReasonRemoteQualityLimit, audioWhy: ReasonRemoteQualityLimit, playable: true}},
		{"a second audio track: a plain file plays only the default",
			Input{Caps: browser, Source: Source{Container: mp4, Video: h264(), Audio: audio("aac", 2), DefaultAudio: false, Remuxable: true}},
			want{delivery: DeliverStream, video: DirectStream, audio: DirectStream, videoWhy: ReasonTrackSelectionUnsupported, audioWhy: ReasonTrackSelectionUnsupported, playable: true}},
		{"direct play turned off on the client",
			Input{Caps: browser, Settings: Settings{DisableDirectPlay: true}, Source: Source{Container: mp4, Video: h264(), Audio: audio("aac", 2), DefaultAudio: true, Remuxable: true}},
			want{delivery: DeliverStream, video: DirectStream, audio: DirectStream, videoWhy: ReasonDirectPlayDisabled, audioWhy: ReasonDirectPlayDisabled, playable: true}},
		{"DTS to Apple HLS: audio to AAC",
			Input{Caps: safari, Source: Source{Container: mkv, Video: h264(), Audio: audio("dts", 6), DefaultAudio: true, Remuxable: true}},
			want{delivery: DeliverStream, video: DirectStream, audio: Transcode, videoWhy: ReasonAudioCodecUnsupported, audioWhy: ReasonAudioCodecUnsupported, playable: true}},
		{"PGS on AVPlayer: burned in",
			Input{Caps: safari, Source: Source{Container: mp4, Video: hevc10(), Audio: audio("aac", 2), DefaultAudio: true, Remuxable: true, Subtitle: sub(2, "hdmv_pgs_subtitle")}},
			want{delivery: DeliverStream, video: Transcode, audio: DirectStream, sub: DeliverBurn, videoWhy: ReasonSubtitleCodecUnsupported, audioWhy: ReasonSubtitleCodecUnsupported, playable: true}},
		{"Dolby Vision 7 in mkv on AVPlayer: remuxed",
			Input{Caps: safari, Source: Source{Container: mkv, Video: dv7(), Audio: audio("eac3", 6), DefaultAudio: true, Remuxable: true}},
			want{delivery: DeliverStream, video: DirectStream, audio: DirectStream, videoWhy: ReasonContainerUnsupported, audioWhy: ReasonContainerUnsupported, playable: true}},
		{"HEVC 10-bit without an accelerator: H.264 on the CPU",
			Input{Caps: browser, Encoders: []media.Codec{media.H264}, Source: Source{Container: mkv, Video: hevc10(), Audio: audio("aac", 2), DefaultAudio: true, Remuxable: true}},
			want{delivery: DeliverStream, video: Transcode, audio: DirectStream, videoWhy: ReasonVideoCodecUnsupported, audioWhy: ReasonVideoCodecUnsupported, playable: true}},
		{"a fallback from direct play: everything transcoded",
			Input{Caps: browser, Fallback: true, Source: Source{Container: mp4, Video: h264(), Audio: audio("aac", 2), DefaultAudio: true, Remuxable: true}},
			want{delivery: DeliverStream, video: Transcode, audio: Transcode, videoWhy: ReasonFallback, audioWhy: ReasonFallback, playable: true}},
		{"a fallback keeps a transcoded track's own reason",
			Input{Caps: browser, Fallback: true, Source: Source{Container: mkv, Video: hevc10(), Audio: audio("aac", 2), DefaultAudio: true, Remuxable: true}},
			want{delivery: DeliverStream, video: Transcode, audio: Transcode, videoWhy: ReasonVideoCodecUnsupported, audioWhy: ReasonFallback, playable: true}},
		{"a fallback burns in prepared ASS the client would draw",
			Input{Caps: browser, Fallback: true, Source: Source{Container: mkv, Video: h264(), Audio: audio("aac", 2), DefaultAudio: true, Remuxable: true, Subtitle: prepared(sub(2, "ass"))}},
			want{delivery: DeliverStream, video: Transcode, audio: Transcode, sub: DeliverBurn, videoWhy: ReasonFallback, audioWhy: ReasonFallback, playable: true}},
		{"a fallback with SubRip not extracted yet: in the stream",
			Input{Caps: browser, Fallback: true, Source: Source{Container: mkv, Video: h264(), Audio: audio("aac", 2), DefaultAudio: true, Remuxable: true, Subtitle: sub(2, "subrip")}},
			want{delivery: DeliverStream, video: Transcode, audio: Transcode, sub: DeliverStream, videoWhy: ReasonFallback, audioWhy: ReasonFallback, playable: true}},
		{"a fallback on a client without HLS: not playable",
			Input{Caps: noHLS(assTV), Fallback: true, Source: Source{Container: mkv, Video: h264(), Audio: audio("aac", 2), DefaultAudio: true, Remuxable: true}},
			want{}},
		{"an m4b book in a browser: direct play",
			Input{Caps: browser, Source: Source{Container: mp4, Audio: audio("aac", 2), DefaultAudio: true}},
			want{delivery: DeliverFile, audio: DirectPlay, playable: true}},
		{"an MP3 book on a client that plays MP3 files: direct play",
			Input{Caps: alike([]string{"mp4", "mp3"}, true, nil, browser.Audio), Source: Source{Container: "mp3", Audio: audio("mp3", 2), DefaultAudio: true}},
			want{delivery: DeliverFile, audio: DirectPlay, playable: true}},
		{"audio the client can't play is never transcoded: not playable",
			Input{Caps: browser, Source: Source{Container: mp4, Audio: audio("alac", 2), DefaultAudio: true}},
			want{}},
		{"a fallback from a book's direct play: not playable",
			Input{Caps: browser, Fallback: true, Source: Source{Container: mp4, Audio: audio("aac", 2), DefaultAudio: true}},
			want{}},
		{"no HLS and an mkv: not playable",
			Input{Caps: alike([]string{"mp4"}, false, browser.Video, browser.Audio), Source: Source{Container: mkv, Video: h264(), Audio: audio("aac", 2), DefaultAudio: true}},
			want{}},
		{"a download of an mp4 AVPlayer plays: the original",
			Input{Caps: safari, Target: TargetFile, Source: Source{Container: mp4, Video: hevc10(), Audio: audio("eac3", 6), DefaultAudio: true}},
			want{delivery: DeliverFile, video: DirectPlay, audio: DirectPlay, playable: true}},
		{"a download of an mkv to AVPlayer: remuxed into MP4, with or without a keyframe index",
			Input{Caps: safari, Target: TargetFile, Source: Source{Container: mkv, Video: h264(), Audio: audio("aac", 2), DefaultAudio: true}},
			want{delivery: DeliverConversion, video: DirectStream, audio: DirectStream, videoWhy: ReasonContainerUnsupported, audioWhy: ReasonContainerUnsupported, playable: true}},
		{"a download with DTS to AVPlayer: audio to AAC",
			Input{Caps: safari, Target: TargetFile, Source: Source{Container: mkv, Video: h264(), Audio: audio("dts", 6), DefaultAudio: true}},
			want{delivery: DeliverConversion, video: DirectStream, audio: Transcode, videoWhy: ReasonAudioCodecUnsupported, audioWhy: ReasonAudioCodecUnsupported, playable: true}},
		{"a download with PGS to AVPlayer: burned in",
			Input{Caps: safari, Target: TargetFile, Source: Source{Container: mp4, Video: h264(), Audio: audio("aac", 2), DefaultAudio: true, Subtitle: prepared(sub(2, "hdmv_pgs_subtitle"))}},
			want{delivery: DeliverConversion, video: Transcode, audio: DirectStream, sub: DeliverBurn, videoWhy: ReasonSubtitleCodecUnsupported, audioWhy: ReasonSubtitleCodecUnsupported, playable: true}},
		{"a download with SubRip not extracted yet: the original, the subtitle as a file once it's made",
			Input{Caps: androidTV, Target: TargetFile, Source: Source{Container: mkv, Video: h264(), Audio: audio("aac", 2), DefaultAudio: true, Subtitle: sub(2, "subrip")}},
			want{delivery: DeliverFile, video: DirectPlay, audio: DirectPlay, sub: DeliverExternal, playable: true}},
		{"a download of a book the device can't play: not playable",
			Input{Caps: safari, Target: TargetFile, Source: Source{Container: "flac", Audio: audio("flac", 2), DefaultAudio: true}},
			want{}},
		{"a download to a client that plays no MP4: not playable",
			Input{Caps: assTV, Target: TargetFile, Source: Source{Container: mp4, Video: h264(), Audio: audio("aac", 2), DefaultAudio: true}},
			want{}},
		{"a High download of an mp4 no larger than its ceiling: the original",
			Input{Caps: safari, Target: TargetFile, Quality: qHigh, Source: Source{Container: mp4, Bitrate: 8_100_000, Video: h264(), Audio: audio("aac", 2), DefaultAudio: true}},
			want{delivery: DeliverFile, video: DirectPlay, audio: DirectPlay, playable: true}},
		{"a High download of a larger mp4: converted",
			Input{Caps: safari, Target: TargetFile, Quality: qHigh, Source: Source{Container: mp4, Bitrate: 12_000_000, Video: h264(), Audio: audio("aac", 2), DefaultAudio: true}},
			want{delivery: DeliverConversion, video: Transcode, audio: DirectStream, videoWhy: ReasonDownloadQuality, audioWhy: ReasonDownloadQuality, playable: true}},
		{"a Standard download of an mkv no larger than its ceiling: remuxed",
			Input{Caps: safari, Target: TargetFile, Quality: qStandard, Source: Source{Container: mkv, Bitrate: 1_800_000, Video: h264(), Audio: audio("aac", 2), DefaultAudio: true}},
			want{delivery: DeliverConversion, video: DirectStream, audio: DirectStream, videoWhy: ReasonContainerUnsupported, audioWhy: ReasonContainerUnsupported, playable: true}},
		{"a Standard download of 5.1 under the cap: video copied, audio to stereo",
			Input{Caps: stereoPhone, Target: TargetFile, Quality: qStandard, Source: Source{Container: mp4, Bitrate: 2_000_000, Video: h264(), Audio: audio("eac3", 6), DefaultAudio: true}},
			want{delivery: DeliverConversion, video: DirectStream, audio: Transcode, videoWhy: ReasonAudioChannelsUnsupported, audioWhy: ReasonAudioChannelsUnsupported, playable: true}},
		{"an Original download of a large mp4: the original",
			Input{Caps: safari, Target: TargetFile, Quality: DownloadQuality{Name: "original"}, Source: Source{Container: mp4, Bitrate: 40_000_000, Video: hevc10(), Audio: audio("aac", 2), DefaultAudio: true}},
			want{delivery: DeliverFile, video: DirectPlay, audio: DirectPlay, playable: true}},
		{"a download with SubRip to a device that draws no subtitle files: in the file, remuxed",
			Input{Caps: noExternal(stereoPhone), Target: TargetFile, Quality: qHigh, Source: Source{Container: mp4, Bitrate: 2_000_000, Video: h264(), Audio: audio("aac", 2), DefaultAudio: true, Subtitle: sub(2, "subrip")}},
			want{delivery: DeliverConversion, video: DirectStream, audio: DirectStream, sub: DeliverStream, videoWhy: ReasonSubtitleCodecUnsupported, audioWhy: ReasonSubtitleCodecUnsupported, playable: true}},
		{"a High download keeps 5.1 on a device whose speakers are stereo",
			Input{Caps: stereoPhone, Target: TargetFile, Quality: qHigh, Source: Source{Container: mp4, Bitrate: 2_000_000, Video: h264(), Audio: audio("eac3", 6), DefaultAudio: true}},
			want{delivery: DeliverFile, video: DirectPlay, audio: DirectPlay, playable: true}},
		{"a dialogue boost on a direct play: video copied, audio to AAC",
			Input{Caps: browser, DialogueBoost: true, Source: Source{Container: mp4, Video: h264(), Audio: audio("aac", 2), DefaultAudio: true, Remuxable: true}},
			want{delivery: DeliverStream, video: DirectStream, audio: Transcode, videoWhy: ReasonDialogueBoost, audioWhy: ReasonDialogueBoost, playable: true}},
		{"a dialogue boost on TrueHD passed through: audio to AAC",
			Input{Caps: withAudio(androidTV, "truehd", 8), DialogueBoost: true, Source: Source{Container: mkv, Video: h264(), Audio: audio("truehd", 8), DefaultAudio: true, Remuxable: true}},
			want{delivery: DeliverStream, video: DirectStream, audio: Transcode, videoWhy: ReasonDialogueBoost, audioWhy: ReasonDialogueBoost, playable: true}},
		{"a dialogue boost on audio transcoded anyway keeps its reason",
			Input{Caps: browser, DialogueBoost: true, Source: Source{Container: mkv, Video: h264(), Audio: audio("dts", 6), DefaultAudio: true, Remuxable: true}},
			want{delivery: DeliverStream, video: DirectStream, audio: Transcode, videoWhy: ReasonAudioCodecUnsupported, audioWhy: ReasonAudioCodecUnsupported, playable: true}},
		{"a dialogue boost without a keyframe index: the video encoded too",
			Input{Caps: browser, DialogueBoost: true, Source: Source{Container: mp4, Video: h264(), Audio: audio("aac", 2), DefaultAudio: true}},
			want{delivery: DeliverStream, video: Transcode, audio: Transcode, videoWhy: ReasonKeyframeIndexMissing, audioWhy: ReasonDialogueBoost, playable: true}},
		{"a dialogue boost on a book: ignored",
			Input{Caps: browser, DialogueBoost: true, Source: Source{Container: mp4, Audio: audio("aac", 2), DefaultAudio: true}},
			want{delivery: DeliverFile, audio: DirectPlay, playable: true}},
		{"a dialogue boost on a download: ignored",
			Input{Caps: safari, DialogueBoost: true, Target: TargetFile, Source: Source{Container: mp4, Video: h264(), Audio: audio("eac3", 6), DefaultAudio: true}},
			want{delivery: DeliverFile, video: DirectPlay, audio: DirectPlay, playable: true}},
	} {
		t.Run(c.name, func(t *testing.T) {
			d := decided(c.in)
			got := want{delivery: d.Delivery, playable: d.Playable}
			if v, ok := d.Selected("video"); ok {
				got.video, got.videoWhy = v.Method(), v.Reason
			}
			if a, ok := d.Selected("audio"); ok {
				got.audio, got.audioWhy = a.Method(), a.Reason
			}
			if s, ok := d.Selected("subtitle"); ok {
				got.sub = s.Delivery
			}
			if got != c.want {
				t.Fatalf("got  %+v\nwant %+v", got, c.want)
			}
			for _, tr := range d.Tracks {
				if tr.Selected && (tr.Type == "video" || tr.Type == "audio") && tr.Method() != DirectPlay && tr.Reason == "" {
					t.Fatalf("%s %s has no reason", tr.Type, tr.Method())
				}
			}
		})
	}
}

func dv7() *SourceTrack {
	v := hevc10()
	v.DolbyVisionProfile = 7
	return v
}

func TestRenditions(t *testing.T) {
	mkv := "matroska,webm"
	d := decided(Input{Caps: safari, Source: Source{Container: mkv, Video: dv7(), Audio: audio("eac3", 6), DefaultAudio: true, Remuxable: true}})
	if v, _ := d.Selected("video"); v.Video.Codec != media.Copy || v.Video.HDR != media.StripDolbyVision || v.HDR != "hdr10" {
		t.Fatalf("Dolby Vision 7 copied: %+v as %q", v.Video, v.HDR)
	}
	for _, c := range []struct {
		name string
		in   Input
		want media.VideoRendition
	}{
		{"4K HDR with no accelerator: 1080p H.264, tone-mapped",
			Input{Caps: browser, Encoders: []media.Codec{media.H264}, Source: Source{Container: mkv, Bitrate: 50_000_000, Video: hevc10(), Audio: audio("aac", 2), DefaultAudio: true}},
			media.VideoRendition{Codec: media.H264, Profile: "high", BitDepth: 8, Width: 1920, Height: 1080, Bitrate: 20_000_000, HDR: media.ToneMap}},
		{"HEVC out where an accelerator encodes it and the client plays it",
			Input{Caps: alike(nil, true, []VideoEntry{{Codec: "hevc"}}, browser.Audio), Encoders: []media.Codec{media.H264, media.HEVC},
				Source: Source{Container: mkv, Bitrate: 50_000_000, Video: hevc10(), Audio: audio("aac", 2), DefaultAudio: true}},
			media.VideoRendition{Codec: media.HEVC, Profile: "main", BitDepth: 8, Width: 1920, Height: 1080, Bitrate: 20_000_000, HDR: media.ToneMap}},
		{"a low-bitrate source: no more than half again its bitrate",
			Input{Caps: browser, Source: Source{Container: mkv, Bitrate: 3_000_000, Video: hevc10(), Audio: audio("aac", 2), DefaultAudio: true}},
			media.VideoRendition{Codec: media.H264, Profile: "high", BitDepth: 8, Width: 1920, Height: 1080, Bitrate: 4_500_000, HDR: media.ToneMap}},
		{"2 Mbps: 720p, the cap less the audio",
			Input{Caps: browser, Limit: Limit{MaxBitrate: 2_000_000, Reason: ReasonClientQualityLimit}, Source: Source{Container: mkv, Bitrate: 8_000_000, Video: h264(), Audio: audio("aac", 2), DefaultAudio: true, Remuxable: true}},
			media.VideoRendition{Codec: media.H264, Profile: "high", BitDepth: 8, Width: 1280, Height: 720, Bitrate: 1_872_000}},
		{"1.5 Mbps: 480p",
			Input{Caps: browser, Limit: Limit{MaxBitrate: 1_500_000, Reason: ReasonClientQualityLimit}, Source: Source{Container: mkv, Bitrate: 8_000_000, Video: h264(), Audio: audio("aac", 2), DefaultAudio: true, Remuxable: true}},
			media.VideoRendition{Codec: media.H264, Profile: "high", BitDepth: 8, Width: 852, Height: 480, Bitrate: 1_372_000}},
		{"a client's own bitrate limit caps the transcode",
			Input{Caps: alike(nil, true, []VideoEntry{{Codec: "h264", MaxBitrate: 10_000_000}}, browser.Audio),
				Source: Source{Container: mkv, Bitrate: 50_000_000, Video: hevc10(), Audio: audio("aac", 2), DefaultAudio: true}},
			media.VideoRendition{Codec: media.H264, Profile: "high", BitDepth: 8, Width: 1920, Height: 1080, Bitrate: 10_000_000, HDR: media.ToneMap}},
		{"a High download of 4K HDR: HEVC capped at 8 Mbps where an accelerator encodes it",
			Input{Caps: stereoPhone, Target: TargetFile, Quality: qHigh, Encoders: []media.Codec{media.H264, media.HEVC},
				Source: Source{Container: mkv, Bitrate: 50_000_000, Video: hevc10(), Audio: audio("aac", 2), DefaultAudio: true}},
			media.VideoRendition{Codec: media.HEVC, Profile: "main", BitDepth: 8, Width: 1920, Height: 1080, Bitrate: 8_000_000, HDR: media.ToneMap}},
		{"a Standard download: 1080p capped at 3 Mbps, at Standard's fidelity",
			Input{Caps: stereoPhone, Target: TargetFile, Quality: qStandard, Source: Source{Container: mkv, Bitrate: 8_000_000, Video: h264(), Audio: audio("aac", 2), DefaultAudio: true}},
			media.VideoRendition{Codec: media.H264, Profile: "high", BitDepth: 8, Width: 1920, Height: 1080, Bitrate: 3_000_000, Fidelity: media.FidelityStandard}},
	} {
		t.Run(c.name, func(t *testing.T) {
			v, _ := decided(c.in).Selected("video")
			if v.Video == nil || *v.Video != c.want {
				t.Fatalf("got  %+v\nwant %+v", v.Video, c.want)
			}
		})
	}
}

// TestLadder checks that each rung carries the height a transcode at its
// cap is sent at.
func TestLadder(t *testing.T) {
	want := []Rung{{20_000_000, 1080}, {12_000_000, 1080}, {8_000_000, 1080}, {4_000_000, 720},
		{2_000_000, 720}, {1_500_000, 480}, {720_000, 360}}
	if got := Ladder(); !slices.Equal(got, want) {
		t.Errorf("Ladder() = %v, want %v", got, want)
	}
}

// TestSummary checks that the play's method is the video's, so copied
// video is Direct Stream whatever happens to the audio. Without video,
// it's the audio's method.
func TestSummary(t *testing.T) {
	mkv, mp4 := "matroska,webm", "mov,mp4,m4a,3gp,3g2,mj2"
	for _, c := range []struct {
		name string
		src  Source
		want Method
	}{
		{"everything direct", Source{Container: mp4, Video: h264(), Audio: audio("aac", 2), DefaultAudio: true, Remuxable: true}, DirectPlay},
		{"a remux", Source{Container: mkv, Video: h264(), Audio: audio("aac", 2), DefaultAudio: true, Remuxable: true}, DirectStream},
		{"video copied, audio transcoded", Source{Container: mkv, Video: h264(), Audio: audio("dts", 6), DefaultAudio: true, Remuxable: true}, DirectStream},
		{"video copied, subtitle in the stream", Source{Container: mp4, Video: h264(), Audio: audio("aac", 2), DefaultAudio: true, Remuxable: true,
			Subtitle: sub(2, "subrip")}, DirectStream},
		{"video encoded", Source{Container: mkv, Video: hevc10(), Audio: audio("aac", 2), DefaultAudio: true, Remuxable: true}, Transcode},
		// Audio alone can only play directly (see TestDecide).
		{"audio alone", Source{Container: mp4, Audio: audio("aac", 2), DefaultAudio: true}, DirectPlay},
	} {
		if got := decided(Input{Caps: browser, Source: c.src}).Summary(); got != c.want {
			t.Errorf("%s: %s, want %s", c.name, got, c.want)
		}
	}
}

func TestTranscodedAudioIsAAC(t *testing.T) {
	d := decided(Input{Caps: browser, Source: Source{Container: "matroska,webm", Video: h264(), Audio: audio("dts", 8), DefaultAudio: true, Remuxable: true}})
	a, _ := d.Selected("audio")
	if a.Audio == nil || a.Audio.Codec != media.AAC || a.Audio.Channels != 6 || a.Codec != media.AAC {
		t.Fatalf("audio %+v as %s", a.Audio, a.Codec)
	}
}

// speakers caps every audio entry of c at n channels, the way a client
// reports what its device outputs.
func speakers(c Capabilities, n int) Capabilities {
	c.Audio = slices.Clone(c.Audio)
	for i := range c.Audio {
		c.Audio[i].MaxChannels = min(c.Audio[i].MaxChannels, n)
	}
	return c
}

// TestAudioDownmix checks that audio with more channels than the client
// plays is transcoded down to the client's channel count, with the video
// copied. Audio with fewer channels is never mixed up.
func TestAudioDownmix(t *testing.T) {
	mp4 := "mov,mp4,m4a,3gp,3g2,mj2"
	for _, c := range []struct {
		name     string
		caps     Capabilities
		audio    *SourceTrack
		method   Method
		why      Reason
		channels int // the transcoded audio's channel count
	}{
		{"5.1 AAC to stereo speakers: Direct Stream, mixed down to stereo", speakers(browser, 2), audio("aac", 6), Transcode, ReasonAudioChannelsUnsupported, 2},
		{"5.1 AAC to 5.1 speakers: Direct Play", speakers(browser, 6), audio("aac", 6), DirectPlay, "", 0},
		{"7.1 E-AC-3 to 5.1 speakers: mixed down to 5.1", speakers(safari, 6), audio("eac3", 8), Transcode, ReasonAudioChannelsUnsupported, 6},
		{"5.1 E-AC-3 to stereo speakers: mixed down to stereo", speakers(safari, 2), audio("eac3", 6), Transcode, ReasonAudioChannelsUnsupported, 2},
		{"stereo to stereo speakers: Direct Play", speakers(browser, 2), audio("aac", 2), DirectPlay, "", 0},
		{"mono to stereo speakers: Direct Play", speakers(browser, 2), audio("aac", 1), DirectPlay, "", 0},
		{"mono DTS transcoded stays mono", speakers(browser, 6), audio("dts", 1), Transcode, ReasonAudioCodecUnsupported, 1},
		{"stereo DTS transcoded stays stereo", speakers(browser, 6), audio("dts", 2), Transcode, ReasonAudioCodecUnsupported, 2},
	} {
		t.Run(c.name, func(t *testing.T) {
			d := decided(Input{Caps: c.caps, Source: Source{Container: mp4, Video: h264(), Audio: c.audio, DefaultAudio: true, Remuxable: true}})
			a, _ := d.Selected("audio")
			if a.Method() != c.method || a.Reason != c.why {
				t.Fatalf("audio %s (%s), want %s (%s)", a.Method(), a.Reason, c.method, c.why)
			}
			if c.method == DirectPlay {
				return
			}
			if v, _ := d.Selected("video"); v.Method() != DirectStream || d.Summary() != DirectStream {
				t.Errorf("video %s, play %s: want the video copied, Direct Stream", v.Method(), d.Summary())
			}
			if a.Audio.Codec != media.AAC || a.Audio.Channels != c.channels {
				t.Errorf("audio %+v, want AAC with %d channels", a.Audio, c.channels)
			}
		})
	}
}

var rank = map[Method]int{DirectPlay: 0, DirectStream: 1, Transcode: 2}

// methodRank orders methods so a "lower" decision is a better one.
func methodRank(d Decision) []int {
	if !d.Playable {
		return []int{9, 9}
	}
	out := []int{0, 0}
	if v, ok := d.Selected("video"); ok {
		out[0] = rank[v.Method()]
	}
	if a, ok := d.Selected("audio"); ok {
		out[1] = rank[a.Method()]
	}
	return out
}

func worse(a, b []int) bool { // reports whether a is worse than b on some track
	for i := range a {
		if a[i] > b[i] {
			return true
		}
	}
	return false
}

func randomInput(r *rand.Rand) Input {
	pick := func(xs ...string) string { return xs[r.IntN(len(xs))] }
	v := &SourceTrack{ID: tid(1), Track: media.Track{Index: 0, Kind: media.Video, Codec: media.Codec(pick("h264", "hevc", "vp9", "mpeg2video")),
		BitDepth: []int{8, 10}[r.IntN(2)], Width: 1920, Height: []int{720, 1080, 2160}[r.IntN(3)],
		Range: media.DynamicRange(pick("", "sdr", "hdr10"))}}
	a := &SourceTrack{ID: tid(2), Track: media.Track{Index: 1, Kind: media.Audio, Codec: media.Codec(pick("aac", "ac3", "dts", "truehd", "opus")),
		Channels: []int{2, 6, 8}[r.IntN(3)]}}
	in := Input{
		Source: Source{Container: pick("matroska,webm", "mov,mp4,m4a,3gp,3g2,mj2"), Video: v, Audio: a, DefaultAudio: r.IntN(3) > 0, Remuxable: r.IntN(3) > 0},
	}
	if r.IntN(3) == 0 {
		s := sub(2, media.Codec(pick("subrip", "ass", "hdmv_pgs_subtitle")))
		s.Prepared = r.IntN(2) == 0
		in.Source.Subtitle = s
	}
	// A file and a stream, each with its own random codecs.
	entries := func(d Delivery) {
		for _, v := range []media.Codec{"h264", "hevc", "vp9"} {
			if r.IntN(2) == 0 {
				in.Caps.Video = append(in.Caps.Video, VideoEntry{Codec: v, Delivery: []Delivery{d}, MaxBitDepth: []int{8, 10}[r.IntN(2)]})
			}
		}
		for _, a := range []media.Codec{"aac", "ac3", "dts", "opus"} {
			if r.IntN(2) == 0 {
				in.Caps.Audio = append(in.Caps.Audio, AudioEntry{Codec: a, Delivery: []Delivery{d}, MaxChannels: []int{2, 6, 8}[r.IntN(3)]})
			}
		}
	}
	entries(DeliverFile)
	in.Caps.File = &FileCaps{TrackSelection: r.IntN(2) == 0}
	for _, c := range []string{"mp4", "mkv", "webm"} {
		if r.IntN(2) == 0 {
			in.Caps.File.Containers = append(in.Caps.File.Containers, c)
		}
	}
	if r.IntN(4) > 0 {
		entries(DeliverStream)
		in.Caps.Subtitles = append(in.Caps.Subtitles, vttStream)
	}
	for _, e := range []SubtitleEntry{vttFile, assFile, pgsFile} {
		if r.IntN(2) == 0 {
			in.Caps.Subtitles = append(in.Caps.Subtitles, e)
		}
	}
	return in
}

// streams reports whether a test's client has a stream.
func streams(in Input) bool { return in.Caps.streams() }

func TestAddingACapabilityNeverWorsens(t *testing.T) {
	r := rand.New(rand.NewPCG(1, 2))
	for range 5000 {
		in := randomInput(r)
		before := methodRank(decided(in))
		more := in
		more.Caps = Capabilities{File: &FileCaps{TrackSelection: true,
			Containers: append(slices.Clone(in.Caps.File.Containers), "mp4", "mkv")}}
		more.Caps.Video = append(slices.Clone(in.Caps.Video), VideoEntry{Codec: in.Source.Video.Codec, Delivery: []Delivery{DeliverFile}, MaxBitDepth: 10,
			HDR: []media.DynamicRange{media.HDR10}})
		more.Caps.Audio = append(slices.Clone(in.Caps.Audio), AudioEntry{Codec: in.Source.Audio.Codec, Delivery: []Delivery{DeliverFile}, MaxChannels: 8})
		more.Caps.Subtitles = append(slices.Clone(in.Caps.Subtitles), pgsFile, assFile)
		// HLS lets a play carry a text subtitle in its stream that a direct
		// play would need as a file, prepared and in a format the client
		// draws. That trade is intended.
		if sub := in.Source.Subtitle; streams(in) || sub == nil || !formatOf(sub.Codec).Text() {
			more.Caps.Video = append(more.Caps.Video, VideoEntry{Codec: in.Source.Video.Codec, Delivery: []Delivery{DeliverStream}, MaxBitDepth: 10,
				HDR: []media.DynamicRange{media.HDR10}})
			more.Caps.Audio = append(more.Caps.Audio, AudioEntry{Codec: in.Source.Audio.Codec, Delivery: []Delivery{DeliverStream}, MaxChannels: 8})
			more.Caps.Subtitles = append(more.Caps.Subtitles, vttStream)
		}
		if after := methodRank(decided(more)); worse(after, before) {
			t.Fatalf("more capabilities made it worse: %v → %v for %+v", before, after, in)
		}
	}
}

func TestFallbackTranscodesEverything(t *testing.T) {
	r := rand.New(rand.NewPCG(3, 4))
	for range 5000 {
		in := randomInput(r)
		if !streams(in) {
			in.Caps.Video = append(in.Caps.Video, VideoEntry{Codec: "h264", Delivery: []Delivery{DeliverStream}})
			in.Caps.Subtitles = append(in.Caps.Subtitles, vttStream)
		}
		before := decided(in)
		in.Fallback = true
		d := decided(in)
		if !d.Playable || d.Delivery != DeliverStream || d.Summary() != Transcode {
			t.Fatalf("a fallback: %+v for %+v", d, in)
		}
		for _, tr := range d.Tracks {
			if !tr.Selected || tr.Track.IsZero() {
				continue
			}
			was, _ := before.Selected(tr.Type)
			switch {
			case tr.Type == "subtitle":
				// The subtitle is burned in, or carried in the stream while
				// an embedded text track to burn in is extracted.
				sub := in.Source.Subtitle
				if tr.Delivery != DeliverBurn && (tr.Delivery != DeliverStream || sub.Prepared || sub.Codec == pgsCodec) {
					t.Fatalf("a fallback's subtitle: %+v for %+v", tr, sub)
				}
			case tr.Method() != Transcode:
				t.Fatalf("a fallback's %s: %s", tr.Type, tr.Method())
			case was.Method() != Transcode && tr.Reason != ReasonFallback:
				t.Fatalf("a fallback's %s, %s before: reason %q", tr.Type, was.Method(), tr.Reason)
			case tr.Reason != ReasonFallback && tr.Reason != was.Reason:
				t.Fatalf("a fallback's %s, a transcode before: reason %q, was %q", tr.Type, tr.Reason, was.Reason)
			}
		}
	}
}

// TestAudioTarget checks that an audio transcode is AAC, unless the client
// plays more of the source's channels as Dolby Digital Plus, else Dolby
// Digital, as a TV passing Dolby to a soundbar does.
func TestAudioTarget(t *testing.T) {
	truehd := media.Track{Codec: "truehd", Channels: 8}
	for _, c := range []struct {
		name  string
		audio []AudioEntry
		src   media.Track
		want  media.AudioRendition
	}{
		{"stereo AAC and 5.1 E-AC-3: E-AC-3 5.1", []AudioEntry{{Codec: "aac", MaxChannels: 2}, {Codec: "eac3", MaxChannels: 6}, {Codec: "ac3", MaxChannels: 6}},
			truehd, media.AudioRendition{Codec: media.EAC3, Channels: 6, Bitrate: 640_000, SampleRate: 48_000}},
		{"stereo AAC and 5.1 AC-3: AC-3 5.1", []AudioEntry{{Codec: "aac", MaxChannels: 2}, {Codec: "ac3", MaxChannels: 6}},
			truehd, media.AudioRendition{Codec: media.AC3, Channels: 6, Bitrate: 640_000, SampleRate: 48_000}},
		{"5.1 AAC: AAC", []AudioEntry{{Codec: "aac", MaxChannels: 6}, {Codec: "eac3", MaxChannels: 6}},
			truehd, media.AudioRendition{Codec: media.AAC, Channels: 6, Bitrate: 384_000, SampleRate: 48_000}},
		{"a stereo source stays AAC", []AudioEntry{{Codec: "aac", MaxChannels: 2}, {Codec: "eac3", MaxChannels: 6}},
			media.Track{Codec: "truehd", Channels: 2}, media.AudioRendition{Codec: media.AAC, Channels: 2, Bitrate: 128_000, SampleRate: 48_000}},
	} {
		in := Input{Caps: alike([]string{"mp4"}, true, browser.Video, c.audio)}
		if got := in.audioTarget(c.src); *got != c.want {
			t.Errorf("%s: %+v, want %+v", c.name, *got, c.want)
		}
	}
}
