package query

import (
	"context"
	"database/sql"
	"path/filepath"
	"time"

	"github.com/mmcdole/kivo/internal/catalog"
	"github.com/mmcdole/kivo/internal/id"
	"github.com/mmcdole/kivo/internal/users"
)

// File is a media file as admins see it, in "Show files" and the "Missing
// files" list.
type File struct {
	ID        id.ID
	Library   id.ID
	Path      string // absolute, as last seen
	Size      int64
	Gone      time.Time // when it went missing; zero while present
	Container string
	Duration  time.Duration
	Bitrate   int   // bits per second
	Version   id.ID // zero for a file in no version
	Part      int   // the file's part number in the version, counting from 0
	// Title is the title of the first work the file's version covers.
	Title string
	// Work is that work. It is set in the "Missing files" list while a
	// library still has the work, and is zero otherwise.
	Work id.ID
	// Tracks and External are the file's tracks, as catalog lists them, for
	// "Show files". The "Missing files" list leaves them out.
	Tracks        []catalog.Track
	SubtitleFiles []catalog.SubtitleFile
}

// Admins see media files in two places: the Missing files list and Show
// files.

// missingFiles reads a page of the Missing files list, newest missing
// first, in one library or in every library. Each file comes with the title
// of its version's first work, and with that work while a library still has
// it. The Viewer filter applies to the file's library. A work whose files
// are all gone is in no library, so the works filter would hide the files
// listed here.
var missingFiles = stmt("MissingFiles", `
SELECT f.id, r.library_id, r.path, f.rel_path, f.size, f.gone_at,
  coalesce(f.container, ''), coalesce(f.duration_ms, 0), coalesce(f.bitrate, 0),
  f.version_id, coalesce(f.part, 0),
  coalesce((SELECT w.title FROM media_version_work vw JOIN work w ON w.id = vw.work_id
    WHERE vw.version_id = f.version_id
    ORDER BY vw.ord LIMIT 1), ''),
  (SELECT vw.work_id FROM media_version_work vw
    WHERE vw.version_id = f.version_id AND EXISTS (SELECT 1 FROM work_library wl WHERE wl.work_id = vw.work_id)
      AND vw.ord = (SELECT min(o.ord) FROM media_version_work o WHERE o.version_id = f.version_id))
FROM media_file f
JOIN library_folder r ON r.id = f.folder_id
JOIN library l ON l.id = r.library_id
WHERE f.status = 'gone'
  AND (f.gone_at < @after_gone_at OR f.gone_at = @after_gone_at AND f.id < @after_id)
  AND (@any_library OR l.id = @library)
  AND `+libraryVisible+`
ORDER BY f.gone_at DESC, f.id DESC
LIMIT @limit`)

// missingCount counts the Missing files list's total.
var missingCount = stmt("MissingCount", `
SELECT count(*)
FROM media_file f
JOIN library_folder r ON r.id = f.folder_id
JOIN library l ON l.id = r.library_id
WHERE f.status = 'gone'
  AND (@any_library OR l.id = @library)
  AND `+libraryVisible)

// workFiles reads Show files: every part of every version of the work,
// with one row per track. The tracks come in the order catalog lists a
// file's tracks: embedded tracks by stream index, then subtitle files next
// to the video, then downloaded subtitles, each by path. A part without
// tracks has one row with no track. A work without versions has one row
// with no file. A work the Viewer may not see has no rows.
var workFiles = stmt("WorkFiles", `
SELECT w.title, f.id, r.library_id, r.path, f.rel_path, f.size, f.status, f.gone_at,
  f.container, f.duration_ms, f.bitrate, f.version_id, f.part,
  t.id, t.source, t.stream_index, t.type, t.codec, t.language, t.title, t.rel_path,
  t.is_default, t.forced, t.hearing_impaired, t.info
FROM work w
LEFT JOIN media_version_work vw ON vw.work_id = w.id
LEFT JOIN media_file f ON f.version_id = vw.version_id
LEFT JOIN library_folder r ON r.id = f.folder_id
LEFT JOIN media_track t ON t.file_id = f.id
WHERE w.id = @work
  AND `+visible+`
ORDER BY vw.version_id, f.part, t.source <> 'embedded', t.source = 'downloaded', t.stream_index, t.rel_path`)

