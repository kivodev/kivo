package catalog

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/mmcdole/kivo/internal/catalog/dbq"
	"github.com/mmcdole/kivo/internal/id"
	"github.com/mmcdole/kivo/internal/store"
)

// Work is a work's identity and matching state. Root is the top of the
// work's tree, such as an episode's series. A work with no parent is its
// own root.
type Work struct {
	ID, Parent, Root id.ID
	Kind             WorkKind
	Title, SortTitle string
	Year             int
	// Number is a season's number, or an episode's number within its
	// season. ParentNumber is an episode's season number.
	Number, ParentNumber int
	Provisional, Locked  bool
	LockedFields         []string
	ExternalIDs          []ExternalID
}

// WorkKind is what a work is.
type WorkKind string

// The work kinds.
const (
	KindMovie   WorkKind = "movie"
	KindSeries  WorkKind = "series"
	KindSeason  WorkKind = "season"
	KindEpisode WorkKind = "episode"
	KindExtra   WorkKind = "extra"
	KindVideo   WorkKind = "video" // an item of a videos library
	KindBook    WorkKind = "book"  // an item of an audiobooks library
)

// ExternalID is a provider's ID for a work, such as {tmdb, movie, 438631},
// or for a person, such as {tmdb, person, 17419}.
type ExternalID struct{ Provider, Kind, ID string }

// WorkInput is a work for EnsureWork to find or create.
type WorkInput struct {
	Kind      WorkKind
	Parent    id.ID // set for seasons, episodes and extras
	Title     string
	SortTitle string // derived from Title when empty
	Year      int
	// Number is the work's place among its siblings. For a season it is the
	// season number, with 0 for specials. For an episode it is the episode
	// number within its season. ParentNumber is an episode's season number.
	Number, ParentNumber int
	ReleaseDate          string // YYYY, YYYY-MM or YYYY-MM-DD
	ExtraType            string
	// Edition is a movie's label, such as "Final Cut", from its folder's
	// edition tag.
	Edition     string
	Provisional bool // made from a parsed file name before any match
	ExternalIDs []ExternalID
}

// WorkMetadata is a work's descriptive data. UpdateMetadata replaces each
// field unless an admin locked it. An empty Title is never written.
type WorkMetadata struct {
	Title, SortTitle, OriginalTitle string
	// OriginalLanguage is the language the work was made in, as the
	// provider gives it ("en", "ja"). Seasons and episodes use their
	// series' language.
	OriginalLanguage string
	AltTitles        []string
	Year             int
	Overview         string
	// ReleaseDate is when the work came out, as YYYY, YYYY-MM or
	// YYYY-MM-DD. It is a movie's release date, an episode's air date or a
	// book's publication date. A book's Year is derived from it.
	ReleaseDate string
	// ContentRating is the rating in the server's content rating country,
	// such as "PG-13" or "TV-MA". ContentRatingAge is the minimum age the
	// rating means. Both are empty when the work is unrated. They are
	// written together as one field.
	ContentRating    string
	ContentRatingAge *int
	Community        *CommunityRating // nil when the provider has no votes
	TrailerURL       string           // a movie's trailer; empty for none
	Genres, Tags     []string
	Companies        []string // studios, networks or a book's publisher
	Credits          []Credit
	Collection       *CollectionRef // nil leaves collections alone
	Book             *BookMetadata  // nil for any kind but a book
}

// CommunityRating is a provider's audience rating. Rating is the average
// vote, from 0 to 10, and Votes is how many votes it averages.
type CommunityRating struct {
	Rating float64
	Votes  int
}

// Credit is a person's role in a work. Credits of the same role are ordered
// by their place in the slice. The credited person is the one named by any
// of ExternalIDs. Failing that, it is the oldest person with that name whose
// IDs don't conflict, and failing that, a new person.
type Credit struct {
	Name, Role, Character string
	ExternalIDs           []ExternalID
}

// CollectionRef puts a work in a collection, keyed like "tmdb:1234".
type CollectionRef struct {
	Key, Name string
	Order     int
}

