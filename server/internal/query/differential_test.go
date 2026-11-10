package query_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/mmcdole/kivo/internal/catalog"
	"github.com/mmcdole/kivo/internal/catalog/catalogtest"
	"github.com/mmcdole/kivo/internal/events"
	"github.com/mmcdole/kivo/internal/id"
	"github.com/mmcdole/kivo/internal/query"
	"github.com/mmcdole/kivo/internal/store"
	"github.com/mmcdole/kivo/internal/store/storetest"
	"github.com/mmcdole/kivo/internal/users"
)

// TestCompileMatchesOld runs a matrix of queries through compile and
// through the compiler it replaced, on the same generated works, and checks
// that both select the same works in the same order and count the same
// total. The matrix is every field with every operator, groups, every sort
// both ways, caps, a list page's kinds and page offsets, over each set of
// kinds, as Viewers with every, some and no libraries, with and without a
// rating ceiling and unrated works.
//
// The whole matrix takes minutes, so it runs only when KIVO_DIFFERENTIAL=1.
// Otherwise one query in 29 runs, which still takes each field, operator
// and sort through several Viewers and sets of kinds, or one in 293 under
// the race detector.
func TestCompileMatchesOld(t *testing.T) {
	db := storetest.New(t)
	d := newDifferential(t, db)
	canonical := canonicalFilter(t)
	now := time.Now()
	every := 29
	switch {
	case os.Getenv("KIVO_DIFFERENTIAL") == "1":
		every = 1
	case raceDetector:
		every = 293
	}
	var ran, found, rows, differed int
	for vi, v := range d.viewers {
		for ki, kinds := range differentialKinds {
			for qi, base := range d.queries {
				n := vi*len(differentialKinds)*len(d.queries) + ki*len(d.queries) + qi
				if n%every != 0 {
					continue
				}
				q := base
				q.Kinds = kinds
				// Rules take each sort in turn, and sorts each rule.
				if q.Sort == nil {
					q.Sort = d.sorts[n%len(d.sorts)]
				} else if q.Where == nil && n%2 == 0 {
					q.Where = where(d.rules[n%len(d.rules)])
				}
				if problems := q.Validate(); len(problems) > 0 {
					t.Fatalf("%s: %v", describe(q), problems)
				}
				variants := []differentialVariant{{limit: 100000}}
				switch n % 6 {
				case 0: // as a list's covers read its first works
					variants = append(variants, differentialVariant{limit: 3})
				case 1:
					variants = append(variants, differentialVariant{limit: 7, offset: 5}, differentialVariant{limit: 50, offset: 100000})
				case 2:
					variants = append(variants, differentialVariant{limit: 100000, only: kinds[:1]}, differentialVariant{limit: 100000, only: []catalog.WorkKind{}})
				case 3:
					variants = append(variants, differentialVariant{limit: 100000, cap: 7}, differentialVariant{limit: 3, offset: 2, cap: 7, only: kinds[:1]})
				case 4:
					variants = append(variants, differentialVariant{limit: 100000, cap: 1}, differentialVariant{limit: 100000, cap: 5000, only: []catalog.WorkKind{catalog.KindEpisode, catalog.KindMovie}})
				case 5:
					variants = append(variants, differentialVariant{limit: 100000, only: []catalog.WorkKind{catalog.KindSeries, catalog.KindBook}}, differentialVariant{limit: 100000, cap: 40, only: []catalog.WorkKind{}})
				}
				for _, variant := range variants {
					q := q
					if variant.cap > 0 {
						q.MaxWorks = &variant.cap
					}
					count, page, args := query.Compiled(q, v.v, now, variant.only)
					oldCount, oldPage, oldArgs := query.CompiledOld(q, v.v, now, variant.only)
					checkCompiledFilter(t, canonical, count)
					checkCompiledFilter(t, canonical, page)
					got, total := d.run(t, count, page, args, variant)
					want, oldTotal := d.run(t, oldCount, oldPage, oldArgs, variant)
					ran++
					rows += len(want)
					if len(want) > 0 {
						found++
					}
					if total != oldTotal || !slices.Equal(got, want) {
						t.Errorf("as %s, %s, %+v:\n got %d of %d\nwant %d of %d", v.name, describe(q), variant, len(got), total, len(want), oldTotal)
						if differed++; differed == 10 {
							t.Fatal("and more")
						}
					}
				}
			}
		}
	}
	t.Logf("%d queries compared, %d with works, %d works in all", ran, found, rows)
	if found < ran/4 {
		t.Errorf("only %d of %d queries selected any work: the matrix compares too little", found, ran)
	}
}

