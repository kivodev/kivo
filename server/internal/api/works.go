package api

import (
	"context"
	"slices"
	"strings"
	"time"

	"github.com/mmcdole/kivo/internal/api/internal/display"
	"github.com/mmcdole/kivo/internal/api/internal/gen"
	"github.com/mmcdole/kivo/internal/catalog"
	"github.com/mmcdole/kivo/internal/id"
	"github.com/mmcdole/kivo/internal/identify"
	"github.com/mmcdole/kivo/internal/markers"
	"github.com/mmcdole/kivo/internal/media"
	"github.com/mmcdole/kivo/internal/metadata"
	"github.com/mmcdole/kivo/internal/playback"
	"github.com/mmcdole/kivo/internal/query"
	"github.com/mmcdole/kivo/internal/users"
	"github.com/mmcdole/kivo/internal/userstate"
)

// ListWorkChildren lists a series' seasons or a season's episodes. The
// list is not paged.
func (h *handlers) ListWorkChildren(ctx context.Context, req gen.ListWorkChildrenRequestObject) (gen.ListWorkChildrenResponseObject, error) {
	items, err := h.deps.Query.Children(ctx, callerOf(ctx).viewer, req.ID)
	if err != nil {
		return nil, err
	}
	return gen.ListWorkChildren200JSONResponse{Items: summaries(items)}, nil
}

// GetCollection returns a collection with all of its root works. They are
// not paged.
func (h *handlers) GetCollection(ctx context.Context, req gen.GetCollectionRequestObject) (gen.GetCollectionResponseObject, error) {
	p, err := h.deps.Query.Collection(ctx, callerOf(ctx).viewer, req.ID)
	if err != nil {
		return nil, err
	}
	c := p.Collection
	return gen.GetCollection200JSONResponse{
		ID: c.ID, Name: c.Name, Source: collectionSource(c.Source), Items: summaries(p.Items),
		Images: gen.WorkImages{Poster: imageRef(c.Images.Poster), Backdrop: imageRef(c.Images.Backdrop)},
	}, nil
}

// ListUnmatchedWorks returns a page of the Unmatched list, which holds the
// unmatched works of every library.
func (h *handlers) ListUnmatchedWorks(ctx context.Context, req gen.ListUnmatchedWorksRequestObject) (gen.ListUnmatchedWorksResponseObject, error) {
	p := req.Params
	pg, err := pagingOf(p.After, p.Limit)
	if err != nil {
		return nil, err
	}
	page, err := h.deps.Query.Unmatched(ctx, callerOf(ctx).viewer, query.Cursor(pg.after), pg.limit)
	if err != nil {
		return nil, err
	}
	return gen.ListUnmatchedWorks200JSONResponse{Items: summaries(page.Items), Next: opt(string(page.Next))}, nil
}

func (h *handlers) GetWork(ctx context.Context, req gen.GetWorkRequestObject) (gen.GetWorkResponseObject, error) {
	w, err := h.detail(ctx, callerOf(ctx).viewer, req.ID)
	if err != nil {
		return nil, err
	}
	return gen.GetWork200JSONResponse(w), nil
}

// detail returns a work's detail page as query builds it. For each version
// it adds the tracks the user's play would start with and the markers a
// play would have.
func (h *handlers) detail(ctx context.Context, v users.Viewer, w id.ID) (gen.WorkDetail, error) {
	d, err := h.deps.Query.Detail(ctx, v, w)
	if err != nil {
		return gen.WorkDetail{}, err
	}
	var tracks playback.Preferred
	var marks map[id.ID][]markers.Marker
	if len(d.Versions) > 0 {
		if tracks, err = h.deps.Playback.Preferred(ctx, v, w); err != nil {
			return gen.WorkDetail{}, err
		}
		if marks, err = h.deps.Markers.WorkMarkers(ctx, w); err != nil {
			return gen.WorkDetail{}, err
		}
	}
	return workDetail(d, tracks, marks, v), nil
}

