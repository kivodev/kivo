package metadata

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/gif"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/mmcdole/kivo/internal/store"

	"github.com/mmcdole/kivo/internal/catalog"
	"github.com/mmcdole/kivo/internal/id"
)

// what summarizes a listing as "kind source language selected" strings, in
// order.
func what(artworks []Artwork) []string {
	var out []string
	for _, a := range artworks {
		parts := []string{a.Kind, a.Source}
		if a.Language != "" {
			parts = append(parts, a.Language)
		}
		if a.Selected {
			parts = append(parts, "selected")
		}
		out = append(out, strings.Join(parts, " "))
	}
	return out
}

func (e *env) artwork(work id.ID, kind string) []Artwork {
	e.t.Helper()
	artworks, err := e.s.Artwork(ctx, e.viewer(), work, kind)
	if err != nil {
		e.t.Fatal(err)
	}
	return artworks
}

// Choose artwork lists, for each kind, the current artwork, then the files
// next to the media, then TMDb's images ranked best first. Offered artwork
// keeps its ID for as long as it is offered.
func TestListArtwork(t *testing.T) {
	e := newEnv(t)
	_, folder, dir := e.localLibrary(catalog.Movies)
	writeFile(t, filepath.Join(dir, "Dune (2021)", "folder.png"), pngBytes(200, 300))
	setMtime(t, filepath.Join(dir, "Dune (2021)", "folder.png"), time.Hour)
	w := e.movie(folder, duneRel)
	e.mustRefresh(w) // the local folder.png becomes the poster
	writeFile(t, filepath.Join(dir, "Dune (2021)", "poster.jpg"), pngBytes(200, 300))

	artworks := e.artwork(w, "")
	want := []string{
		"poster local selected", // folder.png
		"poster local",          // poster.jpg
		"poster provider en",
		"poster provider en",
		"poster provider fr", // other languages last, despite the higher vote
		"backdrop provider selected",
		"backdrop provider en", // textless first, and the current one isn't listed twice
		"logo provider en selected",
	}
	if got := what(artworks); !slices.Equal(got, want) {
		t.Fatalf("listing:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	if a := artworks[2]; a.Provider != provTMDb || a.Width != 2000 || a.Height != 3000 {
		t.Errorf("a TMDb poster's size and provider: %+v", a)
	}
	if artworks[0].Blurhash == "" || artworks[0].ID != e.images(w)[artworkPoster].ID {
		t.Errorf("the current poster %+v", artworks[0])
	}

	again := e.artwork(w, artworkPoster)
	if len(again) != 5 || again[2].ID != artworks[2].ID {
		t.Errorf("listed again, an offered artwork's ID changed: %+v then %+v", artworks[2], again)
	}
	if _, err := e.s.Artwork(ctx, e.viewer(), w, artworkStill); !errors.Is(err, ErrNoSuchArtwork) {
		t.Errorf("a movie's stills: %v", err)
	}
}

// Offered artwork's preview is made the first time it is shown, from TMDb's
// image at a small size, re-encoded at the kind's smallest width.
func TestArtworkPreview(t *testing.T) {
	e := newEnv(t)
	_, folder := e.b.Library(catalog.Movies)
	w := e.movie(folder, duneRel)
	artworks := e.artwork(w, artworkPoster)
	path, err := e.s.ImagePath(ctx, artworks[0].ID, 480)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(path) != artworks[0].ID.String()+"-240.webp" {
		t.Errorf("preview at %s", path)
	}
	if e.fx.count("/t/p/w342/dune-poster-en.jpg") != 1 || e.fx.count("/t/p/original/dune-poster-en.jpg") != 0 {
		t.Error("the preview wasn't fetched at TMDb's small size")
	}
	if _, err := e.s.ImagePath(ctx, artworks[0].ID, 240); err != nil || e.fx.count("/t/p/w342/dune-poster-en.jpg") != 1 {
		t.Errorf("a made preview was fetched again: %v", err)
	}
	if _, err := e.s.ImagePath(ctx, id.New(), 240); !errors.Is(err, ErrNotFound) {
		t.Errorf("an unknown ID: %v", err)
	}
}

// Choosing offered artwork fetches it in full and marks it chosen, so a
// refresh keeps it. Choosing the current artwork just marks it chosen.
func TestSelectArtworkSurvivesRefresh(t *testing.T) {
	e := newEnv(t)
	_, folder := e.b.Library(catalog.Movies)
	w := e.movie(folder, duneRel)
	e.mustRefresh(w)
	artworks := e.artwork(w, artworkPoster)
	second := artworks[1] // dune-poster-en-2, after the refresh's pick
	got, err := e.s.SelectArtwork(ctx, e.viewer(), w, second.ID)
	if err != nil {
		t.Fatal(err)
	}
	poster := e.images(w)[artworkPoster]
	if !got.Selected || got.ID != poster.ID || !poster.Chosen || poster.Origin != tmdbImages+"/dune-poster-en-2.jpg" {
		t.Fatalf("chosen %+v, stored %+v", got, poster)
	}
	if _, err := e.s.ImagePath(ctx, artworks[0].ID, 240); !errors.Is(err, ErrNotFound) {
		t.Errorf("the replaced poster is still served: %v", err)
	}

	e.mustRefresh(w)
	if after := e.images(w)[artworkPoster]; after.ID != poster.ID {
		t.Errorf("a refresh replaced the chosen poster: %+v", after)
	}

	backdrop := e.artwork(w, artworkBackdrop)[0]
	if _, err := e.s.SelectArtwork(ctx, e.viewer(), w, backdrop.ID); err != nil || !e.images(w)[artworkBackdrop].Chosen || e.images(w)[artworkBackdrop].ID != backdrop.ID {
		t.Errorf("choosing the current backdrop: %v, %+v", err, e.images(w)[artworkBackdrop])
	}
	_, movies := e.b.Library(catalog.Movies)
	other := e.b.Movie(movies, "Heat", 1995).Work
	if _, err := e.s.SelectArtwork(ctx, e.viewer(), other, second.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("another work's artwork: %v", err)
	}
}

func TestUploadArtwork(t *testing.T) {
	e := newEnv(t)
	_, folder := e.b.Library(catalog.Movies)
	w := e.movie(folder, duneRel)
	e.mustRefresh(w)
	got, err := e.s.UploadArtwork(ctx, e.viewer(), w, artworkPoster, pngBytes(400, 600))
	if err != nil {
		t.Fatal(err)
	}
	poster := e.images(w)[artworkPoster]
	if got.Source != sourceUpload || !got.Selected || poster.ID != got.ID || !poster.Chosen || got.Width != 400 {
		t.Fatalf("uploaded %+v, stored %+v", got, poster)
	}
	original := filepath.Join(e.uploads, poster.Origin)
	if _, err := os.Stat(original); err != nil {
		t.Fatalf("the original isn't kept: %v", err)
	}
	if artworks := e.artwork(w, artworkPoster); artworks[0].ID != got.ID || artworks[0].Source != sourceUpload {
		t.Errorf("listed after upload: %+v", artworks[0])
	}

	// The cache lost it, so a refresh rebuilds it from the original.
	e.s.deleteFiles(poster.ID, artworkPoster)
	e.mustRefresh(w)
	again := e.images(w)[artworkPoster]
	if again.Source != sourceUpload || !again.Chosen || again.Origin != poster.Origin {
		t.Fatalf("after the cache lost the upload: %+v", again)
	}
	if _, err := e.s.ImagePath(ctx, again.ID, 240); err != nil {
		t.Errorf("the upload wasn't made again: %v", err)
	}

	// Once it is replaced, its original is deleted.
	if _, err := e.s.UploadArtwork(ctx, e.viewer(), w, artworkPoster, pngBytes(300, 450)); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(original); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("the replaced upload's original is still kept: %v", err)
	}

	// Unmatching clears it, including the original.
	e.b.Write(func(ctx context.Context, tx *store.Tx) error { return e.s.ClearArtwork(ctx, tx, w) })
	if entries, _ := os.ReadDir(e.uploads); len(entries) != 0 {
		t.Errorf("originals kept after clearing: %v", entries)
	}
}

