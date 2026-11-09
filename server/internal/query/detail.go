package query

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/mmcdole/kivo/internal/catalog"
	"github.com/mmcdole/kivo/internal/id"
	"github.com/mmcdole/kivo/internal/media"
	"github.com/mmcdole/kivo/internal/users"
)

// WorkDetail is a work's detail page.
type WorkDetail struct {
	Summary       WorkSummary
	OriginalTitle string
	Overview      string
	// ContentRating is the work's content rating. A season or episode
	// without one takes its series' rating. It is empty when unrated.
	ContentRating string
	// OriginalLanguage is the language the work was made in, as the
	// metadata provider gives it. A season or episode without one takes its
	// series' language. It is empty when unknown.
	OriginalLanguage string
	TrailerURL       string // a movie's trailer; empty for none
	Genres           []string
	Companies        []string
	Credits          []Credit             // cast in billing order, then crew
	ExternalIDs      []catalog.ExternalID // by provider, then kind
	// LockedFields are the fields an admin edited, as catalog names them.
	// Only admins get them.
	LockedFields []string
	// Versions is each version the Viewer may play, in the order
	// catalog.CompareVersions gives: untagged versions first, then by
	// edition label, and the best picture first within an edition. A
	// version with a part in a library the Viewer isn't granted is left out.
	Versions    []Version
	Extras      []WorkSummary
	Collections []Collection
	// Children is how many seasons a series has, or how many episodes a
	// season has, counted as Children lists them. It is zero for other kinds.
	Children int
	// NextUp is the episode a series' primary button plays. That is the
	// episode Continue Watching shows for the series, or else its first
	// episode. It is nil for other kinds.
	NextUp *WorkSummary
	// PreviousEpisode and NextEpisode are an episode's neighbours in its
	// series' list order. The order runs across seasons, with specials last.
	// Each is zero at its end of the list.
	PreviousEpisode, NextEpisode id.ID
	// Book holds a book's own fields, and is nil for other kinds. A book's
	// authors, narrators and series are on its Summary.
	Book *Book
}

// Credit is a person's part in a work.
type Credit struct {
	Person    id.ID
	Name      string
	Role      string // actor, director, and so on
	Character string
	Photo     *Image // nil without one
}

// Version is one playable copy of a work: one or more file parts.
type Version struct {
	ID      id.ID
	Library id.ID
	// Edition is the cut its files' edition tag names, such as "Final
	// Cut". It is empty for an untagged version.
	Edition  string
	Duration time.Duration
	Size     int64
	// Unavailable means a part's folder can't be reached.
	Unavailable bool
	Parts       []Part
	Works       []id.ID // the works it covers, in order
	Tracks      []Track // the first part's tracks
	// Chapters counts the chapters on the version's timeline. Each part's
	// own chapters count. In a version of several parts, a part with no
	// chapters counts as one.
	Chapters int
	// ChapterList is a book's chapters on the timeline, timed as playback
	// times them, for the book's page to list. It is empty for other kinds.
	ChapterList []Chapter
}

// Chapter is a chapter on a version's timeline.
type Chapter struct {
	Start, End time.Duration
	Title      string
}

// Part is one file of a version.
type Part struct {
	File        id.ID
	Name        string // the file's name
	Path        string // the last known path, for admins only
	Size        int64
	Duration    time.Duration
	Container   string
	Bitrate     int // bits per second
	Unavailable bool
	// Chapters are the chapters in an episode's or a movie's file, which
	// its markers come from. Other kinds have none.
	Chapters []Chapter
}

// Track is one track of a version's first part. Fields that don't apply to
// its type are zero.
type Track struct {
	ID                 id.ID
	Type               string // video, audio or subtitle
	Source             string // embedded, external (a subtitle file next to the video) or downloaded
	Codec              string
	Language           string
	Title              string
	Default            bool
	Forced             bool
	HearingImpaired    bool
	Channels           int
	ChannelLayout      string // as ffmpeg names it: "5.1(side)"
	Width, Height      int
	Range              string // sdr, hdr10 or hlg
	DolbyVisionProfile int
	Profile            string // as the codec names it: "Main 10"
	Commentary         bool   // as the file marks it
}