func (h *handlers) UpdateWork(ctx context.Context, req gen.UpdateWorkRequestObject) (gen.UpdateWorkResponseObject, error) {
	v := callerOf(ctx).viewer
	if err := h.deps.Identify.EditWork(ctx, v, req.ID, parseWorkEdit(req.Body)); err != nil {
		return nil, err
	}
	w, err := h.detail(ctx, v, req.ID)
	if err != nil {
		return nil, err
	}
	return gen.UpdateWork200JSONResponse(w), nil
}

// maxBookEdit is the most books one batch edit takes. It matches maxItems
// of WorksUpdate.workIds in the spec.
const maxBookEdit = 500

func (h *handlers) UpdateWorks(ctx context.Context, req gen.UpdateWorksRequestObject) (gen.UpdateWorksResponseObject, error) {
	b := req.Body
	v := callerOf(ctx).viewer
	if len(b.WorkIds) > maxBookEdit {
		return nil, invalid("workIds lists too many works", field("/workIds", "tooLarge"))
	}
	e := catalog.BookEdit{Authors: deref(b.Authors), Narrators: deref(b.Narrators), Genres: deref(b.Genres)}
	if b.Authors != nil {
		e.Authors = nonNil(e.Authors)
	}
	if b.Narrators != nil {
		e.Narrators = nonNil(e.Narrators)
	}
	if b.Genres != nil {
		e.Genres = nonNil(e.Genres)
	}
	if s := b.Series; s != nil {
		e.Series = &catalog.SeriesEdit{Name: s.Name, KeepPosition: deref(s.KeepPosition)}
	}
	if err := h.deps.Catalog.EditBooks(ctx, v, b.WorkIds, e); err != nil {
		return nil, err
	}
	books, err := h.deps.Query.Summaries(ctx, v, b.WorkIds)
	if err != nil {
		return nil, err
	}
	return gen.UpdateWorks200JSONResponse(gen.WorkSummaryList{Items: summaries(books)}), nil
}

func (h *handlers) UpdateWorkUserState(ctx context.Context, req gen.UpdateWorkUserStateRequestObject) (gen.UpdateWorkUserStateResponseObject, error) {
	c, err := parseUserStateChange(req.Body)
	if err != nil {
		return nil, err
	}
	v := callerOf(ctx).viewer
	if err := h.deps.Userstate.Update(ctx, v, req.ID, c); err != nil {
		return nil, err
	}
	st, err := h.deps.Query.Userstate(ctx, v, req.ID)
	if err != nil {
		return nil, err
	}
	return gen.UpdateWorkUserState200JSONResponse(userState(st)), nil
}

func (h *handlers) RemoveFromContinue(ctx context.Context, req gen.RemoveFromContinueRequestObject) (gen.RemoveFromContinueResponseObject, error) {
	if err := h.deps.Userstate.RemoveFromContinue(ctx, callerOf(ctx).viewer, req.ID); err != nil {
		return nil, err
	}
	return gen.RemoveFromContinue204Response{}, nil
}

func (h *handlers) ListWorkMatches(ctx context.Context, req gen.ListWorkMatchesRequestObject) (gen.ListWorkMatchesResponseObject, error) {
	p := req.Params
	s := identify.MatchSearch{Provider: optAs[string](deref(p.Provider)), Query: p.Query, Year: p.Year, Author: p.Author}
	matches, err := h.deps.Identify.SearchMatches(ctx, callerOf(ctx).viewer, req.ID, s)
	if err != nil {
		return nil, err
	}
	return gen.ListWorkMatches200JSONResponse{Items: each(matches, match)}, nil
}

func (h *handlers) MatchWork(ctx context.Context, req gen.MatchWorkRequestObject) (gen.MatchWorkResponseObject, error) {
	b := req.Body
	x, ok := parseMatchID(b.MatchID)
	if !ok {
		return nil, invalid("unknown match", field("/matchId", "invalid"))
	}
	v := callerOf(ctx).viewer
	matched, err := h.deps.Identify.FixMatch(ctx, v, req.ID, x)
	if err != nil {
		return nil, err
	}
	w, err := h.detail(ctx, v, matched)
	if err != nil {
		return nil, err
	}
	return gen.MatchWork200JSONResponse(w), nil
}

