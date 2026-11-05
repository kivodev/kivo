package query

import (
	"cmp"
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/mmcdole/kivo/internal/id"
	"github.com/mmcdole/kivo/internal/store"
	"github.com/mmcdole/kivo/internal/users"
)

// In the search statements, @match is the query's words, each quoted, with
// the last one a prefix. @key is the query as store.SearchKey makes it.
// match_rank is store.MatchRank, an FTS5 auxiliary function.

// searchWorks lists the works with every word of the query in their titles,
// alternative titles or people, best first. Works whose title matches come
// first, ranked by how it matches: the title is the query, then it starts
// with the query, then it holds the query as a phrase, then it holds the
// query's words. Works matched by alternative titles come next, then works
// matched only by their people. Within each rank the most voted come first,
// then by title. Only the page's rows are made into summaries.
var searchWorks = stmt("SearchWorks", `
SELECT `+summaryColumns+`
FROM (
  SELECT w.id, w.community_votes AS votes, w.sort_title,
    match_rank(work_fts, w.title, @key) AS rank
  FROM work_fts
  JOIN work w ON w.rid = work_fts.rowid
  WHERE work_fts MATCH @match
    AND w.kind IN ('movie', 'series', 'episode', 'video', 'book')
    AND `+visible+`
  ORDER BY rank, coalesce(w.community_votes, 0) DESC, w.sort_title, w.id
  LIMIT @limit
) m
JOIN work w ON w.id = m.id
LEFT JOIN user_state us ON us.user_id = @user AND us.work_id = w.id
ORDER BY m.rank, coalesce(m.votes, 0) DESC, m.sort_title, w.id`)

// creditedOn selects a person p's credits on works the Viewer sees, leaving
// out extras.
var creditedOn = `FROM credit c JOIN work w ON w.id = c.work_id
    WHERE c.person_id = p.id AND w.kind <> 'extra'
      AND ` + visible

// searchPeople lists the people whose names have every word of the query
// and who are credited on a work the Viewer sees, best first. They rank by
// how their name matches, the way works rank by title, then by how many
// credits they have, then by name. Each comes with their photo, their most
// common role on the root works the Viewer sees, and how many root works
// have that role. Those are movies, shows, videos and books, and an
// episode counts as its series.
//
// The candidates are ranked first, unfiltered. LIMIT -1 keeps SQLite from
// flattening them into the outer query, so their rows come in order and the
// Viewer filter stops once the page is full.
var searchPeople = stmt("SearchPeople", `
SELECT p.id, p.name, `+photoColumns+`,
  coalesce((SELECT c.role `+creditedOn+`
    GROUP BY c.role ORDER BY count(DISTINCT w.root_id) DESC, c.role LIMIT 1), ''),
  (SELECT count(DISTINCT w.root_id) `+creditedOn+`)
FROM (
  SELECT p.id, match_rank(person_fts, p.name, @key) AS rank,
    (SELECT count(*) FROM credit c WHERE c.person_id = p.id) AS credits
  FROM person_fts
  JOIN person p ON p.rid = person_fts.rowid
  WHERE person_fts MATCH @match
  ORDER BY rank, credits DESC, p.sort_name, p.id
  LIMIT -1
) m
JOIN person p ON p.id = m.id
LEFT JOIN image i ON i.person_id = p.id AND i.kind = 'photo'
WHERE EXISTS (SELECT 1 `+creditedOn+`)
LIMIT @limit`)

// searchCollections lists the collections whose names have every word of
// the query and that hold a work the Viewer sees, best first. They rank by
// how their name matches, the way works rank by title, then by name. Each
// comes with how many of its works the Viewer sees. Like SearchPeople, it
// ranks the candidates first and filters them after.
var searchCollections = stmt("SearchCollections", `
SELECT `+collectionColumns+`, (`+seenInCollection+`) AS items
FROM (
  SELECT c.id, match_rank(collection_fts, c.name, @key) AS rank
  FROM collection_fts
  JOIN collection c ON c.rid = collection_fts.rowid
  WHERE collection_fts MATCH @match
  ORDER BY rank, c.sort_name, c.id
  LIMIT -1
) m
JOIN collection c ON c.id = m.id
WHERE (`+seenInCollection+`) > 0
LIMIT @limit`)

