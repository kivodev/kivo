package metadata

import (
	"cmp"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/mmcdole/kivo/internal/atomicfile"
	"github.com/mmcdole/kivo/internal/catalog"
	"github.com/mmcdole/kivo/internal/fault"
	"github.com/mmcdole/kivo/internal/id"
	"github.com/mmcdole/kivo/internal/media"
	"github.com/mmcdole/kivo/internal/metadata/dbq"
	"github.com/mmcdole/kivo/internal/store"
	"github.com/mmcdole/kivo/internal/users"
)

// artworkKinds lists the artwork kinds each kind of work has. An admin can
// choose every kind except banners and thumbs (see choosable).
var artworkKinds = map[catalog.WorkKind][]string{
	catalog.KindMovie:   {artworkPoster, artworkBackdrop, artworkLogo, artworkBanner, artworkThumb},
	catalog.KindVideo:   {artworkPoster, artworkBackdrop, artworkLogo, artworkBanner, artworkThumb},
	catalog.KindSeries:  {artworkPoster, artworkBackdrop, artworkLogo, artworkBanner, artworkThumb},
	catalog.KindSeason:  {artworkPoster},
	catalog.KindEpisode: {artworkStill},
	catalog.KindBook:    {artworkPoster},
}

// choosable returns the artwork kinds an admin can choose for a kind of
// work. That is every kind it has except banners and thumbs, which only
// refreshes set.
func choosable(k catalog.WorkKind) []string {
	return slices.DeleteFunc(slices.Clone(artworkKinds[k]), func(kind string) bool { return kind == artworkBanner || kind == artworkThumb })
}

// MaxUpload is the largest image UploadArtwork takes, in bytes.
const MaxUpload = maxImageBytes

// Artwork is one image an admin can choose for a work. It is either the
// work's current artwork of a kind, or an image offered from TMDb or from a
// file next to the media. SelectArtwork takes its ID, and ImagePath serves
// it by ID. For artwork not yet chosen, ImagePath serves a preview.
type Artwork struct {
	ID            id.ID
	Kind          string
	Source        string // provider, local or upload
	Provider      string // tmdb when Source is provider
	Language      string // ISO 639-1; empty when textless or unknown
	Width, Height int    // the original's size, zero when unknown
	Selected      bool   // whether this is the work's current artwork of its kind
	Blurhash      string // set only on the selected one
}

// requestTimeout bounds Artwork and SelectArtwork, since a client waits on
// them.
const requestTimeout = 10 * time.Second

// choice is an image a listing found, before it is recorded.
type choice struct {
	source, provider, origin, preview, folder, lang string
	width, height                                   int
}

