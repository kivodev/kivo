package api

import (
	"bytes"
	"context"
	"errors"
	"io"
	"math/rand/v2"
	"net/http"
	"os"
	"slices"
	"strconv"
	"time"
	"unicode/utf8"

	"github.com/mmcdole/kivo/internal/api/internal/gen"
	"github.com/mmcdole/kivo/internal/auth"
	"github.com/mmcdole/kivo/internal/id"
	"github.com/mmcdole/kivo/internal/media"
	"github.com/mmcdole/kivo/internal/playback"
	"github.com/mmcdole/kivo/internal/query"
	"github.com/mmcdole/kivo/internal/transcode"
	"github.com/mmcdole/kivo/internal/users"
)

func (h *handlers) StartPlay(ctx context.Context, req gen.StartPlayRequestObject) (gen.StartPlayResponseObject, error) {
	r, err := parseStartRequest(req.Body, requestOf(ctx))
	if err != nil {
		return nil, err
	}
	c := callerOf(ctx)
	st, err := h.deps.Playback.Start(ctx, c.Caller, c.viewer, r)
	if err != nil {
		return nil, h.playbackError(ctx, c.viewer, err)
	}
	return gen.StartPlay201JSONResponse(playStarted(st)), nil
}

// playbackError adds the user's plays to a stream-limit refusal, so the
// client can offer to stop one.
func (h *handlers) playbackError(ctx context.Context, v users.Viewer, err error) error {
	limit, ok := errors.AsType[*playback.StreamLimitError](err)
	if !ok {
		return err
	}
	p, _ := problemFor(err)
	titles, _ := h.deps.Query.Titles(ctx, v, works(limit.Plays)) // best effort, on an error path
	for _, pl := range limit.Plays {
		p.plays = append(p.plays, gen.PlaySummary{
			ID: pl.ID, WorkID: pl.Work, Title: titles[pl.Work], ClientName: opt(pl.Client), StartedAt: pl.Started.UTC(),
		})
	}
	return p
}

// works lists the works being played, for their titles.
func works(plays []playback.Play) []id.ID {
	out := make([]id.ID, len(plays))
	for i, pl := range plays {
		out[i] = pl.Work
	}
	return out
}

func (h *handlers) ListPlays(ctx context.Context, _ gen.ListPlaysRequestObject) (gen.ListPlaysResponseObject, error) {
	plays := h.deps.Playback.Plays()
	byWork, err := h.playSummaries(ctx, plays)
	if err != nil {
		return nil, err
	}
	items := []gen.Play{}
	for _, pl := range plays {
		items = append(items, playInfo(pl, byWork[pl.Work]))
	}
	return gen.ListPlays200JSONResponse(gen.PlayList{Items: items}), nil
}

func (h *handlers) GetPlay(ctx context.Context, req gen.GetPlayRequestObject) (gen.GetPlayResponseObject, error) {
	pl, err := h.deps.Playback.Play(req.ID)
	if err != nil {
		return nil, err
	}
	byWork, err := h.playSummaries(ctx, []playback.Play{pl})
	if err != nil {
		return nil, err
	}
	return gen.GetPlay200JSONResponse(playInfo(pl, byWork[pl.Work])), nil
}

// playSummaries reads the summaries of the played works that the admin may
// see.
func (h *handlers) playSummaries(ctx context.Context, plays []playback.Play) (map[id.ID]query.WorkSummary, error) {
	ws := slices.Compact(slices.SortedFunc(slices.Values(works(plays)), func(a, b id.ID) int { return bytes.Compare(a[:], b[:]) }))
	found, err := h.deps.Query.Summaries(ctx, callerOf(ctx).viewer, ws)
	if err != nil {
		return nil, err
	}
	out := make(map[id.ID]query.WorkSummary, len(found))
	for _, c := range found {
		out[c.ID] = c
	}
	return out, nil
}

