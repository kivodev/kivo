package metadata

import (
	"cmp"
	"context"
	"encoding/json"
	"maps"
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/mmcdole/kivo/internal/catalog"
	"github.com/mmcdole/kivo/internal/metadata/dbq"
)

// discFolder matches a CD or disc folder. Its files belong to the book in
// the folder above it.
var discFolder = regexp.MustCompile(`(?i)^(?:cd|dis[ck])\s*\d{1,3}$`)

// BookFolder returns the folder a book's file belongs to. That is the folder
// the file is in, or the one above it when the file is in a CD or disc
// folder. It returns "" for a file directly in the library folder, which is
// a book of its own.
func BookFolder(rel string) string {
	dir := path.Dir(rel)
	if discFolder.MatchString(path.Base(dir)) {
		dir = path.Dir(dir)
	}
	if dir == "." {
		return ""
	}
	return dir
}

// Disc returns the number of the disc folder a book's file is in, or zero.
func Disc(rel string) int {
	d := path.Base(path.Dir(rel))
	if !discFolder.MatchString(d) {
		return 0
	}
	n, _ := strconv.Atoi(strings.TrimLeft(d, "cdiskCDISK \t"))
	return n
}

// bookPath holds what a book's folder names say about it.
type bookPath struct {
	title, series, position, narrator, asin string
	authors                                 []string
	year                                    int
}

var (
	// bracketedASIN matches an ASIN in brackets, alone or between spaces,
	// such as "[B0CTKSG1VB]".
	bracketedASIN = regexp.MustCompile(`(?:^| )\[(` + asinPattern + `)\](?: |$)`)
	// narratorSuffix matches a narrator in braces at the end, such as
	// "Title {Ray Porter}".
	narratorSuffix = regexp.MustCompile(`^(.*) \{(.*)\}$`)
	// positionPart matches one " - " part of a title folder that gives a
	// series position, such as "Book 2", "Vol. 3", "Volume 12. Title",
	// "Part 1", "6. Title", "1.5" or "1-3". A bare number followed by more
	// words needs a label or a dot, so "101 Dalmatians" stays a title.
	positionPart = regexp.MustCompile(`(?i)^(vol\.? |volume |book |part )?(\d{1,3}(?:\.\d{1,2})?(?:-\d{1,3}(?:\.\d{1,2})?)?)(\.?)(?: (.*))?$`)
	// leadingYear matches a leading year, such as "1999 - Title" or
	// "(1999) - Title".
	leadingYear = regexp.MustCompile(`^ *\(?([0-9]{4})\)? * - *(.+)`)
)

// parseBookPath reads a book's folder, relative to the library folder. The
// last folder is the title. With one folder above it, that folder is the
// author. With two or more above, they are the series and then the
// author.
func parseBookPath(folder string) bookPath {
	var b bookPath
	if folder == "" {
		return b
	}
	dirs := strings.Split(folder, "/")
	title := dirs[len(dirs)-1]
	dirs = dirs[:len(dirs)-1]
	if len(dirs) > 1 {
		b.series = dirs[len(dirs)-1]
		dirs = dirs[:len(dirs)-1]
	}
	if len(dirs) > 0 {
		b.authors = splitAuthors(dirs[len(dirs)-1])
	}

	if m := bracketedASIN.FindStringSubmatchIndex(title); m != nil {
		b.asin = title[m[2]:m[3]]
		title = strings.TrimSpace(title[:m[0]] + " " + title[m[1]:])
	}
	if m := narratorSuffix.FindStringSubmatch(title); m != nil {
		title, b.narrator = m[1], m[2]
	}
	if b.series != "" {
		title, b.position = position(title)
	}
	if m := leadingYear.FindStringSubmatch(title); m != nil {
		b.year, _ = strconv.Atoi(m[1])
		title = m[2]
	}
	b.title = strings.TrimSpace(title)
	return b
}

