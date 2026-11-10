package catalog

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/mmcdole/kivo/internal/catalog/dbq"
	"github.com/mmcdole/kivo/internal/id"
)

// CheckInvariants checks the catalog's integrity rules across the whole
// database and returns every violation. Tests run it after each test.
func (c *Catalog) CheckInvariants(ctx context.Context) error {
	var errs []error
	for _, rule := range []struct {
		name  string
		query func(context.Context) ([]id.ID, error)
	}{
		{"version without parts", c.q.VersionsWithoutParts},
		{"version without works", c.q.VersionsWithoutWorks},
		{"version with parts in several libraries", c.q.VersionsAcrossLibraries},
		{"identified version with a part outside its root work's library", c.q.VersionsOutsideTheirRootsLibrary},
		{"root work outside its folder's or its movie's library", c.q.RootsOutsideTheirFoldersLibrary},
		{"root work of another kind than its library holds", c.q.RootsOfAnotherKindThanTheirLibrary},
		{"work with a parent that has a library", c.q.LibrariesOfNonRoots},
		{"version with parts not numbered 1 to n", c.q.VersionsWithGappedParts},
		{"folder linked to a work that isn't a root", c.q.FoldersOfNonRoots},
		{"work whose root isn't its top ancestor", c.q.WorksWithWrongRoot},
		{"work under the wrong kind of parent", c.q.WorksWithWrongParent},
		{"season without a number, or episode without its season's", c.q.WorksWithoutNumbers},
		{"episode whose season number isn't its season's", c.q.EpisodesWithWrongSeasonNumber},
	} {
		bad, err := rule.query(ctx)
		if err != nil {
			return fmt.Errorf("check %s: %w", rule.name, err)
		}
		for _, b := range bad {
			errs = append(errs, fmt.Errorf("%s: %v", rule.name, b))
		}
	}

	want, err := c.q.ExpectedWorkLibraries(ctx)
	if err != nil {
		return err
	}
	got, err := c.q.ActualWorkLibraries(ctx)
	if err != nil {
		return err
	}
	exp := make([]dbq.ActualWorkLibrariesRow, len(want))
	for i, w := range want {
		exp[i] = dbq.ActualWorkLibrariesRow(w)
	}
	if !slices.Equal(exp, got) {
		errs = append(errs, fmt.Errorf("work_library is stale:\n want %v\n got  %v", exp, got))
	}

	rows, err := c.db.Reader().QueryContext(ctx, `PRAGMA foreign_key_check`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var table, parent string
		var rowid, fk any
		if err := rows.Scan(&table, &rowid, &parent, &fk); err != nil {
			return err
		}
		errs = append(errs, fmt.Errorf("foreign key: %s row %v references a missing %s", table, rowid, parent))
	}
	return errors.Join(append(errs, rows.Err())...)
}
