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
	base := "https://i.ytimg.com/vi/" + t.Key + "/"
	return []string{base + "maxresdefault.jpg", base + "sddefault.jpg", base + "hqdefault.jpg"}
}

// trailerPlan is a trailer's thumbnail, already encoded, for a refresh to
// record.
type trailerPlan struct {
	key   string
	thumb dbq.InsertTrailerImageParams
}

// planTrailers encodes the thumbnails of the trailers a refresh writes,
// before the refresh's transaction. A trailer that has its thumbnail keeps
// it. A thumbnail that can't be used is logged and skipped, since the
// trailer is shown without one. A trailer whose site has no thumbnail at any
// size is dropped from m, since its video is gone. An outage of its site is
// returned, as it is for artwork, so the refresh runs again once the site is
// back. What it encodes is added to enc.
func (s *Service) planTrailers(ctx context.Context, w catalog.Work, m *catalog.WorkMetadata, enc *encoded) ([]trailerPlan, error) {
	if m == nil {
		return nil, nil
	}
	var plans []trailerPlan
	kept := m.Trailers[:0]
	for _, t := range m.Trailers {
		urls := thumbnailsOf(t)
		cur, err := s.q.TrailerThumb(ctx, dbq.TrailerThumbParams{WorkID: w.ID, Key: t.Key})
		switch {
		case err == nil:
			if slices.Contains(urls, cur.Origin) && s.present(cur) {
				kept = append(kept, t)
				continue
			}
		case !errors.Is(err, sql.ErrNoRows):
			return nil, err
		}
		p, gone, err := s.fetchThumbnail(ctx, w, t, urls)
		if err != nil {
			return nil, err
		}
		if gone {
			continue
		}
		kept = append(kept, t)
		if p != nil {
			plans = append(plans, *p)
			enc.add(p.thumb.ID, artworkThumb)
		}
	}
	m.Trailers = kept
	return plans, nil
}

// fetchThumbnail encodes the largest thumbnail a trailer has. It returns
// nil when the trailer has none that can be used, and gone when its site has
// none at any size.
func (s *Service) fetchThumbnail(ctx context.Context, w catalog.Work, t catalog.Trailer, urls []string) (_ *trailerPlan, gone bool, _ error) {
	for _, u := range urls {
		img, err := s.encodeRemote(ctx, artworkThumb, remoteImage{provider: provYouTube, url: u})
		switch {
		case ctx.Err() != nil:
			return nil, false, ctx.Err()
		case errors.Is(err, ErrNotFound):
			continue // the video has no thumbnail this large
		case errors.Is(err, ErrUnavailable):
			return nil, false, err
		case err != nil:
			slog.WarnContext(ctx, "metadata: trailer thumbnail skipped", "work", w.ID, "err", err)
			return nil, false, nil
		}
		return &trailerPlan{key: t.Key, thumb: dbq.InsertTrailerImageParams{
			ID: img.ID, Kind: artworkThumb, Source: sourceProvider, Origin: u,
			Width: img.Width, Height: img.Height, Blurhash: img.Blurhash, FetchedAt: time.Now().UnixMilli(),
		}}, false, nil
	}
	return nil, len(urls) > 0, nil
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
		trailer, err := q.TrailerOf(ctx, dbq.TrailerOfParams{WorkID: work, Key: p.key})
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