// position takes a series position out of a title folder. The position
// comes from the first " - " part that gives one.
func position(title string) (rest, pos string) {
	parts := strings.Split(title, " - ")
	for i, p := range parts {
		m := positionPart.FindStringSubmatch(p)
		if m == nil || m[4] != "" && m[1] == "" && m[3] == "" {
			continue
		}
		parts = slices.Delete(parts, i, i+1)
		if m[4] != "" {
			parts = slices.Insert(parts, i, m[4])
		}
		return strings.Join(parts, " - "), positionLabel(m[2])
	}
	return title, ""
}

// positionLabel normalizes a single number ("01" becomes "1", "1.50"
// becomes "1.5"). A range is returned unchanged.
func positionLabel(s string) string {
	if n, err := strconv.ParseFloat(s, 64); err == nil {
		return strconv.FormatFloat(n, 'f', -1, 64)
	}
	return s
}

// splitAuthors splits a string naming several authors on " & " and ";". It
// doesn't split on commas, since an author folder may be "Last, First".
func splitAuthors(s string) []string { return splitNames(s, " & ", ";") }

// splitNarrators splits a string naming several narrators on " & ", ";"
// and commas ("Kevin R. Free, Jane Doe").
func splitNarrators(s string) []string { return splitNames(s, " & ", ";", ",") }

// splitNames splits s on each separator, keeping each name once.
func splitNames(s string, seps ...string) []string {
	parts := []string{s}
	for _, sep := range seps {
		var next []string
		for _, p := range parts {
			next = append(next, strings.Split(p, sep)...)
		}
		parts = next
	}
	var out []string
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" && !slices.Contains(out, p) {
			out = append(out, p)
		}
	}
	return out
}

// tagDate matches a release date as tags write it: a year, optionally
// followed by a month and day.
var tagDate = regexp.MustCompile(`^\d{4}(?:-\d{2}(?:-\d{2})?)?`)

// BookTitle returns a book's title. It comes from the book's folder, else
// its album tag, else its file name.
func BookTitle(rel string, tags map[string]string) string {
	return bookOf(rel, tags).Title
}

// bookOf describes a book from its first file's path and tags.
//
// The folders come first, laid out as Author/Title, Author/Series/Title or
// Author/Series/Book N - Title. The tags fill in what the folders don't say.
// The narrator, description, year and publisher can always come from tags.
// The series comes from tags only when there is no series folder. Names are
// used as written.
func bookOf(rel string, tags map[string]string) catalog.WorkMetadata {
	folder := BookFolder(rel)
	p := parseBookPath(folder)
	b := &catalog.BookMetadata{Series: []catalog.SeriesRef{}}
	m := catalog.WorkMetadata{Book: b, Overview: clean(tags["description"], maxOverview), Companies: []string{}}
	if p := clean(tags["publisher"], maxName); p != "" {
		m.Companies = []string{p} // the publisher
	}

	stem := strings.TrimSuffix(path.Base(rel), path.Ext(rel))
	m.Title = clean(cmp.Or(p.title, tags["album"], stem), maxTitle)

	b.Authors = p.authors
	if len(b.Authors) == 0 {
		b.Authors = splitAuthors(cmp.Or(tags["album_artist"], tags["artist"]))
	}
	b.Narrators = splitNarrators(cmp.Or(p.narrator, tags["composer"]))

	date := tagDate.FindString(tags["date"])
	if p.year != 0 && year(date) != p.year {
		date = strconv.Itoa(p.year)
	}
	m.ReleaseDate, m.Year = date, year(date)

	b.ASIN = p.asin
	if a := bracketedASIN.FindStringSubmatch(stem); b.ASIN == "" && a != nil {
		b.ASIN = a[1]
	}
	b.ASIN = cmp.Or(b.ASIN, tags["asin"])

	author := ""
	if len(b.Authors) > 0 {
		author = b.Authors[0]
	}
	series, pos := p.series, p.position
	if series == "" {
		series = cmp.Or(tags["series"], tags["grouping"])
		if slices.Contains(b.Authors, series) {
			series = "" // a grouping tag that names the author isn't a series
		}
		if series != "" {
			pos = positionLabel(tags["part"])
		}
	}
	if series != "" {
		b.Series = append(b.Series, catalog.SeriesRef{
			Key: catalog.LocalSeriesKey(author, series), Name: clean(series, maxTitle), Position: pos, Primary: true,
		})
	}
	return m
}

