package catalog

import (
	"cmp"
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"time"
	"unicode/utf8"

	"github.com/mmcdole/kivo/internal/catalog/dbq"
	"github.com/mmcdole/kivo/internal/config"
	"github.com/mmcdole/kivo/internal/fault"
	"github.com/mmcdole/kivo/internal/id"
	"github.com/mmcdole/kivo/internal/store"
)

// Library is a library and its folders.
type Library struct {
	ID               id.ID
	Name             string
	Kind             LibraryKind
	UseLocalMetadata bool
	Previews         Schedule
	// Markers is when the library finds intro and credits markers in its
	// episodes. It is always Never outside a shows library.
	Markers Schedule
	// Subtitles is when the library extracts its files' embedded subtitles
	// ahead of any play. A play always extracts its own file's subtitles
	// regardless. It is always Never in an audiobooks library.
	Subtitles Schedule
	// RefreshDays is how many days pass between metadata refreshes of the
	// library's items. It is 30, 60 or 90, or 0 for never.
	RefreshDays int
	// MetadataLanguage and ContentRatingCountry override the server's
	// settings for this library. Empty means the server's setting applies.
	MetadataLanguage, ContentRatingCountry string
	Folders                                []Folder
}

// LibraryKind is what a library holds, which decides how its folders are
// identified.
type LibraryKind string

// The library kinds.
const (
	Movies     LibraryKind = "movies"
	Shows      LibraryKind = "shows"
	Videos     LibraryKind = "videos"
	Audiobooks LibraryKind = "audiobooks"
)

// Schedule is when a library does a kind of background work, such as
// making preview thumbnails, finding intro and credits markers, or
// extracting subtitles.
type Schedule string

// The schedules.
const (
	Never           Schedule = "never"
	Nightly         Schedule = "nightly"
	NightlyAndAdded Schedule = "nightly_and_added"
)

// LibraryInput creates a library.
type LibraryInput struct {
	Name             string
	Kind             LibraryKind
	Folders          []string // absolute folder paths
	UseLocalMetadata bool
	Previews         Schedule // defaults to Nightly
	Markers          Schedule // defaults to NightlyAndAdded in a shows library
	Subtitles        Schedule // defaults to NightlyAndAdded
	RefreshDays      int      // 30, 60 or 90, or 0 for never
	// MetadataLanguage is a BCP 47 tag and ContentRatingCountry is an ISO
	// 3166-1 alpha-2 code. They override the server's settings for this
	// library. Empty keeps the server's setting.
	MetadataLanguage, ContentRatingCountry string
}

// LibraryChange edits a library. Each field that is set replaces the
// library's value, and a nil field keeps it. A library's kind can't change.
type LibraryChange struct {
	Name                                   *string
	Folders                                *[]string // absolute folder paths
	UseLocalMetadata                       *bool
	Previews                               *Schedule
	Markers                                *Schedule
	Subtitles                              *Schedule
	RefreshDays                            *int
	MetadataLanguage, ContentRatingCountry *string
}

// NewLibrary returns the input to create a library of the given kind, with
// ch applied to a new library's defaults. By default a library uses local
// metadata, makes preview thumbnails nightly, finds markers and extracts
// subtitles nightly and as media is added, refreshes metadata every 30
// days, and uses the server's metadata language and content rating country.
func NewLibrary(kind LibraryKind, ch LibraryChange) LibraryInput {
	return ch.apply(Library{Kind: kind, UseLocalMetadata: true, Previews: Nightly, Markers: NightlyAndAdded, Subtitles: NightlyAndAdded, RefreshDays: 30})
}