// Fields an admin can edit, which then lock against automatic updates.
const (
	FieldTitle         = "title"
	FieldSortTitle     = "sort_title"
	FieldOriginalTitle = "original_title"
	FieldYear          = "year"
	FieldOverview      = "overview"
	FieldReleaseDate   = "release_date" // a book's year too
	FieldContentRating = "content_rating"
	FieldGenres        = "genres"
	FieldTags          = "tags"
	FieldCompanies     = "companies" // studios, networks, a book's publisher
	FieldCredits       = "credits"

	// FieldCommunityRating, FieldOriginalLanguage and FieldTrailerURL are
	// written only by providers. No admin edit names them, so they never
	// lock.
	FieldCommunityRating  = "community_rating"
	FieldOriginalLanguage = "original_language"
	FieldTrailerURL       = "trailer_url"
)

// EnsureWork returns the work that in describes, creating it if needed.
//
// A movie, series, video, book or extra is always created new, because a
// root work belongs to its folder and is never found by its description. A
// season is found by its number under its series. An episode is found by
// any of in.ExternalIDs within its series, or else by its number in its
// season. The external IDs are added to the work found.
//
// A provisional episode is found only for a provisional input, so a match
// never adopts one. Instead, identify re-points the episode's versions and
// merges it.
func (c *Catalog) EnsureWork(ctx context.Context, tx *store.Tx, in WorkInput) (id.ID, error) {
	if err := in.validate(); err != nil {
		return id.ID{}, err
	}
	q := dbq.New(tx)
	work, err := existingWork(ctx, q, in)
	if err != nil {
		return id.ID{}, err
	}
	if work.IsZero() {
		return insertWork(ctx, tx, in)
	}
	if err := addExternalIDs(ctx, q, work, in.ExternalIDs); err != nil {
		return id.ID{}, err
	}
	return work, nil
}

// existingWork returns the episode named by any of in's external IDs within
// its series. Failing that, it returns the season or episode with in's
// number under its parent. It returns zero when there is none.
func existingWork(ctx context.Context, q *dbq.Queries, in WorkInput) (id.ID, error) {
	if in.Kind == KindEpisode && len(in.ExternalIDs) > 0 {
		season, err := q.GetWork(ctx, in.Parent)
		if err != nil {
			return id.ID{}, notFound(err)
		}
		for _, x := range in.ExternalIDs {
			w, err := q.EpisodeByExternalID(ctx, dbq.EpisodeByExternalIDParams{Provider: x.Provider, Kind: x.Kind, ExtID: x.ID, RootID: season.RootID})
			if !errors.Is(err, sql.ErrNoRows) {
				return w, err
			}
		}
	}
	return numberedChild(ctx, q, in)
}

// numberedChild returns the season or episode with in's number under its
// parent, or zero for none. A child that has external IDs other than in's
// is a different episode that a new ordering is renumbering, so it doesn't
// count.
func numberedChild(ctx context.Context, q *dbq.Queries, in WorkInput) (id.ID, error) {
	number, _ := numbers(in)
	var found id.ID
	var err error
	switch in.Kind {
	case KindSeason:
		found, err = q.ChildSeason(ctx, dbq.ChildSeasonParams{ParentID: in.Parent, Number: number})
	case KindEpisode:
		found, err = q.ChildEpisode(ctx, dbq.ChildEpisodeParams{ParentID: in.Parent, Number: number, Provisional: in.Provisional})
	default:
		return id.ID{}, nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		return id.ID{}, nil
	}
	if err != nil || len(in.ExternalIDs) == 0 {
		return found, err
	}
	xs, err := q.ExternalIDs(ctx, found)
	if err != nil || len(xs) > 0 {
		return id.ID{}, err
	}
	return found, nil
}

// insertWork creates the work that in describes, with its external IDs.
func insertWork(ctx context.Context, tx *store.Tx, in WorkInput) (id.ID, error) {
	q := dbq.New(tx)
	work := id.New()
	root := work
	if !in.Parent.IsZero() {
		p, err := q.GetWork(ctx, in.Parent)
		if err != nil {
			return id.ID{}, notFound(err)
		}
		root = p.RootID
	}
	sort := in.SortTitle
	if sort == "" {
		sort = SortTitle(in.Title)
	}
	number, parent := numbers(in)
	_, err := q.InsertWork(ctx, dbq.InsertWorkParams{
		ID: work, Kind: string(in.Kind), ParentID: in.Parent, RootID: root, Title: in.Title, SortTitle: sort,
		Year: nullInt(in.Year), Number: number, ParentNumber: parent,
		ReleaseDate: nullString(in.ReleaseDate), ExtraType: nullString(in.ExtraType), Edition: nullString(in.Edition),
		Provisional: in.Provisional, AddedAt: time.Now().UnixMilli(),
	})
	if err != nil {
		return id.ID{}, err
	}
	if err := addExternalIDs(ctx, q, work, in.ExternalIDs); err != nil {
		return id.ID{}, err
	}
	if err := reindex(ctx, tx, work); err != nil {
		return id.ID{}, err
	}
	return work, nil
}