// MissingFiles pages the "Missing files" list: files gone from disk, newest
// missing first. It lists one library, or every library the Viewer may see
// when lib is the zero ID. The page holds at most limit files, or the
// default number when limit is 0.
func (s *Service) MissingFiles(ctx context.Context, v users.Viewer, lib id.ID, after Cursor, limit int) (Page[File], error) {
	vw := viewerOf(v)
	const list = "missing"
	limit, err := pageLimit(limit)
	if err != nil {
		return Page[File]{}, err
	}
	pos, more, err := after.decode(list, 1)
	if err != nil {
		return Page[File]{}, err
	}
	if !more {
		pos = position{N: []int64{startDesc}}
		for i := range pos.ID {
			pos.ID[i] = 0xff
		}
	}
	var items []File
	total := 0
	err = s.db.Read(ctx, func(tx *sql.Tx) error {
		var err error
		items, err = rowsOf(ctx, tx, missingFiles, vw.args(sql.Named("library", lib), sql.Named("any_library", lib.IsZero()),
			sql.Named("after_gone_at", pos.N[0]), sql.Named("after_id", pos.ID), sql.Named("limit", limit+1)),
			func(r scanner, f *File) error {
				var folder, rel string
				var goneAt, durationMs, part int64
				if err := r.Scan(&f.ID, &f.Library, &folder, &rel, &f.Size, &goneAt, &f.Container, &durationMs, &f.Bitrate,
					&f.Version, &part, &f.Title, &f.Work); err != nil {
					return err
				}
				f.Path, f.Gone = filepath.Join(folder, rel), time.UnixMilli(goneAt).UTC()
				f.Duration, f.Part = time.Duration(durationMs)*time.Millisecond, max(int(part)-1, 0)
				return nil
			})
		if err != nil || more {
			return err
		}
		return rowOf(ctx, tx, missingCount, vw.args(sql.Named("library", lib), sql.Named("any_library", lib.IsZero())), &total)
	})
	if err != nil {
		return Page[File]{}, err
	}
	if items == nil {
		items = []File{}
	}
	return page(items, limit, total, func(f File) position {
		return position{List: list, N: []int64{f.Gone.UnixMilli()}, ID: f.ID}
	}), nil
}

// WorkFiles returns "Show files": every part of every version of a work,
// with its tracks. The list isn't paged, because one work bounds its size.
func (s *Service) WorkFiles(ctx context.Context, v users.Viewer, work id.ID) ([]File, error) {
	var out []File
	found := false
	err := each(ctx, s.db.Reader(), workFiles, viewerOf(v).args(sql.Named("work", work)), func(r scanner) error {
		found = true
		var title string
		var file, lib, version, track id.ID
		var folder, rel, status, container sql.NullString
		var size, goneAt, durationMs, bitrate, part, index sql.NullInt64
		var source, kind, codec, language, trackTitle, trackPath, info sql.NullString
		var isDefault, forced, hearingImpaired sql.NullBool
		if err := r.Scan(&title, &file, &lib, &folder, &rel, &size, &status, &goneAt, &container, &durationMs, &bitrate,
			&version, &part, &track, &source, &index, &kind, &codec, &language, &trackTitle, &trackPath,
			&isDefault, &forced, &hearingImpaired, &info); err != nil {
			return err
		}
		if file.IsZero() {
			return nil // the work has no versions
		}
		if len(out) == 0 || out[len(out)-1].ID != file {
			f := File{
				ID: file, Library: lib, Path: filepath.Join(folder.String, rel.String), Size: size.Int64,
				Container: container.String,
				Duration:  time.Duration(durationMs.Int64) * time.Millisecond, Bitrate: int(bitrate.Int64),
				Version: version, Part: int(part.Int64) - 1, Title: title,
				Tracks: []catalog.Track{}, SubtitleFiles: []catalog.SubtitleFile{},
			}
			if catalog.FileStatus(status.String) == catalog.FileGone {
				f.Gone = time.UnixMilli(goneAt.Int64).UTC()
			}
			out = append(out, f)
		}
		if track.IsZero() {
			return nil
		}
		f := &out[len(out)-1]
		if source.String != catalog.SourceEmbedded {
			f.SubtitleFiles = append(f.SubtitleFiles, catalog.SubtitleFile{
				ID: track, RelPath: trackPath.String, Format: catalog.SubtitleFormat(trackPath.String),
				Language: language.String, Title: trackTitle.String, Default: isDefault.Bool, Forced: forced.Bool,
				HearingImpaired: hearingImpaired.Bool, Downloaded: source.String == catalog.SourceDownloaded,
			})
			return nil
		}
		f.Tracks = append(f.Tracks, catalog.Track{
			ID: track, Index: int(index.Int64), Type: kind.String, Codec: codec.String, Language: language.String,
			Title: trackTitle.String, Default: isDefault.Bool, Forced: forced.Bool, HearingImpaired: hearingImpaired.Bool,
			Info: []byte(info.String),
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, ErrNotFound
	}
	if out == nil {
		out = []File{}
	}
	return out, nil
}