// apply returns the library's settings with the change made.
func (ch LibraryChange) apply(l Library) LibraryInput {
	in := LibraryInput{
		Name: valueOr(ch.Name, l.Name), Kind: l.Kind, UseLocalMetadata: valueOr(ch.UseLocalMetadata, l.UseLocalMetadata),
		Previews: valueOr(ch.Previews, l.Previews), Markers: valueOr(ch.Markers, l.Markers), Subtitles: valueOr(ch.Subtitles, l.Subtitles),
		RefreshDays:          valueOr(ch.RefreshDays, l.RefreshDays),
		MetadataLanguage:     valueOr(ch.MetadataLanguage, l.MetadataLanguage),
		ContentRatingCountry: valueOr(ch.ContentRatingCountry, l.ContentRatingCountry),
	}
	if ch.Folders != nil {
		in.Folders = *ch.Folders
	} else {
		for _, f := range l.Folders {
			in.Folders = append(in.Folders, f.Path)
		}
	}
	return in
}

// valueOr returns *v when v is set, and cur otherwise.
func valueOr[T any](v *T, cur T) T {
	if v != nil {
		return *v
	}
	return cur
}

// Folder is one folder of a library.
type Folder struct {
	ID, Library id.ID
	Path        string
	Reachable   bool
	LastScan    time.Time // zero before the first scan
}

// OpenFolder opens the library folder at path as a file system rooted
// there, so no path read through it can escape the folder. It also returns
// the function that closes it.
func OpenFolder(path string) (fs.FS, func() error, error) {
	r, err := os.OpenRoot(path)
	if err != nil {
		return nil, nil, err
	}
	return r.FS(), r.Close, nil
}

// CreateLibrary creates a library and its folders, and returns it.
// scan.AddLibrary calls it, and then watches and scans the folders once the
// transaction commits.
func (c *Catalog) CreateLibrary(ctx context.Context, tx *store.Tx, in LibraryInput) (Library, error) {
	if err := in.validate(); err != nil {
		return Library{}, err
	}
	if !slices.Contains([]LibraryKind{Movies, Shows, Videos, Audiobooks}, in.Kind) {
		return Library{}, fmt.Errorf("%w: library kind %q", ErrInvalid, in.Kind)
	}
	q := dbq.New(tx)
	in = in.forKind(in.Kind)
	l := Library{
		ID: id.New(), Name: in.Name, Kind: in.Kind, UseLocalMetadata: in.UseLocalMetadata,
		Previews: in.previews(), Markers: in.markers(), Subtitles: in.subtitles(), RefreshDays: in.RefreshDays,
		MetadataLanguage: in.MetadataLanguage, ContentRatingCountry: in.ContentRatingCountry,
	}
	err := q.InsertLibrary(ctx, dbq.InsertLibraryParams{
		ID: l.ID, Name: l.Name, Kind: string(l.Kind), UseLocalMetadata: l.UseLocalMetadata,
		Previews: string(l.Previews), Markers: string(l.Markers), Subtitles: string(l.Subtitles), RefreshDays: int64(l.RefreshDays),
		MetadataLanguage: nullString(l.MetadataLanguage), ContentRatingCountry: nullString(l.ContentRatingCountry),
		CreatedAt: time.Now().UnixMilli(),
	})
	if err != nil {
		return Library{}, err
	}
	for _, p := range in.Folders {
		r := Folder{ID: id.New(), Library: l.ID, Path: filepath.Clean(p), Reachable: true}
		if err := insertFolder(ctx, q, r); err != nil {
			return Library{}, err
		}
		l.Folders = append(l.Folders, r)
	}
	c.tell(tx, []id.ID{l.ID})
	return l, nil
}

