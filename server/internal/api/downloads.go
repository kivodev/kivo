package api

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/mmcdole/kivo/internal/api/internal/gen"
	"github.com/mmcdole/kivo/internal/id"
	"github.com/mmcdole/kivo/internal/playback"
	"github.com/mmcdole/kivo/internal/users"
)

// maxDownloadCheck is the most downloads one check or one estimate may ask
// about.
const maxDownloadCheck = 1000

func (h *handlers) CreateDownload(ctx context.Context, req gen.CreateDownloadRequestObject) (gen.CreateDownloadResponseObject, error) {
	b := req.Body
	r := playback.DownloadRequest{
		Work: b.WorkID, Version: deref(b.VersionID), AudioTrack: b.AudioTrackID, Caps: parseCapabilities(b.Capabilities),
		Settings: parseSettings(b.Settings), Quality: string(deref(b.Quality)),
	}
	if _, ok := playback.DownloadQualityNamed(r.Quality); r.Quality != "" && !ok {
		return nil, invalid("quality: original, high or standard", field("/quality", "invalid"))
	}
	var err error
	if r.SubtitleTrack, err = parseSubtitleChoice(b.SubtitleTrackID); err != nil {
		return nil, err
	}
	c := callerOf(ctx)
	d, err := h.deps.Downloads.Request(ctx, c.Caller, c.viewer, r)
	if err != nil {
		return nil, err
	}
	return gen.CreateDownload201JSONResponse(download(d)), nil
}

func (h *handlers) ListDownloads(ctx context.Context, _ gen.ListDownloadsRequestObject) (gen.ListDownloadsResponseObject, error) {
	downloads, err := h.deps.Downloads.List(ctx, callerOf(ctx).Caller)
	if err != nil {
		return nil, err
	}
	return gen.ListDownloads200JSONResponse(gen.DownloadList{Items: each(downloads, func(d playback.DownloadStatus) gen.Download { return download(d) })}), nil
}

func (h *handlers) GetDownload(ctx context.Context, req gen.GetDownloadRequestObject) (gen.GetDownloadResponseObject, error) {
	c := callerOf(ctx)
	d, err := h.deps.Downloads.Status(ctx, c.Caller, req.ID)
	if err != nil {
		return nil, err
	}
	return gen.GetDownload200JSONResponse(download(d)), nil
}

func (h *handlers) DeleteDownload(ctx context.Context, req gen.DeleteDownloadRequestObject) (gen.DeleteDownloadResponseObject, error) {
	if err := h.deps.Downloads.Delete(ctx, callerOf(ctx).Caller, req.ID); err != nil {
		return nil, err
	}
	return gen.DeleteDownload204Response{}, nil
}

func (h *handlers) RetryDownload(ctx context.Context, req gen.RetryDownloadRequestObject) (gen.RetryDownloadResponseObject, error) {
	c := callerOf(ctx)
	d, err := h.deps.Downloads.Retry(ctx, c.Caller, c.viewer, req.ID)
	if err != nil {
		return nil, err
	}
	return gen.RetryDownload200JSONResponse(download(d)), nil
}

func (h *handlers) EstimateDownloads(ctx context.Context, req gen.EstimateDownloadsRequestObject) (gen.EstimateDownloadsResponseObject, error) {
	b := req.Body
	if len(b.Items) > maxDownloadCheck {
		return nil, invalid("at most 1000 works an estimate", field("/items", "tooLarge"))
	}
	items := make([]playback.EstimateRequest, len(b.Items))
	for i, it := range b.Items {
		sub, err := parseSubtitleChoice(it.SubtitleTrackID)
		if err != nil {
			return nil, err
		}
		items[i] = playback.EstimateRequest{Work: it.WorkID, Version: deref(it.VersionID), AudioTrack: it.AudioTrackID, SubtitleTrack: sub}
	}
	c := callerOf(ctx)
	estimates, err := h.deps.Downloads.Estimate(ctx, c.Caller, c.viewer, parseCapabilities(b.Capabilities), parseSettings(b.Settings), items)
	if err != nil {
		return nil, err
	}
	out := gen.DownloadEstimateList{Items: make([]gen.DownloadEstimate, len(estimates))}
	for i, e := range estimates {
		ge := gen.DownloadEstimate{
			WorkID: e.Work, VersionID: opt(e.Version), Playable: e.Playable, Reason: optAs[gen.RefusalReason](e.Reason),
			Options: []gen.DownloadOption{},
		}
		for _, o := range e.Options {
			ge.Options = append(ge.Options, gen.DownloadOption{
				Quality: gen.DownloadQuality(o.Quality), SizeBytes: o.Size, AsIs: o.AsIs, Delivery: gen.DownloadDelivery(o.Delivery),
			})
		}
		out.Items[i] = ge
	}
	return gen.EstimateDownloads200JSONResponse(out), nil
}

