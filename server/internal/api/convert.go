package api

import (
	"context"
	"encoding/json"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/mmcdole/kivo/internal/api/internal/display"
	"github.com/mmcdole/kivo/internal/api/internal/gen"
	"github.com/mmcdole/kivo/internal/auth"
	"github.com/mmcdole/kivo/internal/catalog"
	"github.com/mmcdole/kivo/internal/id"
	"github.com/mmcdole/kivo/internal/markers"
	"github.com/mmcdole/kivo/internal/media"
	"github.com/mmcdole/kivo/internal/query"
	"github.com/mmcdole/kivo/internal/users"
)

func ptr[T any](v T) *T { return &v }

// opt returns a pointer to v, or nil for its zero value. Absent optional
// fields are then omitted, never sent as null or zero.
func opt[T comparable](v T) *T {
	var zero T
	if v == zero {
		return nil
	}
	return &v
}

// each converts every item of xs with f.
func each[T, U any](xs []T, f func(T) U) []U {
	out := make([]U, len(xs))
	for i, x := range xs {
		out[i] = f(x)
	}
	return out
}

func deref[T any](p *T) T {
	var zero T
	if p == nil {
		return zero
	}
	return *p
}

func nonNil[T any](xs []T) []T {
	if xs == nil {
		return []T{}
	}
	return xs
}

// optAs is opt with a conversion between string types. The empty string
// becomes nil.
func optAs[U, T ~string](v T) *U {
	if v == "" {
		return nil
	}
	u := U(v)
	return &u
}

func ms(d time.Duration) int { return int(d.Milliseconds()) }

// duration converts a request's milliseconds to a time.Duration.
func duration(ms int) time.Duration { return time.Duration(ms) * time.Millisecond }

func optTime(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	u := t.UTC()
	return &u
}

func date(s string) *openapi_types.Date {
	t, err := time.Parse(time.DateOnly, s)
	if err != nil {
		return nil
	}
	return &openapi_types.Date{Time: t}
}

// summary is a work's summary on the wire. Fields specific to one kind go
// under that kind.
func summary(c query.WorkSummary) gen.WorkSummary {
	st := userState(c.Userstate)
	w := gen.WorkSummary{
		ID: c.ID, Kind: gen.WorkKind(c.Kind), Title: c.Title, Year: opt(c.Year), DurationMs: opt(ms(c.Duration)),
		AddedAt: optTime(c.Added), MatchState: gen.MatchState(c.MatchState), Available: !c.Unavailable,
		Images: images(c.Images), UserState: &st, ContentRating: opt(c.ContentRating), Path: opt(c.Path),
	}
	if !c.Library.IsZero() {
		w.LibraryID = &c.Library
	}
	if c.Community != nil {
		w.CommunityRating, w.CommunityVotes = &c.Community.Rating, &c.Community.Votes
	}
	switch c.Kind {
	case catalog.KindMovie:
		if c.Edition != "" {
			w.Movie = &gen.MovieInfo{Edition: opt(c.Edition)}
		}
	case catalog.KindSeries:
		w.Series = &gen.SeriesInfo{UnplayedCount: c.Unplayed, RecentlyAddedCount: opt(c.Bundled)}
	case catalog.KindSeason:
		w.Season = &gen.SeasonInfo{
			Number: c.Number, SeriesID: c.Root, SeriesTitle: c.SeriesTitle, EpisodeCount: opt(c.Episodes), UnplayedCount: c.Unplayed,
		}
		if c.Bundled > 0 { // a recently added bundle carries its count where a series' summary's goes
			w.Series = &gen.SeriesInfo{UnplayedCount: c.Unplayed, RecentlyAddedCount: &c.Bundled}
		}
	case catalog.KindEpisode:
		w.Episode = &gen.EpisodeInfo{
			SeasonNumber: c.ParentNumber, Number: c.Number, EndNumber: opt(c.NumberEnd), AirDate: date(c.ReleaseDate),
			SeriesID: c.Root, SeriesTitle: c.SeriesTitle, SeasonID: opt(c.Parent), Overview: opt(c.Overview),
		}
	case catalog.KindExtra:
		w.Extra = &gen.ExtraInfo{Type: extraType(c.ExtraType), ParentID: opt(c.Parent)}
	case catalog.KindBook:
		w.Book = bookInfo(c.Book)
	}
	return w
}