// UpdatePlay is the play's heartbeat. It reports the position, which also
// keeps the play alive.
func (h *handlers) UpdatePlay(ctx context.Context, req gen.UpdatePlayRequestObject) (gen.UpdatePlayResponseObject, error) {
	b := req.Body
	sub, err := parseSubtitleChoice(b.SubtitleTrackID)
	if err != nil {
		return nil, err
	}
	hb := playback.Heartbeat{
		Position: duration(b.PositionMs), State: playback.PlayState(b.State), Subtitle: sub, Audio: b.AudioTrackID,
		SeekToFirstFrame: duration(deref(b.SeekToFirstFrameMs)), Bandwidth: int(deref(b.BandwidthBps)),
	}
	if err := h.deps.Playback.Heartbeat(ctx, callerOf(ctx).Caller, req.ID, hb); err != nil {
		return nil, err
	}
	return gen.UpdatePlay204Response{}, nil
}

// StopPlay stops a play, with a message of at most 500 characters for its
// viewer.
func (h *handlers) StopPlay(ctx context.Context, req gen.StopPlayRequestObject) (gen.StopPlayResponseObject, error) {
	b := req.Body
	if utf8.RuneCountInString(deref(b.Message)) > 500 {
		return nil, invalid("message: at most 500 characters", field("/message", "tooLarge"))
	}
	c := callerOf(ctx)
	failure, err := stopFailure(b, c)
	if err != nil {
		return nil, err
	}
	r := playback.StopRequest{Message: deref(b.Message), Failure: failure}
	if b.PositionMs != nil {
		r.Position = ptr(duration(*b.PositionMs))
	}
	if err := h.deps.Playback.Stop(ctx, c.Caller, c.viewer, req.ID, r); err != nil {
		return nil, err
	}
	return gen.StopPlay204Response{}, nil
}

// stopFailure is the failure a stop reports, if it has a code. Only the
// play's own player reports one, with the position it failed at: not the
// Cast receiver, whose failures are its sender's to report, and not an
// admin stopping someone else's play, which the service refuses.
func stopFailure(b *gen.StopPlayJSONRequestBody, c caller) (*playback.FailReport, error) {
	if b.Code == nil {
		if b.Detail != nil {
			return nil, invalid("detail goes with a code", field("/code", "required"))
		}
		return nil, nil
	}
	if !c.CastToken.IsZero() {
		return nil, problem{status: http.StatusForbidden, code: codeForbidden, detail: "only the play's owner reports its failure"}
	}
	if b.PositionMs == nil {
		return nil, invalid("a code needs the position playback failed at", field("/positionMs", "required"))
	}
	if utf8.RuneCountInString(deref(b.Detail)) > 1000 {
		return nil, invalid("detail: at most 1000 characters", field("/detail", "tooLarge"))
	}
	return &playback.FailReport{Position: duration(*b.PositionMs), Code: string(*b.Code), Detail: deref(b.Detail)}, nil
}

func (h *handlers) FallbackPlay(ctx context.Context, req gen.FallbackPlayRequestObject) (gen.FallbackPlayResponseObject, error) {
	c := callerOf(ctx)
	b := req.Body
	failed := playback.FailReport{Position: duration(b.PositionMs), Code: string(b.Code), Detail: deref(b.Detail)}
	st, err := h.deps.Playback.Fallback(ctx, c.Caller, c.viewer, req.ID, failed)
	if err != nil {
		return nil, h.playbackError(ctx, c.viewer, err)
	}
	return gen.FallbackPlay201JSONResponse(playStarted(st)), nil
}

// maxBandwidthTest is the most a bandwidth test sends. That is a few
// seconds at the rates where remote quality matters.
const maxBandwidthTest = 10_000_000

// GetBandwidthTest sends bytes for a remote client to time its download
// rate. A local client needs no measurement.
func (h *handlers) GetBandwidthTest(ctx context.Context, req gen.GetBandwidthTestRequestObject) (gen.GetBandwidthTestResponseObject, error) {
	size := req.Params.Size
	if size < 1 || size > maxBandwidthTest {
		return nil, invalid("size is out of range", field("size", "outOfRange"))
	}
	if h.deps.Settings.Get().IsLocal(requestOf(ctx).ip) {
		return gen.GetBandwidthTest204Response{}, nil
	}
	// Random bytes, so nothing on the way can compress them.
	var seed [32]byte
	body := io.LimitReader(rand.NewChaCha8(seed), int64(size))
	return gen.GetBandwidthTest200ApplicationOctetStreamResponse{Body: body, ContentLength: int64(size)}, nil
}

