package query

import (
	"cmp"
	"context"
	"database/sql"

	"github.com/mmcdole/kivo/internal/id"
)

// imagesOf reads the artwork of a set of works. It is attached after a page
// is read, using one statement rather than widening every summary's
// columns.
var imagesOf = stmt("ImagesOf", `
SELECT i.work_id, i.kind, i.id, i.blurhash, i.width, i.height
FROM image i
JOIN work w ON w.id = i.work_id
WHERE i.work_id IN (SELECT unhex(value) FROM json_each(@works))
  AND i.kind IN ('poster', 'backdrop', 'still', 'logo')
  AND `+visible)

// Image is one piece of a work's artwork. The metadata package serves it
// re-encoded.
type Image struct {
	ID            id.ID
	Blurhash      string
	Width, Height int
}

// Images is a summary's artwork. A season or episode without its own poster,
// backdrop or logo shows its series'.
type Images struct {
	Poster, Backdrop, Still, Logo *Image
}

// fill takes the poster, backdrop and logo that im lacks from above, which
// may be nil.
func (im *Images) fill(above *Images) {
	if above == nil {
		return
	}
	im.Poster = cmp.Or(im.Poster, above.Poster)
	im.Backdrop = cmp.Or(im.Backdrop, above.Backdrop)
	im.Logo = cmp.Or(im.Logo, above.Logo)
}

// withImages attaches artwork to summaries, using one statement for all of
// them. The statement carries the same Viewer predicate as every other read.
func (s *Service) withImages(ctx context.Context, vw viewer, lists ...[]WorkSummary) error {
	var works []id.ID
	seen := map[id.ID]bool{}
	add := func(w id.ID) {
		if !w.IsZero() && !seen[w] {
			seen[w] = true
			works = append(works, w)
		}
	}
	for _, l := range lists {
		for _, c := range l {
			add(c.ID)
			add(c.Root)
		}
	}
	if len(works) == 0 {
		return nil
	}
	byWork := map[id.ID]*Images{}
	err := each(ctx, s.db.Reader(), imagesOf, vw.args(sql.Named("works", id.Set(works...))), func(r scanner) error {
		return scanImage(r, byWork)
	})
	if err != nil {
		return err
	}
	for _, l := range lists {
		for i := range l {
			c := &l[i]
			if own := byWork[c.ID]; own != nil {
				c.Images = *own
			}
			c.Images.fill(byWork[c.Root])
		}
	}
	return nil
}

// scanImage reads a row holding an owner's ID, an artwork kind and the
// image's columns, and stores the image in the owner's artwork.
func scanImage(r scanner, by map[id.ID]*Images) error {
	var owner id.ID
	var kind string
	var img Image
	if err := r.Scan(&owner, &kind, &img.ID, &img.Blurhash, &img.Width, &img.Height); err != nil {
		return err
	}
	im := by[owner]
	if im == nil {
		im = &Images{}
		by[owner] = im
	}
	switch kind {
	case "poster":
		im.Poster = &img
	case "backdrop":
		im.Backdrop = &img
	case "still":
		im.Still = &img
	case "logo":
		im.Logo = &img
	}
	return nil
}

// collectionImagesOf reads the artwork of a set of collections: each
// collection's own poster and backdrop. A collection with no poster of its
// own takes the poster of its first work that the Viewer sees and that has
// a poster, in the order the collection's page lists its works.
var collectionImagesOf = stmt("CollectionImagesOf", `
SELECT i.collection_id, i.kind, i.id, i.blurhash, i.width, i.height
FROM image i
WHERE i.collection_id IN (SELECT unhex(value) FROM json_each(@collections))
  AND i.kind IN ('poster', 'backdrop')
UNION ALL
SELECT c.id, 'poster', i.id, i.blurhash, i.width, i.height
FROM collection c
JOIN image i ON i.id = (
  SELECT wi.id FROM collection_work cw
  JOIN work w ON w.id = cw.work_id
  JOIN image wi ON wi.work_id = w.id AND wi.kind = 'poster'
  WHERE cw.collection_id = c.id AND NOT w.provisional
    AND `+visible+`
  ORDER BY coalesce(w.year, 0), w.sort_title, w.id
  LIMIT 1)
WHERE c.id IN (SELECT unhex(value) FROM json_each(@collections))
  AND NOT EXISTS (SELECT 1 FROM image o WHERE o.collection_id = c.id AND o.kind = 'poster')`)

// withCollectionImages attaches artwork to collections, using one statement
// for all of them. The statement carries the same Viewer predicate as every
// other read.
func (s *Service) withCollectionImages(ctx context.Context, vw viewer, colls []Collection) error {
	if len(colls) == 0 {
		return nil
	}
	ids := make([]id.ID, len(colls))
	for i, c := range colls {
		ids[i] = c.ID
	}
	byColl := map[id.ID]*Images{}
	err := each(ctx, s.db.Reader(), collectionImagesOf, vw.args(sql.Named("collections", id.Set(ids...))), func(r scanner) error {
		return scanImage(r, byColl)
	})
	if err != nil {
		return err
	}
	for i := range colls {
		if im := byColl[colls[i].ID]; im != nil {
			colls[i].Images = *im
		}
	}
	return nil
}
