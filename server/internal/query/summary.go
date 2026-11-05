package query

import (
	"context"
	"database/sql"
	"time"

	"github.com/mmcdole/kivo/internal/catalog"
	"github.com/mmcdole/kivo/internal/id"
	"github.com/mmcdole/kivo/internal/users"
)

// WorkSummary is a work as every listing shows it: library grids, Home's rows, a
// series' seasons and episodes, search results and the top of a detail page.
type WorkSummary struct {
	ID   id.ID
	Kind catalog.WorkKind
	// Title is the work's title. For the first episode a multi-episode
	// version covers, it is every covered episode's title joined with " / ".
	Title     string
	SortTitle string // the title without a leading article, for the letter rail
	// Parent is a season's series, an episode's season or an extra's work.
	// Root is the top of the work's tree, such as an episode's series. Both
	// are zero for a top-level work.
	Parent, Root id.ID
	// SeriesTitle is the title of a season's or episode's series.
	SeriesTitle string
	Year        int
	// Number is a season's number, or an episode's number in its season.
	// ParentNumber is an episode's season number. The api package names
	// them by kind.
	Number, ParentNumber int
	// NumberEnd is the number of the last episode a multi-episode version
	// with no gone file covers, set on the first episode it covers (E1 to
	// E2). It is zero otherwise.
	NumberEnd   int
	ReleaseDate string // YYYY, YYYY-MM or YYYY-MM-DD when known; an episode's is its air date
	ExtraType   string // an extra's type: trailer, featurette, and so on
	Edition     string // a movie's label from its folder's edition tag, such as "Final Cut"
	// Duration is the running time of the work's first version with no
	// gone file. It is zero for a series or season.
	Duration   time.Duration
	Added      time.Time
	Images     Images
	MatchState MatchState
	// Unavailable means none of the work's versions can play right now,
	// because their folders can't be reached.
	Unavailable bool
	// Unplayed counts a series' or season's episodes the user hasn't
	// played.
	Unplayed int
	// Bundled is how many episodes a season's or series' summary bundles in
	// "Recently Added". It is zero elsewhere.
	Bundled int
	// Episodes is how many episodes a season's list shows, set when the
	// season is listed among its series' children. It is zero elsewhere.
	Episodes int
	// Overview is an episode's synopsis, set when the episode is listed
	// among its season's children. It is empty elsewhere.
	Overview string
	// Community is the provider's audience rating for a movie, series or
	// episode with votes. It is nil otherwise.
	Community *catalog.CommunityRating
	// ContentRating is the content rating for the server's content rating
	// country, such as "PG-13". It is empty when unrated.
	ContentRating string
	Userstate     Userstate
	// Book holds a book's authors, narrators and series. It is nil for
	// other kinds.
	Book *BookInfo
	// Library is the library the work is in, if the Viewer sees it. It is
	// zero otherwise.
	Library id.ID
	// Path is set only in the Unmatched list. It is the folder a series or
	// book was parsed from, or the file for other kinds.
	Path string

	// library is the library the summary is listed under. The Unmatched
	// list keys on it.
	library id.ID
}

// MatchState is how far identification has got with a work.
//
// TODO: an Identifying state for a provisional work whose first identify
// pass is waiting on a provider; identify doesn't record that.
type MatchState string

// Match states.
const (
	Matched   MatchState = "matched"
	Unmatched MatchState = "unmatched" // a provisional work, from a parsed name
)

// Userstate is the user's state for a work: played, progress, favorite and rating.
type Userstate struct {
	Played    bool
	PlayCount int
	// Position is where playing resumes for a work in progress. It is zero
	// otherwise.
	Position time.Duration
	// Version is the version the user last played, which Position is on.
	// It is zero if never, and the version may be gone.
	Version  id.ID
	Favorite bool
	Rating   *int // the user's own rating
	// LastPlayed is when the user last played the work. For a series, it is
	// when the user last played any of its episodes. It is zero if never.
	LastPlayed time.Time
}

