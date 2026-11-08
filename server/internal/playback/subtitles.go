package playback

import (
	"cmp"
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"golang.org/x/text/language"

	"github.com/mmcdole/kivo/internal/catalog"
	"github.com/mmcdole/kivo/internal/id"
	"github.com/mmcdole/kivo/internal/media"
	"github.com/mmcdole/kivo/internal/media/subtitle"
	"github.com/mmcdole/kivo/internal/transcode"
)

// ExternalSubtitle is a subtitle track delivered external, as its own
// file. The client fetches it by Name from the play's or the download's
// subtitles.
type ExternalSubtitle struct {
	Track  id.ID
	Format media.SubtitleFormat // VTT, ASS or PGS
	Name   string               // the media name to fetch the file by
	// Fonts are the SHA-256 hashes of the fonts an ASS track uses. The
	// api package's fonts route serves them.
	Fonts []string
	// Ready means the file exists. Fetching a file that isn't ready
	// returns ErrNotReady. A download's tracks are listed before they're
	// extracted.
	Ready bool
}

// formatOf is a subtitle codec's format, or empty if the codec has none.
func formatOf(codec media.Codec) media.SubtitleFormat {
	f, _ := codec.SubtitleFormat()
	return f
}

// subtitleFileCodec converts a subtitle file's format, which is its
// extension, into the codec name the probe would use.
func subtitleFileCodec(x catalog.SubtitleFile) media.Codec {
	return media.Codec(catalog.SubtitleCodec(x.Format))
}

// subtitleSource finds a subtitle track among a file's embedded tracks and
// its subtitle files, which sit next to it or were downloaded for it.
func subtitleSource(f catalog.File, track id.ID) (SubtitleSource, bool) {
	subs := subtitleSources(f)
	i := slices.IndexFunc(subs, func(s SubtitleSource) bool { return s.Track == track })
	if i < 0 {
		return SubtitleSource{}, false
	}
	return subs[i], true
}

// subtitleSources lists a file's subtitle tracks, embedded and then its
// subtitle files, as Decide sees them before they're looked up as prepared.
func subtitleSources(f catalog.File) []SubtitleSource {
	var out []SubtitleSource
	for _, t := range f.Tracks {
		if media.TrackKind(t.Type) == media.Subtitle {
			out = append(out, SubtitleSource{Track: t.ID, Index: t.Index, Codec: media.Codec(t.Codec)})
		}
	}
	for _, x := range f.SubtitleFiles {
		out = append(out, SubtitleSource{Track: x.ID, Index: -1, Codec: subtitleFileCodec(x), OwnFile: true})
	}
	return out
}

// embeddedSubtitle returns a file's embedded subtitle track with this ID.
func embeddedSubtitle(f catalog.File, track id.ID) (catalog.Track, bool) {
	i := slices.IndexFunc(f.Tracks, func(t catalog.Track) bool { return media.TrackKind(t.Type) == media.Subtitle && t.ID == track })
	if i < 0 {
		return catalog.Track{}, false
	}
	return f.Tracks[i], true
}

// findSubtitleFile returns the subtitle file with this ID, and its path.
func (s *Service) findSubtitleFile(f catalog.File, track id.ID) (catalog.SubtitleFile, string, bool) {
	for _, x := range f.SubtitleFiles {
		if x.ID != track {
			continue
		}
		if x.Downloaded {
			return x, s.deps.Subtitles.DownloadedPath(x), true
		}
		return x, filepath.Join(f.FolderPath, filepath.FromSlash(x.RelPath)), true
	}
	return catalog.SubtitleFile{}, "", false
}

// styled is a file's styled-subtitle preparation. A play reads it when it
// starts and keeps it for its whole length.
type styled struct {
	ok    bool
	fonts []string // the fonts' paths in the store
}

// styledOf reads a file's styled-subtitle preparation, if it has an
// embedded ASS track.
func (s *Service) styledOf(ctx context.Context, f catalog.File) styled {
	if !slices.ContainsFunc(f.Tracks, func(t catalog.Track) bool {
		return media.TrackKind(t.Type) == media.Subtitle && formatOf(media.Codec(t.Codec)) == media.ASS
	}) {
		return styled{}
	}
	fonts, ok, err := s.deps.Subtitles.Styled(ctx, f.ID)
	if err != nil {
		slog.WarnContext(ctx, "playback: styled subtitles", "file", f.ID, "err", err)
		return styled{}
	}
	return styled{ok: ok, fonts: fonts}
}

