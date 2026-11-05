package query

import (
	"context"
	"database/sql"
	"time"

	"github.com/mmcdole/kivo/internal/catalog"
	"github.com/mmcdole/kivo/internal/id"
	"github.com/mmcdole/kivo/internal/users"
)

// Library is a library the Viewer may see.
type Library struct {
	ID   id.ID
	Name string
	Kind catalog.LibraryKind
	// Pinned is whether the user shows the library in the sidebar and on
	// Home. Every library is pinned until the user chooses otherwise (see
	// users.SetLibraries).
	Pinned bool
	Items  int // how many works the library's grid shows the Viewer
}

// AdminLibrary is a library as an admin manages it: its folders and
// settings, and how many of its works are unmatched.
type AdminLibrary struct {
	Library
	Unmatched        int
	Folders          []catalog.Folder
	UseLocalMetadata bool
	Previews         catalog.Schedule
	Markers          catalog.Schedule
	Subtitles        catalog.Schedule
	RefreshDays      int // 30, 60 or 90, or 0 for never
	// MetadataLanguage and ContentRatingCountry are the library's own
	// settings. Each is empty when the library uses the server's setting.
	MetadataLanguage, ContentRatingCountry string
}

// libraryList lists the libraries in the user's own order, with whether
// each is pinned. A library the user hasn't placed is pinned, and comes
// after the placed ones in the order libraries were added. A library being
// deleted leaves the list at once. Its items leave the other lists as its
// delete-library job deletes their files.
var libraryList = stmt("Libraries", `
SELECT l.id, l.name, l.kind, coalesce(ul.pinned, 1) AS pinned
FROM library l
LEFT JOIN user_library ul ON ul.user_id = @user AND ul.library_id = l.id
WHERE l.deleting_at IS NULL
  AND `+libraryVisible+`
ORDER BY ul.position IS NULL, ul.position, l.created_at, l.id`)

func scanLibrary(r scanner, l *Library) error { return r.Scan(&l.ID, &l.Name, &l.Kind, &l.Pinned) }

// libraryCounts reads each library's grid count, as BrowseCount gives it
// with no filter, and how many of those works are unmatched. It reads each
// library's works in one pass.
var libraryCounts = stmt("LibraryCounts", `
SELECT l.id, count(w.id) AS items, coalesce(sum(w.provisional), 0) AS unmatched
FROM library l
LEFT JOIN work_library wl ON wl.library_id = l.id AND wl.is_root
LEFT JOIN work w ON w.id = wl.work_id
  AND `+visible+`
WHERE l.deleting_at IS NULL
  AND `+libraryVisible+`
GROUP BY l.id`)

// librarySettings reads the libraries' settings, for an admin.
var librarySettings = stmt("LibrarySettings", `
SELECT l.id, l.use_local_metadata, l.previews, l.markers, l.subtitles, l.refresh_days,
  coalesce(l.metadata_language, '') AS metadata_language, coalesce(l.content_rating_country, '') AS content_rating_country
FROM library l
WHERE l.deleting_at IS NULL
  AND `+libraryVisible)

// libraryFolders lists the libraries' folders, except those being removed.
// Each library's folders come by path.
var libraryFolders = stmt("LibraryFolders", `
SELECT f.id, f.library_id, f.path, f.reachable, f.last_scan_at
FROM library l
JOIN library_folder f ON f.library_id = l.id AND f.removing_at IS NULL
WHERE l.deleting_at IS NULL
  AND `+libraryVisible+`
ORDER BY f.library_id, f.path`)

var libraryOne = stmt("Library", `
SELECT l.id
FROM library l
WHERE l.id = @library AND l.deleting_at IS NULL
  AND `+libraryVisible)

// library checks that the Viewer sees lib. If the Viewer doesn't, it
// returns sql.ErrNoRows.
func library(ctx context.Context, q querier, vw viewer, lib id.ID) error {
	var got id.ID
	return rowOf(ctx, q, libraryOne, vw.args(sql.Named("library", lib)), &got)
}

// Libraries returns the libraries the Viewer may see, in the user's own
// order: the ones the user placed, then the rest in the order they were
// added.
func (s *Service) Libraries(ctx context.Context, v users.Viewer) ([]Library, error) {
	var out []Library
	err := s.db.Read(ctx, func(tx *sql.Tx) error {
		var err error
		out, _, err = libraries(ctx, tx, viewerOf(v))
		return err
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// AdminLibraries returns the libraries the Viewer may see as Libraries
// does, each with its folders and settings.
func (s *Service) AdminLibraries(ctx context.Context, v users.Viewer) ([]AdminLibrary, error) {
	vw := viewerOf(v)
	var out []AdminLibrary
	err := s.db.Read(ctx, func(tx *sql.Tx) error {
		libs, unmatched, err := libraries(ctx, tx, vw)
		if err != nil {
			return err
		}
		out = make([]AdminLibrary, len(libs))
		at := make(map[id.ID]*AdminLibrary, len(libs))
		for i, l := range libs {
			out[i] = AdminLibrary{Library: l, Unmatched: unmatched[l.ID], Folders: []catalog.Folder{}}
			at[l.ID] = &out[i]
		}
		if err := each(ctx, tx, librarySettings, vw.args(), scanSettings(at)); err != nil {
			return err
		}
		return each(ctx, tx, libraryFolders, vw.args(), scanFolder(at))
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// scanSettings reads a library's settings into its AdminLibrary, by ID.
func scanSettings(at map[id.ID]*AdminLibrary) func(scanner) error {
	return func(r scanner) error {
		var lib id.ID
		var a AdminLibrary
		if err := r.Scan(&lib, &a.UseLocalMetadata, &a.Previews, &a.Markers, &a.Subtitles, &a.RefreshDays,
			&a.MetadataLanguage, &a.ContentRatingCountry); err != nil {
			return err
		}
		if o := at[lib]; o != nil {
			a.Library, a.Unmatched, a.Folders = o.Library, o.Unmatched, o.Folders
			*o = a
		}
		return nil
	}
}

// scanFolder reads a folder onto its library's AdminLibrary, by ID.
func scanFolder(at map[id.ID]*AdminLibrary) func(scanner) error {
	return func(r scanner) error {
		var f catalog.Folder
		var lastScan sql.NullInt64
		if err := r.Scan(&f.ID, &f.Library, &f.Path, &f.Reachable, &lastScan); err != nil {
			return err
		}
		if lastScan.Valid {
			f.LastScan = time.UnixMilli(lastScan.Int64)
		}
		if o := at[f.Library]; o != nil {
			o.Folders = append(o.Folders, f)
		}
		return nil
	}
}

// libraries reads the Viewer's libraries in the user's order, with their
// item counts. It also returns how many of each library's works are
// unmatched.
func libraries(ctx context.Context, q querier, vw viewer) ([]Library, map[id.ID]int, error) {
	out, err := rowsOf(ctx, q, libraryList, vw.args(), scanLibrary)
	if err != nil {
		return nil, nil, err
	}
	if out == nil {
		out = []Library{}
	}
	items, unmatched := map[id.ID]int{}, map[id.ID]int{}
	err = each(ctx, q, libraryCounts, vw.args(), func(r scanner) error {
		var lib id.ID
		var n, u int
		err := r.Scan(&lib, &n, &u)
		items[lib], unmatched[lib] = n, u
		return err
	})
	if err != nil {
		return nil, nil, err
	}
	for i := range out {
		out[i].Items = items[out[i].ID]
	}
	return out, unmatched, nil
}
