package playback

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"slices"

	"github.com/mmcdole/kivo/internal/catalog"
	"github.com/mmcdole/kivo/internal/id"
	"github.com/mmcdole/kivo/internal/media"
	"github.com/mmcdole/kivo/internal/playback/dbq"
	"github.com/mmcdole/kivo/internal/transcode"
	"github.com/mmcdole/kivo/internal/users"
)

// delivered is a download as its device gets it: how it's delivered, its
// parts and its tracks. It's stored with the download as JSON, first as
// planned, then, once the download is ready, as it was made. Its fields
// are only ever added to, each under the JSON name it has, so a stored one
// always reads.
type delivered struct {
	Delivery Delivery         `json:"delivery"`
	Quality  string           `json:"quality"`
	Size     int64            `json:"size"` // an estimate in bytes, shown until the download is ready
	Parts    []deliveredPart  `json:"parts"`
	Tracks   []deliveredTrack `json:"tracks"`
}

// deliveredPart is one of a download's files. A converted part has its
// path, size and hash once it's made.
type deliveredPart struct {
	File   id.ID  `json:"file"` // the version's file it's made from
	Path   string `json:"path,omitempty"`
	Size   int64  `json:"size,omitempty"`
	SHA256 string `json:"sha256,omitempty"`
}

// deliveredTrack is one choice of a download, as a TrackDecision is of a
// play: a track inside its file, at its stream index there, or its own
// file fetched with it.
type deliveredTrack struct {
	Type     media.TrackKind     `json:"type"`
	Track    id.ID               `json:"track,omitzero"`
	Index    int                 `json:"index"`
	Selected bool                `json:"selected,omitempty"`
	Delivery Delivery            `json:"delivery,omitempty"`
	Codec    media.Codec         `json:"codec,omitempty"`
	HDR      string              `json:"hdr,omitempty"`
	Reason   Reason              `json:"reason,omitempty"`
	Unmet    *Unmet              `json:"unmet,omitempty"`
	Made     *deliveredRendition `json:"made,omitempty"`
	Encoder  string              `json:"encoder,omitempty"`
}

// deliveredRendition is what a conversion makes of a video or audio track:
// its codec, which is copy for a track kept as it is, and the size it's
// encoded at.
type deliveredRendition struct {
	Codec    media.Codec `json:"codec"`
	Width    int         `json:"width,omitempty"`
	Height   int         `json:"height,omitempty"`
	Channels int         `json:"channels,omitempty"`
	Bitrate  int         `json:"bitrate,omitempty"`
}

// planDownload chooses a download's version and decides its delivery the
// same way a play does, but for whole files.
func (s *Service) planDownload(ctx context.Context, user id.ID, prefs trackPrefs, r DownloadRequest) (*choice, error) {
	q, ok := DownloadQualityNamed(cmp.Or(r.Quality, DefaultDownloadQuality))
	if !ok {
		return nil, fmt.Errorf("unknown download quality %q", r.Quality)
	}
	sr := StartRequest{Work: r.Work, Version: r.Version, AudioTrack: r.AudioTrack, SubtitleTrack: r.SubtitleTrack, Caps: r.Caps, Settings: r.Settings}
	base := Input{Caps: r.Caps, Settings: r.Settings, Encoders: s.encoders(false), Target: TargetFile, Quality: q}
	c, err := s.choose(ctx, user, sr, prefs, base)
	if err != nil {
		return nil, err
	}
	if c.input.Source.Video == nil {
		c.input.Quality = DownloadQuality{Name: "original"} // a book is always its original
	}
	c.styled = s.styledOf(ctx, c.files[0])
	return &c, nil
}

// planAsked plans a download as its user asks for it now. It returns the
// choice, and what it delivers as planned, to store.
func (s *Service) planAsked(ctx context.Context, user users.User, r DownloadRequest) (*choice, string, error) {
	c, err := s.planDownload(ctx, user.ID, s.prefsFor(ctx, user, r.Work), r)
	if err != nil {
		return nil, "", err
	}
	planned, err := s.deliveredBy(ctx, c, nil)
	return c, planned, err
}

// replan decides a stored download again from what its device asked, for
// its files as they are now. It keeps the download's version and the audio
// and subtitle it chose, whatever the user has come to prefer since.
func (s *Service) replan(ctx context.Context, d dbq.Download) (*choice, error) {
	var r DownloadRequest
	if err := json.Unmarshal([]byte(d.Request), &r); err != nil {
		return nil, err
	}
	var dl delivered
	if err := json.Unmarshal([]byte(d.Delivered), &dl); err != nil {
		return nil, err
	}
	r.Version, r.AudioTrack, r.SubtitleTrack = d.VersionID, dl.chose(media.Audio), cmp.Or(dl.chose(media.Subtitle), &id.ID{})
	return s.planDownload(ctx, d.UserID, trackPrefs{}, r)
}

// chose is the track of a kind a download selected, or nil for none. For
// subtitles, the zero ID is the choice off.
func (dl delivered) chose(kind media.TrackKind) *id.ID {
	for _, t := range dl.Tracks {
		if t.Type == kind && t.Selected {
			return &t.Track
		}
	}
	return nil
}