func (h *handlers) UnmatchWork(ctx context.Context, req gen.UnmatchWorkRequestObject) (gen.UnmatchWorkResponseObject, error) {
	v := callerOf(ctx).viewer
	work, err := h.deps.Identify.Unmatch(ctx, v, req.ID)
	if err != nil {
		return nil, err
	}
	w, err := h.detail(ctx, v, work)
	if err != nil {
		return nil, err
	}
	return gen.UnmatchWork200JSONResponse(w), nil
}

func (h *handlers) RefreshWorkMetadata(ctx context.Context, req gen.RefreshWorkMetadataRequestObject) (gen.RefreshWorkMetadataResponseObject, error) {
	if err := h.deps.Identify.Refresh(ctx, callerOf(ctx).viewer, req.ID); err != nil {
		return nil, err
	}
	return gen.RefreshWorkMetadata202JSONResponse{}, nil
}

func (h *handlers) ListContentRatings(ctx context.Context, req gen.ListContentRatingsRequestObject) (gen.ListContentRatingsResponseObject, error) {
	ratings, err := h.contentRatings(ctx, req.Params.WorkID)
	if err != nil {
		return nil, err
	}
	out := gen.ListContentRatings200JSONResponse{Items: []gen.ContentRating{}}
	for _, r := range ratings {
		out.Items = append(out.Items, gen.ContentRating{Name: r.Name, Age: r.Age})
	}
	return out, nil
}

// contentRatings lists the content ratings of a work's content rating
// country. Without a work it uses the server's country.
func (h *handlers) contentRatings(ctx context.Context, work *id.ID) ([]metadata.ContentRating, error) {
	if work == nil {
		return h.deps.Metadata.ContentRatings(""), nil
	}
	return h.deps.Metadata.WorkContentRatings(ctx, callerOf(ctx).viewer, *work)
}

// collectionSummary is a collection's card on the wire, with its poster.
func collectionSummary(c query.Collection) gen.CollectionSummary {
	return gen.CollectionSummary{
		ID: c.ID, Name: c.Name, WorkCount: c.Items, Source: collectionSource(c.Source),
		Images: gen.WorkImages{Poster: imageRef(c.Images.Poster)},
	}
}

func collectionSource(s query.CollectionSource) *gen.CollectionSource {
	return opt(gen.CollectionSource(s))
}

// authorSummary is a person on an audiobooks library's "Authors" tab. Its
// WorkCount counts the books in the library that the person wrote.
func authorSummary(a query.Author) gen.PersonSummary {
	return gen.PersonSummary{ID: a.ID, Name: a.Name, Photo: imageRef(a.Photo), Role: catalog.RoleAuthor, WorkCount: a.Books}
}

// seriesSummary is a series on an audiobooks library's "Series" tab, with
// its first book's cover.
func seriesSummary(s query.SeriesSummary) gen.BookSeriesSummary {
	return gen.BookSeriesSummary{ID: s.ID, Name: s.Name, BookCount: s.Books, Cover: imageRef(s.Images.Poster)}
}

