package metadata

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"time"

	"github.com/mmcdole/kivo/internal/catalog"
	"github.com/mmcdole/kivo/internal/id"
	"github.com/mmcdole/kivo/internal/metadata/dbq"
	"github.com/mmcdole/kivo/internal/store"
)

// photoPlan is a credited person's photo, and an author's biography, for a
// refresh to write.
type photoPlan struct {
	person   catalog.ExternalID
	describe bool // write bio as the person's biography; set for authors
	bio      string
	photo    *dbq.InsertPersonImageParams // already encoded; nil keeps the current photo
	remove   bool                         // the person has a photo the provider no longer gives
}

// planPhotos encodes the authors' photos that changed, before the refresh's
// transaction. A photo that can't be fetched or used is logged, and the
// author keeps their current photo. Photos only add to a book, so a failure
// isn't fatal. What it encodes is added to enc.
func (s *Service) planPhotos(ctx context.Context, authors []*Author, enc *encoded) ([]photoPlan, error) {
	var plans []photoPlan
	for _, a := range authors {
		p := photoPlan{person: catalog.ExternalID{Provider: s.books.Name(), Kind: kindAuthor, ID: a.ID}, describe: true}
		if a.Bio != nil {
			p.bio = *a.Bio
		}
		cur, err := s.q.PersonPhoto(ctx, dbq.PersonPhotoParams{Provider: p.person.Provider, Kind: p.person.Kind, ExtID: p.person.ID})
		have := err == nil
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
		switch {
		case a.Image == nil:
			p.remove = have
		case have && cur.Origin == *a.Image && s.present(cur):
		default:
			img, err := s.encodeRemote(ctx, personPhoto, remoteImage{provider: s.books.Name(), url: *a.Image})
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			if err != nil {
				slog.WarnContext(ctx, "metadata: author photo skipped", "author", a.ID, "err", err)
				break
			}
			enc.add(img.ID, personPhoto)
			p.photo = &dbq.InsertPersonImageParams{
				ID: img.ID, Kind: personPhoto, Source: sourceProvider, Origin: *a.Image,
				Width: img.Width, Height: img.Height, Blurhash: img.Blurhash, FetchedAt: time.Now().UnixMilli(),
			}
		}
		plans = append(plans, p)
	}
	return plans, nil
}

// planCastPhotos encodes the provider's photos of a work's credited people,
// before the refresh's transaction. It covers each person who has no photo,
// or whose photo from this provider changed. A photo from another provider,
// such as an author's from Audnexus, is kept. A photo that can't be fetched
// or used is logged and skipped, since photos only add to a page. What it
// encodes is added to enc.
func (s *Service) planCastPhotos(ctx context.Context, photos []remotePhoto, enc *encoded) ([]photoPlan, error) {
	var plans []photoPlan
	for _, ph := range photos {
		cur, err := s.q.PersonPhoto(ctx, dbq.PersonPhotoParams{Provider: ph.person.Provider, Kind: ph.person.Kind, ExtID: ph.person.ID})
		switch {
		case err == nil:
			if (cur.Origin == ph.image.url || s.providerOf(cur.Origin) != ph.image.provider) && s.present(cur) {
				continue
			}
		case !errors.Is(err, sql.ErrNoRows):
			return nil, err
		}
		img, err := s.encodeRemote(ctx, personPhoto, ph.image)
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if err != nil {
			slog.WarnContext(ctx, "metadata: person photo skipped", "provider", ph.person.Provider, "person", ph.person.ID, "err", err)
			continue
		}
		enc.add(img.ID, personPhoto)
		plans = append(plans, photoPlan{person: ph.person, photo: &dbq.InsertPersonImageParams{
			ID: img.ID, Kind: personPhoto, Source: sourceProvider, Origin: ph.image.url,
			Width: img.Width, Height: img.Height, Blurhash: img.Blurhash, FetchedAt: time.Now().UnixMilli(),
		}})
	}
	return plans, nil
}

// recordPhotos writes the people's photos and the authors' biographies in
// the refresh's transaction. A person no credit names is left alone. That
// happens, for example, when the book's authors are locked to other
// names.
func (s *Service) recordPhotos(ctx context.Context, tx *store.Tx, plans []photoPlan) error {
	q := dbq.New(tx)
	for _, p := range plans {
		var (
			person id.ID
			err    error
		)
		if p.describe {
			person, err = s.deps.Catalog.DescribePerson(ctx, tx, p.person, p.bio)
		} else {
			person, err = s.deps.Catalog.PersonOf(ctx, tx, p.person)
		}
		if errors.Is(err, catalog.ErrNotFound) {
			if p.photo != nil {
				tx.AfterCommit(func() { s.deleteFiles(p.photo.ID, personPhoto) })
			}
			continue
		}
		if err != nil {
			return err
		}
		if p.photo == nil && !p.remove {
			continue
		}
		old, err := q.DeletePersonImages(ctx, dbq.DeletePersonImagesParams{PersonID: person, Kind: personPhoto})
		if err != nil {
			return err
		}
		tx.AfterCommit(func() {
			for _, r := range old {
				s.deleteImage(r)
			}
		})
		if p.photo != nil {
			p.photo.PersonID = person
			if err := q.InsertPersonImage(ctx, *p.photo); err != nil {
				return err
			}
		}
	}
	return nil
}
