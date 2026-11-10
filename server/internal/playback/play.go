package playback

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/netip"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/mmcdole/kivo/internal/store"

	"github.com/mmcdole/kivo/internal/activity"
	"github.com/mmcdole/kivo/internal/auth"
	"github.com/mmcdole/kivo/internal/catalog"
	"github.com/mmcdole/kivo/internal/id"
	"github.com/mmcdole/kivo/internal/markers"
	"github.com/mmcdole/kivo/internal/media"
	"github.com/mmcdole/kivo/internal/transcode"
	"github.com/mmcdole/kivo/internal/users"
)

// StartRequest is a request to play a work.
type StartRequest struct {
	Work          id.ID
	Version       id.ID          // zero means the chosen track's version, else the work's default version
	Position      *time.Duration // nil means the resume position
	AudioTrack    *id.ID         // nil means choose by preference
	SubtitleTrack *id.ID         // nil means choose by preference; the zero ID means off
	Caps          Capabilities
	Settings      Settings
	Quality       Quality     // the client's quality setting
	RemoteQuality *Quality    // the client's separate setting for remote viewing, if it has one
	DialogueBoost bool        // the client's dialogue boost switch (Input.DialogueBoost)
	Device        auth.Device // the app and device that asked, as its request identified them
	ClientIP      netip.Addr
}

// Quality is the quality a client asks for.
type Quality struct {
	Mode       QualityMode
	MaxBitrate int // in bits per second, for QualityLimit; zero for the other modes
	// Bandwidth is the client's measured download rate, for QualityAuto.
	// The play stays below about 80% of it. Zero means unknown, and it's
	// always zero for the other modes.
	Bandwidth int
}

// QualityMode is how a client picks its quality.
type QualityMode string

// The quality modes. The zero QualityMode means QualityAuto.
const (
	QualityAuto     QualityMode = "auto"     // capped by the limits and the measured bandwidth
	QualityOriginal QualityMode = "original" // no cap from the client itself
	QualityLimit    QualityMode = "limit"    // capped at MaxBitrate
)

// qualityFor is the quality a play applies. It's the client's remote
// quality when the play is remote and the client has one, and otherwise
// the client's quality.
func (r StartRequest) qualityFor(local bool) Quality {
	if !local && r.RemoteQuality != nil {
		return *r.RemoteQuality
	}
	return r.Quality
}

// Started describes a started play for the client.
type Started struct {
	ID, Work, Version id.ID
	Start, Duration   time.Duration
	Local             bool
	Quality           Quality // the client's quality the play applies (StartRequest.Quality or RemoteQuality)
	MaxBitrate        int     // the lowest cap that applies, including the client's own; zero means none
	Cap               Limit   // the cap the server sets regardless of the quality the client picks
	Bitrate           int     // the stream's bitrate, as "Now playing" shows it
	Decision          Decision
	Parts             []time.Duration // the durations of the version's parts, in order
	Chapters          []catalog.Chapter
	Markers           []markers.Marker
	ExternalSubtitles []ExternalSubtitle // the subtitle tracks the decision delivers as files
	Previews          bool               // whether the version's scrub previews are made
}

// Start begins a play for the viewer, after checking that the viewer can
// see the work and that the user's limits allow it.
func (s *Service) Start(ctx context.Context, p auth.Caller, v users.Viewer, r StartRequest) (Started, error) {
	return s.start(ctx, p, v, r, nil)
}

