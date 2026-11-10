package markers

import (
	"cmp"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/mmcdole/kivo/internal/cachedir"
	"github.com/mmcdole/kivo/internal/catalog"
	"github.com/mmcdole/kivo/internal/config"
	"github.com/mmcdole/kivo/internal/id"
	"github.com/mmcdole/kivo/internal/markers/dbq"
	"github.com/mmcdole/kivo/internal/markers/detect"
	"github.com/mmcdole/kivo/internal/media"
)

// The stretches of an episode that are fingerprinted for its markers. The
// head, for the intro, is the first quarter, up to headMax. The tail, for
// the credits, is the last tailShare of the episode or the last tailMax,
// whichever is shorter.
const (
	headShare = 0.25
	headMax   = 10 * time.Minute
	tailShare = 0.4
	tailMax   = 450 * time.Second
	// readSlack is how long reading a stretch may take beyond its length.
	// A disk that plays a file reads it at least as fast as it plays, so a
	// read still going past that has stalled.
	readSlack = 2 * time.Minute
	// fingerprintFile is the name of a file's fingerprint in its result
	// folder.
	fingerprintFile = "fingerprint.json"
)

// fileToRead is a file to fingerprint, with its part number in its version
// and the version's number of parts.
type fileToRead struct {
	catalog.File
	part, parts int
}

// find finds one episode file's markers. It fingerprints the file, unless
// its library finds no markers or the file is fingerprinted already. Then
// it decides the file's season once none of the season's other files is
// still to be read. A file fingerprinted before is decided at once, because
// a later read may never have decided its season. A file the media package
// can't read is recorded as failed. It isn't read again until it changes,
// and it counts as read for its season.
func (s *Service) find(ctx context.Context, file id.ID) error {
	f, err := s.deps.Catalog.File(ctx, file)
	switch {
	case errors.Is(err, catalog.ErrNotFound):
		return nil // deleted, or merged into a gone file by a move
	case err != nil:
		return err
	case !f.Present || f.Probe.Duration <= 0 || !s.deps.Media.Chromaprint(ctx):
		return nil
	}
	if settled, err := s.settled(ctx, f.ID, detect.RulesVersion); settled || err != nil {
		return err
	}
	ep, err := s.q.FileSeason(ctx, f.ID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil // not an episode's
	}
	if err != nil {
		return err
	}
	lib, err := s.deps.Catalog.Library(ctx, f.Library)
	if err != nil || lib.Markers == catalog.Never {
		return err
	}
	if !s.fingerprinted(f.ID, f.Generation) {
		if err := s.room.Check(); err != nil {
			return err
		}
		fp, err := s.fingerprint(ctx, fileToRead{File: f, part: int(ep.Part.Int64), parts: int(ep.Parts)}, ep.OriginalLanguage)
		switch {
		case ctx.Err() != nil:
			return ctx.Err() // the task stopped or the window closed, which isn't the file's failure
		case errors.Is(err, media.ErrSourceCorrupt), errors.Is(err, media.ErrUnsupported):
			err = s.fail(ctx, f, err)
		case err == nil:
			err = s.keepFingerprint(ctx, f, fp)
		}
		if err != nil {
			return err
		}
		if waiting, err := s.waiting(ctx, ep.SeasonID); waiting || err != nil {
			return err // the season's last file to be read decides it
		}
	}
	return s.decide(ctx, ep.SeasonID)
}

// settled reports whether a file's markers are decided at its generation
// by rules, or failed at it.
func (s *Service) settled(ctx context.Context, file id.ID, rules int64) (bool, error) {
	return s.q.Settled(ctx, dbq.SettledParams{ID: file, Rules: rules})
}

// waiting reports whether any of a season's files is still to be read,
// meaning it is neither fingerprinted nor failed.
func (s *Service) waiting(ctx context.Context, season id.ID) (bool, error) {
	sn, err := s.season(ctx, season)
	if err != nil {
		return false, err
	}
	for _, f := range sn.files {
		if s.fingerprinted(f.ID, f.Generation) {
			continue
		}
		failed, err := s.settled(ctx, f.ID, -1) // no rules version is -1, so only a failure counts
		if err != nil || !failed {
			return !failed, err
		}
	}
	return false, nil
}