// Media returns the track as a media.Track.
func (t Track) Media() media.Track {
	return media.Track{
		Kind: media.TrackKind(t.Type), Codec: media.Codec(t.Codec), Language: t.Language, Title: t.Title,
		Default: t.Default, Forced: t.Forced, HearingImpaired: t.HearingImpaired, Commentary: t.Commentary,
		Width: t.Width, Height: t.Height, Range: media.DynamicRange(t.Range), DolbyVisionProfile: t.DolbyVisionProfile, Profile: t.Profile,
		Channels: t.Channels, ChannelLayout: t.ChannelLayout,
	}
}

// A work's detail page takes its summary from WorkSummary and the rest from
// the statements below. Each statement filters on the work itself, so
// nothing about a hidden work leaks.

// above is the work whose content rating and language w takes when it has
// none: a season's or episode's series.
const above = `CASE WHEN w.parent_id IS NOT NULL THEN w.root_id END`

var detail = stmt("WorkDetail", `
SELECT coalesce(w.original_title, ''), coalesce(w.overview, ''),
  coalesce(w.content_rating, (SELECT r.content_rating FROM work r WHERE r.id = `+above+`), ''),
  coalesce(w.original_language, (SELECT r.original_language FROM work r WHERE r.id = `+above+`), ''),
  coalesce(w.locked_fields, ''), coalesce(w.trailer_url, '')
FROM work w
WHERE w.id = @work
  AND `+visible)

// visibleTitle is a work's title, if the Viewer may see it, for Titles.
var visibleTitle = stmt("VisibleTitle", `
SELECT w.title
FROM work w
WHERE w.id = @work
  AND `+visible)

var genres = stmt("Genres", `
SELECT g.name
FROM work w
JOIN work_genre wg ON wg.work_id = w.id
JOIN genre g ON g.id = wg.genre_id
WHERE w.id = @work
  AND `+visible+`
ORDER BY g.name`)

var companies = stmt("Companies", `
SELECT c.name
FROM work w
JOIN work_company wc ON wc.work_id = w.id
JOIN company c ON c.id = wc.company_id
WHERE w.id = @work
  AND `+visible+`
ORDER BY c.name`)

// credits is cast first in billing order, then crew by role, each with
// their photo.
var credits = stmt("Credits", `
SELECT p.id, p.name, c.role, coalesce(c.character, ''), `+photoColumns+`
FROM work w
JOIN credit c ON c.work_id = w.id
JOIN person p ON p.id = c.person_id
LEFT JOIN image i ON i.person_id = p.id AND i.kind = 'photo'
WHERE w.id = @work
  AND `+visible+`
ORDER BY c.role <> 'actor', c.role, c.ord`)

var externalIDs = stmt("ExternalIDs", `
SELECT x.provider, x.kind, x.ext_id
FROM work w
JOIN work_external_id x ON x.work_id = w.id
WHERE w.id = @work
  AND `+visible+`
ORDER BY x.provider, x.kind, x.ext_id`)

// childCount is how many seasons or episodes the work has, counted as
// Children lists them.
var childCount = stmt("ChildCount", `
SELECT count(*)
FROM work w
WHERE w.parent_id = @work AND w.kind IN ('season', 'episode')
  AND `+notHidden+`
  AND `+visible)

// shownVersion keeps the versions, joined as vw, that have no gone part. A
// version whose file is gone is hidden until the file returns, as a work
// whose versions are all gone is.
const shownVersion = `NOT EXISTS (SELECT 1 FROM media_file g WHERE g.version_id = vw.version_id AND g.status = 'gone')`