// numbers returns a work's number and parent number as they are stored. A
// season's number and an episode's season number are always stored, so
// specials store 0. Any other number of 0 means none and is stored as
// NULL. Only an episode has a parent number.
func numbers(in WorkInput) (number, parent sql.NullInt64) {
	switch in.Kind {
	case KindSeason:
		return sql.NullInt64{Int64: int64(in.Number), Valid: true}, sql.NullInt64{}
	case KindEpisode:
		return nullInt(in.Number), sql.NullInt64{Int64: int64(in.ParentNumber), Valid: true}
	}
	return nullInt(in.Number), sql.NullInt64{}
}

// addExternalIDs adds the IDs a work doesn't have yet.
func addExternalIDs(ctx context.Context, q *dbq.Queries, work id.ID, xs []ExternalID) error {
	if len(xs) == 0 {
		return nil
	}
	have, err := q.ExternalIDs(ctx, work)
	if err != nil {
		return err
	}
	for _, x := range xs {
		if slices.Contains(have, idRow(x)) {
			continue
		}
		err := q.InsertExternalID(ctx, dbq.InsertExternalIDParams{Provider: x.Provider, Kind: x.Kind, ExtID: x.ID, WorkID: work})
		if err != nil {
			return err
		}
	}
	return nil
}

// idRow converts an external ID to the row type that ExternalIDs returns.
func idRow(x ExternalID) dbq.ExternalIDsRow {
	return dbq.ExternalIDsRow{Provider: x.Provider, Kind: x.Kind, ExtID: x.ID}
}

func (in WorkInput) validate() error {
	needsParent := in.Kind == KindSeason || in.Kind == KindEpisode || in.Kind == KindExtra
	switch {
	case !slices.Contains([]WorkKind{KindMovie, KindSeries, KindSeason, KindEpisode, KindExtra, KindVideo, KindBook}, in.Kind):
		return fmt.Errorf("%w: work kind %q", ErrInvalid, in.Kind)
	case in.Title == "":
		return fmt.Errorf("%w: a work needs a title", ErrInvalid)
	case needsParent != !in.Parent.IsZero():
		return fmt.Errorf("%w: a %s %s a parent", ErrInvalid, in.Kind, map[bool]string{true: "needs", false: "can't have"}[needsParent])
	case in.Edition != "" && in.Kind != KindMovie:
		return fmt.Errorf("%w: only movies have edition labels", ErrInvalid)
	}
	return nil
}

// Work returns a work's identity and matching state.
func (c *Catalog) Work(ctx context.Context, work id.ID) (Work, error) {
	w, err := c.q.GetWork(ctx, work)
	if err != nil {
		return Work{}, notFound(err)
	}
	xs, err := c.q.ExternalIDs(ctx, work)
	if err != nil {
		return Work{}, err
	}
	out := Work{
		ID: w.ID, Parent: w.ParentID, Root: w.RootID, Kind: WorkKind(w.Kind), Title: w.Title,
		SortTitle: w.SortTitle, Year: int(w.Year.Int64), Number: int(w.Number.Int64), ParentNumber: int(w.ParentNumber.Int64),
		Provisional: w.Provisional, Locked: w.Locked, LockedFields: lockedFields(w),
	}
	for _, x := range xs {
		out.ExternalIDs = append(out.ExternalIDs, ExternalID{Provider: x.Provider, Kind: x.Kind, ID: x.ExtID})
	}
	return out, nil
}

// UpdateMetadata writes metadata from a provider or from local files. It
// leaves alone the fields an admin edited.
func (c *Catalog) UpdateMetadata(ctx context.Context, tx *store.Tx, work id.ID, m WorkMetadata) error {
	q := dbq.New(tx)
	cur, err := q.GetWork(ctx, work)
	if err != nil {
		return notFound(err)
	}
	locked := lockedFields(cur)
	take := func(f string) bool { return !slices.Contains(locked, f) }
	return c.writeMetadata(ctx, tx, cur, m, take, locked)
}