// extraType maps catalog's extra types to the spec's camelCase values.
// Catalog's extra types are the names of the folders extras are found in.
func extraType(t string) gen.ExtraType {
	switch t {
	case "behindthescenes":
		return gen.ExtraTypeBehindTheScenes
	case "deleted":
		return gen.ExtraTypeDeletedScene
	case "":
		return gen.ExtraTypeOther
	}
	return gen.ExtraType(t)
}

func summaries(cs []query.WorkSummary) []gen.WorkSummary {
	out := make([]gen.WorkSummary, len(cs))
	for i, c := range cs {
		out[i] = summary(c)
	}
	return out
}

// bookInfo is a book summary's authors, narrators and series on the wire.
func bookInfo(b *query.BookInfo) *gen.BookInfo {
	out := &gen.BookInfo{Authors: []gen.PersonRef{}, Narrators: []gen.PersonRef{}, Series: []gen.BookSeriesRef{}}
	if b == nil {
		return out
	}
	for _, p := range b.Authors {
		out.Authors = append(out.Authors, gen.PersonRef{ID: p.ID, Name: p.Name})
	}
	for _, p := range b.Narrators {
		out.Narrators = append(out.Narrators, gen.PersonRef{ID: p.ID, Name: p.Name})
	}
	for _, s := range b.Series {
		out.Series = append(out.Series, gen.BookSeriesRef{ID: s.ID, Name: s.Name, Position: opt(s.Position), Primary: s.Primary})
	}
	return out
}

// workPage is a page of a grid of works, with its total when it is the
// first. It includes the letter rail when the grid has one.
func workPage(p query.GridPage[query.WorkSummary], first bool) gen.WorkSummaryPage {
	out := gen.WorkSummaryPage{Items: summaries(p.Items), Letters: letterStarts(p.Letters)}
	if first {
		out.Total = &p.Total
	}
	return out
}

// letterStarts is a grid's letter rail. It is absent when the grid has
// none, which is on a page after the first or a sort other than by title.
func letterStarts(letters []query.Letter) *[]gen.LetterStart {
	if letters == nil {
		return nil
	}
	out := make([]gen.LetterStart, len(letters))
	for i, l := range letters {
		out[i] = gen.LetterStart{Letter: l.Letter, Offset: l.Offset}
	}
	return &out
}

func userState(st query.Userstate) gen.UserState {
	return gen.UserState{
		Played: st.Played, PlayCount: st.PlayCount,
		PositionMs: opt(ms(st.Position)), VersionID: opt(st.Version), LastPlayedAt: optTime(st.LastPlayed),
	}
}

// catalogTrack is a file's probed track on the wire, with the basics that
// catalog records.
func catalogTrack(t catalog.Track) gen.Track {
	var m media.Track
	_ = json.Unmarshal(t.Info, &m) // a track whose details don't parse still has its basics
	m.Kind, m.Codec, m.Language, m.Title = media.TrackKind(t.Type), media.Codec(t.Codec), t.Language, t.Title
	m.Default, m.Forced, m.HearingImpaired = t.Default, t.Forced, t.HearingImpaired
	return track(t.ID, m, catalog.SourceEmbedded)
}

// track is a track on the wire. It has m's basics, probed facts and ID.
// For a subtitle it also says where the subtitle lives.
func track(trackID id.ID, m media.Track, source string) gen.Track {
	out := gen.Track{
		ID: trackID, Type: gen.TrackType(m.Kind), Codec: string(m.Codec), Language: opt(m.Language), Title: opt(m.Title), Default: m.Default,
		Forced: opt(m.Forced), HearingImpaired: opt(m.HearingImpaired), Commentary: opt(m.IsCommentary()),
	}
	switch m.Kind {
	case media.Video:
		out.Video = videoInfo(m)
	case media.Audio:
		out.Audio = &gen.AudioTrackInfo{
			Channels: m.Channels, Layout: opt(media.Layout(m.ChannelLayout)),
			Profile: opt(gen.AudioProfile(media.AudioProfile(m.Profile))),
		}
	case media.Subtitle:
		out.Subtitle = &gen.SubtitleTrackInfo{Source: gen.SubtitleSource(source)}
	}
	return out
}

// named sets the names of ts, which tell apart the tracks of one type
// among them, and returns it.
func named(ts []gen.Track) []gen.Track {
	display.NameTracks(ts)
	return ts
}

// subtitleFileTrack is a subtitle file on the wire.
func subtitleFileTrack(x catalog.SubtitleFile) gen.Track {
	source := catalog.SourceExternal
	if x.Downloaded {
		source = catalog.SourceDownloaded
	}
	return track(x.ID, media.Track{
		Kind: media.Subtitle, Codec: media.Codec(catalog.SubtitleCodec(x.Format)), Language: x.Language, Title: x.Title,
		Default: x.Default, Forced: x.Forced, HearingImpaired: x.HearingImpaired,
	}, source)
}