// versionParts is every part of each shown version of the work. An
// unreachable folder makes its part unavailable.
var versionParts = stmt("VersionParts", `
SELECT vw.version_id, coalesce(mv.edition, ''), f.id, r.library_id, r.path, f.rel_path,
  f.size, coalesce(f.container, ''), coalesce(f.duration_ms, 0),
  coalesce(f.bitrate, 0), r.reachable AS available,
  (SELECT count(*) FROM media_chapter ch WHERE ch.file_id = f.id) AS chapters
FROM work w
JOIN media_version_work vw ON vw.work_id = w.id
JOIN media_version mv ON mv.id = vw.version_id
JOIN media_file f ON f.version_id = vw.version_id
JOIN library_folder r ON r.id = f.folder_id
WHERE w.id = @work
  AND `+visible+`
  AND `+shownVersion+`
ORDER BY vw.version_id, f.part`)

// versionTracks lists the tracks of the first part of each version that
// VersionParts returns. They come in the order catalog lists a file's
// tracks: embedded tracks by stream index, then subtitle files next to the
// video, then downloaded subtitles, each by path. An embedded track's probe
// details come from media_track.info, as scan records them.
var versionTracks = stmt("VersionTracks", `
SELECT p.version_id, t.id, t.source, t.type, t.codec, coalesce(t.language, ''),
  coalesce(t.title, ''), coalesce(t.is_default, 0), coalesce(t.forced, 0), coalesce(t.hearing_impaired, 0),
  CAST(coalesce(json_extract(t.info, '$.channels'), 0) AS INTEGER),
  CAST(coalesce(json_extract(t.info, '$.width'), 0) AS INTEGER),
  CAST(coalesce(json_extract(t.info, '$.height'), 0) AS INTEGER),
  CAST(coalesce(json_extract(t.info, '$.range'), '') AS TEXT),
  CAST(coalesce(json_extract(t.info, '$.dolby_vision_profile'), 0) AS INTEGER),
  CAST(coalesce(json_extract(t.info, '$.profile'), '') AS TEXT),
  CAST(coalesce(json_extract(t.info, '$.channel_layout'), '') AS TEXT),
  CAST(coalesce(json_extract(t.info, '$.commentary'), 0) AS INTEGER) <> 0
FROM work w
JOIN media_version_work vw ON vw.work_id = w.id
JOIN media_file p ON p.version_id = vw.version_id AND p.part = 1
JOIN media_track t ON t.file_id = p.id AND NOT coalesce(json_extract(t.info, '$.attached_picture'), 0) -- cover art isn't a track to show
WHERE w.id = @work
  AND `+visible+`
  AND `+shownVersion+`
ORDER BY p.version_id, t.source <> 'embedded', t.source = 'downloaded', t.stream_index, t.rel_path`)

// versionWorks lists the works that each version from VersionParts
// covers, in order.
var versionWorks = stmt("VersionWorks", `
SELECT vw.version_id, cw.work_id
FROM work w
JOIN media_version_work vw ON vw.work_id = w.id
JOIN media_version_work cw ON cw.version_id = vw.version_id
WHERE w.id = @work
  AND `+visible+`
  AND `+shownVersion+`
ORDER BY vw.version_id, cw.ord`)

// versionChapters lists a book's chapters. It returns every part of each
// version from VersionParts, in order, with one row per chapter. A part
// with no chapters gets a single row with no chapter.
var versionChapters = stmt("VersionChapters", `
SELECT vw.version_id, f.id, f.rel_path, coalesce(f.duration_ms, 0), ch.start_ms, ch.end_ms, ch.title
FROM work w
JOIN media_version_work vw ON vw.work_id = w.id
JOIN media_file f ON f.version_id = vw.version_id
LEFT JOIN media_chapter ch ON ch.file_id = f.id
WHERE w.id = @work
  AND `+visible+`
  AND `+shownVersion+`
ORDER BY vw.version_id, f.part, ch.idx`)

var extras = stmt("Extras", `
SELECT `+summaryColumns+`
FROM work w
LEFT JOIN user_state us ON us.user_id = @user AND us.work_id = w.id
WHERE w.parent_id = @work AND w.kind = 'extra'
  AND `+visible+`
ORDER BY w.extra_type, w.sort_title, w.id`)

