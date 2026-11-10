package api

import (
	"cmp"
	"context"
	"slices"
	"time"

	"github.com/mmcdole/kivo/internal/api/internal/gen"
	"github.com/mmcdole/kivo/internal/catalog"
	"github.com/mmcdole/kivo/internal/id"
	"github.com/mmcdole/kivo/internal/query"
	"github.com/mmcdole/kivo/internal/scan"
	"github.com/mmcdole/kivo/internal/users"
)

// libraries lists the libraries the caller sees, each with its count of
// visible items. For admins, each library also has its folders, settings,
// unmatched works and scan status.
func (h *handlers) libraries(ctx context.Context, v users.Viewer) ([]gen.Library, error) {
	return h.librariesOf(ctx, v, nil)
}

// visibleLibrary returns one library the caller sees. A library the caller
// can't see is not found.
func (h *handlers) visibleLibrary(ctx context.Context, v users.Viewer, lib id.ID) (gen.Library, error) {
	libs, err := h.librariesOf(ctx, v, &lib)
	if err != nil {
		return gen.Library{}, err
	}
	if len(libs) == 0 {
		return gen.Library{}, errNotFound
	}
	return libs[0], nil
}

// librariesOf is libraries limited to one library when only is set.
func (h *handlers) librariesOf(ctx context.Context, v users.Viewer, only *id.ID) ([]gen.Library, error) {
	other := func(lib id.ID) bool { return only != nil && lib != *only }
	if !v.Admin() {
		libs, err := h.deps.Query.Libraries(ctx, v)
		if err != nil {
			return nil, err
		}
		libs = slices.DeleteFunc(libs, func(l query.Library) bool { return other(l.ID) })
		return each(libs, library), nil
	}
	libs, err := h.deps.Query.AdminLibraries(ctx, v)
	if err != nil {
		return nil, err
	}
	libs = slices.DeleteFunc(libs, func(l query.AdminLibrary) bool { return other(l.ID) })
	var waiting map[id.ID]int
	if identifying(h.deps.Tasks) {
		if waiting, err = h.deps.Identify.Waiting(ctx); err != nil {
			return nil, err
		}
	}
	scans, asked, now := h.deps.Tasks.Items(scan.TaskScanLibraries), h.deps.Tasks.Asked(scan.TaskScanLibraries), time.Now()
	out := make([]gen.Library, len(libs))
	for i, l := range libs {
		out[i] = adminLibrary(l, libraryScanStatus(l, scans, asked, waiting[l.ID], now))
	}
	return out, nil
}

func (h *handlers) ListLibraries(ctx context.Context, _ gen.ListLibrariesRequestObject) (gen.ListLibrariesResponseObject, error) {
	all, err := h.libraries(ctx, callerOf(ctx).viewer)
	if err != nil {
		return nil, err
	}
	return gen.ListLibraries200JSONResponse{Items: all}, nil
}

func (h *handlers) GetLibrary(ctx context.Context, req gen.GetLibraryRequestObject) (gen.GetLibraryResponseObject, error) {
	l, err := h.visibleLibrary(ctx, callerOf(ctx).viewer, req.ID)
	if err != nil {
		return nil, err
	}
	return gen.GetLibrary200JSONResponse(l), nil
}

func (h *handlers) GetLibraryFacets(ctx context.Context, req gen.GetLibraryFacetsRequestObject) (gen.GetLibraryFacetsResponseObject, error) {
	f, err := h.deps.Query.Facets(ctx, callerOf(ctx).viewer, req.ID)
	if err != nil {
		return nil, err
	}
	// The content ratings of the library's country come first, in that
	// country's order (G, PG, PG-13, R, NC-17, then the TV ratings). Other
	// ratings keep query's order.
	lib, err := h.deps.Catalog.Library(ctx, req.ID)
	if err != nil {
		return nil, err
	}
	order := map[string]int{}
	for i, r := range h.deps.Metadata.ContentRatings(lib.ContentRatingCountry) {
		order[r.Name] = i + 1
	}
	slices.SortStableFunc(f.ContentRatings, func(a, b string) int {
		return cmp.Compare(cmp.Or(order[a], len(order)+1), cmp.Or(order[b], len(order)+1))
	})
	narrators := make([]gen.PersonRef, len(f.Narrators))
	for i, n := range f.Narrators {
		narrators[i] = gen.PersonRef{ID: n.ID, Name: n.Name}
	}
	return gen.GetLibraryFacets200JSONResponse{
		Genres: f.Genres, ContentRatings: f.ContentRatings, Years: f.Years, Narrators: narrators,
		Resolutions:   each(f.Resolutions, func(r query.Resolution) gen.Resolution { return gen.Resolution(r) }),
		DynamicRanges: each(f.Ranges, func(r query.Range) gen.DynamicRange { return gen.DynamicRange(r) }),
		Collections:   f.Collections,
	}, nil
}

