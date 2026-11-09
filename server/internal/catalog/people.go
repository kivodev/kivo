package catalog

import (
	"context"
	"database/sql"
	"errors"
	"slices"
	"strings"

	"github.com/mmcdole/kivo/internal/catalog/dbq"
	"github.com/mmcdole/kivo/internal/id"
	"github.com/mmcdole/kivo/internal/store"
	"github.com/mmcdole/kivo/internal/users"
)

// RenamePerson renames an author or narrator and returns the resulting
// person.
//
// Renaming to someone else's name merges the two. Every credit and
// external ID moves to the other person, the oldest with that name, who is
// returned. The renamed person is deleted.
//
// Either way, each of the person's books counts as edited, so its authors
// or narrators are locked and a rescan keeps the change. The person's works
// are rewritten a batch at a time, and each batch is safe to repeat. A
// rename interrupted by a restart is finished by asking again.
func (c *Catalog) RenamePerson(ctx context.Context, person id.ID, name string) (id.ID, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return id.ID{}, invalidField("/name", "required", "a name is required")
	}
	p, err := c.q.GetPerson(ctx, person)
	if err != nil {
		return id.ID{}, notFound(err)
	}
	works, err := c.q.PersonWorks(ctx, person)
	if err != nil {
		return id.ID{}, err
	}
	if p.Name == name {
		return person, nil
	}
	into, err := c.q.PersonByName(ctx, dbq.PersonByNameParams{Name: name, Other: person})
	switch {
	case errors.Is(err, sql.ErrNoRows):
		into = person
	case err != nil:
		return id.ID{}, err
	}
	rename := func(ctx context.Context, tx *store.Tx) error {
		if into != person {
			return nil
		}
		return reindexPerson(ctx, tx, person, func() error {
			return dbq.New(tx).RenamePerson(ctx, dbq.RenamePersonParams{ID: person, Name: name, SortName: SortTitle(name)})
		})
	}
	err = store.Batches(ctx, c.db, works, func(ctx context.Context, tx *store.Tx, batch []dbq.PersonWorksRow) error {
		q := dbq.New(tx)
		if err := rename(ctx, tx); err != nil {
			return err
		}
		for _, w := range batch {
			if into != person {
				if err := moveCredits(ctx, q, w.WorkID, w.Role, person, into); err != nil {
					return err
				}
			}
			if field := roleField(w.Role); WorkKind(w.Kind) == KindBook && field != "" {
				if err := lockField(ctx, q, w.WorkID, field); err != nil {
					return err
				}
			}
			if err := reindex(ctx, q, tx, w.WorkID); err != nil {
				return err
			}
			if err := c.rebuildWork(ctx, tx, w.WorkID); err != nil { // work_library holds the author sort key
				return err
			}
		}
		return nil
	})
	if err != nil {
		return id.ID{}, err
	}
	err = c.db.Write(ctx, func(ctx context.Context, tx *store.Tx) error {
		q := dbq.New(tx)
		if into == person {
			return rename(ctx, tx) // renames a person with no credits
		}
		if err := q.MovePersonExternalIDs(ctx, dbq.MovePersonExternalIDsParams{Into: into, From: person}); err != nil {
			return err
		}
		if err := users.MovePersonInQueries(ctx, tx, person, into); err != nil {
			return err
		}
		return reindexPerson(ctx, tx, person, func() error { return q.DeletePerson(ctx, person) })
	})
	if err != nil {
		return id.ID{}, err
	}
	return into, nil
}

// PersonOf returns the person an external ID names. It returns ErrNotFound
// when no one has the ID.
func (c *Catalog) PersonOf(ctx context.Context, tx *store.Tx, x ExternalID) (id.ID, error) {
	person, err := dbq.New(tx).PersonByExternalID(ctx, dbq.PersonByExternalIDParams{Provider: x.Provider, Kind: x.Kind, ExtID: x.ID})
	if err != nil {
		return id.ID{}, notFound(err)
	}
	return person, nil
}

