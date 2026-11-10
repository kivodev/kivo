package catalog

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"

	"github.com/mmcdole/kivo/internal/id"
	"github.com/mmcdole/kivo/internal/users"
)

// VisibleWork is the rule for whether a Viewer may see a work, as a SQL
// predicate on the work row aliased w. It reads @all_libraries,
// @libraries, @ceiling and @show_unrated, which VisibleArgs binds, so a
// statement that carries it takes VisibleArgs.
//
// The work must be available in a granted library, or in any library when
// the Viewer has every library. Its rating rank must be within the
// Viewer's ceiling. The rank is w's own, or else its root's for a work
// under one. An unrated work passes if the Viewer shows unrated works or
// the work isn't video: blocking unrated works hides only movies, series
// with their seasons and episodes, videos and extras.
//
// The "-- viewer-filter" marker lets tests find the predicate in a
// statement.
var VisibleWork = `EXISTS ( -- viewer-filter
  SELECT 1 FROM work_library vl
  WHERE vl.work_id = w.id
    AND ` + Granted("vl.library_id") + `
    AND coalesce(coalesce(w.content_rating_age,
      (SELECT vr.content_rating_age FROM work vr WHERE vr.id = CASE WHEN w.parent_id IS NOT NULL THEN w.root_id END)) <= @ceiling,
      @show_unrated OR w.kind NOT IN ('movie', 'series', 'season', 'episode', 'extra', 'video')))`

// Granted applies the Viewer's library grants to a library ID column. It
// reads @all_libraries and @libraries, which VisibleArgs binds.
func Granted(library string) string {
	return "(@all_libraries OR " + library + " IN (SELECT unhex(value) FROM json_each(@libraries)))"
}

// VisibleArgs returns the named parameters that VisibleWork and Granted
// read, for v.
func VisibleArgs(v users.Viewer) []any {
	ceiling := int64(math.MaxInt64)
	if c, ok := v.RatingCeiling(); ok {
		ceiling = c
	}
	return []any{
		sql.Named("all_libraries", v.AllLibraries()),
		sql.Named("libraries", id.Set(v.Libraries()...)),
		sql.Named("ceiling", ceiling),
		sql.Named("show_unrated", v.ShowsUnrated()),
	}
}

var workVisible = "SELECT 1 FROM work w WHERE w.id = @work AND " + VisibleWork

// Visible returns nil when a viewer may see a work. It returns ErrNotFound
// when the viewer may not, or when there's no such work.
func (c *Catalog) Visible(ctx context.Context, v users.Viewer, work id.ID) error {
	var one int
	err := c.db.Reader().QueryRowContext(ctx, workVisible, append(VisibleArgs(v), sql.Named("work", work))...).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("%w: work %s", ErrNotFound, work)
	}
	return err
}