// EditMetadata applies an admin's edit. It writes the named fields and
// locks them against automatic updates. identify.EditWork calls it, with
// SetOrdering, in one transaction.
func (c *Catalog) EditMetadata(ctx context.Context, tx *store.Tx, work id.ID, m WorkMetadata, fields ...string) error {
	return c.editMetadata(ctx, tx, work, m, fields)
}

// SetASIN writes a book's ASIN as an admin's edit, which locks it.
func (c *Catalog) SetASIN(ctx context.Context, tx *store.Tx, book id.ID, asin string) error {
	return c.editMetadata(ctx, tx, book, WorkMetadata{Book: &BookMetadata{ASIN: asin}}, []string{FieldASIN})
}

func (c *Catalog) editMetadata(ctx context.Context, tx *store.Tx, work id.ID, m WorkMetadata, fields []string) error {
	q := dbq.New(tx)
	cur, err := q.GetWork(ctx, work)
	if err != nil {
		return notFound(err)
	}
	return c.edit(ctx, tx, cur, m, fields)
}

// edit writes the named fields of an admin's edit and locks them.
func (c *Catalog) edit(ctx context.Context, tx *store.Tx, cur dbq.Work, m WorkMetadata, fields []string) error {
	locked := lockedFields(cur)
	for _, f := range fields {
		if !slices.Contains(locked, f) {
			locked = append(locked, f)
		}
	}
	slices.Sort(locked)
	return c.writeMetadata(ctx, tx, cur, m, func(f string) bool { return slices.Contains(fields, f) }, locked)
}

// writeMetadata writes over cur the fields of m that take allows. It stores
// locked as the work's locked fields.
func (c *Catalog) writeMetadata(ctx context.Context, tx *store.Tx, cur dbq.Work, m WorkMetadata, take func(field string) bool, locked []string) error {
	if m.Book != nil && WorkKind(cur.Kind) != KindBook {
		return fmt.Errorf("%w: a book's fields are for books only", ErrInvalid)
	}
	q := dbq.New(tx)
	p := dbq.UpdateWorkMetadataParams{
		ID: cur.ID, Title: cur.Title, SortTitle: cur.SortTitle, OriginalTitle: cur.OriginalTitle, OriginalLanguage: cur.OriginalLanguage,
		AltTitles: cur.AltTitles, Year: cur.Year, Overview: cur.Overview, ReleaseDate: cur.ReleaseDate,
		ContentRating: cur.ContentRating, ContentRatingAge: cur.ContentRatingAge,
		CommunityRating: cur.CommunityRating, CommunityVotes: cur.CommunityVotes, TrailerUrl: cur.TrailerUrl,
	}
	if take(FieldTitle) && m.Title != "" {
		p.Title = m.Title
		if !slices.Contains(locked, FieldSortTitle) {
			p.SortTitle = SortTitle(m.Title)
		}
	}
	if take(FieldSortTitle) && m.SortTitle != "" {
		p.SortTitle = m.SortTitle
	}
	if take(FieldOriginalTitle) {
		p.OriginalTitle = nullString(m.OriginalTitle)
	}
	if take(FieldOriginalLanguage) {
		p.OriginalLanguage = nullString(m.OriginalLanguage)
	}
	if take(FieldTitle) && m.AltTitles != nil {
		b, _ := json.Marshal(m.AltTitles)
		p.AltTitles = nullString(string(b))
	}
	if take(FieldYear) {
		p.Year = nullInt(m.Year)
	}
	if take(FieldOverview) {
		p.Overview = nullString(m.Overview)
	}
	if take(FieldReleaseDate) {
		p.ReleaseDate = nullString(strings.TrimSpace(m.ReleaseDate))
		if WorkKind(cur.Kind) == KindBook {
			p.Year = nullInt(releaseYear(p.ReleaseDate.String))
		}
	}
	if take(FieldContentRating) {
		p.ContentRating, p.ContentRatingAge = nullString(m.ContentRating), nullIntPtr(m.ContentRatingAge)
	}
	if take(FieldCommunityRating) {
		p.CommunityRating, p.CommunityVotes = sql.NullFloat64{}, sql.NullInt64{}
		if c := m.Community; c != nil && c.Votes > 0 {
			p.CommunityRating = sql.NullFloat64{Float64: c.Rating, Valid: true}
			p.CommunityVotes = nullInt(c.Votes)
		}
	}
	if take(FieldTrailerURL) {
		p.TrailerUrl = nullString(m.TrailerURL)
	}
	if err := q.UpdateWorkMetadata(ctx, p); err != nil {
		return err
	}
	if err := setLocked(ctx, q, cur.ID, locked); err != nil {
		return err
	}

	for _, l := range []struct {
		field string
		names []string
		lookup
	}{
		{FieldGenres, m.Genres, lookup{q.DeleteWorkGenres, q.UpsertGenre, func(ctx context.Context, w id.ID, v int64) error {
			return q.InsertWorkGenre(ctx, dbq.InsertWorkGenreParams{WorkID: w, GenreID: v})
		}}},
		{FieldTags, m.Tags, lookup{q.DeleteWorkTags, q.UpsertTag, func(ctx context.Context, w id.ID, v int64) error {
			return q.InsertWorkTag(ctx, dbq.InsertWorkTagParams{WorkID: w, TagID: v})
		}}},
		{FieldCompanies, m.Companies, lookup{q.DeleteWorkCompanies, q.UpsertCompany, func(ctx context.Context, w id.ID, v int64) error {
			return q.InsertWorkCompany(ctx, dbq.InsertWorkCompanyParams{WorkID: w, CompanyID: v})
		}}},
	} {
		if take(l.field) && l.names != nil {
			if err := l.set(ctx, cur.ID, l.names); err != nil {
				return err
			}
		}
	}
	if take(FieldCredits) && m.Credits != nil {
		if err := setCredits(ctx, q, cur.ID, m.Credits); err != nil {
			return err
		}
	}
	if m.Collection != nil {
		if err := joinCollection(ctx, tx, cur.ID, *m.Collection); err != nil {
			return err
		}
	}
	if m.Book != nil {
		if err := writeBook(ctx, q, cur.ID, m.Book, take); err != nil {
			return err
		}
	}
	if err := reindex(ctx, tx, cur.ID); err != nil {
		return err
	}
	return c.rebuildWork(ctx, tx, cur.ID) // work_library holds copies of the sort title and browse keys
}