// DescribePerson writes a provider's biography of the person an external
// ID names, and returns that person. It returns ErrNotFound when no one has
// the ID.
func (c *Catalog) DescribePerson(ctx context.Context, tx *store.Tx, x ExternalID, overview string) (id.ID, error) {
	q := dbq.New(tx)
	person, err := q.PersonByExternalID(ctx, dbq.PersonByExternalIDParams{Provider: x.Provider, Kind: x.Kind, ExtID: x.ID})
	if err != nil {
		return id.ID{}, notFound(err)
	}
	if err := q.SetPersonOverview(ctx, dbq.SetPersonOverviewParams{ID: person, Overview: nullString(overview)}); err != nil {
		return id.ID{}, err
	}
	return person, nil
}

// moveCredits moves a work's credits of one role from one person to
// another. A credit the other person already has is dropped instead.
func moveCredits(ctx context.Context, q *dbq.Queries, work id.ID, role string, from, into id.ID) error {
	ords, err := q.PersonRoleCredits(ctx, dbq.PersonRoleCreditsParams{WorkID: work, Role: role, PersonID: from})
	if err != nil {
		return err
	}
	for _, ord := range ords {
		has, err := q.CreditedAs(ctx, dbq.CreditedAsParams{WorkID: work, Role: role, PersonID: into})
		if err != nil {
			return err
		}
		if has != 0 {
			err = q.DeleteCredit(ctx, dbq.DeleteCreditParams{WorkID: work, Role: role, Ord: ord})
		} else {
			err = q.MoveCredit(ctx, dbq.MoveCreditParams{WorkID: work, Role: role, Ord: ord, PersonID: into})
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// roleField returns the book field that holds credits of a role, or "".
func roleField(role string) string {
	switch role {
	case RoleAuthor:
		return FieldAuthors
	case RoleNarrator:
		return FieldNarrators
	}
	return ""
}

// resolvePerson returns the person a credit names, and records ids as that
// person's. It picks the first of these:
//   - the person already named by any of ids;
//   - the oldest person with that name who has no other ID of the same
//     provider and kind;
//   - a new person.
//
// So two people who share a name are told apart once a provider says so,
// while credits from names alone, such as a book's tags, still merge.
func resolvePerson(ctx context.Context, q *dbq.Queries, name string, ids []ExternalID) (id.ID, error) {
	var person id.ID
	for _, x := range ids {
		p, err := q.PersonByExternalID(ctx, dbq.PersonByExternalIDParams{Provider: x.Provider, Kind: x.Kind, ExtID: x.ID})
		if err == nil {
			person = p
			break
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return id.ID{}, err
		}
	}
	if person.IsZero() {
		named, err := q.PeopleNamed(ctx, name)
		if err != nil {
			return id.ID{}, err
		}
		for _, p := range named {
			theirs, err := q.PersonExternalIDs(ctx, p)
			if err != nil {
				return id.ID{}, err
			}
			conflict := slices.ContainsFunc(theirs, func(t dbq.PersonExternalIDsRow) bool {
				return slices.ContainsFunc(ids, func(x ExternalID) bool {
					return x.Provider == t.Provider && x.Kind == t.Kind && x.ID != t.ExtID
				})
			})
			if !conflict {
				person = p
				break
			}
		}
	}
	if person.IsZero() {
		person = id.New()
		if err := q.InsertPerson(ctx, dbq.InsertPersonParams{ID: person, Name: name, SortName: SortTitle(name)}); err != nil {
			return id.ID{}, err
		}
	}
	for _, x := range ids {
		err := q.InsertPersonExternalID(ctx, dbq.InsertPersonExternalIDParams{Provider: x.Provider, Kind: x.Kind, ExtID: x.ID, PersonID: person})
		if err != nil {
			return id.ID{}, err
		}
	}
	return person, nil
}
