package markers

import (
	"context"
	"database/sql"
	"errors"
	"slices"

	"github.com/mmcdole/kivo/internal/catalog"
	"github.com/mmcdole/kivo/internal/id"
	"github.com/mmcdole/kivo/internal/markers/detect"
)

// seasonFiles is a season and its episodes' files, which the season's
// detect-markers job decides together.
type seasonFiles struct {
	id     id.ID
	number int
	// language is the language the season's series was made in.
	language string
	files    []seasonFile
}

// seasonFile is an episode's file.
type seasonFile struct {
	fileToRead
	number int // the episode's number
	place  int // the episode's place among the season's episodes with files
	works  int // how many works the file's version covers
	// fingerprint is the file's fingerprint. It is nil until the file is
	// fingerprinted.
	fingerprint *detect.Fingerprint
}

// season reads a season and its episodes' files, if its library finds
// markers. Otherwise it returns an empty seasonFiles. A file of several
// episodes is listed once.
func (s *Service) season(ctx context.Context, season id.ID) (seasonFiles, error) {
	r, err := s.q.Season(ctx, season)
	if errors.Is(err, sql.ErrNoRows) {
		return seasonFiles{}, nil
	}
	if err != nil {
		return seasonFiles{}, err
	}
	rows, err := s.q.SeasonFiles(ctx, season)
	if err != nil || len(rows) == 0 {
		return seasonFiles{}, err
	}
	lib, err := s.deps.Catalog.Library(ctx, rows[0].LibraryID)
	if err != nil || lib.Markers == catalog.Never {
		return seasonFiles{}, err
	}
	sn := seasonFiles{id: season, number: int(r.SeasonNumber.Int64), language: r.OriginalLanguage}
	place := -1
	var last id.ID
	for _, row := range rows {
		if row.EpisodeID != last {
			place, last = place+1, row.EpisodeID
		}
		if slices.ContainsFunc(sn.files, func(f seasonFile) bool { return f.ID == row.ID }) {
			continue
		}
		f, err := s.deps.Catalog.File(ctx, row.ID)
		if errors.Is(err, catalog.ErrNotFound) || err == nil && f.Probe.Duration <= 0 {
			continue
		}
		if err != nil {
			return seasonFiles{}, err
		}
		sn.files = append(sn.files, seasonFile{
			fileToRead: fileToRead{File: f, part: int(row.Part.Int64), parts: int(row.Parts)},
			number:     int(row.EpisodeNumber.Int64), place: place, works: int(row.Works),
		})
	}
	return sn, nil
}

// fingerprints loads each file's kept fingerprint, if it has one.
func (s *Service) fingerprints(sn *seasonFiles) {
	for i := range sn.files {
		f := &sn.files[i]
		f.fingerprint, _ = s.fingerprintAt(f.ID, f.Generation)
	}
}

// markerFiles returns the season's files in the form detect decides from.
func (s seasonFiles) markerFiles() []detect.File {
	out := make([]detect.File, 0, len(s.files))
	for _, f := range s.files {
		mf := detect.File{ID: f.ID, Episode: f.place, Part: f.part, Parts: f.parts, Duration: f.Probe.Duration, Fingerprint: f.fingerprint}
		for _, c := range f.Chapters {
			mf.Chapters = append(mf.Chapters, c.Start)
		}
		out = append(out, mf)
	}
	return out
}