// videoInfo is a video track's facts on the wire. These are its size,
// resolution class, profile and HDR format.
func videoInfo(m media.Track) *gen.VideoTrackInfo {
	out := &gen.VideoTrackInfo{
		Width: m.Width, Height: m.Height, Resolution: ptr(gen.Resolution(query.ResolutionOf(m.Width))),
		FrameRate: opt(float32(m.FrameRate)), Profile: opt(m.Profile),
	}
	switch {
	case m.DolbyVisionProfile > 0:
		out.Hdr = ptr(gen.HdrFormatDolbyVision)
	case m.Range == media.HDR10:
		out.Hdr = ptr(gen.HdrFormatHdr10)
	case m.Range == media.HLG:
		out.Hdr = ptr(gen.HdrFormatHlg)
	}
	return out
}

// images is a summary's artwork on the wire. Clients fetch each image by
// ID at the width they need.
func images(im query.Images) gen.WorkImages {
	return gen.WorkImages{Poster: imageRef(im.Poster), Backdrop: imageRef(im.Backdrop), Still: imageRef(im.Still), Logo: imageRef(im.Logo)}
}

func imageRef(i *query.Image) *gen.ImageRef {
	if i == nil {
		return nil
	}
	return &gen.ImageRef{ID: i.ID, Blurhash: opt(i.Blurhash), Width: opt(i.Width), Height: opt(i.Height)}
}

func bookSeries(s query.Series) gen.BookSeries {
	return gen.BookSeries{ID: s.ID, Name: s.Name, Items: summaries(s.Books)}
}

// parseSubtitleChoice reads a subtitle track choice. A track's ID picks
// that track. The zero ID turns subtitles off. An absent choice follows
// the user's preference.
func parseSubtitleChoice(s *string) (*id.ID, error) {
	if s == nil {
		return nil, nil
	}
	var sub id.ID
	if *s != "off" {
		var err error
		if sub, err = id.Parse(*s); err != nil {
			return nil, invalid("subtitleTrackId: a track ID or off", field("/subtitleTrackId", "invalid"))
		}
	}
	return &sub, nil
}

func chapters(chapters []catalog.Chapter) []gen.Chapter {
	out := []gen.Chapter{}
	for _, c := range chapters {
		out = append(out, gen.Chapter{StartMs: ms(c.Start), EndMs: ms(c.End), Title: opt(c.Title)})
	}
	return out
}

// markerList is a version's markers on the wire.
func markerList(list []markers.Marker) []gen.Marker {
	out := []gen.Marker{}
	for _, m := range list {
		gm := gen.Marker{Kind: gen.MarkerKind(m.Kind), StartMs: ms(m.Start), EndMs: ms(m.End), Source: gen.MarkerSource(m.Source)}
		if m.Final {
			gm.Final = ptr(true)
		}
		out = append(out, gm)
	}
	return out
}

// parseSignIn reads a sign-in from the request's client. The auth package
// checks it.
func parseSignIn(ctx context.Context, c *gen.SessionCreate) auth.SignInRequest {
	r := auth.SignInRequest{
		Username: c.Username, Password: c.Password, RemoteIP: requestOf(ctx).ip, Kind: auth.SessionKind(deref(c.Kind)),
		Device: requestOf(ctx).device,
	}
	if c.Name != nil {
		r.Name = *c.Name
	}
	if c.ClientID != nil {
		r.ClientID = *c.ClientID
	}
	if r.ClientID == "" && r.Kind != auth.SessionApp {
		r.ClientID = browserClientID(ctx)
	}
	return r
}

func libraryAccess(g users.Grants) gen.LibraryAccess {
	return gen.LibraryAccess{All: g.All, LibraryIds: nonNil(g.Libraries)}
}

func userLimits(l users.Limits) gen.UserLimits {
	return gen.UserLimits{MaxStreams: opt(l.StreamLimit), RemoteQualityCapBps: opt(l.RemoteQualityCap)}
}

func parseGrants(a gen.LibraryAccess) users.Grants {
	return users.Grants{All: a.All, Libraries: []id.ID(a.LibraryIds)}
}

func parseLimits(l gen.UserLimits) users.Limits {
	return users.Limits{StreamLimit: deref(l.MaxStreams), RemoteQualityCap: deref(l.RemoteQualityCapBps)}
}