// start begins a play. For a fallback, failed is the play whose playback
// failed and which the new play replaces. For a plain start, failed is
// nil.
func (s *Service) start(ctx context.Context, p auth.Caller, v users.Viewer, r StartRequest, failed *play) (Started, error) {
	if ok, err := s.visible(ctx, v, r.Work); err != nil {
		return Started{}, err
	} else if !ok {
		return Started{}, fmt.Errorf("%w: work %s", ErrNotFound, r.Work)
	}
	user, err := s.deps.Users.User(ctx, p.User)
	if err != nil {
		return Started{}, err
	}
	if err := s.reserveStream(p.User, user.Limits.StreamLimit); err != nil {
		return Started{}, err
	}
	held := reservation{user: p.User}
	published := false
	defer func() {
		if !published {
			s.mu.Lock()
			s.releaseLocked(held)
			s.mu.Unlock()
		}
	}()

	local := s.deps.Settings.Get().IsLocal(r.ClientIP)
	pl := &play{
		id: id.New(), user: p.User, username: user.Username, caller: p, request: r,
		client: p.Name, local: local, cap: capFor(user, s.deps.Settings.Get().RemoteQualityCap, local), started: time.Now(),
		state: StatePlaying,
	}
	prefs := s.prefsFor(ctx, user, r.Work)
	cpu := failed != nil && failed.fallbackOnCPU()
	base := Input{Caps: r.Caps, Settings: r.Settings, Limit: limitFor(r, pl.cap, local), Encoders: s.encoders(cpu),
		Fallback: failed != nil, DialogueBoost: r.DialogueBoost}
	c, err := s.choose(ctx, p.User, r, prefs, base)
	if err != nil {
		if refused, ok := errors.AsType[*RefusedError](err); ok {
			slog.InfoContext(ctx, "playback: refused", "work", r.Work, "client", p.Name, "local", local, "reason", refused.Reason)
		}
		return Started{}, err
	}
	c.cpu = cpu
	pl.choice, pl.subtitle, pl.audio = c, c.selectedSubtitle(), c.selectedAudio()
	if r.Position != nil {
		pl.start = *r.Position
	} else if pl.start, err = s.deps.Userstate.Resume(ctx, p.User, r.Work, c.version.ID); err != nil {
		return Started{}, err
	}
	pl.position = pl.start
	if pl.transcodesVideo() {
		if err := s.reserveTranscode(); err != nil {
			pl.logStart("playback: refused", "reason", RefuseTranscodeLimit)
			return Started{}, err
		}
		held.transcode = true
	}
	if err := s.open(ctx, pl); err != nil {
		if refused, ok := errors.AsType[*RefusedError](err); ok {
			pl.logStart("playback: refused", "reason", refused.Reason)
		}
		return Started{}, err
	}
	s.mu.Lock()
	pl.seen = time.Now()
	s.plays[pl.id] = pl
	s.releaseLocked(held) // the published play now counts itself
	s.mu.Unlock()
	published = true
	s.notify()
	if failed != nil {
		pl.logStart("playback: play started", "reason", ReasonFallback)
	} else {
		pl.logStart("playback: play started", "reason", "start")
		s.remember(ctx, pl, prefs, chosen{audio: r.AudioTrack, subtitle: r.SubtitleTrack}) // a fallback reuses the failed play's tracks, so it isn't a new choice
		s.recordStart(ctx, pl)
	}
	// TODO: no webhook is sent for a play's start.
	return s.started(ctx, pl), nil
}

// recordStart records a play's start in activity. If recording fails, the
// play still goes on.
func (s *Service) recordStart(ctx context.Context, pl *play) {
	err := s.db.Write(ctx, func(ctx context.Context, tx *store.Tx) error {
		return s.deps.Activity.Played(ctx, tx, activity.PlayStarted, activity.PlayActivity{User: pl.user, Work: pl.work, Client: pl.client, Local: pl.local})
	})
	if err != nil {
		slog.WarnContext(ctx, "playback: record a play's start", "work", pl.work, "err", err)
	}
}

// choice is a version of a work that was chosen and decided, for a play
// or a download. It holds the files, the decision and the input it was
// decided from, and what the transcode needs.
type choice struct {
	work     id.ID
	version  catalog.Version
	files    []catalog.File
	input    Input
	decision Decision
	styled   styled // the first part's styled subtitles, as last looked up
	// cpu means the video transcode runs on the CPU, because an
	// accelerator failed it or failed the play it replaces.
	cpu bool
	// keyframes holds each part's keyframes, where a remux cuts it, read
	// when the choice was decided. It's nil when a part can't be remuxed
	// or the choice wasn't decided for a stream.
	keyframes [][]media.Keyframe
}

// selectedSubtitle is the subtitle track the choice selected, or the zero
// ID for none.
func (c *choice) selectedSubtitle() id.ID {
	if sub := c.input.Source.Subtitle; sub != nil {
		return sub.Track
	}
	return id.ID{}
}