// The media routes' URLs are relative to the API root (/api/v1/) and take
// the form <owner>/<id>/<kind>/<name>. The owner is a play or a download,
// and the kind is files, hls, subtitles or previews.
const (
	playMedia     = "plays"
	downloadMedia = "downloads"
)

// mediaURL is a media URL relative to the API root.
func mediaURL(owner string, id id.ID, kind, name string) string {
	return owner + "/" + id.String() + "/" + kind + "/" + name
}

// serveHLS serves a play's playlists and segments. Like every media route,
// it takes the play's ID in the path as the credential. Playback checks
// the ID and its user's current access on every request.
func (s *Server) serveHLS(w http.ResponseWriter, r *http.Request) {
	s.servePlayMedia(w, r, "no-cache", func(ctx context.Context, play id.ID) (playback.Media, error) {
		return s.deps.Playback.HLS(ctx, play, r.PathValue("name"))
	})
}

func (s *Server) serveFile(w http.ResponseWriter, r *http.Request) {
	part, err := strconv.Atoi(r.PathValue("part"))
	if err != nil || part < 0 {
		s.writeProblem(w, r, errNotFound)
		return
	}
	s.servePlayMedia(w, r, "", func(ctx context.Context, play id.ID) (playback.Media, error) {
		return s.deps.Playback.File(ctx, play, part)
	})
}

func (s *Server) serveSubtitle(w http.ResponseWriter, r *http.Request) {
	s.servePlayMedia(w, r, noStore, func(ctx context.Context, play id.ID) (playback.Media, error) {
		return s.deps.Playback.Subtitle(ctx, play, r.PathValue("name"))
	})
}

func (s *Server) servePreview(w http.ResponseWriter, r *http.Request) {
	s.servePlayMedia(w, r, "private, no-cache", func(ctx context.Context, play id.ID) (playback.Media, error) {
		m, found, err := s.deps.Playback.Preview(ctx, play, r.PathValue("name"))
		if err == nil && !found {
			return playback.Media{}, errNotFound
		}
		return m, err
	})
}

// servePlayMedia serves the media that get returns for the play the path
// names. Bytes made for the request are cached as cache says.
func (s *Server) servePlayMedia(w http.ResponseWriter, r *http.Request, cache string, get func(context.Context, id.ID) (playback.Media, error)) {
	play, err := id.Parse(r.PathValue("id"))
	if err != nil {
		s.writeProblem(w, r, errNotFound)
		return
	}
	m, err := get(r.Context(), play)
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	s.serveMedia(w, r, m, cache)
}

// noStore keeps play and download media out of the browser cache. They're
// never fetched again, and caching them would evict fonts and artwork.
const noStore = "private, no-store, no-transform"

// serveMedia serves either a file or bytes made for the request. A file
// is served with range requests and never cached. Made bytes are cached as
// cache says.
func (s *Server) serveMedia(w http.ResponseWriter, r *http.Request, m playback.Media, cache string) {
	if m.Path == "" {
		w.Header().Set("Content-Type", m.Type)
		w.Header().Set("Cache-Control", cache)
		http.ServeContent(w, r, "", time.Time{}, bytes.NewReader(m.Data))
		return
	}
	f, modified, err := openServed(m.Path)
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	defer f.Close()
	if m.ETag != "" {
		w.Header().Set("ETag", m.ETag)
	}
	w.Header().Set("Content-Type", m.Type)
	w.Header().Set("Cache-Control", noStore)
	rec := &rangeWriter{ResponseWriter: w}
	http.ServeContent(rec, r, "", modified, f)
	if rec.status == http.StatusRequestedRangeNotSatisfiable && !rec.wrote {
		s.writeProblem(w, r, problem{status: http.StatusRequestedRangeNotSatisfiable, code: codeRangeNotSatisfiable})
	}
}

// openServed opens a file to serve, with its modification time.
func openServed(path string) (*os.File, time.Time, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, time.Time{}, err
	}
	info, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, time.Time{}, err
	}
	return f, info.ModTime(), nil
}