// lookup links a work to names that are kept in a table of their own, such
// as its genres, tags or companies.
type lookup struct {
	clear  func(ctx context.Context, work id.ID) error
	upsert func(ctx context.Context, name string) (int64, error)
	link   func(ctx context.Context, work id.ID, name int64) error
}

// set replaces a work's names, skipping blank ones.
func (l lookup) set(ctx context.Context, work id.ID, names []string) error {
	if err := l.clear(ctx, work); err != nil {
		return err
	}
	for _, n := range names {
		if n = strings.TrimSpace(n); n == "" {
			continue
		}
		v, err := l.upsert(ctx, n)
		if err != nil {
			return err
		}
		if err := l.link(ctx, work, v); err != nil {
			return err
		}
	}
	return nil
}

func setCredits(ctx context.Context, q *dbq.Queries, work id.ID, credits []Credit) error {
	if err := q.DeleteCredits(ctx, work); err != nil {
		return err
	}
	order := map[string]int{}
	for _, cr := range credits {
		if cr.Name == "" || cr.Role == "" {
			continue
		}
		person, err := resolvePerson(ctx, q, cr.Name, cr.ExternalIDs)
		if err != nil {
			return err
		}
		order[cr.Role]++
		err = q.InsertCredit(ctx, dbq.InsertCreditParams{
			WorkID: work, PersonID: person, Role: cr.Role, Ord: int64(order[cr.Role]), Character: nullString(cr.Character),
		})
		if err != nil {
			return err
		}
	}
	return nil
}