// selectedAudio is the audio track the choice selected, or the zero ID for
// none.
func (c *choice) selectedAudio() id.ID {
	if a := c.input.Source.Audio; a != nil {
		return a.ID
	}
	return id.ID{}
}

// play is an active play. It's complete before it's published to the
// service's map and doesn't change afterwards, except for the fields that
// heartbeats and media requests update. Those are read and written under
// s.mu.
type play struct {
	choice
	id, user id.ID
	caller   auth.Caller  // who started the play; its session or API key must stay alive
	request  StartRequest // the request that started the play, so a fallback can start it again
	client   string       // the name of the session or API key that started the play
	username string       // the user's name when the play started
	local    bool
	cap      Limit // the cap the server sets regardless of the quality the client picks
	start    time.Duration
	tc       *transcode.Stream
	started  time.Time

	// These fields are guarded by the service's mu.
	seen      time.Time
	state     PlayState // as the last heartbeat reported it
	seeks     []time.Duration
	position  time.Duration // as the last heartbeat or the stop reported it
	reported  time.Time     // when a heartbeat or the stop reported the position; zero if none has
	bandwidth int           // the client's measured download rate, as the last heartbeat reported it
	subtitle  id.ID         // the subtitle track showing, from the start or the last heartbeat; zero means off
	audio     id.ID         // the audio track playing, from the start or the last heartbeat; zero means none
}

// transcodesVideo reports whether the play counts against the server's
// video transcode limit.
func (pl *play) transcodesVideo() bool {
	v, ok := pl.decision.Selected(media.Video)
	return ok && v.Transcoded()
}

// accelerator is the accelerator encoding the choice's video. It's empty
// when the video isn't encoded on an accelerator.
func (c *choice) accelerator() media.Accelerator {
	if v, ok := c.decision.Selected(media.Video); ok && v.Transcoded() {
		return v.Accelerator
	}
	return ""
}

// fallbackOnCPU reports whether a fallback from the play encodes its video
// on the CPU. It does when the play encoded its video on an accelerator,
// or had already moved it off one.
func (pl *play) fallbackOnCPU() bool {
	return pl.cpu || pl.accelerator() != ""
}

// transcodesAll reports whether the play already encodes all its video
// and audio on the CPU, so a fallback has nothing else to try.
func (pl *play) transcodesAll() bool {
	for _, t := range pl.decision.Tracks {
		if t.Selected && (t.Type == media.Video || t.Type == media.Audio) && (!t.Transcoded() || t.Accelerator != "") {
			return false
		}
	}
	return true
}

// choose picks the version and decides it from the stored probe results.
// Starting a play never runs ffprobe, and a file replaced since its last
// probe is decided again. A version that doesn't belong to the work
// returns ErrNotFound. A request that names no version plays the version
// its chosen track belongs to, or with no track chosen, the work's default
// version (see defaultVersion).
func (s *Service) choose(ctx context.Context, user id.ID, r StartRequest, prefs trackPrefs, base Input) (choice, error) {
	versions, err := s.deps.Catalog.VersionsOf(ctx, r.Work)
	if err != nil {
		return choice{}, err
	}
	switch {
	case !r.Version.IsZero():
		versions = slices.DeleteFunc(versions, func(ver catalog.Version) bool { return ver.ID != r.Version })
	case r.AudioTrack == nil && (r.SubtitleTrack == nil || r.SubtitleTrack.IsZero()):
		last, err := s.deps.Userstate.LastVersion(ctx, user, r.Work)
		if err != nil {
			return choice{}, err
		}
		versions = defaultVersion(versions, last)
	}
	for _, ver := range versions {
		c, err := s.decideVersion(ctx, ver, r, prefs, base)
		if err != nil {
			return choice{}, err
		}
		if c == nil {
			continue
		}
		if !c.decision.Playable {
			return choice{}, &RefusedError{Err: ErrNotPlayable, Reason: c.decision.Refusal, Unmet: c.decision.Unmet}
		}
		return *c, nil
	}
	return choice{}, fmt.Errorf("%w: no version of work %s to play", ErrNotFound, r.Work) // no file is present, or no version has the chosen tracks
}

