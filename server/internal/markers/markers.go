package markers

import (
	"cmp"
	"context"
	"log/slog"
	"slices"
	"time"

	"github.com/mmcdole/kivo/internal/catalog"
	"github.com/mmcdole/kivo/internal/id"
	"github.com/mmcdole/kivo/internal/markers/dbq"
)

// Markers returns a version's chapters and its markers, for a play of
// work. For each kind, an episode's
// markers come from an admin, or else from what detection found in its
// files, or else from its chapters. Each marker is moved onto the version's
// timeline. An admin's marker with no end says the episode has no marker
// of that kind.
func (s *Service) Markers(ctx context.Context, work id.ID, parts []catalog.File) ([]catalog.Chapter, []Marker) {
	kind := s.workKind(ctx, work)
	cs, ms := chapters(kind, parts)
	if kind != catalog.KindEpisode {
		return cs, ms
	}
	return cs, s.withStored(ctx, ms, parts)
}

// withStored lays the markers stored for a version's parts over the
// markers its chapters give. An admin's marker replaces the others of its
// kind, or removes them if it has no end. A found marker replaces the
// chapters' markers of its kind. Final credits stay final only in the last
// part.
func (s *Service) withStored(ctx context.Context, ms []Marker, parts []catalog.File) []Marker {
	files := make([]id.ID, len(parts))
	for i, f := range parts {
		files[i] = f.ID
	}
	rows, err := s.q.StoredMarkers(ctx, id.Set(files...))
	if err != nil {
		slog.WarnContext(ctx, "markers: read stored markers", "err", err)
		return ms
	}
	return overlay(ms, rows, parts)
}

// overlay applies withStored's rule to rows already read.
func overlay(ms []Marker, rows []dbq.StoredMarkersRow, parts []catalog.File) []Marker {
	offsets := map[id.ID]time.Duration{}
	last := map[id.ID]bool{}
	var at time.Duration
	for i, f := range parts {
		offsets[f.ID], last[f.ID] = at, i == len(parts)-1
		at += f.Probe.Duration
	}
	var manual, found []Marker
	cleared := map[MarkerKind]bool{}
	for _, r := range rows {
		k := MarkerKind(r.Kind)
		if r.Source == string(ManualSource) && !r.EndMs.Valid {
			cleared[k] = true
			continue
		}
		off := offsets[r.FileID]
		m := Marker{Kind: k, Source: MarkerSource(r.Source), Start: off + millis(r.StartMs), End: off + millis(r.EndMs.Int64), Final: r.Final && last[r.FileID]}
		if r.Source == string(ManualSource) {
			manual = append(manual, m)
		} else {
			found = append(found, m)
		}
	}
	for _, m := range manual {
		cleared[m.Kind] = true
	}
	found = slices.DeleteFunc(found, func(m Marker) bool { return cleared[m.Kind] })
	replaced := map[MarkerKind]bool{}
	for _, m := range found {
		replaced[m.Kind] = true
	}
	out := slices.DeleteFunc(slices.Clone(ms), func(m Marker) bool { return cleared[m.Kind] || replaced[m.Kind] })
	out = append(append(out, manual...), found...)
	slices.SortStableFunc(out, func(a, b Marker) int { return cmp.Compare(a.Start, b.Start) })
	return out
}

func millis(v int64) time.Duration { return time.Duration(v) * time.Millisecond }

// WorkMarkers returns the markers a play of each of a work's versions has,
// keyed by version. Only episodes and movies have markers.
func (s *Service) WorkMarkers(ctx context.Context, work id.ID) (map[id.ID][]Marker, error) {
	kind := s.workKind(ctx, work)
	if kind != catalog.KindEpisode && kind != catalog.KindMovie {
		return nil, nil
	}
	versions, err := s.deps.Catalog.VersionsOf(ctx, work)
	if err != nil {
		return nil, err
	}
	out := map[id.ID][]Marker{}
	for _, v := range versions {
		parts := make([]catalog.File, 0, len(v.Parts))
		for _, p := range v.Parts {
			f, err := s.deps.Catalog.File(ctx, p)
			if err != nil {
				return nil, err
			}
			parts = append(parts, f)
		}
		_, out[v.ID] = s.Markers(ctx, work, parts)
	}
	return out, nil
}