// joinCollection puts a work in a collection. The work leaves any other
// collection from the same source. The collection's row in the search index
// is rewritten from its name.
func joinCollection(ctx context.Context, tx *store.Tx, work id.ID, ref CollectionRef) error {
	source, _, ok := strings.Cut(ref.Key, ":")
	if !ok || ref.Name == "" {
		return fmt.Errorf("%w: collection key %q", ErrInvalid, ref.Key)
	}
	q := dbq.New(tx)
	if err := q.LeaveCollections(ctx, dbq.LeaveCollectionsParams{WorkID: work, Prefix: sql.NullString{String: source + ":", Valid: true}}); err != nil {
		return err
	}
	coll, err := q.UpsertCollection(ctx, dbq.UpsertCollectionParams{
		ID: id.New(), Key: ref.Key, Name: ref.Name, SortName: SortTitle(ref.Name), CreatedAt: time.Now().UnixMilli(),
	})
	if err != nil {
		return err
	}
	if err := reindexCollection(ctx, tx, coll, ref.Name); err != nil {
		return err
	}
	return q.JoinCollection(ctx, dbq.JoinCollectionParams{CollectionID: coll, WorkID: work, Ord: int64(ref.Order)})
}

// lockField locks one field of a work, as an admin's edit of it does.
func lockField(ctx context.Context, q *dbq.Queries, work id.ID, field string) error {
	cur, err := q.GetWork(ctx, work)
	if err != nil {
		return notFound(err)
	}
	locked := lockedFields(cur)
	if slices.Contains(locked, field) {
		return nil
	}
	locked = append(locked, field)
	slices.Sort(locked)
	return setLocked(ctx, q, work, locked)
}

// setLocked stores a work's locked fields as a JSON array, or as NULL when
// there are none.
func setLocked(ctx context.Context, q *dbq.Queries, work id.ID, locked []string) error {
	var s sql.NullString
	if len(locked) > 0 {
		b, _ := json.Marshal(locked)
		s = nullString(string(b))
	}
	return q.SetLockedFields(ctx, dbq.SetLockedFieldsParams{ID: work, LockedFields: s})
}

func lockedFields(w dbq.Work) []string {
	var out []string
	if w.LockedFields.Valid {
		_ = json.Unmarshal([]byte(w.LockedFields.String), &out)
	}
	return out
}

// Unmatch resets a movie or series to the state of a file that nothing
// matched. This covers the seasons and episodes under it.
// They lose their external IDs, metadata and collections. Names come from
// the file names: the given title and year for the movie or series, and names
// like "Season 2" and "Episode 5" below it. Everything but the seasons
// becomes provisional.
//
// The movie or series is locked, so no automatic match touches it again
// until a Fix match. Every match revision is bumped, so work already in
// flight for the old match is dropped.
func (b *Batch) Unmatch(ctx context.Context, work id.ID, title string, year int) error {
	if title == "" {
		return fmt.Errorf("%w: a work needs a title", ErrInvalid)
	}
	q := b.q()
	if _, err := rootRow(ctx, q, work, "can be unmatched"); err != nil {
		return err
	}
	rows, err := q.UnmatchTargets(ctx, work)
	if err != nil {
		return err
	}
	var all []id.ID
	for _, r := range rows {
		p := dbq.ResetWorkParams{ID: r.ID, Title: title, Year: nullInt(year), Provisional: true, Locked: true}
		switch {
		case WorkKind(r.Kind) == KindSeason:
			p.Title, p.Year, p.Provisional, p.Locked = "Season "+strconv.Itoa(int(r.Number.Int64)), sql.NullInt64{}, false, false
			if r.Number.Int64 == 0 {
				p.Title = "Specials"
			}
		case WorkKind(r.Kind) == KindEpisode:
			// An episode named by date keeps its date. Any other is named by its number.
			p.Title, p.Year, p.ReleaseDate, p.Locked = r.Title, sql.NullInt64{}, r.ReleaseDate, false
			if r.Number.Valid {
				p.Title, p.ReleaseDate = "Episode "+strconv.Itoa(int(r.Number.Int64)), sql.NullString{}
			}
		}
		p.SortTitle = SortTitle(p.Title)
		if err := q.ResetWork(ctx, p); err != nil {
			return err
		}
		for _, clear := range []func(context.Context, id.ID) error{
			q.DeleteExternalIDs, q.DeleteWorkGenres, q.DeleteWorkTags, q.DeleteWorkCompanies, q.DeleteCredits, q.LeaveAllCollections,
		} {
			if err := clear(ctx, r.ID); err != nil {
				return err
			}
		}
		if err := reindex(ctx, b.tx, r.ID); err != nil {
			return err
		}
		all = append(all, r.ID)
	}
	return b.add(ctx, q, all)
}