// UpdateLibrary edits a library, reading and writing it in tx. It returns
// the folders it added and the folders it removed.
//
// A folder no longer listed is marked as being removed. That takes it off
// the library's folder list and stops scans of it. Once the transaction
// commits, the caller asks TaskLibraryChanges to remove it, which deletes
// its files as if they were gone and then deletes the folder.
//
// scan.EditLibrary calls it, and then watches and scans the added folders.
func (c *Catalog) UpdateLibrary(ctx context.Context, tx *store.Tx, lib id.ID, ch LibraryChange) (added, removed []Folder, err error) {
	q := dbq.New(tx)
	row, err := q.GetLibrary(ctx, lib)
	if err != nil || row.DeletingAt.Valid {
		return nil, nil, notFound(cmp.Or(err, sql.ErrNoRows))
	}
	cur, err := libraryFrom(ctx, q, row)
	if err != nil {
		return nil, nil, err
	}
	in := ch.apply(cur)
	if err := in.validate(); err != nil {
		return nil, nil, err
	}
	in = in.forKind(cur.Kind)
	_, err = q.UpdateLibrary(ctx, dbq.UpdateLibraryParams{
		ID: lib, Name: in.Name, UseLocalMetadata: in.UseLocalMetadata,
		Previews: string(in.previews()), Markers: string(in.markers()), Subtitles: string(in.subtitles()), RefreshDays: int64(in.RefreshDays),
		MetadataLanguage: nullString(in.MetadataLanguage), ContentRatingCountry: nullString(in.ContentRatingCountry),
	})
	if err != nil {
		return nil, nil, err
	}

	want := map[string]bool{}
	for _, p := range in.Folders {
		want[filepath.Clean(p)] = true
	}
	now := time.Now().UnixMilli()
	for _, r := range cur.Folders {
		if want[r.Path] {
			delete(want, r.Path)
			continue
		}
		if err := q.MarkFolderRemoving(ctx, dbq.MarkFolderRemovingParams{ID: r.ID, Now: sql.NullInt64{Int64: now, Valid: true}}); err != nil {
			return nil, nil, err
		}
		removed = append(removed, r)
	}
	for _, p := range slices.Sorted(maps.Keys(want)) {
		r := Folder{ID: id.New(), Library: lib, Path: p, Reachable: true}
		if err := insertFolder(ctx, q, r); err != nil {
			return nil, nil, err
		}
		added = append(added, r)
	}
	c.tell(tx, []id.ID{lib})
	return added, removed, nil
}

func insertFolder(ctx context.Context, q *dbq.Queries, r Folder) error {
	err := q.InsertFolder(ctx, dbq.InsertFolderParams{ID: r.ID, LibraryID: r.Library, Path: r.Path})
	if store.IsUnique(err) {
		return fmt.Errorf("%w: %s", ErrFolderInUse, r.Path)
	}
	return err
}

// maxLibraryName is the most runes a library's name may have.
const maxLibraryName = 100

func (in LibraryInput) validate() error {
	if in.Name == "" {
		return invalidField("/name", "required", "a library needs a name")
	}
	if utf8.RuneCountInString(in.Name) > maxLibraryName {
		return invalidField("/name", "tooLong", fmt.Sprintf("a library's name is at most %d characters", maxLibraryName))
	}
	if len(in.Folders) == 0 {
		return invalidField("/folders", "required", "a library needs at least one folder")
	}
	for i, p := range in.Folders {
		if !filepath.IsAbs(p) {
			return invalidField(fmt.Sprintf("/folders/%d", i), "invalid", fmt.Sprintf("folder %q is not an absolute path", p))
		}
	}
	if !slices.Contains([]Schedule{"", Never, Nightly, NightlyAndAdded}, in.Previews) {
		return invalidField("/previews", "invalid", fmt.Sprintf("previews %q", in.Previews))
	}
	if !slices.Contains([]Schedule{"", Never, Nightly, NightlyAndAdded}, in.Markers) {
		return invalidField("/markers", "invalid", fmt.Sprintf("markers %q", in.Markers))
	}
	if !slices.Contains([]Schedule{"", Never, Nightly, NightlyAndAdded}, in.Subtitles) {
		return invalidField("/subtitles", "invalid", fmt.Sprintf("subtitles %q", in.Subtitles))
	}
	if !slices.Contains([]int{0, 30, 60, 90}, in.RefreshDays) {
		return invalidField("/refreshDays", "invalid", fmt.Sprintf("refresh days %d", in.RefreshDays))
	}
	if l := in.MetadataLanguage; l != "" && !config.ValidMetadataLanguage(l) {
		return invalidField("/metadataLanguage", "invalid", fmt.Sprintf("metadata language %q is not a language tag such as en or pt-BR", l))
	}
	if c := in.ContentRatingCountry; c != "" && !config.ValidContentRatingCountry(c) {
		return invalidField("/contentRatingCountry", "invalid", fmt.Sprintf("content rating country %q is not a two-letter country code", c))
	}
	return nil
}

