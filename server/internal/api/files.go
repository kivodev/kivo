package api

import (
	"context"

	"github.com/mmcdole/kivo/internal/api/internal/gen"
	"github.com/mmcdole/kivo/internal/catalog"
	"github.com/mmcdole/kivo/internal/query"
	"github.com/mmcdole/kivo/internal/subtitles"
)

// ListWorkFiles lists "Show files", which is every part of every version
// of a work. The list is not paged.
func (h *handlers) ListWorkFiles(ctx context.Context, req gen.ListWorkFilesRequestObject) (gen.ListWorkFilesResponseObject, error) {
	files, err := h.deps.Query.WorkFiles(ctx, callerOf(ctx).viewer, req.ID)
	if err != nil {
		return nil, err
	}
	return gen.ListWorkFiles200JSONResponse(gen.MediaFileList{Items: each(files, mediaFile)}), nil
}

// ListFiles is the "Missing files" list, newest missing first.
func (h *handlers) ListFiles(ctx context.Context, req gen.ListFilesRequestObject) (gen.ListFilesResponseObject, error) {
	p := req.Params
	if !p.Status.Valid() {
		return nil, invalid("status is gone", field("status", "invalid"))
	}
	pg, err := pagingOf(p.After, p.Limit)
	if err != nil {
		return nil, err
	}
	page, err := h.deps.Query.MissingFiles(ctx, callerOf(ctx).viewer, deref(p.LibraryID), query.Cursor(pg.after), pg.limit)
	if err != nil {
		return nil, err
	}
	out := gen.MediaFileFeed{Items: make([]gen.MediaFile, len(page.Items)), Next: opt(string(page.Next)), Total: pg.total(page.Total)}
	for i, f := range page.Items {
		out.Items[i] = mediaFile(f)
	}
	return gen.ListFiles200JSONResponse(out), nil
}

func (h *handlers) DeleteFile(ctx context.Context, req gen.DeleteFileRequestObject) (gen.DeleteFileResponseObject, error) {
	if err := h.deps.Catalog.DeleteMissing(ctx, req.ID); err != nil {
		return nil, err
	}
	return gen.DeleteFile204Response{}, nil
}

func (h *handlers) DeleteMissingFiles(ctx context.Context, req gen.DeleteMissingFilesRequestObject) (gen.DeleteMissingFilesResponseObject, error) {
	if !req.Params.Status.Valid() {
		return nil, invalid("status is gone", field("/status", "invalid"))
	}
	if err := h.deps.Catalog.DeleteAllMissing(ctx); err != nil {
		return nil, err
	}
	return gen.DeleteMissingFiles204Response{}, nil
}

func (h *handlers) ListFileSubtitleResults(ctx context.Context, req gen.ListFileSubtitleResultsRequestObject) (gen.ListFileSubtitleResultsResponseObject, error) {
	q := subtitles.SubtitleSearch{Languages: deref(req.Params.Language)}
	cs, err := h.deps.Subtitles.SearchSubtitles(ctx, callerOf(ctx).viewer, req.ID, q)
	if err != nil {
		return nil, err
	}
	items := make([]gen.SubtitleResult, len(cs))
	for i, c := range cs {
		items[i] = gen.SubtitleResult{
			ID: c.ID, Language: c.Language, Release: opt(c.Release), Downloads: c.Downloads,
			HashMatch: c.HashMatch, Forced: c.Forced, HearingImpaired: c.HearingImpaired,
		}
	}
	return gen.ListFileSubtitleResults200JSONResponse(gen.SubtitleResultList{Items: items}), nil
}

func (h *handlers) AddFileSubtitle(ctx context.Context, req gen.AddFileSubtitleRequestObject) (gen.AddFileSubtitleResponseObject, error) {
	d, err := h.deps.Subtitles.DownloadSubtitle(ctx, callerOf(ctx).viewer, req.ID, req.Body.ResultID)
	if err != nil {
		return nil, err
	}
	return gen.AddFileSubtitle200JSONResponse(named([]gen.Track{subtitleFileTrack(d.Track)})[0]), nil
}

// mediaFile is a file on the wire, for "Show files" and "Missing files".
func mediaFile(f query.File) gen.MediaFile {
	out := gen.MediaFile{
		ID: f.ID, LibraryID: f.Library, Path: f.Path, SizeBytes: f.Size, Status: gen.MediaFileStatus(catalog.FilePresent),
		Container: opt(f.Container), DurationMs: opt(ms(f.Duration)), BitrateBps: opt(int64(f.Bitrate)),
		VersionID: opt(f.Version), Title: opt(f.Title), WorkID: opt(f.Work),
		Tracks: named(append(each(f.Tracks, catalogTrack), each(f.SubtitleFiles, subtitleFileTrack)...)),
	}
	if !f.Version.IsZero() {
		out.Part = ptr(f.Part)
	}
	if !f.Gone.IsZero() {
		out.Status, out.GoneAt = gen.MediaFileStatus(catalog.FileGone), &f.Gone
	}
	return out
}