// workDetail is a detail page on the wire. For each version it carries the
// tracks the user's play would start with and the markers a play would have.
func workDetail(d query.WorkDetail, tracks playback.Preferred, marks map[id.ID][]markers.Marker, v users.Viewer) gen.WorkDetail {
	out := gen.WorkDetail{
		WorkSummary:   summary(d.Summary),
		OriginalTitle: opt(d.OriginalTitle), SortTitle: opt(d.Summary.SortTitle), Overview: opt(d.Overview),
		Genres: nonNil(d.Genres), Studios: nonNil(d.Companies),
		Credits: []gen.Credit{}, ExternalIds: []gen.ExternalID{}, Versions: []gen.Version{},
		Extras: summaries(d.Extras), Collections: each(d.Collections, collectionSummary), Similar: summaries(d.Similar),
		ListIds: ptr(nonNil(d.ListIDs)),
	}
	// A season or episode without its own content rating takes its series'
	// rating.
	out.ContentRating = opt(d.ContentRating)
	for _, c := range d.Credits {
		out.Credits = append(out.Credits, gen.Credit{PersonID: c.Person, Name: c.Name, Role: c.Role, Character: opt(c.Character), Photo: imageRef(c.Photo)})
	}
	for _, e := range d.ExternalIDs {
		out.ExternalIds = append(out.ExternalIds, gen.ExternalID{Provider: gen.ExternalProvider(e.Provider), Kind: e.Kind, ID: e.ID})
	}
	if v.Admin() {
		locked := make([]string, 0, len(d.LockedFields))
		for _, f := range d.LockedFields {
			locked = append(locked, lockedFieldName(d.Summary.Kind, f))
		}
		slices.Sort(locked)
		out.LockedFields = &locked
	}
	out.Versions = versions(d, tracks, marks)
	if len(out.Versions) > 0 {
		out.RemembersTracks = &tracks.Remembers
	}
	if out.Series != nil {
		out.Series.SeasonCount = &d.Children
		if d.NextUp != nil {
			n := summary(*d.NextUp)
			out.Series.NextUp = &n
		}
	}
	if out.Season != nil {
		out.Season.EpisodeCount = &d.Children
	}
	if out.Episode != nil {
		out.Episode.PreviousID, out.Episode.NextID = opt(d.PreviousEpisode), opt(d.NextEpisode)
	}
	setTrailers(&out, d.Trailers)
	if b := d.Book; b != nil && out.Book != nil {
		out.Book.Subtitle, out.Book.Publisher, out.Book.Language = opt(b.Subtitle), opt(b.Publisher), opt(b.Language)
		out.Book.ReleaseDate = opt(b.ReleaseDate)
	}
	return out
}

// setTrailers puts a movie's or series' trailers on its movie or series
// object. A work without trailers gets none.
func setTrailers(out *gen.WorkDetail, trailers []query.Trailer) {
	if len(trailers) == 0 {
		return
	}
	ts := make([]gen.Trailer, len(trailers))
	for i, t := range trailers {
		ts[i] = gen.Trailer{ID: t.ID, Title: t.Title, URL: t.URL, Image: imageRef(t.Image)}
	}
	if out.Series != nil {
		out.Series.Trailers = &ts
		return
	}
	if out.Movie == nil { // a summary has a movie's object only for its edition
		out.Movie = &gen.MovieInfo{}
	}
	out.Movie.Trailers = &ts
}

// versions converts a detail page's versions. For an episode or a movie,
// each version carries the tracks the user's play would start with and the
// markers a play would have.
func versions(d query.WorkDetail, tracks playback.Preferred, marks map[id.ID][]markers.Marker) []gen.Version {
	out := []gen.Version{}
	for _, ver := range d.Versions {
		gv := version(ver, d.OriginalLanguage)
		if k := d.Summary.Kind; k == catalog.KindEpisode || k == catalog.KindMovie {
			gv.Markers = ptr(markerList(marks[ver.ID]))
		}
		if t, ok := tracks.Versions[ver.ID]; ok {
			gv.AudioTrackID, gv.SubtitleTrackID = opt(t.Audio), opt(t.Subtitle)
		}
		out = append(out, gv)
	}
	return out
}

