package metadata

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"slices"
	"time"

	"github.com/mmcdole/kivo/internal/catalog"
	"github.com/mmcdole/kivo/internal/id"
	"github.com/mmcdole/kivo/internal/metadata/dbq"
	"github.com/mmcdole/kivo/internal/store"
)

// thumbnailsOf lists the addresses a trailer's thumbnail is fetched from,
// largest first. YouTube serves each video's thumbnail at fixed addresses
// that need no key. The 1280×720 one exists only for videos uploaded in HD.
// The 640×480 and 480×360 ones pad the picture to 4:3, and the last exists
// for every video.
func thumbnailsOf(t catalog.Trailer) []string {
	if t.Site != catalog.TrailerYouTube {
		return nil
	}
	base := "https://i.ytimg.com/vi/" + t.Key + "/"
	return []string{base + "maxresdefault.jpg", base + "sddefault.jpg", base + "hqdefault.jpg"}
}

// trailerPlan is a trailer's thumbnail, already encoded, for a refresh to
// record.
type trailerPlan struct {
	site, key string
	thumb     dbq.InsertTrailerImageParams
}

// planTrailers encodes the thumbnails of the trailers a refresh writes,
// before the refresh's transaction. A trailer that has its thumbnail keeps
// it. A thumbnail that can't be fetched or used is logged and skipped, since
// the trailer is shown without one.
func (s *Service) planTrailers(ctx context.Context, w catalog.Work, m *catalog.WorkMetadata) ([]trailerPlan, error) {
	if m == nil {
		return nil, nil
	}
	var plans []trailerPlan
	for _, t := range m.Trailers {
		urls := thumbnailsOf(t)
		cur, err := s.q.TrailerThumb(ctx, dbq.TrailerThumbParams{WorkID: w.ID, Site: t.Site, Key: t.Key})
		switch {
		case err == nil:
			if slices.Contains(urls, cur.Origin) && s.present(cur) {
				continue
			}
		case !errors.Is(err, sql.ErrNoRows):
			s.deleteEncodedTrailers(plans)
			return nil, err
		}
		for _, u := range urls {
			img, err := s.encodeRemote(ctx, artworkThumb, remoteImage{provider: t.Site, url: u})
			if ctx.Err() != nil {
				s.deleteEncodedTrailers(plans)
				return nil, ctx.Err()
			}
			if errors.Is(err, ErrNotFound) {
				continue // the video has no thumbnail this large
			}
			if err != nil {
				slog.WarnContext(ctx, "metadata: trailer thumbnail skipped", "work", w.ID, "site", t.Site, "err", err)
				break
			}
			plans = append(plans, trailerPlan{site: t.Site, key: t.Key, thumb: dbq.InsertTrailerImageParams{
				ID: img.ID, Kind: artworkThumb, Source: sourceProvider, Origin: u,
				Width: img.Width, Height: img.Height, Blurhash: img.Blurhash, FetchedAt: time.Now().UnixMilli(),
			}})
			break
		}
	}
	return plans, nil
}

// recordTrailers writes the trailers' thumbnails in the refresh's
// transaction, after the work's trailers are written. before holds the
// thumbnails the work's trailers had until then. The files of those whose
// trailer is gone are deleted once the transaction commits. A thumbnail
// whose trailer wasn't written is dropped.
func (s *Service) recordTrailers(ctx context.Context, tx *store.Tx, work id.ID, before []dbq.Image, plans []trailerPlan) error {
	q := dbq.New(tx)
	kept, err := q.TrailerImages(ctx, work)
	if err != nil {
		return err
	}
	for _, b := range before {
		if !slices.ContainsFunc(kept, func(k dbq.Image) bool { return k.ID == b.ID }) {
			tx.AfterCommit(func() { s.deleteImage(b) })
		}
	}
	for _, p := range plans {
		trailer, err := q.TrailerOf(ctx, dbq.TrailerOfParams{WorkID: work, Site: p.site, Key: p.key})
		if errors.Is(err, sql.ErrNoRows) {
			tx.AfterCommit(func() { s.deleteFiles(p.thumb.ID, artworkThumb) })
			continue
		}
		if err != nil {
			return err
		}
		old, err := q.DeleteTrailerImages(ctx, dbq.DeleteTrailerImagesParams{TrailerID: trailer, Kind: artworkThumb})
		if err != nil {
			return err
		}
		tx.AfterCommit(func() {
			for _, r := range old {
				s.deleteImage(r)
			}
		})
		p.thumb.TrailerID = trailer
		if err := q.InsertTrailerImage(ctx, p.thumb); err != nil {
			return err
		}
	}
	return nil
}

// deleteEncodedTrailers removes the files of encoded thumbnails that won't
// be recorded.
func (s *Service) deleteEncodedTrailers(plans []trailerPlan) {
	for _, p := range plans {
		s.deleteFiles(p.thumb.ID, artworkThumb)
	}
}