func (h *handlers) CheckDownloads(ctx context.Context, req gen.CheckDownloadsRequestObject) (gen.CheckDownloadsResponseObject, error) {
	b := req.Body
	if len(b.Items) > maxDownloadCheck {
		return nil, invalid("at most 1000 downloads a check", field("/items", "tooLarge"))
	}
	downloads := make([]playback.KeptVersion, len(b.Items))
	for i, d := range b.Items {
		downloads[i] = playback.KeptVersion{Work: d.WorkID, Version: d.VersionID}
	}
	available, err := h.deps.Downloads.Available(ctx, callerOf(ctx).viewer, downloads)
	if err != nil {
		return nil, err
	}
	out := gen.DownloadCheckResult{Items: make([]gen.DownloadAvailability, len(b.Items))}
	for i, d := range b.Items {
		out.Items[i] = gen.DownloadAvailability{WorkID: d.WorkID, VersionID: d.VersionID, Available: available[i]}
	}
	return gen.CheckDownloads200JSONResponse(out), nil
}

// serveDownloadFile serves a part of a download. It sends the part's
// strong ETag so a transfer can resume by range with If-Range.
func (s *Server) serveDownloadFile(w http.ResponseWriter, r *http.Request) {
	part, err := strconv.Atoi(r.PathValue("part"))
	if err != nil || part < 0 {
		s.writeProblem(w, r, errNotFound)
		return
	}
	s.serveDownloadMedia(w, r, func(ctx context.Context, v users.Viewer, download id.ID) (playback.Media, error) {
		return s.deps.Downloads.File(ctx, v, download, part)
	})
}

func (s *Server) serveDownloadSubtitle(w http.ResponseWriter, r *http.Request) {
	s.serveDownloadMedia(w, r, func(ctx context.Context, v users.Viewer, download id.ID) (playback.Media, error) {
		return s.deps.Downloads.Subtitle(ctx, v, download, r.PathValue("name"))
	})
}

// serveDownloadMedia serves the media that get returns for the caller's
// download that the path names.
func (s *Server) serveDownloadMedia(w http.ResponseWriter, r *http.Request, get func(context.Context, users.Viewer, id.ID) (playback.Media, error)) {
	download, err := id.Parse(r.PathValue("id"))
	if err != nil {
		s.writeProblem(w, r, errNotFound)
		return
	}
	m, err := get(r.Context(), callerOf(r.Context()).viewer, download)
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	s.serveMedia(w, r, m, noStore)
}

func download(d playback.DownloadStatus) gen.Download {
	out := gen.Download{
		ID: d.ID, WorkID: d.Work, VersionID: d.Version, Quality: gen.DownloadQuality(d.Quality), State: gen.DownloadState(d.State),
		Delivery: gen.DownloadDelivery(d.Decision.Delivery), Tracks: playTracks(d.Decision, d.ExternalSubtitles, downloadMedia, d.ID),
		DurationMs: ms(d.Duration), SizeBytes: d.Size, SizeExact: d.SizeExact, Parts: []gen.DownloadPart{},
		Chapters: chapters(d.Chapters), Markers: ptr(markerList(d.Markers)), Reason: optAs[gen.RefusalReason](d.Reason),
	}
	if d.State == playback.DownloadPreparing {
		out.Progress = ptr(float32(d.Progress))
		if d.Remaining > 0 {
			out.RemainingMs = ptr(ms(d.Remaining.Round(time.Second)))
		}
	}
	if !d.Expires.IsZero() {
		out.ExpiresAt = ptr(d.Expires.UTC())
	}
	for i, p := range d.Parts {
		part := gen.DownloadPart{DurationMs: ms(p.Duration), URL: mediaURL(downloadMedia, d.ID, "files", strconv.Itoa(i)), ContentType: p.Type, Etag: opt(p.ETag), Sha256: opt(p.SHA256)}
		if p.ETag != "" {
			part.SizeBytes = ptr(p.Size)
		}
		out.Parts = append(out.Parts, part)
	}
	return out
}