// deliveredBy is what a choice delivers as a download, as JSON to store:
// as planned, or once converted is made, as made.
func (s *Service) deliveredBy(ctx context.Context, c *choice, converted []transcode.ConvertedFile) (string, error) {
	dl := delivered{Delivery: c.decision.Delivery, Quality: c.input.Quality.Name, Size: estimateBytes(c.input, c.decision, c.files)}
	for i, f := range c.files {
		p := deliveredPart{File: f.ID}
		if i < len(converted) {
			p.Path, p.Size, p.SHA256 = converted[i].Path, converted[i].Size, converted[i].SHA256
		}
		dl.Parts = append(dl.Parts, p)
	}
	for _, t := range s.downloaded(ctx, c) {
		dt := deliveredTrack{
			Type: t.Type, Track: t.Track, Index: t.Index, Selected: t.Selected, Delivery: t.Delivery, Codec: t.Codec, HDR: t.HDR,
			Reason: t.Reason, Unmet: t.Unmet, Encoder: t.Encoder,
		}
		if v := t.Video; v != nil {
			dt.Made = &deliveredRendition{Codec: v.Codec, Width: v.Width, Height: v.Height, Bitrate: v.Bitrate}
		} else if a := t.Audio; a != nil {
			dt.Made = &deliveredRendition{Codec: a.Codec, Channels: a.Channels, Bitrate: a.Bitrate}
		}
		dl.Tracks = append(dl.Tracks, dt)
	}
	b, err := json.Marshal(dl)
	return string(b), err
}

// decision is what a download delivers, as the decision its status shows.
func (dl delivered) decision() Decision {
	d := Decision{Delivery: dl.Delivery, Playable: true}
	for _, t := range dl.Tracks {
		td := TrackDecision{
			Type: t.Type, Track: t.Track, Index: t.Index, Selected: t.Selected, Delivery: t.Delivery, Codec: t.Codec, HDR: t.HDR,
			Reason: t.Reason, Unmet: t.Unmet, Encoder: t.Encoder,
		}
		switch m := t.Made; {
		case m == nil:
		case t.Type == media.Video:
			td.Video = &media.VideoRendition{Codec: m.Codec, Width: m.Width, Height: m.Height, Bitrate: m.Bitrate}
		default:
			td.Audio = &media.AudioRendition{Codec: m.Codec, Channels: m.Channels, Bitrate: m.Bitrate}
		}
		d.Tracks = append(d.Tracks, td)
	}
	return d
}

// kept reads what a stored download delivers, with its parts' files as the
// catalog has them now.
func (s *Service) kept(ctx context.Context, d dbq.Download) (delivered, []catalog.File, error) {
	var dl delivered
	if err := json.Unmarshal([]byte(d.Delivered), &dl); err != nil {
		return dl, nil, err
	}
	if len(dl.Parts) == 0 {
		return dl, nil, fmt.Errorf("download %s has no files", d.ID)
	}
	files := make([]catalog.File, len(dl.Parts))
	for i, p := range dl.Parts {
		f, err := s.deps.Catalog.File(ctx, p.File)
		if err != nil {
			return dl, nil, err
		}
		files[i] = f
	}
	return dl, files, nil
}

// needsPreparing reports whether the server must work on a download
// before it's ready, either to convert it or to extract subtitles.
func (s *Service) needsPreparing(ctx context.Context, c *choice) bool {
	return c.decision.Delivery == DeliverConversion || s.subtitlesToExtract(ctx, c)
}

// subtitlesToExtract reports whether a download needs its file's subtitles
// extracted before it's ready. That's either its chosen subtitle or the
// tracks it delivers as separate files.
func (s *Service) subtitlesToExtract(ctx context.Context, c *choice) bool {
	return s.subtitleToExtract(ctx, c) || s.externalSubtitlesToExtract(ctx, c)
}

// externalSubtitlesToExtract reports whether a download delivers an
// embedded subtitle track external, as a file that isn't ready yet, while the file's
// subtitles aren't extracted. The download waits so every track comes
// down with it. A track still missing after extraction couldn't be
// extracted, and is left out.
func (s *Service) externalSubtitlesToExtract(ctx context.Context, c *choice) bool {
	f := c.files[0]
	waits := slices.ContainsFunc(s.externalSubtitles(ctx, f, c.styled, c.decision.Tracks), func(d ExternalSubtitle) bool {
		_, embedded := embeddedSubtitle(f, d.Track)
		return embedded && !d.Ready
	})
	return waits && !s.extractedAll(ctx, f)
}

// extractedAll reports whether a file's subtitle extraction has run and
// every font it lists is in the store. It uses Styled's answer, which
// covers the whole extraction.
func (s *Service) extractedAll(ctx context.Context, f catalog.File) bool {
	_, ok, err := s.deps.Subtitles.Styled(ctx, f.ID)
	if err != nil {
		slog.WarnContext(ctx, "playback: subtitle extraction", "file", f.ID, "err", err)
	}
	return ok
}

// subtitleToExtract reports whether a download's subtitle isn't prepared
// yet. That applies to an embedded text track kept as a file or burned in,
// and to a PGS track kept as a file. Subtitle files, a track the
// conversion carries in its file or the client reads from the original,
// and a burned-in PGS track are all read from the source, so they need
// nothing.
func (s *Service) subtitleToExtract(ctx context.Context, c *choice) bool {
	sub := c.input.Source.Subtitle
	t, ok := c.decision.Selected(media.Subtitle)
	if sub == nil || !ok || sub.OwnFile || t.Delivery == DeliverStream || t.Delivery == DeliverFile ||
		t.Delivery == DeliverBurn && !formatOf(sub.Codec).Text() {
		return false
	}
	return !s.prepared(ctx, c.files[0], *sub, s.styledOf(ctx, c.files[0]))
}