// version is a technical version on the wire. It lists the works it
// covers, its name and its video's resolution and HDR format. Audio tracks in the
// work's original language are marked as original.
func version(v query.Version, original string) gen.Version {
	out := gen.Version{
		ID: v.ID, Edition: opt(v.Edition), Available: !v.Unavailable, LibraryID: opt(v.Library),
		DurationMs: ms(v.Duration), SizeBytes: v.Size, Parts: []gen.VersionPart{}, Tracks: []gen.Track{},
		WorkIds: nonNil(v.Works), ChapterCount: opt(v.Chapters),
	}
	if len(v.Parts) > 0 {
		out.Container = opt(v.Parts[0].Container)
		out.BitrateBps = opt(int64(v.Parts[0].Bitrate))
	}
	for _, p := range v.Parts {
		out.Parts = append(out.Parts, gen.VersionPart{FileID: p.File, Name: p.Name, DurationMs: ms(p.Duration), SizeBytes: p.Size})
	}
	probed := each(v.Tracks, query.Track.Media)
	originals := media.OriginalAudio(probed, original)
	for i, t := range v.Tracks {
		gt := track(t.ID, probed[i], t.Source)
		gt.Original = opt(originals[i])
		if gt.Video != nil && out.Resolution == nil {
			out.Resolution, out.Hdr = gt.Video.Resolution, gt.Video.Hdr
		}
		out.Tracks = append(out.Tracks, gt)
	}
	named(out.Tracks)
	out.Name = display.VersionName(out)
	for _, c := range v.ChapterList {
		out.Chapters = ptr(append(deref(out.Chapters), gen.Chapter{StartMs: ms(c.Start), EndMs: ms(c.End), Title: opt(c.Title)}))
	}
	return out
}

// lockedFields maps catalog's snake_case locked field names to the API's
// camelCase ones. Catalog's release date is an episode's air date or a
// book's release date. Its companies are a work's studios or a book's
// publisher.
var lockedFields = map[string]string{
	catalog.FieldTitle: "title", catalog.FieldSortTitle: "sortTitle", catalog.FieldOriginalTitle: "originalTitle",
	catalog.FieldYear: "year", catalog.FieldOverview: "overview", catalog.FieldReleaseDate: "episode.airDate",
	catalog.FieldContentRating: "contentRating", catalog.FieldGenres: "genres", catalog.FieldTags: "tags",
	catalog.FieldCompanies: "studios", catalog.FieldCredits: "credits",
	catalog.FieldAuthors: "book.authors", catalog.FieldNarrators: "book.narrators", catalog.FieldSeries: "book.series",
	catalog.FieldSubtitle: "book.subtitle", catalog.FieldASIN: "book.asin",
}

var bookLockedFields = map[string]string{catalog.FieldReleaseDate: "book.releaseDate", catalog.FieldCompanies: "book.publisher"}

func lockedFieldName(kind catalog.WorkKind, f string) string {
	if n, ok := bookLockedFields[f]; ok && kind == catalog.KindBook {
		return n
	}
	if n, ok := lockedFields[f]; ok {
		return n
	}
	return f
}

// match is a "Fix match" result on the wire. A book's result also carries
// its authors, narrators, subtitle, overview and running time.
func match(m metadata.Match) gen.Match {
	out := gen.Match{
		ID: matchID(m.ID), Provider: gen.ExternalProvider(m.ID.Provider), Kind: gen.MatchKind(m.ID.Kind), Title: m.Title,
		OriginalTitle: opt(m.OriginalTitle), Year: opt(m.Year),
	}
	if m.ID.Provider == catalog.ASIN.Provider {
		out.Subtitle, out.Overview = m.Subtitle, m.Overview
		out.Authors, out.Narrators = &m.Authors, &m.Narrators
		out.DurationMs = opt(int(m.Duration.Milliseconds()))
	}
	return out
}

// matchID is a match's opaque ID. It encodes the provider, the kind and
// the provider's ID.
func matchID(x catalog.ExternalID) string { return x.Provider + ":" + x.Kind + ":" + x.ID }

func parseMatchID(s string) (catalog.ExternalID, bool) {
	parts := strings.SplitN(s, ":", 3)
	if len(parts) != 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
		return catalog.ExternalID{}, false
	}
	return catalog.ExternalID{Provider: parts[0], Kind: parts[1], ID: parts[2]}, true
}

