package playback

import (
	"context"
	"errors"
	"fmt"

	"github.com/mmcdole/kivo/internal/auth"
	"github.com/mmcdole/kivo/internal/catalog"
	"github.com/mmcdole/kivo/internal/id"
	"github.com/mmcdole/kivo/internal/media"
	"github.com/mmcdole/kivo/internal/users"
)

// EstimateRequest asks for an estimate of a work's download, with the
// choices the download would be requested with.
type EstimateRequest struct {
	Work, Version             id.ID  // a zero Version means the version a play would pick
	AudioTrack, SubtitleTrack *id.ID // the same as in DownloadRequest
}

// Estimate is what a work's download would be at each quality, before it's
// asked for, so a client can show the sizes and check they fit.
type Estimate struct {
	Work, Version id.ID
	// Playable is false for a work that can't be downloaded. That's a work
	// the user can't see, a work with no file present, or a work refused
	// with Reason.
	Playable bool
	Reason   Refusal
	Options  []EstimateOption // one per quality, in the download menu's order; a book has only its original
}

// EstimateOption is a download at one quality.
type EstimateOption struct {
	Quality  string
	Size     int64 // in bytes; exact when AsIs, otherwise an estimate from the quality's cap
	AsIs     bool  // the original file is sent unchanged
	Delivery Delivery
}

// Estimate says what downloading each work would produce at each quality,
// on a client with the given capabilities. It decides the same way
// Download does.
func (s *Service) Estimate(ctx context.Context, p auth.Caller, v users.Viewer, caps Capabilities, settings Settings, items []EstimateRequest) ([]Estimate, error) {
	user, err := s.deps.Users.User(ctx, p.User)
	if err != nil {
		return nil, err
	}
	out := make([]Estimate, len(items))
	for i, it := range items {
		out[i] = Estimate{Work: it.Work, Version: it.Version}
		if !user.Downloads {
			out[i].Reason = RefuseDownloadsNotAllowed
			continue
		}
		if ok, err := s.visible(ctx, v, it.Work); err != nil || !ok {
			if err != nil {
				return nil, fmt.Errorf("estimate work %s: %w", it.Work, err)
			}
			continue
		}
		c, err := s.planDownload(ctx, user, DownloadRequest{
			Work: it.Work, Version: it.Version, AudioTrack: it.AudioTrack, SubtitleTrack: it.SubtitleTrack, Caps: caps, Settings: settings,
		})
		if refused, ok := errors.AsType[*RefusedError](err); ok {
			out[i].Reason = refused.Reason
			continue
		}
		if errors.Is(err, ErrNotFound) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("estimate work %s: %w", it.Work, err)
		}
		out[i].Version, out[i].Playable = c.version.ID, true
		qualities := DownloadQualities
		if c.input.Source.Video == nil {
			qualities = []DownloadQuality{{Name: "original"}}
		}
		for _, q := range qualities {
			in := c.input
			in.Quality = q
			d := Decide(in)
			out[i].Options = append(out[i].Options, EstimateOption{
				Quality: q.Name, Size: estimateBytes(in, d, c.files), AsIs: d.Delivery == DeliverFile, Delivery: d.Delivery,
			})
		}
	}
	return out, nil
}

// estimateBytes is a download's size before it's prepared. It's the
// original's size when the file is sent unchanged or its video is copied.
// Otherwise it's the length times the conversion's bitrates: the video's
// cap and the audio's bitrate. A conversion that aims at its quality
// usually comes in under that, and a hard title on an encoder whose
// quality mode has no cap can come in over it.
func estimateBytes(in Input, d Decision, files []catalog.File) int64 {
	var original int64
	for _, f := range files {
		original += f.Size
	}
	length := duration(files)
	v, ok := d.Selected(media.Video)
	if d.Delivery == DeliverFile || !ok || !v.Transcoded() {
		return original
	}
	bitrate := v.Video.Bitrate
	if a, ok := d.Selected(media.Audio); ok {
		if a.Transcoded() {
			bitrate += a.Audio.Bitrate
		} else {
			bitrate += in.audioBitrate()
		}
	}
	return int64(length.Seconds() * float64(bitrate) / 8)
}