// rangeWriter turns http.ServeContent's plain-text 416 into a problem
// document. It holds that one response back for the caller to write.
type rangeWriter struct {
	http.ResponseWriter
	status int
	wrote  bool
}

func (w *rangeWriter) WriteHeader(code int) {
	w.status = code
	if code != http.StatusRequestedRangeNotSatisfiable {
		w.wrote = true
		w.ResponseWriter.WriteHeader(code)
	}
}

func (w *rangeWriter) Write(b []byte) (int, error) {
	if w.status == http.StatusRequestedRangeNotSatisfiable && !w.wrote {
		return len(b), nil
	}
	w.wrote = true
	return w.ResponseWriter.Write(b)
}

// parseStartRequest is a request to play a work, from the client that
// sent req.
func parseStartRequest(b *gen.StartPlayJSONRequestBody, req *request) (playback.StartRequest, error) {
	r := playback.StartRequest{
		Work: b.WorkID, Version: deref(b.VersionID), AudioTrack: b.AudioTrackID, Caps: parseCapabilities(b.Capabilities),
		Settings: parseSettings(b.Settings), DialogueBoost: deref(b.DialogueBoost), Device: req.device, ClientIP: req.ip,
	}
	var err error
	if r.SubtitleTrack, err = parseSubtitleChoice(b.SubtitleTrackID); err != nil {
		return playback.StartRequest{}, err
	}
	if b.PositionMs != nil {
		r.Position = ptr(duration(*b.PositionMs))
	}
	var local, remote *gen.QualityRequest
	if s := b.Settings; s != nil {
		local, remote = s.LocalQuality, s.RemoteQuality
	}
	if r.Quality, err = parseQuality(local, "/settings/localQuality"); err != nil {
		return playback.StartRequest{}, err
	}
	if remote != nil {
		q, err := parseQuality(remote, "/settings/remoteQuality")
		if err != nil {
			return playback.StartRequest{}, err
		}
		r.RemoteQuality = &q
	}
	return r, nil
}

// parseQuality reads a quality the user picked. An absent quality means
// auto.
func parseQuality(q *gen.QualityRequest, at string) (playback.Quality, error) {
	switch {
	case q == nil:
		return playback.Quality{}, nil
	case q.Mode == gen.QualityModeLimit:
		if q.MaxBitrateBps == nil || *q.MaxBitrateBps <= 0 {
			return playback.Quality{}, invalid("a limit needs maxBitrateBps", field(at+"/maxBitrateBps", "required"))
		}
		return playback.Quality{Mode: playback.QualityLimit, MaxBitrate: int(*q.MaxBitrateBps)}, nil
	case q.Mode == gen.QualityModeOriginal:
		return playback.Quality{Mode: playback.QualityOriginal}, nil
	}
	return playback.Quality{Mode: playback.QualityAuto, Bandwidth: int(deref(q.BandwidthBps))}, nil
}

// playInfo describes a play for an admin. w is the zero WorkSummary when
// the admin may not see the work.
func playInfo(pl playback.Play, w query.WorkSummary) gen.Play {
	info := gen.Play{
		ID: pl.ID, WorkID: pl.Work, VersionID: opt(pl.Version), UserID: pl.User, Username: pl.Username, ClientName: opt(pl.Client),
		StartedAt: pl.Started.UTC(), State: gen.PlayState(pl.State), Local: pl.Local, Device: clientDevice(pl.Device),
		Delivery: gen.PlayDelivery(pl.Decision.Delivery), Tracks: playTracks(pl.Decision, nil, "", id.ID{}),
		PositionMs: ms(pl.Position), DurationMs: opt(ms(pl.Duration)), BitrateBps: opt(int64(pl.Bitrate)),
		BandwidthBps: opt(int64(pl.Bandwidth)), Title: w.Title, SourceTracks: &[]gen.Track{},
		VideoProcessing: videoProcessing(pl.Decision),
	}
	if !w.ID.IsZero() {
		info.Work = ptr(summary(w))
	}
	if pl.Address.IsValid() {
		info.Address = ptr(pl.Address.Unmap().String())
	}
	for _, t := range pl.Tracks {
		*info.SourceTracks = append(*info.SourceTracks, catalogTrack(t))
	}
	for _, t := range pl.Subtitles {
		*info.SourceTracks = append(*info.SourceTracks, subtitleFileTrack(t))
	}
	named(*info.SourceTracks)
	if j := pl.Transcode; j.State != "" || j.Diagnostics.Command != "" {
		info.Transcode = &gen.TranscodeInfo{
			State: optAs[gen.TranscodeState](j.State), Command: opt(j.Diagnostics.Command), Stderr: opt(j.Diagnostics.Stderr),
			EncodedMs: opt(ms(j.Encoded)), Speed: opt(j.Speed),
		}
		if j.State == transcode.JobFailed {
			info.Transcode.ExitCode = ptr(j.Diagnostics.ExitCode)
		}
	}
	for _, d := range pl.Seeks {
		info.SeekToFirstFrameMs = ptr(append(deref(info.SeekToFirstFrameMs), ms(d)))
	}
	return info
}