// Describe makes a movie or series into the root work a match describes, in
// place. The work keeps its ID and everything attached to it, and x becomes
// its only external ID. The work stops being provisional.
// It takes the match's title and year unless an admin locked them. When
// lock is set it is locked, as an admin's Fix match does.
//
// If the root had other external IDs, they described the old match. So it
// leaves its collections, and its seasons and episodes lose their external
// IDs, which means its episodes are found again by their numbers. Every
// match revision under it is bumped, so work in flight for the old
// description is dropped.
func (b *Batch) Describe(ctx context.Context, work id.ID, x ExternalID, title string, year int, lock bool) error {
	q := b.q()
	cur, err := rootRow(ctx, q, work, "is described by a match")
	if err != nil {
		return err
	}
	if err := replaceExternalIDs(ctx, q, work, x); err != nil {
		return err
	}
	if err := q.DescribeWork(ctx, described(cur, title, year, lock)); err != nil {
		return err
	}
	if err := q.BumpTreeMatchRev(ctx, work); err != nil {
		return err
	}
	if err := reindex(ctx, b.tx, work); err != nil {
		return err
	}
	return b.add(ctx, q, []id.ID{work}) // its sort title is copied to work_library
}

// rootRow returns a movie's or series' row. Any other work is ErrInvalid,
// with an error saying only a movie or series does what does says.
func rootRow(ctx context.Context, q *dbq.Queries, work id.ID, does string) (dbq.Work, error) {
	w, err := q.GetWork(ctx, work)
	if err != nil {
		return dbq.Work{}, notFound(err)
	}
	if k := WorkKind(w.Kind); k != KindMovie && k != KindSeries {
		return dbq.Work{}, fmt.Errorf("%w: only a movie or series %s", ErrInvalid, does)
	}
	return w, nil
}

// replaceExternalIDs makes x a root work's only external ID. If the root
// had a different one, it leaves its collections, and its seasons and episodes
// lose their external IDs.
func replaceExternalIDs(ctx context.Context, q *dbq.Queries, work id.ID, x ExternalID) error {
	had, err := q.ExternalIDs(ctx, work)
	if err != nil {
		return err
	}
	clear := []func(context.Context, id.ID) error{q.DeleteExternalIDs}
	if len(had) > 0 && !slices.Contains(had, idRow(x)) {
		clear = append(clear, q.DeleteTreeExternalIDs, q.LeaveAllCollections)
	}
	for _, c := range clear {
		if err := c(ctx, work); err != nil {
			return err
		}
	}
	return addExternalIDs(ctx, q, work, []ExternalID{x})
}

// described returns a root work as a match describes it. It takes the match's
// title and year unless an admin locked them.
func described(cur dbq.Work, title string, year int, lock bool) dbq.DescribeWorkParams {
	locked := lockedFields(cur)
	p := dbq.DescribeWorkParams{ID: cur.ID, Title: cur.Title, SortTitle: cur.SortTitle, Year: cur.Year, Locked: lock}
	if title != "" && !slices.Contains(locked, FieldTitle) {
		p.Title = title
		if !slices.Contains(locked, FieldSortTitle) {
			p.SortTitle = SortTitle(title)
		}
	}
	if !slices.Contains(locked, FieldYear) {
		p.Year = nullInt(year)
	}
	return p
}

// MoveRoot puts a root work in another library after its folder moved
// there. The works under it move with it. Their versions must all be in
// that library already, and the library of the root's kind, or MoveRoot
// returns ErrInvalid.
func (c *Catalog) MoveRoot(ctx context.Context, tx *store.Tx, root, lib id.ID) error {
	q := dbq.New(tx)
	if err := holds(ctx, q, lib, id.Set(root)); err != nil {
		return err
	}
	elsewhere, err := q.RootVersionsElsewhere(ctx, dbq.RootVersionsElsewhereParams{RootID: root, LibraryID: lib})
	if err != nil {
		return err
	}
	if elsewhere != 0 {
		return fmt.Errorf("%w: a root work's versions must all be in the library it moves to", ErrInvalid)
	}
	if err := q.MoveRootLibrary(ctx, dbq.MoveRootLibraryParams{RootID: root, LibraryID: lib}); err != nil {
		return err
	}
	works, err := q.RootWorks(ctx, root)
	if err != nil {
		return err
	}
	return c.refreshLibraries(ctx, tx, works)
}