// defaultVersion returns the version a play of a work plays when its
// request chooses neither a version nor a track: the version the user last
// played, while the work still has it, else the first in order. versions
// are the work's, in catalog.CompareVersions order.
func defaultVersion(versions []catalog.Version, last id.ID) []catalog.Version {
	if i := slices.IndexFunc(versions, func(v catalog.Version) bool { return v.ID == last }); i >= 0 {
		return versions[i : i+1]
	}
	return versions[:min(len(versions), 1)]
}

// decideVersion decides a version for the request: its files, the tracks
// it would play, and their deliveries. It returns nil for a version with a
// missing file, or without a track the request chooses. That way a chosen
// track's ID selects its version when the request names none.
func (s *Service) decideVersion(ctx context.Context, v catalog.Version, r StartRequest, prefs trackPrefs, base Input) (*choice, error) {
	c := &choice{work: r.Work, version: v}
	for _, part := range v.Parts {
		f, err := s.deps.Catalog.File(ctx, part)
		if err != nil {
			return nil, err
		}
		if !f.Present {
			return nil, nil
		}
		c.files = append(c.files, f)
	}
	first := c.files[0]
	src := Source{Container: first.Probe.Container, Ext: strings.ToLower(filepath.Ext(first.RelPath)), Bitrate: first.Probe.Bitrate}
	var audio []SourceTrack
	for _, t := range first.Tracks {
		mt := probed(t)
		switch {
		case mt.Kind == media.Video && !mt.AttachedPicture, mt.Kind == media.Audio:
			src.Tracks = append(src.Tracks, SourceTrack{ID: t.ID, Track: mt})
		}
		if mt.Kind == media.Audio {
			audio = append(audio, SourceTrack{ID: t.ID, Track: mt})
		}
	}
	if i := slices.IndexFunc(src.Tracks, func(t SourceTrack) bool { return t.Kind == media.Video }); i >= 0 {
		src.Video = &src.Tracks[i]
	}
	if r.AudioTrack != nil && !slices.ContainsFunc(audio, func(t SourceTrack) bool { return t.ID == *r.AudioTrack }) {
		return nil, nil
	}
	if len(audio) > 0 {
		tracks := make([]media.Track, len(audio))
		for i, a := range audio {
			tracks[i] = a.Track
		}
		pick := pickAudio(tracks, prefs)
		if r.AudioTrack != nil {
			pick = slices.IndexFunc(audio, func(t SourceTrack) bool { return t.ID == *r.AudioTrack })
		}
		src.Audio = &audio[pick]
		def := slices.IndexFunc(tracks, func(t media.Track) bool { return t.Default })
		src.DefaultAudio = pick == max(def, 0)
	}
	var subID id.ID
	if r.SubtitleTrack != nil {
		subID = *r.SubtitleTrack
	} else {
		subID = pickSubtitle(first, mediaTracks(src.Audio), prefs)
	}
	c.styled = s.styledOf(ctx, first)
	for _, sub := range subtitleSources(first) {
		sub.Prepared = s.prepared(ctx, first, sub, c.styled)
		src.Subtitles = append(src.Subtitles, sub)
	}
	if !subID.IsZero() {
		i := slices.IndexFunc(src.Subtitles, func(sub SubtitleSource) bool { return sub.Track == subID })
		if i < 0 {
			return nil, nil
		}
		src.Subtitle = &src.Subtitles[i]
	}
	if src.Video != nil && base.Target == TargetStream {
		c.keyframes = s.remuxKeyframes(ctx, c.files)
		src.Remuxable = c.keyframes != nil
	}
	c.input = base
	c.input.Source = src
	c.decision = Decide(c.input)
	if r.AudioTrack == nil && c.decision.Delivery != DeliverFile {
		c.playableAudio(audio)
	}
	return c, nil
}

