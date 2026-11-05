package identify

import (
	"context"
	"errors"
	"slices"
	"strings"

	"github.com/mmcdole/kivo/internal/store"

	"github.com/mmcdole/kivo/internal/catalog"
	"github.com/mmcdole/kivo/internal/fault"
	"github.com/mmcdole/kivo/internal/id"
	"github.com/mmcdole/kivo/internal/metadata"
	"github.com/mmcdole/kivo/internal/users"
)

// WorkEdit is an admin's edit of a work. It can set the metadata fields that
// Fields names, the content rating, and a series' ordering.
type WorkEdit struct {
	Metadata catalog.WorkMetadata
	Fields   []string
	// ContentRating, when set, names one of the content ratings of the
	// work's content rating country. "" clears the rating.
	ContentRating *string
	// Ordering, when set, is how a series' episodes are numbered. It is
	// aired, dvd, absolute, or group:<ID> naming one of the series' TMDb
	// episode groups.
	Ordering *string
}

// EditWork writes an admin's edit of a work the Viewer can see. It locks the
// fields it sets against automatic updates. A new ordering queues the
// series' folders to be placed again in that ordering. The edit is checked
// first, then written in one transaction.
func (i *Identifier) EditWork(ctx context.Context, v users.Viewer, work id.ID, e WorkEdit) error {
	if err := i.deps.Catalog.Visible(ctx, v, work); err != nil {
		return err
	}
	m, fields, err := i.metadataEdit(ctx, work, e)
	if err != nil {
		return err
	}
	var ordering metadata.Ordering
	if e.Ordering != nil {
		if ordering, err = i.ordering(ctx, work, *e.Ordering); err != nil {
			return err
		}
	}
	var libs map[id.ID]catalog.Library
	if ordering != "" {
		if libs, err = i.libraries(ctx); err != nil {
			return err
		}
	}
	return i.db.Write(ctx, func(ctx context.Context, tx *store.Tx) error {
		if len(fields) > 0 {
			if err := i.deps.Catalog.EditMetadata(ctx, tx, work, m, fields...); err != nil {
				return err
			}
		}
		if ordering == "" {
			return nil
		}
		if err := i.deps.Catalog.SetOrdering(ctx, tx, work, string(ordering)); err != nil {
			return err
		}
		return i.reidentifyWork(ctx, tx, libs, work)
	})
}

// metadataEdit returns an edit's metadata and the fields it sets. A content
// rating is resolved to the named rating of the work's country.
func (i *Identifier) metadataEdit(ctx context.Context, work id.ID, e WorkEdit) (catalog.WorkMetadata, []string, error) {
	m, fields := e.Metadata, slices.Clone(e.Fields)
	if e.ContentRating == nil {
		return m, fields, nil
	}
	if *e.ContentRating != "" {
		r, err := i.contentRating(ctx, work, *e.ContentRating)
		if err != nil {
			return m, nil, err
		}
		m.ContentRating, m.ContentRatingAge = r.Name, &r.Age
	}
	return m, append(fields, catalog.FieldContentRating), nil
}

var (
	// ErrContentRating means an edit named a content rating its work's
	// content rating country doesn't have.
	ErrContentRating = fault.New(fault.Invalid, "not a content rating of the content rating country")
	// ErrOrdering means an edit's ordering isn't a valid ordering.
	ErrOrdering = fault.New(fault.Invalid, "an ordering is aired, dvd, absolute or group:<ID>")
)

// contentRating returns the content rating with the given name in the
// work's content rating country.
func (i *Identifier) contentRating(ctx context.Context, work id.ID, name string) (metadata.ContentRating, error) {
	_, country, err := i.deps.Metadata.Locale(ctx, work)
	if err != nil {
		return metadata.ContentRating{}, err
	}
	ratings := i.deps.Metadata.ContentRatings(country)
	n := slices.IndexFunc(ratings, func(r metadata.ContentRating) bool { return r.Name == name })
	if n < 0 {
		return metadata.ContentRating{}, fault.About(ErrContentRating, fault.Field{Name: "contentRating", Code: "invalid"})
	}
	return ratings[n], nil
}

// ordering checks an edit's ordering for a series. It must be aired, dvd,
// absolute, or group:<ID> naming one of the series' TMDb episode groups.
func (i *Identifier) ordering(ctx context.Context, series id.ID, o string) (metadata.Ordering, error) {
	switch metadata.Ordering(o) {
	case metadata.Aired, metadata.DVD, metadata.Absolute:
		return metadata.Ordering(o), nil
	}
	group, ok := strings.CutPrefix(o, "group:")
	if !ok || group == "" {
		return "", fault.About(ErrOrdering, fault.Field{Name: "/series/ordering", Code: "invalid"})
	}
	w, err := i.deps.Catalog.Work(ctx, series)
	if err != nil {
		return "", err
	}
	lang, _, err := i.deps.Metadata.Locale(ctx, series)
	if err != nil {
		return "", err
	}
	var groups []string
	if n := slices.IndexFunc(w.ExternalIDs, func(x catalog.ExternalID) bool { return x.Provider == "tmdb" }); n >= 0 {
		rctx, cancel := context.WithTimeout(ctx, requestWait)
		groups, err = i.deps.Metadata.EpisodeGroups(rctx, w.ExternalIDs[n], lang)
		cancel()
		if err != nil && !errors.Is(err, metadata.ErrNotFound) {
			return "", err
		}
	}
	if !slices.Contains(groups, group) {
		// The message names the group, so it needs a fault of its own.
		unknown := fault.New(fault.Invalid, "TMDb lists no episode group "+group+" for this series")
		return "", fault.About(unknown, fault.Field{Name: "/series/ordering", Code: "unknownEpisodeGroup"})
	}
	return metadata.Ordering(group), nil
}