// prepared reports whether an embedded track is ready to be drawn by the
// client or burned in. An ASS track is ready once the file is styled and
// the track is extracted. Another text track is ready once it's extracted
// as WebVTT. A PGS track is ready once it's extracted.
func (s *Service) prepared(ctx context.Context, f catalog.File, sub SubtitleSource, st styled) bool {
	if sub.OwnFile {
		return false
	}
	switch formatOf(sub.Codec) {
	case media.PGS:
		_, ok := s.extracted(ctx, f.ID, sub.Index, media.PGS)
		return ok
	case media.ASS:
		_, ok := s.extracted(ctx, f.ID, sub.Index, media.ASS)
		return st.ok && ok
	case media.SRT, media.VTT:
		_, ok := s.extracted(ctx, f.ID, sub.Index, media.VTT)
		return ok
	}
	return false
}

// burnIn returns the subtitle a video transcode draws. An image track is
// read from the source by its stream index. A text track is always read
// from a file, never the source: either the subtitle file, or the
// extraction the subtitles package prepared. A prepared ASS track comes
// with the paths of the fonts it uses.
func (s *Service) burnIn(ctx context.Context, c *choice, t TrackDecision) (*media.SubtitleRef, []string, error) {
	f := c.files[0]
	if x, p, ok := s.findSubtitleFile(f, t.Track); ok {
		if !formatOf(subtitleFileCodec(x)).Text() {
			return nil, nil, &RefusedError{Err: ErrNotPlayable, Reason: RefuseSubtitleUnplayable} // an image subtitle file can't be burned in
		}
		return &media.SubtitleRef{Path: p}, nil, nil
	}
	src := c.input.Source.Subtitle
	if !formatOf(src.Codec).Text() {
		ref := &media.SubtitleRef{Track: t.Index}
		if st, ok := embeddedSubtitle(f, t.Track); ok {
			mt := probed(st)
			ref.Width, ref.Height = mt.Width, mt.Height
		}
		return ref, nil, nil
	}
	format := media.VTT
	if formatOf(src.Codec) == media.ASS {
		format = media.ASS
	}
	p, ok := s.extracted(ctx, f.ID, t.Index, format)
	if !ok {
		// Decide only burns in a text track once it's prepared, so the
		// extraction was removed after the decision.
		return nil, nil, fmt.Errorf("the prepared subtitle track %d of %s is gone", t.Index, f.ID)
	}
	if format == media.ASS {
		return &media.SubtitleRef{Path: p}, c.styled.fonts, nil
	}
	return &media.SubtitleRef{Path: p}, nil, nil
}

// extracted returns the path of an embedded track's extraction, if it's
// made.
func (s *Service) extracted(ctx context.Context, file id.ID, track int, format media.SubtitleFormat) (string, bool) {
	p, ok, err := s.deps.Subtitles.Subtitle(ctx, file, track, format)
	if err != nil {
		slog.WarnContext(ctx, "playback: subtitle extraction", "file", file, "track", track, "err", err)
		return "", false
	}
	return p, ok
}

// externalSubtitles lists the subtitle tracks a choice delivers external,
// as their own files, with whether each file is there to fetch. An embedded ASS track
// lists the fonts it uses.
func (s *Service) externalSubtitles(ctx context.Context, c *choice) []ExternalSubtitle {
	f := c.files[0]
	var out []ExternalSubtitle
	for _, t := range c.decision.Tracks {
		if t.Type != media.Subtitle || t.Delivery != DeliverExternal {
			continue
		}
		format := formatOf(t.Codec)
		d := ExternalSubtitle{Track: t.Track, Format: format, Name: subtitleName(t.Track, format), Ready: true}
		if _, embedded := embeddedSubtitle(f, t.Track); embedded {
			_, d.Ready = s.extracted(ctx, f.ID, t.Index, format)
			if format == media.ASS {
				d.Ready = d.Ready && c.styled.ok
				d.Fonts = fontHashes(c.styled.fonts)
			}
		}
		out = append(out, d)
	}
	return out
}

// textTracks lists the text tracks a stream or a conversion carries, in
// order. A stream carries every subtitle its decision streams, each as a
// rendition, with the selected one the default. A conversion carries its
// streamed subtitle first, then, for a client that draws no WebVTT file,
// every other text track it doesn't get as a file or burned
// in.
func (s *Service) textTracks(ctx context.Context, c *choice) []transcode.TextTrack {
	var out []transcode.TextTrack
	for _, t := range c.decision.Tracks {
		if t.Type == media.Subtitle && t.Delivery == DeliverStream {
			tt := s.textTrack(ctx, c, t.Track)
			tt.Default = t.Selected
			out = append(out, tt)
		}
	}
	if c.decision.Delivery != DeliverConversion || c.input.Caps.drawsTextFiles() {
		return out
	}
	for _, t := range c.decision.Tracks {
		if t.Type != media.Subtitle || t.Track.IsZero() || t.Delivery == DeliverExternal || t.Delivery == DeliverStream || t.Selected {
			continue
		}
		if src, ok := subtitleSource(c.files[0], t.Track); ok && formatOf(src.Codec).Text() {
			out = append(out, s.textTrack(ctx, c, t.Track))
		}
	}
	return out
}