// playableAudio switches a play that can't play its file directly to
// another audio track when that track lets it: the file's only obstacle
// was its audio, as a TV that decodes Dolby Digital but not TrueHD or DTS
// meets in a Blu-ray rip that carries both. The track is in the same
// language, isn't a commentary or an audio description, and keeps as many
// channels, up to the 5.1 a transcode would make anyway. Of several, the
// one with the most channels wins, then E-AC-3 over AC-3, then the first
// in the file.
func (c *choice) playableAudio(audio []SourceTrack) {
	in := c.input
	a := in.Source.Audio
	if a == nil {
		return
	}
	lang, channels := media.BaseLanguage(a.Language), min(sourceChannels(a.Track), 6)
	def := max(slices.IndexFunc(audio, func(t SourceTrack) bool { return t.Default }), 0)
	var candidates []int
	for i, t := range audio {
		if t.ID != a.ID && !t.IsCommentary() && !t.IsDescription() && media.BaseLanguage(t.Language) == lang &&
			sourceChannels(t.Track) >= channels {
			candidates = append(candidates, i)
		}
	}
	slices.SortStableFunc(candidates, func(x, y int) int {
		return cmp.Or(sourceChannels(audio[y].Track)-sourceChannels(audio[x].Track), dolbyRank(audio[x].Codec)-dolbyRank(audio[y].Codec))
	})
	for _, i := range candidates {
		try := in
		try.Source.Audio, try.Source.DefaultAudio = &audio[i], i == def
		if d := Decide(try); d.Delivery == DeliverFile {
			c.input, c.decision = try, d
			return
		}
	}
}

// dolbyRank orders codecs for playableAudio: E-AC-3, then AC-3, then the rest.
func dolbyRank(codec media.Codec) int {
	switch codec {
	case media.EAC3:
		return 0
	case media.AC3:
		return 1
	}
	return 2
}

// mediaTracks returns a selected track as probed, or nil for none.
func mediaTracks(t *SourceTrack) *media.Track {
	if t == nil {
		return nil
	}
	return &t.Track
}

// probed returns a stored track as the probe described it.
func probed(t catalog.Track) media.Track {
	var mt media.Track
	if err := json.Unmarshal(t.Info, &mt); err != nil {
		mt = media.Track{Kind: media.TrackKind(t.Type), Codec: media.Codec(t.Codec)}
	}
	mt.Index = t.Index
	return mt
}

// remuxKeyframes returns each part's keyframes when every part's container
// indexes them closely enough to cut copied video into segments, and nil
// otherwise. They're read again each time a play starts.
func (s *Service) remuxKeyframes(ctx context.Context, files []catalog.File) [][]media.Keyframe {
	out := make([][]media.Keyframe, len(files))
	for i, f := range files {
		if out[i] = s.deps.Transcoder.RemuxKeyframes(ctx, transcode.FileRef{ID: f.ID, Path: f.Path(), Duration: f.Probe.Duration}); out[i] == nil {
			return nil
		}
	}
	return out
}

// open starts the play's transcode, if its decision needs one, before
// the play is published.
func (s *Service) open(ctx context.Context, pl *play) error {
	if pl.decision.Delivery == DeliverFile {
		return nil
	}
	var tc *transcode.Stream
	err := s.transcodeWithFallback(ctx, &pl.choice, func(spec transcode.Spec) error {
		var err error
		tc, err = s.deps.Transcoder.Open(ctx, spec, pl.start)
		return err
	})
	switch {
	case errors.Is(err, transcode.ErrStorageFull):
		return &RefusedError{Err: ErrTranscodeBusy, Reason: RefuseStorageFull}
	case errors.Is(err, transcode.ErrUnsupported):
		return &RefusedError{Err: ErrNotPlayable, Reason: RefuseKeyframeIndex}
	case errors.Is(err, transcode.ErrJobFailed), errors.Is(err, media.ErrSourceUnreadable), errors.Is(err, media.ErrSourceCorrupt):
		return &RefusedError{Err: ErrNotPlayable, Reason: RefuseTranscodeFailed}
	case err != nil:
		return err
	}
	pl.tc = tc
	return nil
}

// transcodeWithFallback runs a play's or a download's transcode. If the
// accelerator is busy or fails as the job starts, the failure is recorded
// and the job runs again on the CPU. The reason goes on the decision's
// video track.
func (s *Service) transcodeWithFallback(ctx context.Context, c *choice, run func(transcode.Spec) error) error {
	// try chooses the encoder for the video, if it transcodes video, and
	// runs the transcode the decision describes.
	try := func() error {
		s.chooseAccelerator(c)
		spec, err := s.spec(ctx, c)
		if err != nil {
			return err
		}
		return run(spec)
	}
	err := try()
	if !errors.Is(err, transcode.ErrAcceleratorFailed) {
		return err
	}
	// The retry's chooseAccelerator clears the accelerator, so it's read first.
	s.acceleratorFailed(c, c.accelerator(), err)
	c.cpu = true
	return try()
}