// workCollections is the collections a matched work is in, with how many
// of their works the Viewer sees. The count's inner alias w shadows the
// outer one on purpose, so the same Viewer predicate applies to each member.
var workCollections = stmt("WorkCollections", `
SELECT `+collectionColumns+`, (
    SELECT count(*) FROM collection_work mw JOIN work w ON w.id = mw.work_id
    WHERE mw.collection_id = c.id AND NOT w.provisional
      AND `+visible+`) AS items
FROM work w
JOIN collection_work cw ON cw.work_id = w.id
JOIN collection c ON c.id = cw.collection_id
WHERE w.id = @work AND NOT w.provisional
  AND `+visible+`
ORDER BY c.sort_name, c.id`)

// upNext returns SQL for the ID of the episode @user watches next in a
// series, given as a column or a parameter.
//
// It starts from the episode the user played last, among those finished or
// played past the resume floor, so an older episode left in progress
// doesn't count. If that episode is still in progress and in a library, it
// is up next. Otherwise the next is the first unwatched episode after it in
// season order, skipping specials, among those in a library. The result is
// NULL when the user has played no episode or no episode comes after.
//
// CROSS JOIN makes SQLite read the series' episodes before the user's
// states. Episodes played together, as a multi-episode version's are, go by
// the first of them.
func upNext(series string) string {
	return `(SELECT CASE WHEN lu.resumable AND EXISTS (SELECT 1 FROM work_library ll WHERE ll.work_id = l.id) THEN l.id ELSE (
      SELECT e.id FROM work e
      WHERE e.root_id = l.root_id AND e.kind = 'episode' AND e.parent_number > 0
        AND (e.parent_number, e.number) > (l.parent_number, l.number)
        AND NOT EXISTS (SELECT 1 FROM user_state nu WHERE nu.user_id = @user AND nu.work_id = e.id AND nu.played)
        AND EXISTS (SELECT 1 FROM work_library el WHERE el.work_id = e.id)
      ORDER BY e.parent_number, e.number LIMIT 1) END
    FROM work l CROSS JOIN user_state lu ON lu.user_id = @user AND lu.work_id = l.id
    WHERE l.root_id = ` + series + ` AND l.kind = 'episode' AND (lu.played OR lu.resumable) AND lu.last_played_at IS NOT NULL
    ORDER BY lu.last_played_at DESC, l.parent_number, l.number, l.id LIMIT 1)`
}

// seriesNextUp is the episode a series' primary button plays: the episode
// up next, or else the first episode.
var seriesNextUp = stmt("SeriesNextUp", `
SELECT `+summaryColumns+`
FROM work w
LEFT JOIN user_state us ON us.user_id = @user AND us.work_id = w.id
WHERE w.id = coalesce(
    `+upNext("@series")+`,
    (SELECT e.id FROM work e
     WHERE e.root_id = @series AND e.kind = 'episode'
       AND EXISTS (SELECT 1 FROM work_library el WHERE el.work_id = e.id)
     ORDER BY e.parent_number = 0, e.parent_number, e.number LIMIT 1))
  AND `+visible)

// listed keeps the episodes w of c's series that the series' list shows.
// Those are the episodes the Viewer sees, except the later episodes of a
// multi-episode version. Children skips those, because the version shows
// in its first episode's row.
var listed = notHidden + `
      AND ` + visible