// Search result limits: the default when a request asks for none, and the
// most a request may ask for.
const (
	defaultSearchLimit = 20
	maxSearchLimit     = 100
)

// Results are a search's matches: works, people and collections, each best
// first.
type Results struct {
	Works       []WorkSummary
	People      []PersonMatch
	Collections []Collection
}

// PersonMatch is a person that search found. Role is the person's most
// common role on the root works the Viewer sees, and Roots is how many
// root works have that role.
type PersonMatch struct {
	ID    id.ID
	Name  string
	Photo *Image
	Role  string // a credit role, such as actor, director or author
	Roots int
}

// Search finds the movies, series, episodes, videos and books, the people
// and the collections that have every word of q. It returns at most limit
// of each, or 20 when limit is zero.
//
// Words are runs of letters and digits, compared without case or
// diacritics. The last word is a prefix, because q is still being typed.
//
// Works rank by their title first. A title that is the query ranks highest,
// then one that starts with it, then one that holds it as a phrase, then
// one that holds its words, ignoring a leading article. Works matched by
// alternative titles come next, then works matched by the people credited
// on them. Ties go to the most voted. People rank by their name the same
// way, then by credits. Collections rank by their name the same way. A
// collection is found only when the Viewer sees one of its works.
func (s *Service) Search(ctx context.Context, v users.Viewer, q string, limit int) (Results, error) {
	vw := viewerOf(v)
	switch {
	case limit == 0:
		limit = defaultSearchLimit
	case limit < 0 || limit > maxSearchLimit:
		return Results{}, invalid("limit", "outOfRange", fmt.Sprintf("limit %d", limit))
	}
	out := Results{Works: []WorkSummary{}, People: []PersonMatch{}, Collections: []Collection{}}
	words := store.SearchWords(q)
	if len(words) == 0 {
		return out, nil
	}
	args := vw.args(sql.Named("match", matchQuery(words)), sql.Named("key", store.SearchKey(q)), sql.Named("limit", limit))
	// Works and people are read at once, each on its own connection.
	var peopleErr error
	done := make(chan struct{})
	go func() {
		defer close(done)
		peopleErr = s.db.Read(ctx, func(tx *sql.Tx) error {
			people, err := rowsOf(ctx, tx, searchPeople, args, func(r scanner, p *PersonMatch) error {
				var ph photo
				err := r.Scan(append(append([]any{&p.ID, &p.Name}, ph.dest()...), &p.Role, &p.Roots)...)
				p.Photo = ph.image()
				return err
			})
			if people != nil {
				out.People = people
			}
			return err
		})
	}()
	err := s.db.Read(ctx, func(tx *sql.Tx) error {
		var err error
		if out.Works, err = summaries(ctx, tx, searchWorks, args); err != nil {
			return err
		}
		colls, err := rowsOf(ctx, tx, searchCollections, args, scanCollection)
		if colls != nil {
			out.Collections = colls
		}
		return err
	})
	if err == nil {
		err = s.decorate(ctx, vw, out.Works)
	}
	if err == nil {
		err = s.withCollectionImages(ctx, vw, out.Collections)
	}
	<-done
	if err := cmp.Or(err, peopleErr); err != nil {
		return Results{}, err
	}
	return out, nil
}

// matchQuery makes words into an FTS5 query that requires all of them, with
// the last one a prefix. Each word is quoted as a string, so no user input
// is read as FTS5 syntax.
func matchQuery(words []string) string {
	terms := make([]string, len(words))
	for i, w := range words {
		terms[i] = `"` + strings.ReplaceAll(w, `"`, `""`) + `"`
	}
	return strings.Join(terms, " ") + "*"
}