// summaryColumns are a WorkSummary's columns, with the work aliased as w and
// the user's state as us. Every statement that returns summaries selects
// them first, and scanSummary reads them.
var summaryColumns = `w.id, w.kind, w.title, w.sort_title, w.parent_id, w.root_id,
  CASE WHEN w.kind IN ('season', 'episode') THEN (SELECT r.title FROM work r WHERE r.id = w.root_id) ELSE '' END AS series_title,
  coalesce(w.year, 0) AS year, coalesce(w.parent_number, 0) AS parent_number, coalesce(w.number, 0) AS number,
  coalesce(CASE WHEN w.kind = 'episode' THEN (
    SELECT max(o.number) FROM media_version_work b JOIN work o ON o.id = b.work_id
    WHERE b.version_id = (SELECT a.version_id FROM media_version_work a WHERE a.work_id = w.id AND a.ord = 0
      AND EXISTS (SELECT 1 FROM media_version_work c WHERE c.version_id = a.version_id AND c.ord > 0)
      AND NOT EXISTS (SELECT 1 FROM media_file g WHERE g.version_id = a.version_id AND g.status = 'gone')))
  END, 0) AS number_end,
  coalesce(CASE WHEN w.kind = 'episode' THEN (
    SELECT group_concat(t, ' / ') FROM (SELECT o.title AS t FROM media_version_work b JOIN work o ON o.id = b.work_id
      WHERE b.version_id = (SELECT a.version_id FROM media_version_work a WHERE a.work_id = w.id AND a.ord = 0
        AND EXISTS (SELECT 1 FROM media_version_work c WHERE c.version_id = a.version_id AND c.ord > 0)
        AND NOT EXISTS (SELECT 1 FROM media_file g WHERE g.version_id = a.version_id AND g.status = 'gone'))
      ORDER BY b.ord))
  END, '') AS combined_title,
  coalesce(w.release_date, '') AS release_date, coalesce(w.extra_type, '') AS extra_type, coalesce(w.edition, '') AS edition,
  coalesce((SELECT sum(df.duration_ms) FROM media_file df
    WHERE df.version_id = (SELECT dw.version_id FROM media_version_work dw WHERE dw.work_id = w.id
      AND NOT EXISTS (SELECT 1 FROM media_file g WHERE g.version_id = dw.version_id AND g.status = 'gone'))), 0) AS duration_ms,
  w.added_at, w.provisional,
  w.kind NOT IN ('series', 'season') AND NOT EXISTS (
    SELECT 1 FROM media_version_work aw WHERE aw.work_id = w.id AND NOT EXISTS (
      SELECT 1 FROM media_file af JOIN library_folder ar ON ar.id = af.folder_id
      WHERE af.version_id = aw.version_id AND (af.status = 'gone' OR NOT ar.reachable
        OR NOT ` + granted("ar.library_id") + `))) AS unavailable,
  coalesce(us.played, 0) AS played, coalesce(us.play_count, 0) AS play_count,
  CASE WHEN us.resumable THEN us.position_ms ELSE 0 END AS position_ms,
  coalesce(us.favorite, 0) AS favorite, us.rating,
  coalesce(CASE WHEN w.kind = 'series' THEN (SELECT max(su.last_played_at) FROM work sw
    CROSS JOIN user_state su ON su.user_id = @user AND su.work_id = sw.id WHERE sw.root_id = w.id)
  ELSE us.last_played_at END, 0) AS last_played_at,
  CASE w.kind
    WHEN 'series' THEN (SELECT count(*) FROM work e WHERE e.root_id = w.id AND e.kind = 'episode'
      AND EXISTS (SELECT 1 FROM work_library el WHERE el.work_id = e.id AND ` + granted("el.library_id") + `)
      AND NOT EXISTS (SELECT 1 FROM user_state eu WHERE eu.user_id = @user AND eu.work_id = e.id AND eu.played))
    WHEN 'season' THEN (SELECT count(*) FROM work e WHERE e.parent_id = w.id AND e.kind = 'episode'
      AND EXISTS (SELECT 1 FROM work_library el WHERE el.work_id = e.id AND ` + granted("el.library_id") + `)
      AND NOT EXISTS (SELECT 1 FROM user_state eu WHERE eu.user_id = @user AND eu.work_id = e.id AND eu.played))
    ELSE 0 END AS unplayed,
  w.community_rating, w.community_votes, w.content_rating, us.version_id`