// ListLibraryWorks is a page of a library's grid, sorted and filtered.
func (h *handlers) ListLibraryWorks(ctx context.Context, req gen.ListLibraryWorksRequestObject) (gen.ListLibraryWorksResponseObject, error) {
	v := callerOf(ctx).viewer
	o, err := parseBrowseOptions(req.Params, v)
	if err != nil {
		return nil, err
	}
	g, err := h.deps.Query.Browse(ctx, v, req.ID, o)
	if err != nil {
		return nil, err
	}
	return gen.ListLibraryWorks200JSONResponse(workPage(g)), nil
}

// ListLibraryCollections is a page of a library's collections.
func (h *handlers) ListLibraryCollections(ctx context.Context, req gen.ListLibraryCollectionsRequestObject) (gen.ListLibraryCollectionsResponseObject, error) {
	g, err := libraryGrid(ctx, req.ID, req.Params.Offset, req.Params.Limit, h.deps.Query.Collections)
	if err != nil {
		return nil, err
	}
	return gen.ListLibraryCollections200JSONResponse{Items: each(g.Items, collectionSummary), Total: g.Total, Letters: letterStarts(g.Letters)}, nil
}

// ListLibraryAuthors is a page of an audiobooks library's authors.
func (h *handlers) ListLibraryAuthors(ctx context.Context, req gen.ListLibraryAuthorsRequestObject) (gen.ListLibraryAuthorsResponseObject, error) {
	g, err := libraryGrid(ctx, req.ID, req.Params.Offset, req.Params.Limit, h.deps.Query.Authors)
	if err != nil {
		return nil, err
	}
	return gen.ListLibraryAuthors200JSONResponse{Items: each(g.Items, authorSummary), Total: g.Total, Letters: letterStarts(g.Letters)}, nil
}

// ListLibraryBookSeries is a page of an audiobooks library's book series.
func (h *handlers) ListLibraryBookSeries(ctx context.Context, req gen.ListLibraryBookSeriesRequestObject) (gen.ListLibraryBookSeriesResponseObject, error) {
	g, err := libraryGrid(ctx, req.ID, req.Params.Offset, req.Params.Limit, h.deps.Query.SeriesList)
	if err != nil {
		return nil, err
	}
	return gen.ListLibraryBookSeries200JSONResponse{Items: each(g.Items, seriesSummary), Total: g.Total, Letters: letterStarts(g.Letters)}, nil
}

// libraryGrid reads a grid request's paging and lists that page of a
// library's grid with list.
func libraryGrid[T any](ctx context.Context, lib id.ID, offset, limit *int,
	list func(context.Context, users.Viewer, id.ID, query.GridOptions) (query.GridPage[T], error)) (query.GridPage[T], error) {
	o, err := gridOptionsOf(offset, limit)
	if err != nil {
		return query.GridPage[T]{}, err
	}
	return list(ctx, callerOf(ctx).viewer, lib, o)
}

func (h *handlers) CreateLibrary(ctx context.Context, req gen.CreateLibraryRequestObject) (gen.CreateLibraryResponseObject, error) {
	b := req.Body
	in := catalog.NewLibrary(catalog.LibraryKind(b.Kind), catalog.LibraryChange{
		Name: &b.Name, Folders: &b.Folders, UseLocalMetadata: b.UseLocalMetadata, Previews: parseSchedule(b.Previews), Markers: parseSchedule(b.Markers),
		Subtitles: parseSchedule(b.Subtitles), RefreshDays: (*int)(b.RefreshDays), MetadataLanguage: b.MetadataLanguage, ContentRatingCountry: b.ContentRatingCountry,
	})
	lib, err := h.deps.Scan.AddLibrary(ctx, in)
	if err != nil {
		return nil, err
	}
	l, err := h.visibleLibrary(ctx, callerOf(ctx).viewer, lib)
	if err != nil {
		return nil, err
	}
	return gen.CreateLibrary201JSONResponse(l), nil
}