// adjacentEpisodes is an episode's previous and next episodes in its
// series' list order. The order crosses season boundaries and puts specials
// after the last season.
// Ties in numbering go by rid, the index's own order.
var adjacentEpisodes = stmt("AdjacentEpisodes", `
SELECT p.id, n.id
FROM work c
LEFT JOIN work p ON p.id = coalesce(
    (SELECT w.id FROM work w
    WHERE w.root_id = c.root_id AND w.kind = 'episode'
      AND (w.parent_number, w.number, w.rid) < (c.parent_number, c.number, c.rid) AND (c.parent_number = 0 OR w.parent_number > 0)
      AND `+listed+`
    ORDER BY w.parent_number DESC, w.number DESC, w.rid DESC LIMIT 1),
    -- the first special's previous is the last season's last episode
    (SELECT w.id FROM work w
    WHERE w.root_id = c.root_id AND w.kind = 'episode'
      AND c.parent_number = 0 AND w.parent_number > 0
      AND `+listed+`
    ORDER BY w.parent_number DESC, w.number DESC, w.rid DESC LIMIT 1))
LEFT JOIN work n ON n.id = coalesce(
    (SELECT w.id FROM work w
    WHERE w.root_id = c.root_id AND w.kind = 'episode'
      AND (w.parent_number, w.number, w.rid) > (c.parent_number, c.number, c.rid) AND (c.parent_number > 0 OR w.parent_number = 0)
      AND `+listed+`
    ORDER BY w.parent_number, w.number, w.rid LIMIT 1),
    -- the last season's last episode's next is the first special
    (SELECT w.id FROM work w
    WHERE w.root_id = c.root_id AND w.kind = 'episode'
      AND c.parent_number > 0 AND w.parent_number = 0
      AND `+listed+`
    ORDER BY w.number, w.rid LIMIT 1))
WHERE c.id = @work AND c.kind = 'episode'`)