func TestUploadArtworkRejects(t *testing.T) {
	e := newEnv(t)
	_, folder := e.b.Library(catalog.Movies)
	w := e.movie(folder, duneRel)
	var g bytes.Buffer
	if err := gif.Encode(&g, image.NewPaletted(image.Rect(0, 0, 10, 10), color.Palette{color.Black, color.White}), nil); err != nil {
		t.Fatal(err)
	}
	for name, data := range map[string][]byte{
		"a GIF":       g.Bytes(),
		"an SVG":      []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="10" height="10"/>`),
		"text":        []byte("not an image"),
		"over 25 MB":  append(pngBytes(10, 10), make([]byte, MaxUpload)...),
		"a truncated": pngBytes(10, 10)[:40],
	} {
		if _, err := e.s.UploadArtwork(ctx, e.viewer(), w, artworkPoster, data); !errors.Is(err, ErrBadImage) {
			t.Errorf("%s: %v", name, err)
		}
	}
	if _, err := e.s.UploadArtwork(ctx, e.viewer(), w, artworkStill, pngBytes(10, 10)); !errors.Is(err, ErrNoSuchArtwork) {
		t.Errorf("a movie's still: %v", err)
	}
	if len(e.images(w)) != 0 {
		t.Errorf("a refused upload was stored: %+v", e.images(w))
	}
	entries, _ := os.ReadDir(e.uploads)
	if len(entries) != 0 {
		t.Errorf("a refused upload was kept: %v", entries)
	}
}

// An episode's stills and a season's posters come from TMDb's images of
// that episode and season.
func TestEpisodeAndSeasonArtwork(t *testing.T) {
	e := newEnv(t)
	_, folder := e.b.Library(catalog.Shows)
	series := e.b.Work(catalog.WorkInput{Kind: catalog.KindSeries, Title: "The Expanse",
		ExternalIDs: []catalog.ExternalID{catalogID("tmdb", "series", "63639")}})
	ep, err := e.b.Catalog.Work(ctx, e.b.Episode(folder, series, 1, 2).Work)
	if err != nil {
		t.Fatal(err)
	}
	if got := what(e.artwork(ep.ID, "")); !slices.Equal(got, []string{"still provider", "still provider"}) {
		t.Errorf("episode %v", got)
	}
	// The episode is in the season it is numbered in, so no other season is
	// read.
	if n, m := e.fx.count("/3/tv/63639/season/0"), e.fx.count("/3/tv/63639/season/2"); n+m != 0 {
		t.Errorf("finding S01E02 read seasons 0 and 2: %d, %d requests", n, m)
	}
	if got := what(e.artwork(ep.Parent, "")); !slices.Equal(got, []string{"poster provider en", "poster provider de"}) {
		t.Errorf("season %v", got)
	}
}