// SetOrdering stores how a series' episodes are numbered. The ordering is
// aired, dvd, absolute or a provider's episode group ID. An empty ordering
// means aired. The match revision is bumped, so work already in flight
// starts over. identify.EditWork calls it and identifies the series'
// files again.
func (c *Catalog) SetOrdering(ctx context.Context, tx *store.Tx, series id.ID, ordering string) error {
	q := dbq.New(tx)
	n, err := q.SetOrdering(ctx, dbq.SetOrderingParams{ID: series, Ordering: nullString(ordering)})
	if err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	if _, err := q.GetWork(ctx, series); err != nil {
		return notFound(err)
	}
	return fmt.Errorf("%w: only a series has an episode ordering", ErrInvalid)
}

// SetEdition labels a movie with its folder's edition tag, such as
// "Final Cut". An empty label clears it.
func (c *Catalog) SetEdition(ctx context.Context, tx *store.Tx, movie id.ID, label string) error {
	q := dbq.New(tx)
	cur, err := q.GetWork(ctx, movie)
	if err != nil {
		return notFound(err)
	}
	if WorkKind(cur.Kind) != KindMovie {
		return fmt.Errorf("%w: only a movie takes an edition label", ErrInvalid)
	}
	return q.SetEdition(ctx, dbq.SetEditionParams{ID: movie, Edition: nullString(label)})
}

// MoveEpisode gives an episode a new season and number in its series.
// Episodes are keyed by their provider ID, so a new ordering moves them
// rather than swapping works. Their watch state moves with them.
func (b *Batch) MoveEpisode(ctx context.Context, episode, season id.ID, number int) error {
	q := b.q()
	ep, err := q.GetWork(ctx, episode)
	if err != nil {
		return notFound(err)
	}
	sn, err := q.GetWork(ctx, season)
	if err != nil {
		return notFound(err)
	}
	if WorkKind(ep.Kind) != KindEpisode || WorkKind(sn.Kind) != KindSeason || ep.RootID != sn.RootID {
		return fmt.Errorf("%w: an episode moves only to a season of its own series", ErrInvalid)
	}
	err = q.MoveEpisode(ctx, dbq.MoveEpisodeParams{ID: episode, ParentID: season, ParentNumber: sn.Number, Number: nullInt(number)})
	if err != nil {
		return err
	}
	return b.add(ctx, q, []id.ID{ep.ParentID, episode}) // the season it left, and its new one
}

// DeleteEmptyWorks deletes one batch of works that have no version or
// child left, and returns how many it deleted. It is a store.Drain
// step. A parent emptied by one batch is deleted by a later one, so the
// caller drains until a pass deletes nothing. The works' artwork, folder
// links, watch state, plays and track choices are deleted with them.
func (c *Catalog) DeleteEmptyWorks(ctx context.Context, tx *store.Tx) (int, error) {
	q := dbq.New(tx)
	works, err := q.EmptyWorks(ctx, store.BatchSize)
	if err != nil {
		return 0, err
	}
	var series []id.ID // a book's series, deleted once they have no book left
	for _, w := range works {
		in, err := q.WorkSeries(ctx, w.ID)
		if err != nil {
			return 0, err
		}
		for _, s := range in {
			series = append(series, s.SeriesID)
		}
		if err := unindex(ctx, tx, w.Rid); err != nil {
			return 0, err
		}
		if err := q.DeleteWork(ctx, w.ID); err != nil {
			return 0, err
		}
	}
	if err := q.DeleteEmptySeries(ctx, id.Set(series...)); err != nil {
		return 0, err
	}
	return len(works), nil
}

// SortTitle returns the title to sort under. It drops a leading "¡" or "¿"
// and then a leading article, so "¡Three Amigos!" sorts under T and "The
// Expanse" sorts under E. Other leading punctuation stays, so "The 'Burbs"
// and "*batteries not included" sort before the letters.
func SortTitle(title string) string {
	t := strings.TrimSpace(strings.TrimLeft(strings.TrimSpace(title), "¡¿"))
	for _, a := range []string{"the ", "a ", "an "} {
		if len(t) > len(a) && strings.EqualFold(t[:len(a)], a) {
			return strings.TrimSpace(t[len(a):])
		}
	}
	return t
}