// fromBook describes a book from its files. The description comes from the
// first file's folders and tags. cover is the picture embedded in the first
// file that has one, which is used when the folder has no cover image. d is
// the files' total running time.
func (s *Service) fromBook(ctx context.Context, w catalog.Work) (m *catalog.WorkMetadata, cover *embeddedCover, d time.Duration, err error) {
	rows, err := s.q.BookFiles(ctx, w.ID)
	if err != nil {
		return nil, nil, 0, err
	}
	if len(rows) == 0 {
		return nil, nil, 0, nil // none of the book's files are present
	}
	var tags map[string]string
	if rows[0].Tags.Valid {
		if err := json.Unmarshal([]byte(rows[0].Tags.String), &tags); err != nil {
			return nil, nil, 0, err
		}
	}
	for _, r := range rows {
		d += time.Duration(r.DurationMs) * time.Millisecond
	}
	book := bookOf(rows[0].RelPath, tags)
	if i := slices.IndexFunc(rows, func(r dbq.BookFilesRow) bool { return r.CoverWidth > 0 }); i >= 0 {
		r := rows[i]
		cover = &embeddedCover{
			file:  localFile{folder: r.FolderPath, rel: r.RelPath, mtime: time.UnixMilli(r.Mtime)},
			width: int(r.CoverWidth), height: int(r.CoverHeight),
		}
	}
	return &book, cover, d, nil
}

// embeddedCover is a cover picture embedded in a book's file.
type embeddedCover struct {
	file          localFile
	width, height int
}

// bookImages lists the names a cover image can have in a book's folder.
// An image named after the folder also counts.
var bookImages = []string{"cover", "folder", "poster"}

// bookCovers finds a book's covers in its folder. It looks for an image
// with a cover name or named after the folder, else any image there. Each
// one found is offered as a choice. It reads no NFO from a book's folder,
// because the ones audiobook tools write are plain text.
func (d *dirs) bookCovers(out *local, files []string) {
	seen := map[string]bool{}
	for _, f := range files {
		folder := cmp.Or(BookFolder(f), ".")
		if seen[folder] {
			continue
		}
		seen[folder] = true
		names := append(slices.Clone(bookImages), path.Base(folder))
		if folder == "." { // a one-file book directly in the library folder
			_, base := splitMedia(f)
			names = []string{base}
		}
		for _, n := range names {
			d.artwork(out, artworkPoster, folder, n)
		}
		if _, ok := out.artwork[artworkPoster]; !ok && folder != "." {
			d.anyImage(out, folder)
		}
	}
}

// anyImage offers every image in dir as a book's cover. When no named cover
// was found, it uses the first image by name.
func (d *dirs) anyImage(out *local, dir string) {
	d.find(dir, "") // fills d.list[dir]
	for _, lower := range slices.Sorted(maps.Keys(d.list[dir])) {
		if !slices.Contains(localArtworkExts, path.Ext(lower)) {
			continue
		}
		f, ok := d.find(dir, lower)
		if !ok {
			continue
		}
		if !slices.ContainsFunc(out.choices[artworkPoster], func(c localFile) bool { return c.origin() == f.origin() }) {
			out.choices[artworkPoster] = append(out.choices[artworkPoster], f)
		}
		if _, have := out.artwork[artworkPoster]; !have {
			out.artwork[artworkPoster] = f
		}
	}
}