// videoProcessing is where a play's video is transcoded: on the
// accelerator encoding it, or on the CPU. It's nil when the video isn't
// transcoded.
func videoProcessing(d playback.Decision) *gen.ProcessedOn {
	v, ok := d.Selected(media.Video)
	switch {
	case !ok || !v.Transcoded():
		return nil
	case v.Accelerator != "":
		return ptr(gen.ProcessedOnAccelerator)
	}
	return ptr(gen.ProcessedOnCPU)
}

func playStarted(st playback.Started) gen.PlayStarted {
	out := gen.PlayStarted{
		ID: st.ID, WorkID: st.Work, VersionID: st.Version,
		StartMs: ms(st.Start), DurationMs: ms(st.Duration), Local: st.Local, BitrateBps: opt(int64(st.Bitrate)),
		Tracks: playTracks(st.Decision, st.ExternalSubtitles, playMedia, st.ID), Parts: []gen.PlayPart{},
		Previews: &gen.PreviewsDelivery{Ready: st.Previews},
	}
	if st.Cap.MaxBitrate > 0 {
		out.LimitBps = ptr(int64(st.Cap.MaxBitrate))
	}
	if st.Previews {
		out.Previews.URL = ptr(mediaURL(playMedia, st.ID, "previews", playback.PreviewManifest))
	}
	if st.Decision.Delivery == playback.DeliverStream {
		out.Delivery, out.URL = gen.PlayDeliveryStream, mediaURL(playMedia, st.ID, "hls", transcode.PlaylistName)
	} else {
		out.Delivery, out.URL = gen.PlayDeliveryFile, mediaURL(playMedia, st.ID, "files", "0")
	}
	for i, d := range st.Parts {
		p := gen.PlayPart{DurationMs: ms(d)}
		if out.Delivery == gen.PlayDeliveryFile {
			p.URL = ptr(mediaURL(playMedia, st.ID, "files", strconv.Itoa(i)))
		}
		out.Parts = append(out.Parts, p)
	}
	out.Chapters, out.Markers = chapters(st.Chapters), ptr(markerList(st.Markers))
	out.Quality = quality(st.Quality)
	if b := st.Decision.DialogueBoost; b != nil {
		out.DialogueBoost = &gen.DialogueBoost{Enabled: b.Enabled}
	}
	return out
}

// clientDevice is an app and its device on the wire. It's nil when the
// app identified nothing.
func clientDevice(d auth.Device) *gen.ClientDevice {
	if d.App == "" && d.AppVersion == "" && d.Platform == "" && d.OSVersion == "" && d.Model == "" {
		return nil
	}
	return &gen.ClientDevice{
		App: opt(d.App), AppVersion: opt(d.AppVersion), Platform: optAs[gen.ClientPlatform](d.Platform),
		OsVersion: opt(d.OSVersion), DeviceModel: opt(d.Model),
	}
}

// quality is the quality a play applies, on the wire. It is absent for
// auto.
func quality(q playback.Quality) *gen.QualityRequest {
	switch q.Mode {
	case playback.QualityOriginal:
		return &gen.QualityRequest{Mode: gen.QualityModeOriginal}
	case playback.QualityLimit:
		return &gen.QualityRequest{Mode: gen.QualityModeLimit, MaxBitrateBps: ptr(int64(q.MaxBitrate))}
	}
	return nil
}