// scanSummary reads a row that starts with the summary columns. It scans
// any columns after them into more.
func scanSummary(r scanner, more ...any) (WorkSummary, error) {
	var (
		c                                             WorkSummary
		year, parentNumber, number, numberEnd         int64
		unplayed, playCount                           int64
		durationMs, addedAt, positionMs, lastPlayedAt int64
		root                                          id.ID
		combinedTitle                                 string
		provisional                                   bool
		rating, votes                                 sql.NullInt64
		community                                     sql.NullFloat64
		contentRating                                 sql.NullString
	)
	err := r.Scan(append([]any{&c.ID, &c.Kind, &c.Title, &c.SortTitle, &c.Parent, &root, &c.SeriesTitle, &year, &parentNumber, &number,
		&numberEnd, &combinedTitle, &c.ReleaseDate, &c.ExtraType, &c.Edition, &durationMs,
		&addedAt, &provisional, &c.Unavailable, &c.Userstate.Played, &playCount, &positionMs, &c.Userstate.Favorite,
		&rating, &lastPlayedAt, &unplayed, &community, &votes, &contentRating, &c.Userstate.Version}, more...)...)
	if err != nil {
		return WorkSummary{}, err
	}
	c.Year, c.ParentNumber, c.Number = int(year), int(parentNumber), int(number)
	c.Unplayed, c.ContentRating = int(unplayed), contentRating.String
	c.Duration, c.Added = time.Duration(durationMs)*time.Millisecond, time.UnixMilli(addedAt).UTC()
	c.Userstate.PlayCount, c.Userstate.Position = int(playCount), time.Duration(positionMs)*time.Millisecond
	c.MatchState = Matched
	if provisional {
		c.MatchState = Unmatched
	}
	if root != c.ID {
		c.Root = root
	}
	// A series or season has no state of its own. It's played when every
	// episode is played.
	if c.Kind == catalog.KindSeries || c.Kind == catalog.KindSeason {
		c.Userstate.Played = c.Unplayed == 0
	}
	if numberEnd > number {
		c.NumberEnd = int(numberEnd)
		c.Title = combinedTitle
	}
	if rating.Valid {
		n := int(rating.Int64)
		c.Userstate.Rating = &n
	}
	if lastPlayedAt != 0 {
		c.Userstate.LastPlayed = time.UnixMilli(lastPlayedAt).UTC()
	}
	if community.Valid && votes.Valid {
		c.Community = &catalog.CommunityRating{Rating: community.Float64, Votes: int(votes.Int64)}
	}
	return c, nil
}

// summaries runs a statement that selects the summary columns and returns
// its summaries. It never returns a nil slice.
func summaries(ctx context.Context, q querier, st statement, args []any) ([]WorkSummary, error) {
	out, err := rowsOf(ctx, q, st, args, func(r scanner, c *WorkSummary) (err error) {
		*c, err = scanSummary(r)
		return err
	})
	if out == nil {
		out = []WorkSummary{}
	}
	return out, err
}

var summaryOne = stmt("WorkSummary", `
SELECT `+summaryColumns+`
FROM work w
LEFT JOIN user_state us ON us.user_id = @user AND us.work_id = w.id
WHERE w.id = @work
  AND `+visible)

// summary reads one work's summary. A work the Viewer may not see is
// sql.ErrNoRows.
func summary(ctx context.Context, q querier, vw viewer, work id.ID) (WorkSummary, error) {
	return scanSummary(queryRow(ctx, q, summaryOne, vw.args(sql.Named("work", work))))
}