// differentialKinds are the sets of kinds the matrix asks for.
var differentialKinds = [][]catalog.WorkKind{
	{catalog.KindMovie}, {catalog.KindSeries}, {catalog.KindSeason}, {catalog.KindEpisode}, {catalog.KindVideo}, {catalog.KindBook},
	{catalog.KindSeries, catalog.KindSeason, catalog.KindEpisode}, {catalog.KindMovie, catalog.KindSeries},
	{catalog.KindEpisode, catalog.KindMovie}, queryAllKinds,
}

// differentialVariant is how one comparison runs a query: its page, its cap
// if any, and the kinds a list's page narrows it to.
type differentialVariant struct {
	limit, offset, cap int
	only               []catalog.WorkKind
}

// differential is the generated works, the Viewers and the queries
// TestCompileMatchesOld compares.
type differential struct {
	db      *store.DB
	viewers []struct {
		name string
		v    users.Viewer
	}
	rules   []query.Rule
	sorts   [][]query.SortKey
	queries []query.WorkQuery // without kinds
}

func describe(q query.WorkQuery) string {
	b, _ := json.Marshal(q)
	return string(b)
}

// run reads a compiled query's total and the IDs of its page, in order.
func (d *differential) run(t *testing.T, count, page string, args []any, v differentialVariant) (ids []id.ID, total int) {
	t.Helper()
	args = slices.Clone(args)
	for i, a := range args {
		switch a.(sql.NamedArg).Name {
		case "limit":
			args[i] = sql.Named("limit", v.limit)
		case "offset":
			args[i] = sql.Named("offset", v.offset)
		}
	}
	if err := d.db.Reader().QueryRow(count, args...).Scan(&total); err != nil {
		t.Fatalf("%v in %s", err, count)
	}
	rows, err := d.db.Reader().Query(page, args...)
	if err != nil {
		t.Fatalf("%v in %s", err, page)
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		t.Fatal(err)
	}
	dest := make([]any, len(cols))
	var work id.ID
	dest[0] = &work
	for i := 1; i < len(dest); i++ {
		dest[i] = new(sql.RawBytes)
	}
	for rows.Next() {
		if err := rows.Scan(dest...); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, work)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return ids, total
}

// blobID is the ID generated rows have: a letter and a number.
func blobID(letter byte, n int) id.ID {
	var out id.ID
	copy(out[:], fmt.Sprintf("%c%015d", letter, n))
	return out
}