// fingerprint reads a file's head if it's its version's first part. It
// reads the file's tail, with the pictures of its keyframes, if it's the
// last part. It reads the audio track in the series' language, or else the
// first audio track. A file without audio gets an empty fingerprint.
func (s *Service) fingerprint(ctx context.Context, f fileToRead, language string) (*detect.Fingerprint, error) {
	audio, ok := audioTrack(f.File, language)
	if !ok {
		return &detect.Fingerprint{}, nil
	}
	d := f.Probe.Duration
	var fp detect.Fingerprint
	var err error
	if f.part == 1 {
		fp.Head, err = s.sample(ctx, media.SampleRequest{Path: f.File.Path(), AudioTrack: audio, Length: min(time.Duration(float64(d)*headShare), headMax)})
	}
	if err == nil && f.part == f.parts {
		length := min(time.Duration(float64(d)*tailShare), tailMax)
		req := media.SampleRequest{Path: f.File.Path(), AudioTrack: audio, Start: d - length, Length: length}
		if v, ok := videoTrack(f.File); ok {
			req.Video = &media.SampleVideo{Track: v.Index, Source: v, Accelerator: s.decoder(v)}
		}
		fp.Tail, err = s.sample(ctx, req)
	}
	return &fp, err
}

// sample reads one stretch, within a deadline of its length plus
// readSlack. If a device fails to decode the keyframes, they are read again
// on the CPU.
func (s *Service) sample(ctx context.Context, req media.SampleRequest) (*detect.Window, error) {
	read := func(req media.SampleRequest) (media.Sample, error) {
		ctx, cancel := context.WithTimeout(ctx, req.Length+readSlack)
		defer cancel()
		return s.deps.Media.Sample(ctx, req)
	}
	got, err := read(req)
	if err != nil && ctx.Err() == nil && req.Video != nil && req.Video.Accelerator != "" {
		slog.InfoContext(ctx, "markers: keyframes not decoded on the device, so on the CPU", "accelerator", req.Video.Accelerator, "err", err)
		req.Video.Accelerator = ""
		got, err = read(req)
	}
	if err != nil {
		return nil, err
	}
	return &detect.Window{Start: req.Start, Points: detect.PackPoints(got.Points), Silences: got.Silences, Keyframes: got.Keyframes}, nil
}

// decoder returns the accelerator that decodes a video track's keyframes,
// as the accelerator setting allows. It picks the first accelerator that
// decodes the track's format, among the backends that can hand frames to
// the CPU. It returns "" to decode on the CPU.
func (s *Service) decoder(v media.Track) media.Accelerator {
	setting := s.deps.Settings.Get().Accelerator
	if setting == config.AcceleratorNone {
		return ""
	}
	format := media.VideoFormat{Codec: v.Codec, BitDepth: cmp.Or(v.BitDepth, 8)}
	for _, c := range s.deps.Media.Capabilities().Accelerators {
		if setting != config.AcceleratorAuto && string(c.Accelerator) != setting {
			continue
		}
		if (c.Backend == media.VAAPI || c.Backend == media.QSV || c.Backend == media.NVENC) && slices.Contains(c.Decoders, format) {
			return c.Accelerator
		}
	}
	return ""
}

// audioTrack returns the audio track a file is fingerprinted from. It is
// the first track in language, or else the first track. Commentary tracks
// are left out.
func audioTrack(f catalog.File, language string) (int, bool) {
	first := -1
	for _, t := range f.Tracks {
		if media.TrackKind(t.Type) != media.Audio {
			continue
		}
		var mt media.Track
		if json.Unmarshal(t.Info, &mt) == nil && mt.IsCommentary() {
			continue
		}
		if language != "" && media.BaseLanguage(t.Language) == media.BaseLanguage(language) {
			return t.Index, true
		}
		if first < 0 {
			first = t.Index
		}
	}
	return first, first >= 0
}

// fingerprinted reports whether a file's fingerprint is kept at gen.
func (s *Service) fingerprinted(file id.ID, gen int64) bool {
	_, err := os.Stat(filepath.Join(s.results.Path(file, gen), fingerprintFile))
	return err == nil
}

// fingerprintAt returns a file's kept fingerprint, if it was fingerprinted
// at gen.
func (s *Service) fingerprintAt(file id.ID, gen int64) (*detect.Fingerprint, bool) {
	b, err := os.ReadFile(filepath.Join(s.results.Path(file, gen), fingerprintFile))
	if err != nil {
		return nil, false
	}
	var fp detect.Fingerprint
	if err := json.Unmarshal(b, &fp); err != nil {
		return nil, false
	}
	return &fp, true
}

// keepFingerprint puts a file's fingerprint in its result folder.
func (s *Service) keepFingerprint(ctx context.Context, f catalog.File, fp *detect.Fingerprint) error {
	tmp, err := cachedir.MakeTemp(s.results.Dir)
	if err != nil {
		return s.room.Err(err)
	}
	defer os.RemoveAll(tmp)
	b, err := json.Marshal(fp)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(tmp, fingerprintFile), b, 0o644); err != nil {
		return s.room.Err(err)
	}
	_, err = s.results.Place(ctx, tmp, f.ID, f.Generation) // a stale fingerprint is dropped, and decide skips its file
	return err
}
