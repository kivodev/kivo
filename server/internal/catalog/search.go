package catalog

import (
	"context"
	"database/sql"
	_ "embed"
	"encoding/json"
	"strings"

	"github.com/mmcdole/kivo/internal/catalog/dbq"
	"github.com/mmcdole/kivo/internal/id"
	"github.com/mmcdole/kivo/internal/store"
)

// The search statements are hand-written because sqlc can't parse FTS5.
// They live in queries/ so the write-ownership lint still sees them.
//
//go:embed queries/search.hand.sql
var searchSQL string

var (
	deleteSearch       = store.HandWritten(searchSQL, "DeleteSearch")
	insertSearch       = store.HandWritten(searchSQL, "InsertSearch")
	deletePersonSearch = store.HandWritten(searchSQL, "DeletePersonSearch")
	insertPersonSearch = store.HandWritten(searchSQL, "InsertPersonSearch")
	insertCredited     = store.HandWritten(searchSQL, "InsertCreditedSearch")
	deleteCollection   = store.HandWritten(searchSQL, "DeleteCollectionSearch")
	insertCollection   = store.HandWritten(searchSQL, "InsertCollectionSearch")
)

// reindex rewrites a work's row in the search index from its titles and
// credits. It also indexes any newly credited people. It runs in the same
// transaction as the change.
func reindex(ctx context.Context, tx *store.Tx, work id.ID) error {
	q := dbq.New(tx)
	w, err := q.GetWork(ctx, work)
	if err != nil {
		return notFound(err)
	}
	var alts []string
	if w.AltTitles.Valid {
		if err := json.Unmarshal([]byte(w.AltTitles.String), &alts); err != nil {
			return err
		}
	}
	if w.OriginalTitle.Valid {
		alts = append(alts, w.OriginalTitle.String)
	}
	people, err := q.CreditNames(ctx, work)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, deleteSearch, sql.Named("rid", w.Rid)); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, insertSearch, sql.Named("rid", w.Rid), sql.Named("title", searchTitle(w.Title, w.SortTitle)),
		sql.Named("alt_titles", strings.Join(alts, "\n")), sql.Named("people", strings.Join(people, "\n"))); err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, insertCredited, sql.Named("work_id", work))
	return err
}

// searchTitle returns the text the index holds for a work's title. It is
// the title, plus its hyphenated words joined so "spiderman" finds
// Spider-Man, plus the sort title when the title doesn't contain it. A
// collection's name is indexed the same way.
func searchTitle(title, sortTitle string) string {
	t := title
	if strings.Contains(title, "-") {
		t += "\n" + strings.ReplaceAll(title, "-", "")
	}
	if !strings.Contains(title, sortTitle) {
		t += "\n" + sortTitle
	}
	return t
}

// reindexCollection rewrites a collection's row in the collections index
// from its name. It runs in the transaction that writes the name.
func reindexCollection(ctx context.Context, tx *store.Tx, coll id.ID, name string) error {
	if _, err := tx.ExecContext(ctx, deleteCollection, sql.Named("id", coll)); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, insertCollection, sql.Named("id", coll), sql.Named("name", searchTitle(name, SortTitle(name))))
	return err
}

func unindex(ctx context.Context, tx *store.Tx, rid int64) error {
	_, err := tx.ExecContext(ctx, deleteSearch, sql.Named("rid", rid))
	return err
}

// reindexPerson runs change, which writes a person, and rewrites the
// person's row in the people index around it. The row is rebuilt from the
// person's name, or removed if the person is gone. It runs in the change's
// transaction.
func reindexPerson(ctx context.Context, tx *store.Tx, person id.ID, change func() error) error {
	if _, err := tx.ExecContext(ctx, deletePersonSearch, sql.Named("id", person)); err != nil {
		return err
	}
	if err := change(); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, insertPersonSearch, sql.Named("id", person))
	return err
}