// fontHashes returns the SHA-256 hashes of fonts in the store, taken from
// their paths.
func fontHashes(paths []string) []string {
	out := make([]string, len(paths))
	for i, p := range paths {
		out[i], _, _ = strings.Cut(filepath.Base(p), ".")
	}
	return out
}

// textTrack describes a text track for a stream's playlist or a
// conversion's file. A subtitle file is read whole, and so, for a stream of
// a single file, is an embedded track's extraction once it's made. Any
// other embedded track is read along with the video.
func (s *Service) textTrack(ctx context.Context, c *choice, track id.ID) transcode.TextTrack {
	f := c.files[0]
	out := transcode.TextTrack{ID: track.String(), Track: -1, Title: "Subtitles"}
	if x, p, ok := s.findSubtitleFile(f, track); ok {
		out.Path, out.Format = p, formatOf(subtitleFileCodec(x))
		out.Title, out.Language = cmp.Or(x.Title, x.Language, out.Title), x.Language
		out.Forced, out.SDH = x.Forced, x.HearingImpaired
		return out
	}
	t, ok := embeddedSubtitle(f, track)
	if !ok {
		return out
	}
	out.Track = t.Index
	if tag, err := language.Parse(t.Language); err == nil && t.Language != "" && tag != language.Und {
		out.Language, out.Title = tag.String(), tag.String()
	}
	out.Title = cmp.Or(t.Title, out.Title)
	out.Forced, out.SDH = t.Forced, t.HearingImpaired
	if c.decision.Delivery == DeliverStream && len(c.files) == 1 {
		if p, ok := s.extracted(ctx, f.ID, t.Index, media.VTT); ok {
			out.Path, out.Format = p, media.VTT
		}
	}
	return out
}

// subtitleName is a subtitle's media name: "<track ID>.vtt",
// "<track ID>.ass" or "<track ID>.sup".
func subtitleName(track id.ID, format media.SubtitleFormat) string {
	return track.String() + "." + format.Ext()
}

// servedFormats are the subtitle formats a play serves, by file extension,
// with their content types.
var servedFormats = map[string]struct {
	format media.SubtitleFormat
	typ    string
}{
	media.VTT.Ext(): {media.VTT, "text/vtt; charset=utf-8"},
	media.ASS.Ext(): {media.ASS, "text/x-ssa; charset=utf-8"},
	media.PGS.Ext(): {media.PGS, "application/octet-stream"},
}

// Subtitle returns a subtitle track of a play by the name in its delivery.
// A text subtitle file is converted to WebVTT, and any other subtitle file
// is served unchanged. An embedded track is served as the subtitles
// package extracted it, or returns ErrNotReady if the extraction isn't
// made.
func (s *Service) Subtitle(ctx context.Context, sid id.ID, name string) (Media, error) {
	pl, err := s.authorize(ctx, sid)
	if err != nil {
		return Media{}, err
	}
	return s.subtitleFile(ctx, pl.files[0], name)
}

// subtitleFile returns a subtitle track of a file by its media name.
func (s *Service) subtitleFile(ctx context.Context, f catalog.File, name string) (Media, error) {
	n, ext, ok := strings.Cut(name, ".")
	track, err := id.Parse(n)
	if !ok || err != nil {
		return Media{}, ErrNotFound
	}
	served, ok := servedFormats[ext]
	if !ok {
		return Media{}, ErrNotFound
	}
	format, typ := served.format, served.typ
	if x, p, ok := s.findSubtitleFile(f, track); ok {
		src := formatOf(subtitleFileCodec(x))
		switch {
		case format == media.VTT && src.Text():
			return convertedSubtitle(p, src, typ)
		case format == media.ASS && src == media.ASS, format == media.PGS && x.Format == "sup":
			return Media{Path: p, Type: typ}, nil
		}
		return Media{}, ErrNotFound
	}
	t, ok := embeddedSubtitle(f, track)
	if !ok {
		return Media{}, ErrNotFound
	}
	p, ok := s.extracted(ctx, f.ID, t.Index, format)
	if !ok {
		return Media{}, ErrNotReady
	}
	return Media{Path: p, Type: typ}, nil
}

// convertedSubtitle reads the text subtitle file at p, in format from,
// and converts it to WebVTT.
func convertedSubtitle(p string, from media.SubtitleFormat, typ string) (Media, error) {
	b, err := os.ReadFile(p)
	if err != nil {
		return Media{}, err
	}
	if b, err = subtitle.WebVTT(b, from); err != nil {
		return Media{}, err
	}
	return Media{Data: b, Type: typ}, nil
}