// parseUserStateChange reads a change to the user's state for a work. It
// can set played and an offline position. An offline position
// needs its position, time and version.
func parseUserStateChange(b *gen.UpdateWorkUserStateJSONRequestBody) (userstate.Change, error) {
	c := userstate.Change{Played: b.Played}
	if b.PositionMs == nil && b.PlayedAt == nil && b.VersionID == nil {
		return c, nil
	}
	var missing []gen.FieldError
	for _, f := range []struct {
		name string
		set  bool
	}{{"/positionMs", b.PositionMs != nil}, {"/playedAt", b.PlayedAt != nil}, {"/versionId", b.VersionID != nil}} {
		if !f.set {
			missing = append(missing, field(f.name, "required"))
		}
	}
	if len(missing) > 0 {
		return userstate.Change{}, invalid("an offline position needs positionMs, playedAt and versionId", missing...)
	}
	if *b.PositionMs < 0 {
		return userstate.Change{}, invalid("positionMs is negative", field("/positionMs", "outOfRange"))
	}
	c.Offline = &userstate.Offline{Version: *b.VersionID, Position: duration(*b.PositionMs), At: *b.PlayedAt}
	return c, nil
}

// parseWorkEdit reads an admin's edit of a work, keeping only the fields
// the request sets. A movie's or series' studios and a book's publisher
// are its companies. An episode's air date and a book's release date are
// its release date.
func parseWorkEdit(b *gen.UpdateWorkJSONRequestBody) identify.WorkEdit {
	var e identify.WorkEdit
	m, fields := &e.Metadata, &e.Fields
	setField(fields, catalog.FieldTitle, &m.Title, b.Title)
	setField(fields, catalog.FieldSortTitle, &m.SortTitle, b.SortTitle)
	setField(fields, catalog.FieldOriginalTitle, &m.OriginalTitle, b.OriginalTitle)
	setField(fields, catalog.FieldYear, &m.Year, b.Year)
	setField(fields, catalog.FieldOverview, &m.Overview, b.Overview)
	setField(fields, catalog.FieldGenres, &m.Genres, b.Genres)
	setField(fields, catalog.FieldCompanies, &m.Companies, b.Studios)
	e.ContentRating = b.ContentRating
	if b.Episode != nil && b.Episode.AirDate != nil {
		m.ReleaseDate = b.Episode.AirDate.Format(time.DateOnly)
		*fields = append(*fields, catalog.FieldReleaseDate)
	}
	if b.Series != nil {
		e.Ordering = b.Series.Ordering
	}
	if bk := b.Book; bk != nil {
		m.Book = &catalog.BookMetadata{}
		setField(fields, catalog.FieldSubtitle, &m.Book.Subtitle, bk.Subtitle)
		if bk.Publisher != nil {
			m.Companies = []string{}
			if p := strings.TrimSpace(*bk.Publisher); p != "" {
				m.Companies = []string{p}
			}
			*fields = append(*fields, catalog.FieldCompanies)
		}
		setField(fields, catalog.FieldReleaseDate, &m.ReleaseDate, bk.ReleaseDate)
		setField(fields, catalog.FieldASIN, &m.Book.ASIN, bk.Asin)
		setField(fields, catalog.FieldAuthors, &m.Book.Authors, bk.Authors)
		setField(fields, catalog.FieldNarrators, &m.Book.Narrators, bk.Narrators)
		if bk.Series != nil {
			m.Book.Series = []catalog.SeriesRef{}
			for _, s := range *bk.Series {
				m.Book.Series = append(m.Book.Series, catalog.SeriesRef{Name: s.Name, Position: deref(s.Position), Primary: deref(s.Primary)})
			}
			*fields = append(*fields, catalog.FieldSeries)
		}
	}
	return e
}

// setField copies *v to *dst and adds field to fields when the request
// sets the value.
func setField[T any](fields *[]string, field string, dst, v *T) {
	if v != nil {
		*dst = *v
		*fields = append(*fields, field)
	}
}
