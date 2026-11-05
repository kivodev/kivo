package playback

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"

	"github.com/mmcdole/kivo/internal/catalog"
	"github.com/mmcdole/kivo/internal/id"
	"github.com/mmcdole/kivo/internal/media"
	"github.com/mmcdole/kivo/internal/playback/dbq"
	"github.com/mmcdole/kivo/internal/users"
)

// plan records how the server decided a download: the version's files,
// the decision, and the input it was decided from. It's stored so the
// download stays as it was decided, even if settings or the catalog
// change later.
type plan struct {
	Files    []id.ID  `json:"files"`
	Input    Input    `json:"input"`
	Decision Decision `json:"decision"`
}

// planDownload chooses a download's version and decides its delivery the
// same way a play does, but for whole files.
func (s *Service) planDownload(ctx context.Context, user users.User, r DownloadRequest) (*choice, error) {
	q, ok := DownloadQualityNamed(cmp.Or(r.Quality, DefaultDownloadQuality))
	if !ok {
		return nil, fmt.Errorf("unknown download quality %q", r.Quality)
	}
	sr := StartRequest{Work: r.Work, Version: r.Version, AudioTrack: r.AudioTrack, SubtitleTrack: r.SubtitleTrack, Caps: r.Caps, Settings: r.Settings}
	base := Input{Caps: r.Caps, Settings: r.Settings, Encoders: s.encoders(false), Target: TargetFile, Quality: q}
	c, err := s.choose(ctx, user.ID, sr, s.prefsFor(ctx, user, r.Work), base)
	if err != nil {
		return nil, err
	}
	if c.input.Source.Video == nil {
		c.input.Quality = DownloadQuality{Name: "original"} // a book is always its original
	}
	c.styled = s.styledOf(ctx, c.files[0])
	return &c, nil
}

func planOf(c *choice) plan {
	p := plan{Input: c.input, Decision: c.decision}
	for _, f := range c.files {
		p.Files = append(p.Files, f.ID)
	}
	return p
}

// errPlanUnreadable means a download's stored plan can't be read. The
// download fails, and Retry plans it again from scratch.
var errPlanUnreadable = errors.New("download plan unreadable")

// readPlan reads a download's stored plan. A plan with an unknown field
// returns errPlanUnreadable rather than reading as blank.
func readPlan(d dbq.Download) (plan, error) {
	var p plan
	dec := json.NewDecoder(strings.NewReader(d.Plan))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&p); err != nil {
		return plan{}, fmt.Errorf("%w: %w", errPlanUnreadable, err)
	}
	// Migration (downloads): remove after every client has updated past this release.
	p = currentPlan(p, DownloadState(d.State) == DownloadReady)
	return p, nil
}

// restore rebuilds a download's choice from its stored plan, reading its
// files as the catalog has them now.
func (s *Service) restore(ctx context.Context, d dbq.Download) (*choice, error) {
	p, err := readPlan(d)
	if err != nil {
		return nil, err
	}
	c := &choice{work: d.WorkID, version: catalog.Version{ID: d.VersionID, Parts: p.Files}, input: p.Input, decision: p.Decision}
	for _, fid := range p.Files {
		f, err := s.deps.Catalog.File(ctx, fid)
		if err != nil {
			return nil, err
		}
		c.files = append(c.files, f)
	}
	if len(c.files) == 0 {
		return nil, fmt.Errorf("download %s has no files", d.ID)
	}
	c.styled = s.styledOf(ctx, c.files[0])
	return c, nil
}

// needsPreparing reports whether the server must work on a download
// before it's ready, either to convert it or to extract subtitles.
func (s *Service) needsPreparing(ctx context.Context, c *choice) bool {
	return c.decision.Delivery == DeliverConversion || s.subtitlesToExtract(ctx, c)
}

// subtitlesToExtract reports whether a download needs its file's subtitles
// extracted before it's ready. That's either its chosen subtitle or the
// tracks it delivers as separate files.
func (s *Service) subtitlesToExtract(ctx context.Context, c *choice) bool {
	return s.subtitleToExtract(ctx, c) || s.externalSubtitlesToExtract(ctx, c)
}

// externalSubtitlesToExtract reports whether a download delivers an
// embedded subtitle track external, as a file that isn't ready yet, while the file's
// subtitles aren't extracted. The download waits so every track comes
// down with it. A track still missing after extraction couldn't be
// extracted, and is left out.
func (s *Service) externalSubtitlesToExtract(ctx context.Context, c *choice) bool {
	f := c.files[0]
	waits := slices.ContainsFunc(s.externalSubtitles(ctx, c), func(d ExternalSubtitle) bool {
		_, embedded := embeddedSubtitle(f, d.Track)
		return embedded && !d.Ready
	})
	return waits && !s.extractedAll(ctx, f)
}

// extractedAll reports whether a file's subtitle extraction has run and
// every font it lists is in the store. It uses Styled's answer, which
// covers the whole extraction.
func (s *Service) extractedAll(ctx context.Context, f catalog.File) bool {
	_, ok, err := s.deps.Subtitles.Styled(ctx, f.ID)
	if err != nil {
		slog.WarnContext(ctx, "playback: subtitle extraction", "file", f.ID, "err", err)
	}
	return ok
}

// subtitleToExtract reports whether a download's subtitle isn't prepared
// yet. That applies to an embedded text track kept as a file or burned in,
// and to a PGS track kept as a file. Subtitle files, a track the
// conversion carries in its file or the client reads from the original,
// and a burned-in PGS track are all read from the source, so they need
// nothing.
func (s *Service) subtitleToExtract(ctx context.Context, c *choice) bool {
	sub := c.input.Source.Subtitle
	t, ok := c.decision.Selected(media.Subtitle)
	if sub == nil || !ok || sub.OwnFile || t.Delivery == DeliverStream || t.Delivery == DeliverFile ||
		t.Delivery == DeliverBurn && !formatOf(sub.Codec).Text() {
		return false
	}
	return !s.prepared(ctx, c.files[0], *sub, s.styledOf(ctx, c.files[0]))
}