// Titles returns the titles of the works the Viewer may see, by ID. Works
// the Viewer may not see are left out.
func (s *Service) Titles(ctx context.Context, v users.Viewer, works []id.ID) (map[id.ID]string, error) {
	vw := viewerOf(v)
	out := map[id.ID]string{}
	err := s.db.Read(ctx, func(tx *sql.Tx) error {
		for _, w := range works {
			var t string
			err := rowOf(ctx, tx, visibleTitle, vw.args(sql.Named("work", w)), &t)
			switch {
			case errors.Is(err, sql.ErrNoRows):
			case err != nil:
				return err
			default:
				out[w] = t
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// scanString reads a row of one text column.
func scanString(r scanner, s *string) error { return r.Scan(s) }

// Detail returns a work's detail page, read from one snapshot.
func (s *Service) Detail(ctx context.Context, v users.Viewer, work id.ID) (WorkDetail, error) {
	vw := viewerOf(v)
	args := vw.args(sql.Named("work", work))
	var d WorkDetail
	err := s.db.Read(ctx, func(tx *sql.Tx) error {
		var err error
		if d.Summary, err = summary(ctx, tx, vw, work); err != nil {
			return notFound(err)
		}
		var locked string
		if err := rowOf(ctx, tx, detail, args, &d.OriginalTitle, &d.Overview, &d.ContentRating, &d.OriginalLanguage, &locked, &d.TrailerURL); err != nil {
			return notFound(err)
		}
		if v.Admin() && locked != "" {
			_ = json.Unmarshal([]byte(locked), &d.LockedFields) // a list that doesn't parse locks nothing
		}
		if d.Genres, err = rowsOf(ctx, tx, genres, args, scanString); err != nil {
			return err
		}
		if d.Companies, err = rowsOf(ctx, tx, companies, args, scanString); err != nil {
			return err
		}
		d.Credits, err = rowsOf(ctx, tx, credits, args, func(r scanner, c *Credit) error {
			var p photo
			err := r.Scan(append([]any{&c.Person, &c.Name, &c.Role, &c.Character}, p.dest()...)...)
			c.Photo = p.image()
			return err
		})
		if err != nil {
			return err
		}
		d.ExternalIDs, err = rowsOf(ctx, tx, externalIDs, args, func(r scanner, x *catalog.ExternalID) error {
			return r.Scan(&x.Provider, &x.Kind, &x.ID)
		})
		if err != nil {
			return err
		}

		if d.Versions, err = versions(ctx, tx, args, v.Admin()); err != nil {
			return err
		}
		if err := covers(ctx, tx, args, d.Versions); err != nil {
			return err
		}
		if k := d.Summary.Kind; k == catalog.KindBook || k == catalog.KindEpisode || k == catalog.KindMovie {
			if err := chapters(ctx, tx, args, k, d.Versions); err != nil {
				return err
			}
		}
		inOrder(d.Versions)
		if d.Extras, err = summaries(ctx, tx, extras, args); err != nil {
			return err
		}

		if err := byKind(ctx, tx, vw, args, &d); err != nil {
			return err
		}

		d.Collections, err = rowsOf(ctx, tx, workCollections, args, scanCollection)
		return err
	})
	if err != nil {
		return WorkDetail{}, err
	}
	head := []WorkSummary{d.Summary}
	lists := [][]WorkSummary{head, d.Extras}
	var next []WorkSummary
	if d.NextUp != nil {
		next = []WorkSummary{*d.NextUp}
		lists = append(lists, next)
	}
	if err := s.decorate(ctx, vw, lists...); err != nil {
		return WorkDetail{}, err
	}
	if err := s.withCollectionImages(ctx, vw, d.Collections); err != nil {
		return WorkDetail{}, err
	}
	d.Summary = head[0]
	if next != nil {
		*d.NextUp = next[0]
	}
	return d, nil
}

// byKind reads the fields only some kinds' pages have: the child count of
// a series or season, a series' next up, an episode's neighbours, and a
// book's own fields.
func byKind(ctx context.Context, tx *sql.Tx, vw viewer, args []any, d *WorkDetail) error {
	if k := d.Summary.Kind; k == catalog.KindSeries || k == catalog.KindSeason {
		if err := rowOf(ctx, tx, childCount, args, &d.Children); err != nil {
			return err
		}
	}
	switch d.Summary.Kind {
	case catalog.KindSeries:
		next, err := scanSummary(queryRow(ctx, tx, seriesNextUp, vw.args(sql.Named("series", d.Summary.ID))))
		switch {
		case errors.Is(err, sql.ErrNoRows):
		case err != nil:
			return err
		default:
			d.NextUp = &next
		}
	case catalog.KindEpisode:
		if err := rowOf(ctx, tx, adjacentEpisodes, args, &d.PreviousEpisode, &d.NextEpisode); err != nil {
			return err
		}
	case catalog.KindBook:
		var err error
		if d.Book, err = book(ctx, tx, args); err != nil {
			return err
		}
	}
	return nil
}

// chapters fills in each version's chapters.
//
// A book's chapters go on the version's timeline, timed as playback times
// them. Each part's own chapters are shifted by the length of the parts
// before it. In a version of several parts, a part with no chapters becomes
// one chapter that spans it, named after its file.
//
// An episode's or a movie's chapters go on each part as they are in its
// file.
func chapters(ctx context.Context, q querier, args []any, kind catalog.WorkKind, vs []Version) error {
	type row struct {
		version, file  id.ID
		relPath        string
		durationMs     int64
		startMs, endMs sql.NullInt64
		title          sql.NullString
	}
	rows, err := rowsOf(ctx, q, versionChapters, args, func(r scanner, x *row) error {
		return r.Scan(&x.version, &x.file, &x.relPath, &x.durationMs, &x.startMs, &x.endMs, &x.title)
	})
	if err != nil {
		return err
	}
	byVersion := map[id.ID]*Version{}
	for i := range vs {
		byVersion[vs[i].ID] = &vs[i]
	}
	var offset time.Duration
	for i, r := range rows {
		v := byVersion[r.version]
		if v == nil {
			continue
		}
		switch {
		case i == 0 || r.version != rows[i-1].version:
			offset = 0
		case r.file != rows[i-1].file:
			offset += time.Duration(rows[i-1].durationMs) * time.Millisecond
		}
		if kind != catalog.KindBook {
			if p := partOf(v, r.file); p != nil && r.startMs.Valid {
				p.Chapters = append(p.Chapters, Chapter{
					Start: time.Duration(r.startMs.Int64) * time.Millisecond, End: time.Duration(r.endMs.Int64) * time.Millisecond, Title: r.title.String,
				})
			}
			continue
		}
		ms := func(n int64) time.Duration { return offset + time.Duration(n)*time.Millisecond }
		switch {
		case r.startMs.Valid:
			v.ChapterList = append(v.ChapterList, Chapter{Start: ms(r.startMs.Int64), End: ms(r.endMs.Int64), Title: r.title.String})
		case len(v.Parts) > 1:
			name := path.Base(r.relPath)
			v.ChapterList = append(v.ChapterList, Chapter{
				Start: offset, End: ms(r.durationMs), Title: strings.TrimSuffix(name, path.Ext(name)),
			})
		}
	}
	return nil
}

// covers lists the works each version covers.
func covers(ctx context.Context, q querier, args []any, vs []Version) error {
	i := 0
	return each(ctx, q, versionWorks, args, func(r scanner) error {
		var version, work id.ID
		if err := r.Scan(&version, &work); err != nil {
			return err
		}
		for i < len(vs) && vs[i].ID != version {
			i++
		}
		if i < len(vs) {
			vs[i].Works = append(vs[i].Works, work)
		}
		return nil
	})
}

// partOf returns v's part for file, or nil if v has none.
func partOf(v *Version, file id.ID) *Part {
	for i := range v.Parts {
		if v.Parts[i].File == file {
			return &v.Parts[i]
		}
	}
	return nil
}

// inOrder orders versions as catalog.CompareVersions does, so a cut's
// versions sit together, the best picture first.
func inOrder(vs []Version) {
	slices.SortStableFunc(vs, func(a, b Version) int { return catalog.CompareVersions(a.order(), b.order()) })
}

func (v Version) order() catalog.VersionOrder {
	o := catalog.VersionOrder{Edition: v.Edition, Size: v.Size}
	if i := slices.IndexFunc(v.Tracks, func(t Track) bool { return t.Type == "video" }); i >= 0 {
		o.Height = v.Tracks[i].Height
	}
	return o
}

// versions assembles each version from its parts and its first part's
// tracks, both in version order.
func versions(ctx context.Context, q querier, args []any, admin bool) ([]Version, error) {
	var out []Version
	chapters := map[id.ID][]int{}
	err := each(ctx, q, versionParts, args, func(r scanner) error {
		var version, lib id.ID
		var edition, folder, rel string
		var durationMs int64
		var available bool
		var n int
		var part Part
		if err := r.Scan(&version, &edition, &part.File, &lib, &folder, &rel, &part.Size, &part.Container, &durationMs,
			&part.Bitrate, &available, &n); err != nil {
			return err
		}
		part.Name, part.Duration, part.Unavailable = path.Base(rel), time.Duration(durationMs)*time.Millisecond, !available
		if admin {
			part.Path = filepath.Join(folder, rel)
		}
		if len(out) == 0 || out[len(out)-1].ID != version {
			out = append(out, Version{ID: version, Library: lib, Edition: edition})
		}
		ver := &out[len(out)-1]
		ver.Parts = append(ver.Parts, part)
		ver.Duration += part.Duration
		ver.Size += part.Size
		ver.Unavailable = ver.Unavailable || part.Unavailable
		chapters[ver.ID] = append(chapters[ver.ID], n)
		return nil
	})
	if err != nil {
		return nil, err
	}
	for i := range out {
		for _, n := range chapters[out[i].ID] {
			if n == 0 && len(out[i].Parts) > 1 {
				n = 1 // the part is a chapter, as playback times it
			}
			out[i].Chapters += n
		}
	}
	i := 0
	err = each(ctx, q, versionTracks, args, func(r scanner) error {
		var version id.ID
		var t Track
		if err := r.Scan(&version, &t.ID, &t.Source, &t.Type, &t.Codec, &t.Language, &t.Title, &t.Default, &t.Forced, &t.HearingImpaired,
			&t.Channels, &t.Width, &t.Height, &t.Range, &t.DolbyVisionProfile, &t.Profile, &t.ChannelLayout, &t.Commentary); err != nil {
			return err
		}
		for i < len(out) && out[i].ID != version {
			i++
		}
		if i < len(out) {
			out[i].Tracks = append(out[i].Tracks, t)
		}
		return nil
	})
	return out, err
}
