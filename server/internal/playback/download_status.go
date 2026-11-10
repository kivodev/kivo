package playback

import (
	"context"
	"errors"
	"os"
	"slices"
	"time"

	"github.com/mmcdole/kivo/internal/auth"
	"github.com/mmcdole/kivo/internal/catalog"
	"github.com/mmcdole/kivo/internal/id"
	"github.com/mmcdole/kivo/internal/markers"
	"github.com/mmcdole/kivo/internal/playback/dbq"
)

// DownloadState is where a download is on the server.
type DownloadState string

const (
	DownloadQueued    DownloadState = "queued"    // waiting for its turn
	DownloadPreparing DownloadState = "preparing" // its subtitle or conversion is being made
	DownloadReady     DownloadState = "ready"     // its files can be fetched
	DownloadFailed    DownloadState = "failed"    // failed with a reason; Retry asks for it again
)

// DownloadStatus is a download as its app sees it.
type DownloadStatus struct {
	ID, Work, Version id.ID
	Quality           string
	State             DownloadState
	Progress          float64       // when preparing, a fraction from 0 to 1
	Remaining         time.Duration // when preparing, about how long is left; zero until known
	Reason            Refusal       // when failed, why
	// Decision says how each choice is in the download: a track inside its
	// file, with its stream index there, or its own file fetched with it.
	Decision Decision
	Duration time.Duration
	Parts    []DownloadPart
	// Size is the total bytes of every part. It's exact (SizeExact) once
	// the download is ready, and for an original sent as it is. Until a
	// conversion is ready, it's an estimate from the quality's cap.
	Size              int64
	SizeExact         bool
	Chapters          []catalog.Chapter
	Markers           []markers.Marker
	ExternalSubtitles []ExternalSubtitle // the subtitle tracks it delivers as files
	Expires           time.Time          // once ready or failed, when the server deletes its copy
}

// DownloadPart is one of a download's files. Its size and validators are
// known once the download is ready.
type DownloadPart struct {
	Duration time.Duration
	Type     string // the content type the file is served with
	Size     int64
	ETag     string // a strong ETag, for resuming by range
	SHA256   string // a conversion's hash, to check what was fetched
}

// List lists the downloads the server keeps for the caller's session,
// newest first. An app reattaches to them after a relaunch or a server
// restart. An API key has none.
func (s *Downloads) List(ctx context.Context, p auth.Caller) ([]DownloadStatus, error) {
	rows, err := s.q.SessionDownloads(ctx, p.Session)
	if err != nil {
		return nil, err
	}
	out := make([]DownloadStatus, 0, len(rows))
	for _, d := range rows {
		s, err := s.status(ctx, d)
		if errors.Is(err, catalog.ErrNotFound) {
			continue // the files are gone; the nightly prune and the availability check handle it
		}
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, nil
}

// Status returns one of the user's downloads.
func (s *Downloads) Status(ctx context.Context, p auth.Caller, download id.ID) (DownloadStatus, error) {
	d, err := s.own(ctx, p.User, download)
	if err != nil {
		return DownloadStatus{}, err
	}
	return s.status(ctx, d)
}

// status is a download as its app sees it, from what it delivers and its
// files, subtitles and markers as they are now. Once it's ready, a
// subtitle delivered external that couldn't be extracted isn't in it.
func (s *Downloads) status(ctx context.Context, d dbq.Download) (DownloadStatus, error) {
	dl, files, err := s.kept(ctx, d)
	if err != nil {
		return DownloadStatus{}, err
	}
	out := DownloadStatus{ID: d.ID, Work: d.WorkID, Version: d.VersionID, Quality: dl.Quality, State: DownloadState(d.State), Decision: dl.decision()}
	out.ExternalSubtitles = s.externalSubtitles(ctx, files[0], s.styledOf(ctx, files[0]), out.Decision.Tracks)
	for i, t := range out.Decision.Tracks {
		if t.Delivery == DeliverExternal && out.State == DownloadReady &&
			!slices.ContainsFunc(out.ExternalSubtitles, func(x ExternalSubtitle) bool { return x.Track == t.Track && x.Ready }) {
			out.Decision.Tracks[i] = TrackDecision{Type: t.Type, Track: t.Track, Index: t.Index, Selected: t.Selected, Reason: ReasonSubtitlePreparing}
		}
	}
	if d.Reason.Valid {
		out.Reason = Refusal(d.Reason.String)
	}
	if d.SettledAt.Valid {
		out.Expires = time.UnixMilli(d.SettledAt.Int64).Add(keepDownload)
	}
	out.Chapters, out.Markers = s.deps.Markers.Markers(ctx, d.WorkID, files)
	out.Duration = duration(files)
	for i, f := range files {
		part := DownloadPart{Duration: f.Probe.Duration, Type: "video/mp4"}
		if dl.Delivery == DeliverFile {
			part.Size, part.Type = f.Size, mimeType(f.Probe.Container, f.RelPath)
			if fi, err := os.Stat(f.Path()); err == nil {
				part.Size, part.ETag = fi.Size(), fileETag(fi)
			}
		} else if made := dl.Parts[i]; made.SHA256 != "" {
			part.Size, part.ETag, part.SHA256 = made.Size, `"`+made.SHA256+`"`, made.SHA256
		}
		out.Size += part.Size
		out.Parts = append(out.Parts, part)
	}
	out.SizeExact = out.State == DownloadReady || dl.Delivery == DeliverFile
	if out.State != DownloadReady {
		out.Size = dl.Size
	}
	if out.State == DownloadQueued || out.State == DownloadPreparing {
		s.mu.Lock()
		p := s.preparing[d.ID]
		if p != nil {
			out.State = DownloadPreparing
			out.Progress, out.Remaining = p.progressLocked(time.Now())
		}
		s.mu.Unlock()
		if p == nil {
			// This process isn't preparing it, so it's in line, or back in
			// line after a restart.
			out.State = DownloadQueued
		}
	}
	return out, nil
}