// newDifferential fills db with 240 movies in two libraries, 40 series of 2
// seasons of 6 episodes in two more, 40 videos, 40 books and 12 extras.
// Some are provisional, some under a provisional series, some unrated,
// some episodes rated apart from their series, and some in no library.
// Titles and dates repeat, so every sort breaks ties. Two of its Viewers
// have play states, ratings and a list. It writes the tables directly, as
// generate does.
func newDifferential(t *testing.T, db *store.DB) *differential {
	t.Helper()
	cat := catalog.New(db, events.NewBus())
	libs := map[string]id.ID{}
	now := time.Now()
	err := db.Write(context.Background(), func(ctx context.Context, tx *store.Tx) error {
		for _, l := range []struct {
			name string
			kind catalog.LibraryKind
		}{{"movies1", catalog.Movies}, {"movies2", catalog.Movies}, {"shows1", catalog.Shows}, {"shows2", catalog.Shows},
			{"videos", catalog.Videos}, {"books", catalog.Audiobooks}} {
			lib, err := cat.CreateLibrary(ctx, tx, catalog.LibraryInput{Name: l.name, Kind: l.kind, Folders: []string{catalogtest.Abs("/" + l.name)}})
			if err != nil {
				return err
			}
			libs[l.name] = lib.ID
		}
		args := []any{sql.Named("now", now.UnixMilli())}
		for name, lib := range libs {
			args = append(args, sql.Named(name, lib))
		}
		// root makes n root works of a kind, their IDs starting with letter.
		root := func(kind string, letter byte, n, provisionalEvery int) string {
			return fmt.Sprintf(`WITH RECURSIVE n(i) AS (SELECT 1 UNION ALL SELECT i + 1 FROM n WHERE i < %d)
				INSERT INTO work (id, kind, root_id, title, sort_title, year, release_date, content_rating, content_rating_age,
				  community_rating, community_votes, provisional, added_at)
				SELECT CAST(printf('%c%%015d', i) AS BLOB), '%s', CAST(printf('%c%%015d', i) AS BLOB),
				  printf('The %s %%d', i %% 60), printf('%s %%d', i %% 60),
				  CASE WHEN i %% 13 THEN 1950 + i %% 75 END,
				  CASE WHEN i %% 13 AND i %% 4 THEN printf('%%04d-%%02d-%%02d', 1950 + i %% 75, i %% 12 + 1, i %% 28 + 1) END,
				  CASE WHEN i %% 5 THEN CASE i %% 3 WHEN 0 THEN 'G' WHEN 1 THEN 'PG-13' ELSE 'R' END END,
				  CASE WHEN i %% 5 THEN i %% 19 END,
				  CASE WHEN i %% 10 THEN i %% 91 / 10.0 + 1 END, CASE WHEN i %% 10 AND i %% 7 THEN i * 37 %% 20000 + 1 END,
				  i %% %d = 0, @now - (i %% 50) * 86400000 FROM n`, n, letter, kind, letter, kind, kind, provisionalEvery)
		}
		for _, stmt := range []string{
			root("movie", 'm', 240, 17), root("series", 's', 40, 8), root("video", 'd', 40, 9), root("book", 'b', 40, 10),
			`WITH RECURSIVE n(i) AS (SELECT 1 UNION ALL SELECT i + 1 FROM n WHERE i < 80)
			 INSERT INTO work (id, kind, parent_id, root_id, title, sort_title, number, provisional, added_at)
			 SELECT CAST(printf('z%015d', i) AS BLOB), 'season', CAST(printf('s%015d', (i + 1) / 2) AS BLOB),
			   CAST(printf('s%015d', (i + 1) / 2) AS BLOB), printf('Season %d', (i - 1) % 2 + 1), printf('Season %d', (i - 1) % 2 + 1),
			   (i - 1) % 2 + 1, i % 29 = 0, @now - (i % 9) * 86400000 FROM n`,
			// One episode in eleven has a rating of its own.
			`WITH RECURSIVE n(i) AS (SELECT 1 UNION ALL SELECT i + 1 FROM n WHERE i < 480)
			 INSERT INTO work (id, kind, parent_id, root_id, title, sort_title, parent_number, number, release_date,
			   content_rating, content_rating_age, community_rating, community_votes, provisional, added_at)
			 SELECT CAST(printf('e%015d', i) AS BLOB), 'episode', CAST(printf('z%015d', (i + 5) / 6) AS BLOB),
			   CAST(printf('s%015d', (i + 11) / 12) AS BLOB), printf('Episode %d', i % 30), printf('Episode %d', i % 30),
			   ((i + 5) / 6 - 1) % 2 + 1, (i - 1) % 6 + 1, CASE WHEN i % 9 THEN date(@now / 1000 - i * 172800, 'unixepoch') END,
			   CASE WHEN i % 11 = 0 THEN 'TV-MA' END, CASE WHEN i % 11 = 0 THEN i % 19 END,
			   CASE WHEN i % 4 THEN i % 91 / 10.0 + 1 END, CASE WHEN i % 4 THEN i + 1 END,
			   i % 23 = 0, @now - (i % 40) * 86400000 FROM n`,
			`WITH RECURSIVE n(i) AS (SELECT 1 UNION ALL SELECT i + 1 FROM n WHERE i < 12)
			 INSERT INTO work (id, kind, parent_id, root_id, title, sort_title, extra_type, added_at)
			 SELECT CAST(printf('x%015d', i) AS BLOB), 'extra', CAST(printf('m%015d', i * 10) AS BLOB),
			   CAST(printf('m%015d', i * 10) AS BLOB), 'Trailer', 'Trailer', 'trailer', @now FROM n`,
			// One work in 31 is in no library.
			`INSERT INTO work_library (library_id, work_id, is_root, sort_title, added_at, year, release_key, runtime_sort,
			   resolutions, dynamic_ranges, rating_sort, last_added_at)
			 SELECT CASE r.kind WHEN 'movie' THEN iif(r.rid % 3, @movies1, @movies2) WHEN 'series' THEN iif(r.rid % 4, @shows1, @shows2)
			     WHEN 'video' THEN @videos ELSE @books END,
			   w.id, w.parent_id IS NULL, w.sort_title, w.added_at, k.year, k.release_key,
			   CASE WHEN w.rid % 6 THEN w.rid * 60000 ELSE 9223372036854775807 END, w.rid % 16, w.rid % 4, k.rating_sort,
			   CASE WHEN w.rid % 7 THEN w.added_at + (w.rid % 5) * 1000 ELSE 0 END
			 FROM work w JOIN work r ON r.id = w.root_id JOIN work_browse_keys k ON k.work_id = w.id
			 WHERE w.rid % 31 <> 0`,
			`INSERT INTO genre (name) VALUES ('Drama'), ('Comedy'), ('Action'), ('Horror')`,
			`INSERT INTO work_genre (work_id, genre_id) SELECT id, rid % 4 + 1 FROM work WHERE parent_id IS NULL OR rid % 50 = 0`,
			`INSERT INTO work_genre (work_id, genre_id) SELECT id, (rid % 4 + 1) % 4 + 1 FROM work WHERE parent_id IS NULL AND rid % 3 = 0`,
			`INSERT INTO tag (name) VALUES ('Space'), ('Heist')`,
			`INSERT INTO work_tag (work_id, tag_id) SELECT id, rid % 2 + 1 FROM work WHERE parent_id IS NULL AND rid % 5 = 0`,
			`INSERT INTO company (name) VALUES ('Legendary'), ('Pixar'), ('Audible')`,
			`INSERT INTO work_company (work_id, company_id) SELECT id, rid % 3 + 1 FROM work WHERE parent_id IS NULL AND rid % 6 = 0`,
			`WITH RECURSIVE n(i) AS (SELECT 1 UNION ALL SELECT i + 1 FROM n WHERE i < 20)
			 INSERT INTO person (id, name, sort_name) SELECT CAST(printf('p%015d', i) AS BLOB), printf('Person %d', i), printf('Person %d', i) FROM n`,
			// Some episodes have credits of their own, and the rest take their series'.
			`INSERT INTO credit (work_id, person_id, role, ord)
			 SELECT id, CAST(printf('p%015d', rid % 20 + 1) AS BLOB), 'actor', 1 FROM work
			 WHERE kind IN ('movie', 'series', 'video') OR (kind = 'episode' AND rid % 40 = 0)
			 UNION ALL
			 SELECT id, CAST(printf('p%015d', (rid + 7) % 20 + 1) AS BLOB), 'actor', 2 FROM work WHERE kind IN ('movie', 'series') AND rid % 3 = 0
			 UNION ALL
			 SELECT id, CAST(printf('p%015d', rid % 5 + 1) AS BLOB), 'director', 1 FROM work WHERE kind IN ('movie', 'season') AND rid % 2 = 0
			 UNION ALL
			 SELECT id, CAST(printf('p%015d', rid % 4 + 1) AS BLOB), 'author', 1 FROM work WHERE kind = 'book'
			 UNION ALL
			 SELECT id, CAST(printf('p%015d', rid % 3 + 5) AS BLOB), 'narrator', 1 FROM work WHERE kind = 'book' AND rid % 4`,
			`INSERT INTO collection (id, key, name, created_at) VALUES (CAST(printf('c%015d', 1) AS BLOB), 'c1', 'One', @now),
			   (CAST(printf('c%015d', 2) AS BLOB), 'c2', 'Two', @now)`,
			`INSERT INTO collection_work (collection_id, work_id, ord)
			 SELECT CAST(printf('c%015d', 1) AS BLOB), id, rid FROM work WHERE kind IN ('movie', 'series') AND rid % 10 = 0
			 UNION ALL
			 SELECT CAST(printf('c%015d', 2) AS BLOB), id, rid FROM work WHERE kind = 'movie' AND rid % 15 = 0`,
			`INSERT INTO book_series (id, key, name, sort_name, created_at) VALUES (CAST(printf('q%015d', 1) AS BLOB), 'q1', 'Saga', 'Saga', @now)`,
			`INSERT INTO book_series_book (series_id, work_id) SELECT CAST(printf('q%015d', 1) AS BLOB), id FROM work WHERE kind = 'book' AND rid % 2 = 0`,
		} {
			var named []any // the driver refuses arguments a statement doesn't take
			if strings.Contains(stmt, "@") {
				named = args
			}
			if _, err := tx.ExecContext(ctx, stmt, named...); err != nil {
				return fmt.Errorf("%w in %s", err, stmt)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	d := &differential{db: db}
	svc := users.New(db, events.NewBus())
	sorted := func(names ...string) []id.ID {
		out := make([]id.ID, len(names))
		for i, n := range names {
			out[i] = libs[n]
		}
		slices.SortFunc(out, func(a, b id.ID) int { return slices.Compare(a[:], b[:]) })
		return out
	}
	ceiling := func(age int) *int { return &age }
	var mine id.ID // the first Viewer's list
	for i, u := range []users.NewUser{
		{Username: "everything", Grants: users.Grants{All: true}},
		{Username: "some", Grants: users.Grants{Libraries: sorted("movies1", "shows1", "books")}},
		{Username: "others", Grants: users.Grants{Libraries: sorted("movies2", "shows2", "videos")}, Parental: users.Parental{Ceiling: ceiling(13)}},
		{Username: "one", Grants: users.Grants{Libraries: sorted("shows2")}, Parental: users.Parental{BlockUnrated: true}},
		{Username: "kid", Grants: users.Grants{All: true}, Parental: users.Parental{Ceiling: ceiling(7), BlockUnrated: true}},
		{Username: "teen", Grants: users.Grants{All: true}, Parental: users.Parental{Ceiling: ceiling(13)}},
		{Username: "strict", Grants: users.Grants{All: true}, Parental: users.Parental{BlockUnrated: true}},
		{Username: "nobody", Grants: users.Grants{All: true}},
	} {
		var user id.ID
		err := db.Write(context.Background(), func(ctx context.Context, tx *store.Tx) error {
			created, err := svc.CreateUser(ctx, tx, u)
			if err != nil {
				return err
			}
			user = created.ID
			if u.Username == "nobody" {
				_, err := tx.ExecContext(ctx, `UPDATE user SET all_libraries = 0 WHERE id = ?`, user)
				return err
			}
			if i > 1 {
				return nil
			}
			// The first two have states, ratings and works on their list.
			args := []any{sql.Named("user", user), sql.Named("now", now.UnixMilli()), sql.Named("every", i+2)}
			for _, stmt := range []string{`
				INSERT INTO user_state (user_id, work_id, played, play_count, position_ms, resumable, rating, last_played_at, updated_at)
				SELECT @user, id, rid % 3 = 0, rid % 3 = 0, 600000 * (rid % 3 = 1), rid % 3 = 1,
				  CASE WHEN rid % 5 = 0 THEN rid % 10 + 1 END, CASE WHEN rid % 7 THEN @now - (rid % 40) * 3600000 END, @now
				FROM work WHERE (kind NOT IN ('series', 'season') AND rid % @every = 0) OR rid % 10 = 0`, `
				INSERT INTO list_work (list_id, work_id, added_at)
				SELECT l.id, w.id, @now - w.rid FROM work w, list l WHERE l.user_id = @user AND l.built_in AND w.rid % 9 = 0`,
			} {
				if _, err := tx.ExecContext(ctx, stmt, args...); err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		v, err := svc.Viewer(context.Background(), user)
		if err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			if err := db.Reader().QueryRow(`SELECT id FROM list WHERE user_id = ? AND built_in`, user).Scan(&mine); err != nil {
				t.Fatal(err)
			}
		}
		d.viewers = append(d.viewers, struct {
			name string
			v    users.Viewer
		}{u.Username, v})
	}
	d.viewers = append(d.viewers, struct {
		name string
		v    users.Viewer
	}{"zero", users.Viewer{}})

	// Each field's values: one or more sets, each tried with every operator.
	day := func(ago int) string { return now.UTC().AddDate(0, 0, -ago).Format(time.DateOnly) }
	dates := [][]string{{day(20), day(-1)}, {"1990-01-01", "2015-06-30"}}
	values := map[string][][]string{
		"library":       {{libs["movies1"].String()}, {libs["movies2"].String(), libs["shows1"].String(), libs["books"].String()}},
		"contentRating": {{"PG-13"}, {"G", "TV-MA"}},
		"playState":     {{"played"}, {"inProgress", "unplayed"}},
		"genre":         {{"Drama"}, {"Comedy", "Action"}},
		"tag":           {{"Space"}, {"Space", "Heist"}},
		"company":       {{"Pixar"}, {"Legendary", "Audible"}},
		"resolution":    {{"1080p"}, {"4k", "sd"}},
		"dynamicRange":  {{"hdr"}, {"hdr", "dolbyVision"}},
		"collection":    {{blobID('c', 1).String()}, {blobID('c', 1).String(), blobID('c', 2).String()}},
		"bookSeries":    {{blobID('q', 1).String()}},
		"inList":        {{mine.String()}},
		"rootWork":      {{blobID('s', 3).String()}, {blobID('m', 5).String(), blobID('s', 8).String(), blobID('s', 9).String()}},
		"actor":         {{blobID('p', 3).String()}, {blobID('p', 4).String(), blobID('p', 11).String()}},
		"director":      {{blobID('p', 2).String()}, {blobID('p', 1).String(), blobID('p', 3).String()}},
		"author":        {{blobID('p', 2).String()}},
		"narrator":      {{blobID('p', 6).String()}},
		"rating":        {{"5", "8"}},
		"myRating":      {{"4", "9"}},
		"year":          {{"1965", "2000"}},
		"runtimeMs":     {{"6000000", "30000000"}},
		"title":         {{"movie 1"}, {"e"}},
		"releaseDate":   dates, "addedAt": dates, "lastEpisodeAddedAt": dates, "lastPlayedAt": dates,
	}
	d.sorts = [][]query.SortKey{nil, {{Field: "random"}},
		{{Field: "year"}, {Field: "title", Direction: "desc"}},
		{{Field: "rating", Direction: "asc"}, {Field: "myRating"}, {Field: "addedAt", Direction: "asc"}},
		{{Field: "lastPlayedAt"}, {Field: "releaseDate"}}}
	for _, f := range query.WorkFields() {
		sets, ok := values[f.Field]
		if !ok {
			t.Fatalf("no values for the field %s", f.Field)
		}
		for _, op := range f.Ops {
			switch {
			case op.Op == "within" || op.Op == "notWithin":
				for _, period := range []string{"P30D", "P1Y", "P30Y"} {
					d.rules = append(d.rules, query.Rule{Field: f.Field, Op: op.Op, Values: []string{period}})
				}
			case op.Arity == 0:
				d.rules = append(d.rules, query.Rule{Field: f.Field, Op: op.Op})
			case op.Arity < 0:
				for _, set := range sets {
					d.rules = append(d.rules, query.Rule{Field: f.Field, Op: op.Op, Values: set})
				}
			default:
				for _, set := range sets {
					d.rules = append(d.rules, query.Rule{Field: f.Field, Op: op.Op, Values: []string{set[0], set[len(set)-1]}[:op.Arity]})
				}
			}
		}
		if f.Sortable {
			for _, dir := range []string{"", "asc", "desc"} {
				d.sorts = append(d.sorts, []query.SortKey{{Field: f.Field, Direction: dir}})
			}
		}
	}
	for _, r := range d.rules {
		d.queries = append(d.queries, query.WorkQuery{Where: where(r)})
	}
	for _, s := range d.sorts[1:] {
		d.queries = append(d.queries, query.WorkQuery{Sort: s})
	}
	rule := func(field, op string, values ...string) query.Rule {
		return query.Rule{Field: field, Op: op, Values: values}
	}
	any := func(rules ...query.Rule) query.RuleGroup { return query.RuleGroup{Match: "any", Rules: rules} }
	all := func(rules ...query.Rule) query.RuleGroup { return query.RuleGroup{Match: "all", Rules: rules} }
	drama, recent, played := rule("genre", "anyOf", "Drama"), rule("year", "gte", "2000"), rule("playState", "anyOf", "played")
	inLibrary := rule("library", "anyOf", libs["movies1"].String(), libs["shows1"].String())
	for _, g := range []query.RuleGroup{
		any(drama, recent, played),
		all(drama, recent, rule("contentRating", "noneOf", "R")),
		all(inLibrary, rule("rating", "gte", "4")),
		{Match: "all", Rules: []query.Rule{inLibrary}, Groups: []query.RuleGroup{any(drama, played), any(rule("tag", "isSet"), rule("myRating", "isNotSet"))}},
		{Match: "any", Rules: []query.Rule{rule("title", "startsWith", "movie 2")}, Groups: []query.RuleGroup{all(recent, played), all(drama, rule("resolution", "anyOf", "4k"))}},
		{Match: "any", Groups: []query.RuleGroup{all(rule("rootWork", "anyOf", blobID('s', 3).String())), any(rule("actor", "anyOf", blobID('p', 3).String()), rule("lastPlayedAt", "within", "P30D"))}},
	} {
		d.queries = append(d.queries, query.WorkQuery{Where: &g})
	}
	return d
}