// spec builds a play's or a download's transcode from its decision. The
// spec has the parts, the video (copied or encoded on the chosen
// accelerator), and the audio. It may also have a subtitle to burn in, or
// one to carry in the stream or, for a download, in the file. A download
// also carries its other text tracks in the file.
func (s *Service) spec(ctx context.Context, c *choice) (transcode.Spec, error) {
	var spec transcode.Spec
	for i, f := range c.files {
		ref := transcode.FileRef{ID: f.ID, Path: f.Path(), Duration: f.Probe.Duration, Start: f.Probe.Start, Bitrate: f.Probe.Bitrate}
		if c.keyframes != nil {
			ref.Keyframes = c.keyframes[i]
		}
		spec.Parts = append(spec.Parts, ref)
	}
	if t, ok := c.decision.Selected(media.Subtitle); ok && t.Delivery == DeliverBurn {
		ref, fonts, err := s.burnIn(ctx, c, t)
		if err != nil {
			return spec, err
		}
		spec.BurnIn, spec.Fonts = ref, fonts
	}
	spec.Text = s.textTracks(ctx, c)
	if t, ok := c.decision.Selected(media.Video); ok {
		if t.Transcoded() {
			spec.Accelerator = t.Accelerator
		}
		spec.Source, spec.VideoTrack, spec.Video = c.input.Source.Video.Track, t.Index, *t.Video
	}
	if audio, ok := c.decision.Selected(media.Audio); ok {
		out := media.AudioOut{Track: audio.Index, Out: *audio.Audio}
		if src := c.input.Source.Audio; src != nil {
			out.Source, out.SourceChannels, out.Language = src.Codec, src.Channels, src.Language
		}
		spec.Audio = []media.AudioOut{out}
	}
	return spec, nil
}

// started describes a play that started, for its client.
func (s *Service) started(ctx context.Context, pl *play) Started {
	out := Started{
		ID: pl.id, Work: pl.work, Version: pl.version.ID, Start: pl.start, Local: pl.local,
		Quality: pl.request.qualityFor(pl.local), MaxBitrate: pl.input.Limit.MaxBitrate, Cap: pl.cap,
		Bitrate: pl.bitrate(), Decision: pl.decision, ExternalSubtitles: s.externalSubtitles(ctx, pl.files[0], pl.styled, pl.decision.Tracks),
	}
	if len(pl.files) == 1 {
		_, _, out.Previews = s.deps.Previews.Previews(ctx, pl.files[0].ID)
	}
	out.Duration = duration(pl.files)
	for _, f := range pl.files {
		out.Parts = append(out.Parts, f.Probe.Duration)
	}
	out.Chapters, out.Markers = s.deps.Markers.Markers(ctx, pl.work, pl.files)
	return out
}

// duration is a version's length, the sum of its parts' durations.
func duration(parts []catalog.File) time.Duration {
	var total time.Duration
	for _, f := range parts {
		total += f.Probe.Duration
	}
	return total
}

// Play is an active play, for "Now playing", the player's diagnostics and
// the stream limit.
type Play struct {
	ID, Work, User, Version id.ID
	Username                string     // the user's name when the play started
	Client                  string     // the name of the session or API key that started the play
	Address                 netip.Addr // the client's address, resolved through trusted proxies
	Local                   bool
	Device                  auth.Device // the app and device that started it
	Started                 time.Time
	State                   PlayState // what the player is doing, as its last heartbeat said
	Decision                Decision
	// Tracks and Subtitles are the selected source tracks. Tracks are the
	// first part's embedded tracks, and Subtitles holds a subtitle file.
	Tracks    []catalog.Track
	Subtitles []catalog.SubtitleFile
	Transcode transcode.JobInfo // the transcode job, for HLS plays
	Seeks     []time.Duration   // the client's time to the first frame after each recent seek
	Position  time.Duration     // as the last heartbeat reported it
	Duration  time.Duration     // the length of the whole version, all parts included
	Bitrate   int               // the stream's bitrate: a video transcode's target, or else the source's
	Bandwidth int               // the client's measured download rate; zero when the client reports none
}

