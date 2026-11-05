package api

import (
	"slices"

	"github.com/mmcdole/kivo/internal/api/internal/gen"
	"github.com/mmcdole/kivo/internal/media"
	"github.com/mmcdole/kivo/internal/playback"
)

// parseSettings reads the client's playback settings. Absent, Direct Play
// is on and subtitles aren't burned in.
func parseSettings(s *gen.PlaybackSettings) playback.Settings {
	if s == nil {
		return playback.Settings{}
	}
	return playback.Settings{DisableDirectPlay: s.DirectPlay != nil && !*s.DirectPlay, BurnSubtitles: deref(s.BurnSubtitles)}
}

// The values the capability enums take. Any other value is skipped, so a
// later client's new value never fails a request. Each list only grants
// playback, so skipping a value can only narrow what's played as is.
var (
	knownDeliveries = []gen.CapabilityDelivery{gen.CapabilityDeliveryFile, gen.CapabilityDeliveryStream, gen.CapabilityDeliveryExternal}
	knownHDR        = map[gen.HdrFormat]media.DynamicRange{
		gen.HdrFormatHdr10: media.HDR10, gen.HdrFormatHdr10Plus: playback.HDR10Plus, gen.HdrFormatHlg: media.HLG,
	}
)

// parseCapabilities reads a client's capabilities. An entry without a
// codec or a known delivery is dropped, and so is an audio entry without
// its channels. A video or audio entry's external
// delivery means nothing, and is left out.
func parseCapabilities(c gen.Capabilities) playback.Capabilities {
	var out playback.Capabilities
	if c.File != nil {
		out.File = &playback.FileCaps{Containers: c.File.Containers, TrackSelection: deref(c.File.TrackSelection)}
	}
	for _, v := range c.Video {
		e, ok := videoEntry(v)
		if ok {
			out.Video = append(out.Video, e)
		}
	}
	for _, a := range c.Audio {
		d, ok := deliveries(a.Delivery, false)
		if !ok || a.Codec == "" || a.MaxChannels <= 0 {
			continue
		}
		out.Audio = append(out.Audio, playback.AudioEntry{
			Codec: media.Codec(a.Codec), Delivery: d, Containers: deref(a.Containers), MaxChannels: a.MaxChannels,
			MaxSampleRate: deref(a.MaxSampleRate), MaxBitDepth: deref(a.MaxBitDepth), Profiles: deref(a.Profiles),
		})
	}
	for _, s := range c.Subtitles {
		d, ok := deliveries(s.Delivery, true)
		if !ok || s.Codec == "" {
			continue
		}
		out.Subtitles = append(out.Subtitles, playback.SubtitleEntry{Codec: media.Codec(s.Codec), Delivery: d, Containers: deref(s.Containers)})
	}
	return out
}

func videoEntry(v gen.VideoEntry) (playback.VideoEntry, bool) {
	d, ok := deliveries(v.Delivery, false)
	if !ok || v.Codec == "" {
		return playback.VideoEntry{}, false
	}
	e := playback.VideoEntry{
		Codec: media.Codec(v.Codec), Delivery: d, Containers: deref(v.Containers), Tags: deref(v.Tags), Profiles: deref(v.Profiles),
		MaxLevel: deref(v.MaxLevel), MaxBitDepth: deref(v.MaxBitDepth), MaxWidth: deref(v.MaxWidth), MaxHeight: deref(v.MaxHeight),
		MaxFramerate: deref(v.MaxFrameRate), MaxBitrate: int(deref(v.MaxBitrateBps)), MaxRefFrames: deref(v.MaxRefFrames),
		Interlaced: v.Interlaced, Anamorphic: v.Anamorphic, DoviFallback: deref(v.DoviFallback),
	}
	for _, h := range deref(v.Hdr) {
		if r, ok := knownHDR[h]; ok {
			e.HDR = append(e.HDR, r)
		}
	}
	for _, dv := range deref(v.DolbyVision) {
		e.DolbyVision = append(e.DolbyVision, playback.DolbyVisionEntry{Profile: dv.Profile, Compat: deref(dv.Compat), EL: dv.El})
	}
	return e, true
}

// deliveries reads an entry's deliveries, skipping any it doesn't know.
// It's false when none is left; external counts only for a subtitle.
func deliveries(in []gen.CapabilityDelivery, subtitle bool) ([]playback.Delivery, bool) {
	var out []playback.Delivery
	for _, d := range in {
		if slices.Contains(knownDeliveries, d) && (d != gen.CapabilityDeliveryExternal || subtitle) {
			out = append(out, playback.Delivery(d))
		}
	}
	return out, len(out) > 0
}