func (h *handlers) UpdateLibrary(ctx context.Context, req gen.UpdateLibraryRequestObject) (gen.UpdateLibraryResponseObject, error) {
	b := req.Body
	ch := catalog.LibraryChange{
		Name: b.Name, Folders: b.Folders, UseLocalMetadata: b.UseLocalMetadata, Previews: parseSchedule(b.Previews), Markers: parseSchedule(b.Markers),
		Subtitles: parseSchedule(b.Subtitles), RefreshDays: (*int)(b.RefreshDays), MetadataLanguage: b.MetadataLanguage, ContentRatingCountry: b.ContentRatingCountry,
	}
	if err := h.deps.Scan.EditLibrary(ctx, req.ID, ch); err != nil {
		return nil, err
	}
	l, err := h.visibleLibrary(ctx, callerOf(ctx).viewer, req.ID)
	if err != nil {
		return nil, err
	}
	return gen.UpdateLibrary200JSONResponse(l), nil
}

func (h *handlers) DeleteLibrary(ctx context.Context, req gen.DeleteLibraryRequestObject) (gen.DeleteLibraryResponseObject, error) {
	if err := h.deps.Scan.DeleteLibrary(ctx, req.ID); err != nil {
		return nil, err
	}
	return gen.DeleteLibrary202JSONResponse{}, nil
}

func (h *handlers) ScanLibrary(ctx context.Context, req gen.ScanLibraryRequestObject) (gen.ScanLibraryResponseObject, error) {
	ask := h.deps.Scan.ScanLibrary
	if req.Body != nil && deref(req.Body.Full) {
		ask = h.deps.Scan.ScanLibraryFullWalk
	}
	if err := ask(ctx, req.ID); err != nil {
		return nil, err
	}
	return gen.ScanLibrary202JSONResponse{}, nil
}

func (h *handlers) StopLibraryScan(ctx context.Context, req gen.StopLibraryScanRequestObject) (gen.StopLibraryScanResponseObject, error) {
	if _, err := h.visibleLibrary(ctx, callerOf(ctx).viewer, req.ID); err != nil {
		return nil, err
	}
	if err := h.deps.Tasks.Stop(ctx, scan.TaskScanLibraries, req.ID); err != nil {
		return nil, err
	}
	return gen.StopLibraryScan204Response{}, nil
}

func (h *handlers) RefreshLibraryMetadata(ctx context.Context, req gen.RefreshLibraryMetadataRequestObject) (gen.RefreshLibraryMetadataResponseObject, error) {
	n, err := h.deps.Identify.RefreshLibrary(ctx, req.ID)
	if err != nil {
		return nil, err
	}
	return gen.RefreshLibraryMetadata202JSONResponse{Queued: &n}, nil
}

// library is a library as a viewer sees it.
func library(l query.Library) gen.Library {
	return gen.Library{ID: l.ID, Name: l.Name, Kind: gen.LibraryKind(l.Kind), WorkCount: l.Items, Pinned: l.Pinned}
}

// adminLibrary is a library as an admin manages it, including what its
// scan is doing.
func adminLibrary(l query.AdminLibrary, status gen.LibraryScanStatus) gen.Library {
	out := library(l.Library)
	folders := make([]gen.LibraryFolder, len(l.Folders))
	for i, f := range l.Folders {
		folders[i] = gen.LibraryFolder{ID: f.ID, Path: f.Path, Reachable: f.Reachable, LastScanAt: optTime(f.LastScan)}
	}
	out.Folders = &folders
	out.UseLocalMetadata = &l.UseLocalMetadata
	out.Previews = ptr(schedule(l.Previews))
	out.Markers = ptr(schedule(l.Markers))
	out.Subtitles = ptr(schedule(l.Subtitles))
	out.RefreshDays = ptr(gen.RefreshDays(l.RefreshDays))
	out.MetadataLanguage, out.ContentRatingCountry = opt(l.MetadataLanguage), opt(l.ContentRatingCountry)
	out.UnmatchedCount = ptr(l.Unmatched)
	out.Scan = &status
	return out
}

func schedule(p catalog.Schedule) gen.ProcessingSchedule {
	if p == catalog.NightlyAndAdded {
		return gen.ProcessingScheduleNightlyAndAdded
	}
	return gen.ProcessingSchedule(p)
}

// parseSchedule converts a schedule to catalog's terms. Catalog checks
// it. A nil schedule means none was asked for.
func parseSchedule(p *gen.ProcessingSchedule) *catalog.Schedule {
	if p == nil {
		return nil
	}
	s := catalog.Schedule(*p)
	if *p == gen.ProcessingScheduleNightlyAndAdded {
		s = catalog.NightlyAndAdded
	}
	return &s
}