// Plays lists the active plays, for "Now playing".
func (s *Service) Plays() []Play {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Play, 0, len(s.plays))
	for _, pl := range s.plays {
		out = append(out, pl.infoLocked())
	}
	slices.SortFunc(out, func(a, b Play) int { return a.Started.Compare(b.Started) })
	return out
}

// Play describes one active play, for the player's diagnostics.
func (s *Service) Play(sid id.ID) (Play, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	pl, ok := s.plays[sid]
	if !ok {
		return Play{}, fmt.Errorf("%w: %s", ErrNoPlay, sid)
	}
	return pl.infoLocked(), nil
}

// infoLocked describes the play.
func (pl *play) infoLocked() Play {
	out := Play{
		ID: pl.id, Work: pl.work, User: pl.user, Username: pl.username, Version: pl.version.ID, Client: pl.client,
		Address: pl.request.ClientIP, Local: pl.local, Device: pl.request.Device, Started: pl.started, State: pl.state, Decision: pl.decision,
		Seeks: slices.Clone(pl.seeks), Position: pl.position, Duration: duration(pl.files), Bitrate: pl.bitrate(), Bandwidth: pl.bandwidth,
	}
	if len(pl.files) > 0 {
		for _, d := range pl.decision.Tracks {
			if !d.Selected || d.Track.IsZero() {
				continue
			}
			if i := slices.IndexFunc(pl.files[0].Tracks, func(t catalog.Track) bool { return t.ID == d.Track }); i >= 0 {
				out.Tracks = append(out.Tracks, pl.files[0].Tracks[i])
			} else if i := slices.IndexFunc(pl.files[0].SubtitleFiles, func(t catalog.SubtitleFile) bool { return t.ID == d.Track }); i >= 0 {
				out.Subtitles = append(out.Subtitles, pl.files[0].SubtitleFiles[i])
			}
		}
	}
	if pl.tc != nil {
		out.Transcode = pl.tc.Job()
	}
	return out
}

// bitrate is the play's stream bitrate. For a video transcode it's the
// target plus any transcoded audio. Otherwise it's the source's bitrate.
func (pl *play) bitrate() int {
	v, ok := pl.decision.Selected(media.Video)
	if !ok || !v.Transcoded() {
		return pl.input.Source.Bitrate
	}
	b := v.Video.Bitrate
	if a, ok := pl.decision.Selected(media.Audio); ok && a.Transcoded() {
		b += a.Audio.Bitrate
	}
	return b
}

// SeekTimes returns how many seeks clients have timed since the server
// started, and their total time to the first frame, for metrics.
func (s *Service) SeekTimes() (int, time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.seeks, s.seekTime
}

// logStart logs a play's decision. The play ID is a credential, so it's
// never logged. The work and client identify the play instead.
func (pl *play) logStart(msg string, extra ...any) {
	dev := pl.request.Device
	attrs := []any{"work", pl.work, "version", pl.version.ID, "client", pl.client, "local", pl.local,
		"app", dev.App, "app_version", dev.AppVersion, "platform", dev.Platform, "os_version", dev.OSVersion,
		"device_model", dev.Model,
		"delivery", pl.decision.Delivery, "summary", pl.decision.Summary(), "start", pl.start.Round(time.Second),
		"caps", capsSummary(pl.input.Caps)}
	for _, t := range pl.decision.Tracks {
		if !t.Selected || t.Track.IsZero() {
			continue
		}
		attrs = append(attrs, string(t.Type), string(t.Delivery)+transcodeSuffix(t)+reasonSuffix(t.Reason))
		if t.Type == media.Video && t.Transcoded() {
			attrs = append(attrs, "encoder", t.Encoder+reasonSuffix(t.SoftwareReason))
		}
	}
	if pl.input.Limit.MaxBitrate > 0 {
		attrs = append(attrs, "max_bitrate", pl.input.Limit.MaxBitrate)
	}
	slog.Info(msg, append(attrs, extra...)...)
}

// transcodeSuffix marks an encoded track in the log.
func transcodeSuffix(t TrackDecision) string {
	if t.Transcoded() {
		return " transcode"
	}
	return ""
}

func reasonSuffix(r Reason) string {
	if r == "" {
		return ""
	}
	return " (" + string(r) + ")"
}