// Artwork lists the images an admin can choose for a work's artwork of one
// kind, or of every kind the work has when kind is empty.
//
// Each kind lists the current artwork first. The files next to the media
// follow, newest first. TMDb's images in the metadata language and its
// textless images come last, best first. TMDb is asked through the fetch
// pipeline, so its cache and rate limits apply.
//
// Each image offered is recorded with an ID that stays the same for as long
// as it is offered. The work must be one the Viewer can see.
func (s *Service) Artwork(ctx context.Context, v users.Viewer, work id.ID, kind string) ([]Artwork, error) {
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	if err := s.deps.Catalog.Visible(ctx, v, work); err != nil {
		return nil, err
	}
	w, err := s.deps.Catalog.Work(ctx, work)
	if err != nil {
		return nil, notFound(err)
	}
	kinds := choosable(w.Kind)
	if kind != "" {
		if !slices.Contains(kinds, kind) {
			return nil, fault.About(fmt.Errorf("%w: %s for a %s", ErrNoSuchArtwork, kind, w.Kind), fault.Field{Name: "type", Code: "invalid"})
		}
		kinds = []string{kind}
	}
	remote, err := s.providerChoices(ctx, w)
	if err != nil {
		return nil, err
	}
	l, err := s.fromLocal(ctx, w)
	if err != nil {
		return nil, err
	}
	rows, err := s.q.WorkImages(ctx, work)
	if err != nil {
		return nil, err
	}

	var out []Artwork
	var dropped []dbq.DropArtworkRow
	now := time.Now().UnixMilli()
	err = s.db.Write(ctx, func(ctx context.Context, tx *store.Tx) error {
		out, dropped = nil, nil
		q := dbq.New(tx)
		for _, k := range kinds {
			var cs []choice
			for _, f := range l.choices[k] {
				cs = append(cs, choice{source: sourceLocal, origin: f.origin(), folder: f.folder})
			}
			cs = append(cs, remote[k]...)
			cur, curOrigin := -1, ""
			if i := slices.IndexFunc(rows, func(r dbq.Image) bool { return r.Kind == k }); i >= 0 {
				cur, curOrigin = len(out), rows[i].Origin
				out = append(out, s.artworkOf(rows[i]))
			}
			for i, c := range cs {
				if cur >= 0 && c.origin == curOrigin {
					out[cur].Language = c.lang // the current artwork, as found by this listing
					continue
				}
				cid, err := q.PutArtwork(ctx, dbq.PutArtworkParams{
					ID: id.New(), WorkID: work, Kind: k, Source: c.source, Provider: nullString(c.provider), Origin: c.origin,
					Preview: nullString(c.preview), Folder: nullString(c.folder), Width: nullInt(c.width), Height: nullInt(c.height), Language: nullString(c.lang),
					Ord: int64(i), ListedAt: now,
				})
				if err != nil {
					return err
				}
				out = append(out, Artwork{ID: cid, Kind: k, Source: c.source, Provider: c.provider, Language: c.lang, Width: c.width, Height: c.height})
			}
			d, err := q.DropArtwork(ctx, dbq.DropArtworkParams{WorkID: work, Kind: k, ListedAt: now})
			if err != nil {
				return err
			}
			dropped = append(dropped, d...)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	for _, d := range dropped {
		s.deleteFiles(d.ID, d.Kind)
	}
	return out, nil
}

// providerChoices gathers the video provider's images of a work by kind,
// best first, ranked the same way a refresh ranks them. A work the provider
// doesn't know has no images.
func (s *Service) providerChoices(ctx context.Context, w catalog.Work) (map[string][]choice, error) {
	lang, _, err := s.Locale(ctx, w.ID)
	if err != nil {
		return nil, err
	}
	r, ok, err := s.ref(ctx, w)
	if err != nil || !ok {
		return nil, err
	}
	imgs, err := s.video.Images(ctx, r, lang)
	if errors.Is(err, ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	out := map[string][]choice{}
	for kind, list := range imagesByKind(imgs) {
		for _, img := range ranked(kind, list, lang, imgs.Original) {
			out[kind] = append(out[kind], choice{
				source: sourceProvider, provider: s.video.Name(), origin: img.URL, preview: img.Preview, lang: img.Language,
				width: img.Width, height: img.Height,
			})
		}
	}
	return out, nil
}

// SelectArtwork makes an image that Artwork listed the work's artwork of its
// kind. The image is marked as an admin's choice, so refreshes keep it. An
// image that isn't the current artwork is fetched or read in full and
// re-encoded like any artwork. The current artwork is only marked as chosen.
// The work must be one the Viewer can see.
func (s *Service) SelectArtwork(ctx context.Context, v users.Viewer, work, artwork id.ID) (Artwork, error) {
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	if err := s.deps.Catalog.Visible(ctx, v, work); err != nil {
		return Artwork{}, err
	}
	row, err := s.q.GetImage(ctx, artwork)
	switch {
	case err == nil:
		if row.WorkID != work {
			return Artwork{}, fmt.Errorf("%w: artwork %s", ErrNotFound, artwork)
		}
		if err := s.db.Write(ctx, func(ctx context.Context, tx *store.Tx) error { return dbq.New(tx).ChooseImage(ctx, artwork) }); err != nil {
			return Artwork{}, err
		}
		row.Chosen = true
		return s.artworkOf(row), nil
	case !errors.Is(err, sql.ErrNoRows):
		return Artwork{}, err
	}
	c, err := s.q.GetArtwork(ctx, artwork)
	if errors.Is(err, sql.ErrNoRows) || err == nil && c.WorkID != work {
		return Artwork{}, fmt.Errorf("%w: artwork %s", ErrNotFound, artwork)
	}
	if err != nil {
		return Artwork{}, err
	}
	data, err := s.artworkData(ctx, c)
	if err != nil {
		return Artwork{}, err
	}
	p, err := s.encode(ctx, c.Kind, data)
	if err != nil {
		return Artwork{}, err
	}
	p.Source, p.Origin = c.Source, c.Origin
	a, err := s.put(ctx, work, p)
	if err != nil {
		return Artwork{}, err
	}
	a.Language = c.Language.String
	return a, nil
}

// UploadArtwork makes an admin's image the work's artwork of a kind. The
// image must be JPEG, PNG or WebP, up to 25 MB and 40 megapixels. It is
// marked as chosen, so refreshes keep it. It is re-encoded like any artwork,
// and the original is kept in the image directory so the cache can be
// rebuilt. The work must be one the Viewer can see.
func (s *Service) UploadArtwork(ctx context.Context, v users.Viewer, work id.ID, kind string, data []byte) (Artwork, error) {
	if err := s.deps.Catalog.Visible(ctx, v, work); err != nil {
		return Artwork{}, err
	}
	w, err := s.deps.Catalog.Work(ctx, work)
	if err != nil {
		return Artwork{}, notFound(err)
	}
	if !slices.Contains(choosable(w.Kind), kind) {
		return Artwork{}, fault.About(fmt.Errorf("%w: %s for a %s", ErrNoSuchArtwork, kind, w.Kind), fault.Field{Name: "/type", Code: "invalid"})
	}
	ext := map[string]string{"image/jpeg": ".jpg", "image/png": ".png", "image/webp": ".webp"}[http.DetectContentType(data)]
	if ext == "" {
		return Artwork{}, unsupportedUpload(fmt.Errorf("%w: only JPEG, PNG and WebP can be uploaded", ErrBadImage))
	}
	if s.deps.ImageDir == "" {
		return Artwork{}, errors.New("upload artwork: no image directory is configured")
	}
	p, err := s.encode(ctx, kind, data)
	if errors.Is(err, ErrBadImage) {
		return Artwork{}, unsupportedUpload(err)
	}
	if err != nil {
		return Artwork{}, err
	}
	p.Source, p.Origin = sourceUpload, p.ID.String()+ext
	if err := atomicfile.Write(s.uploadPath(p.Origin), data); err != nil {
		s.deleteFiles(p.ID, kind)
		return Artwork{}, err
	}
	a, err := s.put(ctx, work, p)
	if err != nil {
		os.Remove(s.uploadPath(p.Origin))
		return Artwork{}, err
	}
	return a, nil
}

// unsupportedUpload attaches err, an ErrBadImage, to the upload's file
// field.
func unsupportedUpload(err error) error {
	return fault.About(err, fault.Field{Name: "/file", Code: "unsupported"})
}

// put records a newly encoded image as the work's chosen artwork of its
// kind. It replaces the existing image of that kind, whose files are deleted
// once the transaction commits.
func (s *Service) put(ctx context.Context, work id.ID, p dbq.InsertImageParams) (Artwork, error) {
	p.WorkID, p.Chosen, p.FetchedAt = work, true, time.Now().UnixMilli()
	err := s.db.Write(ctx, func(ctx context.Context, tx *store.Tx) error {
		q := dbq.New(tx)
		rows, err := q.WorkImages(ctx, work)
		if err != nil {
			return err
		}
		for _, r := range rows {
			if r.Kind != p.Kind {
				continue
			}
			if err := q.DeleteImage(ctx, r.ID); err != nil {
				return err
			}
			tx.AfterCommit(func() { s.deleteImage(r) })
		}
		return q.InsertImage(ctx, p)
	})
	if err != nil {
		s.deleteFiles(p.ID, p.Kind)
		return Artwork{}, err
	}
	s.changed(ctx, work)
	return s.artworkOf(dbq.Image{
		ID: p.ID, WorkID: work, Kind: p.Kind, Source: p.Source, Origin: p.Origin, Width: p.Width, Height: p.Height,
		Blurhash: p.Blurhash, Chosen: true,
	}), nil
}

// previewPath returns the path of an offered image's preview. The preview is
// made the first time it is asked for, from TMDb's image at a small size or
// from the file next to the media. It is re-encoded at the kind's smallest
// width.
func (s *Service) previewPath(ctx context.Context, artwork id.ID) (string, error) {
	c, err := s.q.GetArtwork(ctx, artwork)
	if errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("%w: image %s", ErrNotFound, artwork)
	}
	if err != nil {
		return "", err
	}
	width := imageWidths[c.Kind][0]
	p := s.path(c.ID, width)
	if _, err := os.Stat(p); err == nil {
		return p, nil
	}
	data, err := s.previewData(ctx, c)
	if err == nil {
		_, err = s.encodeAs(ctx, c.ID, c.Kind, []int{width}, data)
	}
	if errors.Is(err, ErrBadImage) {
		return "", fmt.Errorf("%w: image %s is unusable", ErrNotFound, artwork)
	}
	if err != nil {
		return "", err
	}
	return p, nil
}

// artworkData reads an offered image in full.
func (s *Service) artworkData(ctx context.Context, c dbq.Artwork) ([]byte, error) {
	return s.readArtwork(ctx, c, c.Origin)
}

// previewData reads an offered image for its preview. A provider's image is
// fetched from the smaller copy the provider offered, if any.
func (s *Service) previewData(ctx context.Context, c dbq.Artwork) ([]byte, error) {
	return s.readArtwork(ctx, c, cmp.Or(c.Preview.String, c.Origin))
}

// readArtwork reads an offered image. A local image is read from its file
// through its library folder. A provider's image is fetched from url.
func (s *Service) readArtwork(ctx context.Context, c dbq.Artwork, url string) ([]byte, error) {
	if c.Source == sourceLocal {
		rel, err := filepath.Rel(c.Folder.String, c.Origin)
		if err != nil {
			return nil, err
		}
		b, err := localFile{folder: c.Folder.String, rel: filepath.ToSlash(rel)}.read()
		if errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("%w: %s", ErrNotFound, c.Origin)
		}
		return b, err
	}
	return s.fetcher.fetch(ctx, request{provider: c.Provider.String, url: url})
}

// artworkPlan holds a refresh's artwork changes. add holds images already
// encoded into the cache, waiting to be recorded. remove holds the rows they
// replace.
type artworkPlan struct {
	add    []dbq.InsertImageParams
	remove []dbq.Image
}

// maxTries is how many of a kind's provider images are tried before giving
// up and keeping the current image.
const maxTries = 3

// planArtwork picks each kind's artwork and encodes what changed. It prefers
// a local file, then the providers' images in order. An admin's choice is
// never replaced. Encoding happens here, before the refresh's transaction,
// because nothing does I/O inside a transaction. On error, whatever was
// encoded is left for pruneImages.
func (s *Service) planArtwork(ctx context.Context, w catalog.Work, f *found, l local) (plan artworkPlan, err error) {
	rows, err := s.q.WorkImages(ctx, w.ID)
	if err != nil {
		return plan, err
	}
	cur := map[string]dbq.Image{}
	for _, r := range rows {
		cur[r.Kind] = r
	}
	now := time.Now()
	for _, kind := range artworkKinds[w.Kind] {
		c, have := cur[kind]
		replace := func(p dbq.InsertImageParams) {
			p.WorkID, p.FetchedAt = w.ID, now.UnixMilli()
			plan.add = append(plan.add, p)
			if have {
				plan.remove = append(plan.remove, c)
			}
		}
		if have && c.Chosen {
			// An admin's choice stays. Its files are only rebuilt if the
			// cache lost them.
			if s.present(c) {
				continue
			}
			p, err := s.rebuild(ctx, c, l)
			if err == nil {
				p.Source, p.Origin, p.OriginMtime, p.Chosen = c.Source, c.Origin, c.OriginMtime, true
				replace(p)
				continue
			}
			if ctx.Err() != nil || errors.Is(err, ErrUnavailable) {
				return plan, err
			}
			slog.WarnContext(ctx, "metadata: chosen artwork can't be made again", "work", w.ID, "kind", kind, "err", err)
			continue
		}

		if lf, ok := l.artwork[kind]; ok {
			mtime := lf.mtime.UnixMilli()
			if have && c.Source == sourceLocal && c.Origin == lf.origin() && c.OriginMtime.Int64 == mtime && s.present(c) {
				continue
			}
			p, err := s.encodeLocal(ctx, kind, lf)
			if err == nil {
				p.Source, p.Origin, p.OriginMtime = sourceLocal, lf.origin(), sql.NullInt64{Int64: mtime, Valid: true}
				replace(p)
				continue
			}
			if ctx.Err() != nil || !errors.Is(err, ErrBadImage) && !errors.Is(err, fs.ErrNotExist) && !errors.Is(err, fs.ErrPermission) {
				return plan, err
			}
			slog.WarnContext(ctx, "metadata: local artwork skipped", "file", lf.origin(), "err", err)
		}

		if c := l.cover; c != nil && kind == artworkPoster {
			mtime := c.file.mtime.UnixMilli()
			if have && c.file.origin() == cur[kind].Origin && cur[kind].OriginMtime.Int64 == mtime && s.present(cur[kind]) {
				continue
			}
			p, err := s.encodeCover(ctx, *c)
			if err == nil {
				p.Source, p.Origin, p.OriginMtime = sourceLocal, c.file.origin(), sql.NullInt64{Int64: mtime, Valid: true}
				replace(p)
				continue
			}
			if ctx.Err() != nil || !errors.Is(err, ErrBadImage) {
				return plan, err
			}
			slog.WarnContext(ctx, "metadata: cover picture skipped", "file", c.file.origin(), "err", err)
		}

		var remotes []remoteImage
		if f != nil {
			remotes = f.artwork[kind]
		}
		p, gone, err := s.remoteArtwork(ctx, kind, remotes, c, have)
		switch {
		case err != nil:
			return plan, err
		case p != nil:
			replace(*p)
		case gone:
			plan.remove = append(plan.remove, c)
		}
	}
	return plan, nil
}

// remoteArtwork picks a kind's artwork from a provider's images, which are
// ordered best first. cur is the current image, valid only when have is set.
//
// It returns the first usable image, encoded, to replace cur. It returns nil
// to keep cur, for example when the best image is already cur. gone reports
// that nothing offers the kind any more, so cur should be removed.
func (s *Service) remoteArtwork(ctx context.Context, kind string, remotes []remoteImage, cur dbq.Image, have bool) (_ *dbq.InsertImageParams, gone bool, _ error) {
	if len(remotes) == 0 {
		return nil, have, nil
	}
	// Providers give a changed image a new URL. So the same URL, with its
	// files present, is the same image, and it is kept with its ID.
	if have && cur.Source == sourceProvider && cur.Origin == remotes[0].url && s.present(cur) {
		return nil, false, nil
	}
	for _, r := range remotes[:min(len(remotes), maxTries)] {
		p, err := s.encodeRemote(ctx, kind, r)
		if err == nil {
			p.Source, p.Origin = sourceProvider, r.url
			return &p, false, nil
		}
		if errors.Is(err, ErrUnavailable) || ctx.Err() != nil || errors.Is(err, media.ErrSourceUnreadable) || errors.Is(err, media.ErrTimeout) {
			return nil, false, err
		}
		// One bad image among a provider's images. Try the next one.
		slog.DebugContext(ctx, "metadata: artwork skipped", "provider", r.provider, "kind", kind, "err", err)
	}
	return nil, false, nil
}

// collectionKinds lists the artwork kinds a collection has of its own.
var collectionKinds = []string{artworkPoster, artworkBackdrop}

// collectionPlan holds a refresh's changes to the artwork of its movie's
// collection. add holds images already encoded into the cache, waiting to be
// recorded. drop holds the kinds the provider no longer offers, to remove.
type collectionPlan struct {
	key  string
	add  []dbq.InsertImageParams
	drop []string
}

// planCollectionArt picks artwork for the collection a movie's refresh puts
// it in, from the provider's images of the collection. It works like
// planArtwork does for a work, and encodes what changed before the refresh's
// transaction. On error, whatever was encoded is left for pruneImages.
func (s *Service) planCollectionArt(ctx context.Context, f *found) (plan collectionPlan, err error) {
	if f == nil || f.meta.Collection == nil {
		return plan, nil
	}
	plan.key = f.meta.Collection.Key
	rows, err := s.q.CollectionImages(ctx, plan.key)
	if err != nil {
		return plan, err
	}
	now := time.Now().UnixMilli()
	for _, kind := range collectionKinds {
		i := slices.IndexFunc(rows, func(r dbq.Image) bool { return r.Kind == kind })
		var cur dbq.Image
		if i >= 0 {
			cur = rows[i]
		}
		p, gone, err := s.remoteArtwork(ctx, kind, f.collection[kind], cur, i >= 0)
		switch {
		case err != nil:
			return plan, err
		case p != nil:
			p.FetchedAt = now
			plan.add = append(plan.add, *p)
		case gone:
			plan.drop = append(plan.drop, kind)
		}
	}
	return plan, nil
}

// recordCollectionArt writes a refresh's collection artwork plan in the
// refresh's transaction, after the movie has been put in the collection.
// Each kind written replaces whatever the collection has of that kind by
// then, which a refresh of another of its movies may have written meanwhile.
// The replaced files are deleted once the transaction commits.
func (s *Service) recordCollectionArt(ctx context.Context, tx *store.Tx, plan collectionPlan) error {
	if len(plan.add) == 0 && len(plan.drop) == 0 {
		return nil
	}
	q := dbq.New(tx)
	coll, err := q.CollectionOf(ctx, plan.key)
	if err != nil {
		return err
	}
	kinds := slices.Clone(plan.drop)
	for _, p := range plan.add {
		kinds = append(kinds, p.Kind)
	}
	for _, kind := range kinds {
		old, err := q.DeleteCollectionImages(ctx, dbq.DeleteCollectionImagesParams{CollectionID: coll, Kind: kind})
		if err != nil {
			return err
		}
		tx.AfterCommit(func() {
			for _, r := range old {
				s.deleteImage(r)
			}
		})
	}
	for _, p := range plan.add {
		err := q.InsertCollectionImage(ctx, dbq.InsertCollectionImageParams{
			ID: p.ID, CollectionID: coll, Kind: p.Kind, Source: p.Source, Origin: p.Origin,
			Width: p.Width, Height: p.Height, Blurhash: p.Blurhash, FetchedAt: p.FetchedAt,
		})
		if err != nil {
			return err
		}
	}
	return nil
}

// ClearArtwork removes all artwork of a movie or series, including chosen
// and uploaded artwork. It also removes the artwork of the seasons and
// episodes under it. It runs in the caller's transaction,
// because identify calls it while unmatching the work. The files are deleted
// once the transaction commits.
func (s *Service) ClearArtwork(ctx context.Context, tx *store.Tx, work id.ID) error {
	q := dbq.New(tx)
	rows, err := q.UnmatchedImages(ctx, work)
	if err != nil {
		return err
	}
	for _, r := range rows {
		if err := q.DeleteImage(ctx, r.ID); err != nil {
			return err
		}
	}
	tx.AfterCommit(func() {
		for _, r := range rows {
			s.deleteImage(r)
		}
	})
	return nil
}

// rebuild encodes a chosen image again from its source, for when the cache
// lost its files.
func (s *Service) rebuild(ctx context.Context, c dbq.Image, l local) (dbq.InsertImageParams, error) {
	var data []byte
	var err error
	switch c.Source {
	case sourceProvider:
		data, err = s.fetcher.fetch(ctx, request{provider: s.providerOf(c.Origin), url: c.Origin})
	case sourceLocal:
		i := slices.IndexFunc(l.choices[c.Kind], func(f localFile) bool { return f.origin() == c.Origin })
		if i < 0 {
			return dbq.InsertImageParams{}, fs.ErrNotExist
		}
		data, err = l.choices[c.Kind][i].read()
	case sourceUpload:
		data, err = os.ReadFile(s.uploadPath(c.Origin))
	}
	if err != nil {
		return dbq.InsertImageParams{}, err
	}
	return s.encode(ctx, c.Kind, data)
}

func (s *Service) uploadPath(name string) string {
	return filepath.Join(s.deps.ImageDir, filepath.Base(name))
}

// providerOf names the provider of an artwork URL. It returns the video
// provider's name if it owns the URL, and empty otherwise.
func (s *Service) providerOf(u string) string {
	if s.video.Owns(u) {
		return s.video.Name()
	}
	return ""
}

func (s *Service) artworkOf(r dbq.Image) Artwork {
	a := Artwork{
		ID: r.ID, Kind: r.Kind, Source: r.Source, Width: int(r.Width), Height: int(r.Height), Selected: true, Blurhash: r.Blurhash,
	}
	if r.Source == sourceProvider {
		a.Provider = s.providerOf(r.Origin)
	}
	return a
}

func nullString(s string) sql.NullString { return sql.NullString{String: s, Valid: s != ""} }

func nullInt(n int) sql.NullInt64 { return sql.NullInt64{Int64: int64(n), Valid: n != 0} }