// Userstate returns the Viewer's state for a work the Viewer sees. Any other
// work is ErrNotFound.
func (s *Service) Userstate(ctx context.Context, v users.Viewer, work id.ID) (Userstate, error) {
	c, err := summary(ctx, s.db.Reader(), viewerOf(v), work)
	if err != nil {
		return Userstate{}, notFound(err)
	}
	return c.Userstate, nil
}

// Summaries returns the summaries of the works the Viewer sees, in the
// order asked. Works the Viewer doesn't see are left out.
func (s *Service) Summaries(ctx context.Context, v users.Viewer, works []id.ID) ([]WorkSummary, error) {
	vw := viewerOf(v)
	var out []WorkSummary
	err := s.db.Read(ctx, func(tx *sql.Tx) error {
		byWork, err := summariesByID(ctx, tx, vw, works)
		for _, w := range works {
			if c, ok := byWork[w]; ok {
				out = append(out, c)
			}
		}
		return err
	})
	if err != nil {
		return nil, err
	}
	if err := s.decorate(ctx, vw, out); err != nil {
		return nil, err
	}
	return out, nil
}

// decorate attaches what every summary carries besides its own row: its
// artwork, its library, and a book's authors, narrators and series.
func (s *Service) decorate(ctx context.Context, vw viewer, lists ...[]WorkSummary) error {
	if err := s.withImages(ctx, vw, lists...); err != nil {
		return err
	}
	if err := s.withLibrary(ctx, vw, lists...); err != nil {
		return err
	}
	return s.withBooks(ctx, vw, lists...)
}

// librariesOf lists, for each of a set of works, the library the work is in
// if the Viewer sees it. The libraries are attached to summaries after the
// summaries are read.
var librariesOf = stmt("LibrariesOf", `
SELECT wl.work_id, wl.library_id
FROM work_library wl
JOIN library l ON l.id = wl.library_id
WHERE wl.work_id IN (SELECT unhex(value) FROM json_each(@works))
  AND l.deleting_at IS NULL
  AND `+libraryVisible)

// withLibrary attaches to each summary the library its work is in, if the
// Viewer sees it, using one statement for all of the summaries.
func (s *Service) withLibrary(ctx context.Context, vw viewer, lists ...[]WorkSummary) error {
	var works []id.ID
	byWork := map[id.ID]id.ID{}
	for _, l := range lists {
		for _, c := range l {
			if _, ok := byWork[c.ID]; !ok {
				byWork[c.ID] = id.ID{}
				works = append(works, c.ID)
			}
		}
	}
	if len(works) == 0 {
		return nil
	}
	type row struct{ work, library id.ID }
	rows, err := rowsOf(ctx, s.db.Reader(), librariesOf, vw.args(sql.Named("works", id.Set(works...))), func(r scanner, x *row) error {
		return r.Scan(&x.work, &x.library)
	})
	if err != nil {
		return err
	}
	for _, r := range rows {
		byWork[r.work] = r.library
	}
	for _, l := range lists {
		for i := range l {
			l[i].Library = byWork[l[i].ID]
		}
	}
	return nil
}

// summariesOf reads the summaries of a set of works in one statement, rather
// than one WorkSummary statement per work. Each work is looked up by its
// primary key, and the rows come in no particular order. A work the Viewer
// may not see has no row.
var summariesOf = stmt("WorkSummariesOf", `
SELECT `+summaryColumns+`
FROM json_each(@works) j
JOIN work w ON w.id = unhex(j.value)
LEFT JOIN user_state us ON us.user_id = @user AND us.work_id = w.id
WHERE `+visible)

// summariesByID reads the summaries of works in one statement, keyed by
// work. A work the Viewer may not see is missing from the map.
func summariesByID(ctx context.Context, q querier, vw viewer, works []id.ID) (map[id.ID]WorkSummary, error) {
	cs, err := summaries(ctx, q, summariesOf, vw.args(sql.Named("works", id.Set(works...))))
	if err != nil {
		return nil, err
	}
	out := make(map[id.ID]WorkSummary, len(cs))
	for _, c := range cs {
		out[c.ID] = c
	}
	return out, nil
}