// invalidField returns ErrInvalid about one field of the request. name is
// the field's JSON pointer, code is its error code, and why says what's
// wrong.
func invalidField(name, code, why string) error {
	return fault.About(fmt.Errorf("%w: %s", ErrInvalid, why), fault.Field{Name: name, Code: code})
}

// forKind turns off the settings a library's kind has no use for. An
// audiobooks library makes no preview thumbnails and has no subtitles.
// Only a shows library has episodes to find markers in.
func (in LibraryInput) forKind(kind LibraryKind) LibraryInput {
	if kind == Audiobooks {
		in.Previews, in.Subtitles = Never, Never
	}
	if kind != Shows {
		in.Markers = Never
	}
	return in
}

func (in LibraryInput) previews() Schedule {
	if in.Previews == "" {
		return Nightly
	}
	return in.Previews
}

func (in LibraryInput) markers() Schedule {
	if in.Markers == "" {
		return NightlyAndAdded
	}
	return in.Markers
}

func (in LibraryInput) subtitles() Schedule {
	if in.Subtitles == "" {
		return NightlyAndAdded
	}
	return in.Subtitles
}

// Library returns a library with its folders. It returns ErrNotFound for a
// library that is being deleted. Folders being removed are left out.
func (c *Catalog) Library(ctx context.Context, lib id.ID) (Library, error) {
	row, err := c.q.GetLibrary(ctx, lib)
	if err != nil || row.DeletingAt.Valid {
		return Library{}, notFound(cmp.Or(err, sql.ErrNoRows))
	}
	return libraryFrom(ctx, c.q, row)
}

// Libraries returns every library, oldest first. Libraries being deleted
// and folders being removed are left out.
func (c *Catalog) Libraries(ctx context.Context) ([]Library, error) {
	rows, err := c.q.ListLibraries(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Library, 0, len(rows))
	for _, r := range rows {
		l, err := libraryFrom(ctx, c.q, r)
		if err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, nil
}

// libraryFrom returns a library row with its folders, read through q.
func libraryFrom(ctx context.Context, q *dbq.Queries, r dbq.Library) (Library, error) {
	folders, err := q.ListFolders(ctx, r.ID)
	if err != nil {
		return Library{}, err
	}
	l := Library{
		ID: r.ID, Name: r.Name, Kind: LibraryKind(r.Kind), UseLocalMetadata: r.UseLocalMetadata,
		Previews: Schedule(r.Previews), Markers: Schedule(r.Markers), Subtitles: Schedule(r.Subtitles), RefreshDays: int(r.RefreshDays), MetadataLanguage: r.MetadataLanguage.String, ContentRatingCountry: r.ContentRatingCountry.String,
	}
	for _, f := range folders {
		l.Folders = append(l.Folders, folderFrom(f))
	}
	return l, nil
}

// Folder returns one library folder. It returns ErrNotFound for a folder
// that is being removed or whose library is being deleted.
func (c *Catalog) Folder(ctx context.Context, folder id.ID) (Folder, error) {
	f, err := c.q.GetFolder(ctx, folder)
	if err != nil || f.RemovingAt.Valid || f.DeletingAt.Valid {
		return Folder{}, notFound(cmp.Or(err, sql.ErrNoRows))
	}
	return folderFrom(dbq.LibraryFolder{ID: f.ID, LibraryID: f.LibraryID, Path: f.Path, Reachable: f.Reachable, LastScanAt: f.LastScanAt}), nil
}

func folderFrom(f dbq.LibraryFolder) Folder {
	folder := Folder{ID: f.ID, Library: f.LibraryID, Path: f.Path, Reachable: f.Reachable}
	if f.LastScanAt.Valid {
		folder.LastScan = time.UnixMilli(f.LastScanAt.Int64)
	}
	return folder
}
