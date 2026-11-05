package api

import (
	"slices"

	"github.com/mmcdole/kivo/internal/api/internal/gen"
	"github.com/mmcdole/kivo/internal/id"
	"github.com/mmcdole/kivo/internal/playback"
)

// playTracks is every choice of a play or a download on the wire. A
// subtitle delivered external takes its URL and fonts from externals,
// which belong to the play or the download (owner). Without externals, as
// for "Now playing", it has neither.
func playTracks(d playback.Decision, externals []playback.ExternalSubtitle, owner string, of id.ID) []gen.PlayTrack {
	out := []gen.PlayTrack{}
	for _, t := range d.Tracks {
		pt := gen.PlayTrack{
			TrackID: offOr(t.Track), Type: gen.TrackType(t.Type), Delivery: optAs[gen.TrackDelivery](t.Delivery),
			Codec: optAs[string](t.Codec), Hdr: optAs[gen.HdrFormat](t.HDR), Reason: optAs[gen.TrackReason](t.Reason),
			Mismatch: capabilityMismatch(t.Unmet),
		}
		if t.Selected {
			pt.Selected = ptr(true)
		}
		if t.Delivery == playback.DeliverFile && t.Index >= 0 {
			pt.Index = ptr(t.Index)
		}
		pt.Transcode = trackTranscode(t)
		if i := slices.IndexFunc(externals, func(s playback.ExternalSubtitle) bool { return s.Track == t.Track }); i >= 0 && t.Delivery == playback.DeliverExternal {
			pt.URL = ptr(mediaURL(owner, of, "subtitles", externals[i].Name))
			for _, hash := range externals[i].Fonts {
				pt.Fonts = ptr(append(deref(pt.Fonts), fontURL(hash)))
			}
		}
		out = append(out, pt)
	}
	return out
}

// offOr is a track's ID on the wire, or off for the zero ID.
func offOr(track id.ID) string {
	if track.IsZero() {
		return "off"
	}
	return track.String()
}

// trackTranscode is what an encoded track becomes. It's nil for a track
// that isn't encoded.
func trackTranscode(t playback.TrackDecision) *gen.TrackTranscode {
	switch {
	case !t.Transcoded():
		return nil
	case t.Video != nil:
		return &gen.TrackTranscode{
			Width: opt(t.Video.Width), Height: opt(t.Video.Height), BitrateBps: opt(int64(t.Video.Bitrate)), Encoder: opt(t.Encoder),
		}
	}
	return &gen.TrackTranscode{Channels: opt(t.Audio.Channels), BitrateBps: opt(int64(t.Audio.Bitrate))}
}

// capabilityMismatch is a failed capability on the wire.
func capabilityMismatch(u *playback.Unmet) *gen.CapabilityMismatch {
	if u == nil {
		return nil
	}
	return &gen.CapabilityMismatch{Field: u.Field, Declared: opt(u.Limit), Actual: u.Actual}
}
