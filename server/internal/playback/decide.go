package playback

import (
	"cmp"
	"slices"
	"strings"

	"github.com/mmcdole/kivo/internal/id"
	"github.com/mmcdole/kivo/internal/media"
)

// Limit is the lowest bitrate cap that applies, and which one it is.
type Limit struct {
	MaxBitrate int    // in bits per second; zero means no cap
	Reason     Reason // clientQualityLimit, remoteQualityLimit or bandwidthLimit
}

// SourceTrack is a video or audio track of the version, with the catalog
// ID clients choose it by.
type SourceTrack struct {
	ID id.ID
	media.Track
}

// Source is the version being decided: its container, its tracks, and
// the ones selected.
type Source struct {
	Container string // as probed, such as "matroska,webm" or "mov,mp4,m4a,3gp,3g2,mj2"
	Ext       string // the first part's extension, lowercase, such as ".mkv"
	Bitrate   int
	Video     *SourceTrack // the video played; nil for audio alone
	Audio     *SourceTrack // the audio track selected
	// DefaultAudio means the selected audio track is the one a plain file
	// plays.
	DefaultAudio bool
	Subtitle     *SubtitleSource // the subtitle track selected; nil means off
	// Tracks are the version's video and audio tracks in the file's order,
	// cover art left out. Subtitles are its subtitle tracks, embedded and
	// then its subtitle files.
	Tracks    []SourceTrack
	Subtitles []SubtitleSource
	Remuxable bool // the container indexes keyframes no more than 20 s apart
}

// SubtitleSource is a subtitle track of the version.
type SubtitleSource struct {
	Track   id.ID       // the catalog track
	Index   int         // an embedded track's stream index; -1 for a subtitle file
	Codec   media.Codec // subrip, ass, webvtt, mov_text, hdmv_pgs_subtitle, dvd_subtitle
	OwnFile bool        // the track is a file next to the video, or one downloaded for it
	// Prepared means an embedded track's extraction is made and, for ASS,
	// every font it uses is ready to serve (the subtitles package's
	// Styled). The client can then draw it from a file. Otherwise a play
	// carries a text track in the stream as plain WebVTT, and burns in a
	// PGS track.
	Prepared bool
}

// ready reports whether the track can be served as a file for the
// target. A download and its subtitles are prepared before the download is
// ready, so its tracks are decided as they will be once prepared.
func (s SubtitleSource) ready(t Target) bool { return s.OwnFile || s.Prepared || t == TargetFile }

// Input is everything Decide looks at.
type Input struct {
	Caps     Capabilities
	Settings Settings
	Limit    Limit
	Source   Source
	// Encoders are the codecs a video transcode may produce. H.264 is
	// always included, and HEVC when an accelerator encodes it.
	Encoders []media.Codec
	// Fallback means the play replaces one whose playback failed. Nothing
	// is played directly or copied. The video and audio are encoded, and a
	// selected subtitle is burned in rather than drawn by the client.
	Fallback bool
	Target   Target
	Quality  DownloadQuality // the quality of a download
	// DialogueBoost means the client asks for dialogue that's easier to
	// follow. The audio is encoded (media.AudioRendition's DialogueBoost)
	// and the video copied. Downloads and plays without video ignore it.
	DialogueBoost bool
}

// Target is what the client is sent: a stream it plays as it arrives, or
// whole files it keeps to play offline.
type Target int

const (
	// TargetStream sends the original file unchanged, or HLS. It's the zero
	// value.
	TargetStream Target = iota
	// TargetFile sends whole files and never HLS. It's used for downloads.
	// The client gets the original, or a conversion to one MP4 file when
	// the original won't play or is larger than Quality's estimate. The
	// conversion copies what it can.
	TargetFile
)

// DownloadQuality is a download's quality: the tallest picture a
// conversion sends, its video's bitrate cap and the quality it aims for,
// and whether its audio is stereo. The zero value is the
// original, which converts only what the client can't play, the same as
// streaming.
type DownloadQuality struct {
	Name     string // original, high, standard
	Height   int
	Bitrate  int // the video's cap, in bits per second
	Fidelity media.Fidelity
	// Stereo means the audio is stereo AAC at the most stereoBitrate. Other
	// downloads keep 5.1 (downloadChannels).
	Stereo bool
}

// DownloadQualities are the qualities a video can be downloaded at, in the
// download menu's order. High is for any screen, and Standard for a phone.
var DownloadQualities = []DownloadQuality{
	{Name: "original"},
	{Name: "high", Height: 1080, Bitrate: 8_000_000},
	{Name: "standard", Height: 1080, Bitrate: 3_000_000, Fidelity: media.FidelityStandard, Stereo: true},
}

// DefaultDownloadQuality is the quality of a download that names none.
const DefaultDownloadQuality = "high"

// stereoBitrate is the most bitrate a Standard download's audio keeps
// rather than encodes, the most Apple's HLS spec gives stereo AAC.
const stereoBitrate = 160_000

// Migration (downloads): remove after every client has updated past this release.

// legacyQualities are the download qualities an earlier release offered,
// each by the quality it is now. A download asked for at one, by an
// earlier app, keeps working: 1080p-high and 1080p were 1080p, which High
// is; 720p and 480p were for saving space, which Standard is.
var legacyQualities = map[string]string{"1080p-high": "high", "1080p": "high", "720p": "standard", "480p": "standard"}

// DownloadQualityNamed returns a download quality by its name.
func DownloadQualityNamed(name string) (DownloadQuality, bool) {
	// Migration (downloads): remove after every client has updated past this release.
	name = cmp.Or(legacyQualities[name], name)
	i := slices.IndexFunc(DownloadQualities, func(q DownloadQuality) bool { return q.Name == name })
	if i < 0 {
		return DownloadQuality{}, false
	}
	return DownloadQualities[i], true
}

// downloadChannels is the fewest channels a download's audio may keep. The
// client mixes 5.1 down itself, and may later play the download on other
// speakers than the ones it reports now.
const downloadChannels = 6

// Delivery is how a play or one of its tracks reaches the client.
type Delivery string

const (
	DeliverFile Delivery = "file" // the original, or a track inside it
	// DeliverStream is HLS, or a track inside it. A subtitle in the stream
	// is a WebVTT rendition, which the play's job reads along with the
	// video or the stream reads from a file. For a download, it's a track
	// inside the conversion's file.
	DeliverStream     Delivery = "stream"
	DeliverExternal   Delivery = "external"   // a subtitle sent to the client as its own file
	DeliverBurn       Delivery = "burn"       // a subtitle rendered into the video
	DeliverNone       Delivery = "none"       // the subtitle choice off
	DeliverConversion Delivery = "conversion" // a download's MP4, once it's converted
)

// Method is how a play is shown: Direct Play, Direct Stream or Transcode.
// It's derived from the video, or without video from the audio.
type Method string

const (
	DirectPlay   Method = "directPlay"
	DirectStream Method = "directStream" // copied into a new container
	Transcode    Method = "transcode"
)

// TrackDecision is how one choice of a play shows: a track of the
// version, or the subtitle choice off. A selected track has a delivery,
// and a reason whenever it isn't delivered unchanged in the file. Any
// other choice has a delivery when the client can show it without a new
// play.
type TrackDecision struct {
	Type     media.TrackKind
	Track    id.ID // the catalog track, which clients identify it by; zero for off
	Index    int   // an embedded track's stream index; -1 for a subtitle file and for off
	Selected bool
	Delivery Delivery    // empty means choosing it starts a new play
	Codec    media.Codec // as delivered
	HDR      string      // a video's HDR as delivered: hdr10, hlg or dolbyVision; empty for SDR
	Reason   Reason
	Unmet    *Unmet // the capability that failed, when one did
	Video    *media.VideoRendition
	Audio    *media.AudioRendition
	// The next three fields apply to a video transcode. Accelerator is the
	// accelerator encoding it, empty for the CPU. Encoder is the encoder's
	// name. SoftwareReason says why the CPU is used when the server has an
	// accelerator. The service fills these in, because Decide doesn't
	// choose accelerators.
	Accelerator    media.Accelerator
	Encoder        string
	SoftwareReason Reason
}

// Transcoded reports whether a video or audio track is encoded rather
// than copied or played as it is.
func (t TrackDecision) Transcoded() bool {
	return t.Video != nil && t.Video.Codec != media.Copy || t.Audio != nil && t.Audio.Codec != media.Copy
}

// Method is the track's rung of the ladder: encoded, copied into a new
// container (a stream, or a download's conversion), or the file as it is.
func (t TrackDecision) Method() Method {
	switch {
	case t.Transcoded():
		return Transcode
	case t.Video != nil || t.Audio != nil:
		return DirectStream
	}
	return DirectPlay
}

// Decision is the whole play's plan.
type Decision struct {
	Delivery Delivery // file, stream, or for a download conversion
	// Tracks are every choice: the version's video, audio and subtitle
	// tracks in order, then subtitles off.
	Tracks   []TrackDecision
	Playable bool
	Refusal  Refusal // why the play isn't playable, when it isn't
	Unmet    *Unmet  // for RefuseFileUnplayable, the capability the file failed
	// DialogueBoost is the play's dialogue boost. It's nil when the play
	// can't boost its dialogue.
	DialogueBoost *DialogueBoost
}

// Selected returns the selected track of a kind, if there is one. The
// subtitle choice off isn't a selected subtitle.
func (d Decision) Selected(kind media.TrackKind) (TrackDecision, bool) {
	for _, t := range d.Tracks {
		if t.Type == kind && t.Selected && !t.Track.IsZero() {
			return t, true
		}
	}
	return TrackDecision{}, false
}

// Summary is the play's method as clients show it. It's the video's, so
// copied video with transcoded audio shows as Direct Stream. Without video,
// it's the audio's.
func (d Decision) Summary() Method {
	if v, ok := d.Selected(media.Video); ok {
		return v.Method()
	}
	if a, ok := d.Selected(media.Audio); ok {
		return a.Method()
	}
	return DirectPlay
}

// burned reports whether the play burns in its subtitle.
func (d Decision) burned() bool {
	s, ok := d.Selected(media.Subtitle)
	return ok && s.Delivery == DeliverBurn
}

// The codecs that HLS with fragmented MP4, and a download's MP4, can
// carry. DTS and TrueHD can't go into Apple HLS, and PGS and ASS can't go
// inside HLS at all, so those are delivered external or transcoded.
var (
	hlsVideo = []media.Codec{media.H264, media.HEVC, "av1", "vp9"}
	hlsAudio = []media.Codec{media.AAC, "ac3", "eac3", "mp3", "opus", "flac"}
)

// The subtitle codecs the server delivers, as ffmpeg names them.
const (
	webVTT   media.Codec = "webvtt"
	pgsCodec media.Codec = "hdmv_pgs_subtitle"
)

// Decide picks a delivery for each track, choosing the first rung that
// works: direct play, then direct stream (a remux, or copied video with
// transcoded audio), then transcode. A source with no video, such as a
// book, can only be played directly. For a download, direct stream and
// transcode produce a conversion to MP4 rather than HLS. It then says how
// each other choice would show during the play. For a play that can boost
// its dialogue, the decision says what the boost changes. Decide is a pure
// function, with no I/O and no clock.
func Decide(in Input) Decision {
	d := decide(in)
	if d.Playable && in.boostable() {
		d.DialogueBoost = dialogueBoost(in, d)
	}
	return d
}

func decide(in Input) Decision {
	if in.Target == TargetFile {
		in.Caps = downloadCaps(in.Caps, in.stereo())
	}
	src := in.Source
	if src.Video == nil && src.Audio == nil {
		return Decision{Refusal: RefuseNoTracks}
	}

	// Each track is judged twice: once as the original file plays it, and
	// once as the play's target (its stream, or a download's conversion)
	// carries it.
	fileVideo, targetVideo := holds, holds
	if src.Video != nil {
		fileVideo, targetVideo = in.judgeVideo(DeliverFile, containerNames(src)), in.judgeVideo(in.targetDelivery(), in.targetContainers())
		if in.Limit.MaxBitrate > 0 && src.Bitrate > in.Limit.MaxBitrate {
			fileVideo.refuse(in.Limit.Reason)
			targetVideo.refuse(in.Limit.Reason)
		}
		// Don't waste space. The original is sent unchanged only when it's
		// no larger than the quality's conversion would be.
		if q := in.Quality; in.Target == TargetFile && q.Bitrate > 0 && src.Bitrate > q.Bitrate+in.audioBitrate() {
			fileVideo.refuse(ReasonDownloadQuality)
			targetVideo.refuse(ReasonDownloadQuality)
		}
	}
	fileAudio, targetAudio := holds, holds
	if src.Audio != nil {
		fileAudio, targetAudio = judgeAudio(in.Caps.Audio, src.Audio.Track, DeliverFile, containerNames(src)), judgeAudio(in.Caps.Audio, src.Audio.Track, in.targetDelivery(), in.targetContainers())
		if in.boosts() {
			fileAudio.refuse(ReasonDialogueBoost)
			targetAudio.refuse(ReasonDialogueBoost)
		}
		if in.stereo() && !stereoAAC(src.Audio.Track) {
			fileAudio.refuse(ReasonDownloadQuality)
			targetAudio.refuse(ReasonDownloadQuality)
		}
	}
	fileSub, targetSub := in.decideSubtitle()
	if fileSub.Delivery == DeliverBurn {
		fileVideo = verdict{why: fileSub.Reason, unmet: fileSub.Unmet}
	}
	if targetSub.Delivery == DeliverBurn {
		targetVideo = verdict{why: targetSub.Reason}
	}
	if in.Fallback {
		// A track that would have played unchanged, from the file or
		// copied, gets the fallback's reason. A track that wouldn't have
		// keeps its own reason.
		for _, pair := range [][2]*verdict{{&fileVideo, &targetVideo}, {&fileAudio, &targetAudio}} {
			if pair[0].ok || pair[1].ok {
				*pair[0], *pair[1] = verdict{why: ReasonFallback}, verdict{why: ReasonFallback}
			}
		}
	}

	direct := in.directPlayable(fileVideo, fileAudio, fileSub)
	var d Decision
	var sub TrackDecision
	if direct.ok {
		d, sub = Decision{Delivery: DeliverFile, Playable: true}, fileSub
		if src.Video != nil {
			d.Tracks = append(d.Tracks, TrackDecision{Type: media.Video, Track: src.Video.ID, Index: src.Video.Index, Selected: true,
				Delivery: DeliverFile, Codec: src.Video.Codec, HDR: in.deliveredHDR(src.Video.Track, DeliverFile, nil)})
		}
		if src.Audio != nil {
			d.Tracks = append(d.Tracks, TrackDecision{Type: media.Audio, Track: src.Audio.ID, Index: src.Audio.Index, Selected: true,
				Delivery: DeliverFile, Codec: src.Audio.Codec})
		}
	} else {
		// The transcoder only makes video streams and files, so audio alone
		// (a book) plays directly or not at all. A stream is HLS, and a
		// download's conversion is MP4.
		if src.Video == nil || !in.hasTarget() {
			return Decision{Refusal: RefuseFileUnplayable, Unmet: direct.unmet}
		}
		d, sub = Decision{Delivery: DeliverStream, Playable: true}, targetSub
		if in.Target == TargetFile {
			d.Delivery = DeliverConversion
		}
		d.Tracks = append(d.Tracks, in.decideVideo(targetVideo, direct))
		if src.Audio != nil {
			d.Tracks = append(d.Tracks, in.decideAudio(targetAudio, direct))
		}
	}
	return in.withOthers(d, sub)
}

// targetDelivery is how a play that isn't the original file carries its
// tracks: a stream, or a download's conversion, which is an MP4 file.
func (in Input) targetDelivery() Delivery {
	if in.Target == TargetFile {
		return DeliverFile
	}
	return DeliverStream
}

// targetContainers are the containers of a play that isn't the original
// file: none for a stream, and MP4 for a download's conversion.
func (in Input) targetContainers() []string {
	if in.Target == TargetFile {
		return []string{"mp4"}
	}
	return nil
}

// hasTarget reports whether the client plays what a play that isn't the
// original file makes: a stream, or a download's MP4.
func (in Input) hasTarget() bool {
	if in.Target == TargetFile {
		return in.Caps.File != nil && slices.Contains(in.Caps.File.Containers, "mp4")
	}
	return in.Caps.streams()
}

// judgeVideo judges the selected video delivered as d in containers.
func (in Input) judgeVideo(d Delivery, containers []string) verdict {
	v := in.Source.Video
	rate := v.Bitrate
	if rate == 0 {
		rate = in.Source.Bitrate
	}
	return judgeVideo(in.Caps.Video, videoCase{track: v.Track, delivery: d, containers: containers, bitrate: rate})
}

// directPlayable decides whether the original file can be played
// unchanged. If not, it returns the verdict that sends the play to HLS.
func (in Input) directPlayable(video, audio verdict, sub TrackDecision) verdict {
	switch {
	case in.Fallback:
		return verdict{why: ReasonFallback}
	case in.Settings.DisableDirectPlay:
		return verdict{why: ReasonDirectPlayDisabled}
	case !video.ok:
		return video
	case !audio.ok:
		return audio
	}
	if names := containerNames(in.Source); in.Caps.File == nil || !overlaps(in.Caps.File.Containers, names) {
		var declared []string
		if in.Caps.File != nil {
			declared = in.Caps.File.Containers
		}
		return failed(ReasonContainerUnsupported, "containers", list(declared), list(names))
	}
	switch {
	case in.Source.Audio != nil && !in.Source.DefaultAudio && !in.Caps.File.TrackSelection:
		return failed(ReasonTrackSelectionUnsupported, "trackSelection", "false", "true")
	case sub.Delivery == DeliverBurn, sub.Delivery == DeliverStream:
		return verdict{why: sub.Reason, unmet: sub.Unmet} // an unprepared subtitle goes in the stream, so a remux carries it
	}
	return holds
}

func (in Input) decideVideo(judged, direct verdict) TrackDecision {
	v := in.Source.Video
	t := TrackDecision{Type: media.Video, Track: v.ID, Index: v.Index, Selected: true, Delivery: DeliverStream}
	switch {
	case judged.ok && !in.Source.Remuxable && in.Target == TargetStream: // a conversion copies the whole file, so it needs no segments
		t.Reason = ReasonKeyframeIndexMissing
	case judged.ok && slices.Contains(hlsVideo, v.Codec):
		t.Reason, t.Unmet = direct.why, direct.unmet
		t.Video = &media.VideoRendition{Codec: media.Copy}
		if v.DolbyVisionProfile == 7 {
			t.Video.HDR = media.StripDolbyVision // falls back to HDR10, because profile 7 can't be rewritten to 8.1
		}
		t.Codec, t.HDR = v.Codec, in.deliveredHDR(v.Track, DeliverStream, t.Video)
		return t
	case judged.ok:
		t.Reason, t.Unmet = ReasonStreamCodecUnsupported, &Unmet{Field: "codec", Actual: string(v.Codec)}
	default:
		t.Reason, t.Unmet = judged.why, judged.unmet
	}
	t.Video = in.videoTarget(v.Track)
	t.Codec = t.Video.Codec
	return t
}

// deliveredHDR is how a video's HDR reaches the client: Dolby Vision when
// the client shows its profile, else its base layer's range. Profile 7
// copied into a stream loses its Dolby Vision, and an encoded video is SDR.
func (in Input) deliveredHDR(v media.Track, d Delivery, r *media.VideoRendition) string {
	switch {
	case r != nil && r.Codec != media.Copy:
		return ""
	case v.DolbyVisionProfile > 0 && (r == nil || r.HDR != media.StripDolbyVision) && slices.ContainsFunc(in.Caps.Video, func(e VideoEntry) bool {
		return isFor(e, v.Codec, d) && slices.ContainsFunc(e.DolbyVision, func(dv DolbyVisionEntry) bool { return dv.shows(v) })
	}):
		return "dolbyVision"
	}
	base := v.Range
	if v.DolbyVisionProfile > 0 {
		base, _ = dolbyVisionBase(v)
	}
	if base == media.HDR10 || base == media.HLG {
		return string(base)
	}
	return ""
}

// maxTranscodeHeight bounds every video transcode. 4K encodes cost more
// than they're worth on household hardware, and a client that plays 4K
// usually plays the file directly.
const maxTranscodeHeight = 1080

// videoTarget is the rendition a video transcode makes. It's 8-bit SDR
// H.264, or HEVC when the client plays HEVC and an accelerator encodes it.
// The picture is scaled down to fit the client and the bitrate, and is at
// most 1080p.
func (in Input) videoTarget(v media.Track) *media.VideoRendition {
	r := &media.VideoRendition{Codec: media.H264, Profile: "high", BitDepth: 8}
	if slices.Contains(in.Encoders, media.HEVC) && in.playsHEVC() {
		r.Codec, r.Profile = media.HEVC, "main"
	}
	w, h := v.Width, v.Height
	if w == 0 || h == 0 {
		w, h = 1920, 1080 // the size is unknown, so media keeps the source's size within this bound
	}
	maxW, maxH := in.sizeLimit(r.Codec)
	bound := min(maxTranscodeHeight, heightFor(in.Limit.MaxBitrate))
	if maxH > 0 {
		bound = min(bound, maxH)
	}
	if qh := in.Quality.Height; qh > 0 {
		bound = min(bound, qh)
	}
	if h > bound {
		w, h = w*bound/h, bound
	}
	if maxW > 0 && w > maxW {
		w, h = maxW, h*maxW/w
	}
	r.Width, r.Height = w&^1, h&^1
	r.Bitrate = bitrateFor(h, in.Source.Bitrate)
	if mb := in.bitrateLimit(r.Codec); mb > 0 {
		r.Bitrate = min(r.Bitrate, mb)
	}
	// A cap is for the whole stream, so the audio's share comes out of it
	// first, as Jellyfin budgets it. A download's quality is its video's.
	if cap := in.Limit.MaxBitrate; cap > 0 {
		r.Bitrate = min(r.Bitrate, max(cap-in.audioBitrate(), minVideoBitrate))
	}
	if qb := in.Quality.Bitrate; qb > 0 {
		r.Bitrate = min(r.Bitrate, qb)
	}
	r.Fidelity = in.Quality.Fidelity
	if v.HDR() {
		r.HDR = media.ToneMap // the 8-bit output is SDR
	}
	return r
}

// minVideoBitrate is the least a video transcode gets, however little of a
// cap the audio leaves.
const minVideoBitrate = 64_000

// bitrateLimit is the most bitrate the client decodes of a codec as the
// target delivers it: the largest any of its entries allows. Zero means no
// limit.
func (in Input) bitrateLimit(codec media.Codec) int {
	var bs []int
	for _, e := range in.Caps.Video {
		if isFor(e, codec, in.targetDelivery()) {
			bs = append(bs, e.MaxBitrate)
		}
	}
	return largest(bs)
}

// sizeLimit is the largest picture the client decodes of a codec as the
// target delivers it: the largest any of its entries allows. Zero means no
// limit.
func (in Input) sizeLimit(codec media.Codec) (w, h int) {
	var ws, hs []int
	for _, e := range in.Caps.Video {
		if isFor(e, codec, in.targetDelivery()) {
			ws, hs = append(ws, e.MaxWidth), append(hs, e.MaxHeight)
		}
	}
	return largest(ws), largest(hs)
}

// largest is the largest of limits, or zero, meaning none, when there are
// none or one of them is zero.
func largest(limits []int) int {
	if len(limits) == 0 || slices.Contains(limits, 0) {
		return 0
	}
	return slices.Max(limits)
}

// Rung is one of the quality menu's fixed qualities: a bitrate cap, and
// the height of a transcode at that cap.
type Rung struct {
	Bitrate int
	Height  int
}

// Ladder is the quality menu's fixed ladder, highest first. Every client
// shows these rungs.
func Ladder() []Rung {
	var out []Rung
	for _, b := range []int{20_000_000, 12_000_000, 8_000_000, 4_000_000, 2_000_000, 1_500_000, 720_000} {
		out = append(out, Rung{Bitrate: b, Height: heightFor(b)})
	}
	return out
}

// heightFor is the tallest picture worth sending at a bitrate cap, as the
// quality menu shows it, by Plex's ladder: 8 Mbps and up gets 1080p, 2 to
// 4 Mbps gets 720p, and 1.5 Mbps gets 480p. A zero cap means no cap.
func heightFor(bitrate int) int {
	switch {
	case bitrate == 0 || bitrate >= 8_000_000:
		return 1080
	case bitrate >= 2_000_000:
		return 720
	case bitrate >= 1_500_000:
		return 480
	}
	return 360
}

// bitrateFor is an uncapped transcode's bitrate. It's the top quality for
// the picture's height, but no more than 1.5 times the source's bitrate,
// since a re-encode can't add detail.
func bitrateFor(height, source int) int {
	b := 4_000_000
	switch {
	case height > 720:
		b = 20_000_000
	case height > 480:
		b = 8_000_000
	}
	if source > 0 {
		b = min(b, max(source*3/2, 4_000_000))
	}
	return b
}

// playsHEVC reports whether the client decodes 8-bit HEVC Main as the
// target delivers it.
func (in Input) playsHEVC() bool {
	return judgeVideo(in.Caps.Video, videoCase{
		track:    media.Track{Codec: media.HEVC, Profile: "Main", BitDepth: 8},
		delivery: in.targetDelivery(), containers: in.targetContainers(),
	}).ok
}

func (in Input) decideAudio(judged, direct verdict) TrackDecision {
	a := in.Source.Audio
	t := TrackDecision{Type: media.Audio, Track: a.ID, Index: a.Index, Selected: true, Delivery: DeliverStream}
	if judged.ok && slices.Contains(hlsAudio, a.Codec) {
		t.Reason, t.Unmet, t.Codec = direct.why, direct.unmet, a.Codec
		t.Audio = &media.AudioRendition{Codec: media.Copy}
		return t
	}
	t.Reason, t.Unmet = judged.why, judged.unmet
	if judged.ok {
		t.Reason, t.Unmet = ReasonStreamCodecUnsupported, &Unmet{Field: "codec", Actual: string(a.Codec)} // the codec is playable, but not inside HLS
	}
	t.Audio = in.audioTarget(a.Track)
	t.Codec = t.Audio.Codec
	return t
}

// audioTarget is the rendition an audio transcode makes. It's AAC, or
// Dolby Digital Plus, else Dolby Digital, when the client plays more of the
// source's channels in that, as a TV passing Dolby to a soundbar does, and
// as Plex sends it 5.1. It keeps no more than the source's channels, and
// at most 5.1. The media package mixes surround down to stereo in a way
// that keeps dialogue clear.
func (in Input) audioTarget(a media.Track) *media.AudioRendition {
	codec, ch := media.AAC, in.aacChannels(a)
	for _, dolby := range []media.Codec{media.EAC3, media.AC3} {
		if n := min(sourceChannels(a), 6, maxChannels(in.Caps.Audio, dolby, in.targetDelivery())); n > ch && !in.stereo() {
			codec, ch = dolby, n
			break
		}
	}
	return &media.AudioRendition{Codec: codec, Channels: ch, Bitrate: audioBitrateOf(codec, ch), SampleRate: 48_000,
		DialogueBoost: in.boosts()}
}

// subtitleCaps are what a client shows of one subtitle track.
type subtitleCaps struct {
	inFile       bool    // read from the played file
	fromFile     verdict // whether it's read from the played file, and why not
	stream       bool    // as a WebVTT rendition of the stream
	externalText bool    // a text track converted to a WebVTT file
	externalASS  bool    // an ASS file, drawn styled
	externalPGS  bool    // a PGS file
}

func (in Input) subtitleCaps(s SubtitleSource) subtitleCaps {
	c := subtitleCaps{
		fromFile:     verdict{why: ReasonSubtitleCodecUnsupported},
		stream:       in.Target == TargetStream && in.Caps.shows(webVTT, DeliverStream),
		externalText: in.Caps.drawsTextFiles(),
		externalASS:  in.Caps.shows("ass", DeliverExternal) || in.Caps.shows("ssa", DeliverExternal),
		externalPGS:  in.Caps.shows(pgsCodec, DeliverExternal),
	}
	if !s.OwnFile {
		c.fromFile = in.Caps.readsFromFile(s.Codec, containerNames(in.Source))
		c.inFile = c.fromFile.ok
	}
	return c
}

// unread is why a play of the file doesn't show the track from the file:
// the container, when the client reads its codec from other containers,
// and otherwise its codec.
func (c subtitleCaps) unread() (Reason, *Unmet) {
	if c.fromFile.why == ReasonContainerUnsupported {
		return c.fromFile.why, c.fromFile.unmet
	}
	return ReasonSubtitleCodecUnsupported, nil
}

// drawsTextFiles reports whether a client draws text tracks as WebVTT
// files of their own.
func (c Capabilities) drawsTextFiles() bool { return c.shows(webVTT, DeliverExternal) }

// decideSubtitle decides the selected subtitle track twice: as a play of
// the original file shows it, and as a stream or a download's conversion
// does. A delivery of stream or burn for the file means the file can't
// play. The burn subtitles setting and a fallback burn the track in,
// though a text track that isn't extracted goes in the stream as plain
// WebVTT until it is.
func (in Input) decideSubtitle() (forFile, forTarget TrackDecision) {
	s := in.Source.Subtitle
	if s == nil {
		return TrackDecision{}, TrackDecision{}
	}
	base := TrackDecision{Type: media.Subtitle, Track: s.Track, Index: s.Index, Selected: true}
	switch {
	case (in.Settings.BurnSubtitles || in.Fallback) && formatOf(s.Codec).Text() && !s.ready(in.Target):
		base.Delivery, base.Codec, base.Reason = DeliverStream, webVTT, ReasonSubtitlePreparing
		return base, base
	case in.Settings.BurnSubtitles:
		base.Delivery, base.Reason = DeliverBurn, ReasonBurnSubtitlesEnabled
		return base, base
	case in.Fallback:
		base.Delivery, base.Reason = DeliverBurn, ReasonFallback
		return base, base
	}
	target := DeliverStream
	if in.Target == TargetFile {
		target = DeliverConversion
	}
	forFile = in.subtitle(DeliverFile, *s, true)
	if forFile.Delivery == DeliverStream && in.Target == TargetStream && !in.Caps.streams() {
		forFile.Delivery = DeliverExternal // with no stream to carry it, the extraction is served as a file once it's made
		if in.Caps.drawsTextFiles() {
			forFile.Codec = webVTT
		}
	}
	return forFile, in.subtitle(target, *s, true)
}

// subtitle decides a subtitle track for a play delivered as play: the
// original file, a stream or a download's conversion. A track shows
// without a new play when the client reads it from the played file, draws
// it as a ready file of its own, or takes it as a WebVTT rendition of the
// stream. A styled ASS track is drawn by a client that draws ASS and
// burned in for one that doesn't, and is plain WebVTT in the stream while
// it's prepared. A PGS track is its own file for a client that draws it.
//
// The selected track must show, so where another track would take a new
// play, it's burned in, or carried as plain WebVTT while it's prepared or
// when the client can't draw it from a file. Another track says why it
// takes a new play.
func (in Input) subtitle(play Delivery, s SubtitleSource, selected bool) TrackDecision {
	t := TrackDecision{Type: media.Subtitle, Track: s.Track, Index: s.Index, Selected: selected}
	c, ready, f := in.subtitleCaps(s), s.ready(in.Target), formatOf(s.Codec)
	why, unmet := ReasonSubtitleCodecUnsupported, (*Unmet)(nil)
	if play == DeliverFile {
		why, unmet = c.unread()
	}
	deliver := func(d Delivery, codec media.Codec, r Reason) TrackDecision {
		t.Delivery, t.Codec, t.Reason = d, codec, r
		return t
	}
	// newPlay is a track that can't show in this play: the selected one is
	// burned in, and another says why.
	newPlay := func(r Reason, u *Unmet) TrackDecision {
		if selected {
			t.Delivery = DeliverBurn
		}
		t.Reason, t.Unmet = r, u
		return t
	}
	switch {
	case play == DeliverFile && c.inFile:
		return deliver(DeliverFile, s.Codec, "")
	case f == media.ASS && c.externalASS && ready:
		return deliver(DeliverExternal, codecOf(media.ASS), "")
	case f == media.ASS && !s.OwnFile && ready:
		return newPlay(why, unmet) // styled, on a client that doesn't draw it
	case f == media.ASS && !s.OwnFile && selected:
		return deliver(DeliverStream, webVTT, ReasonSubtitlePreparing)
	case f.Text():
		return in.textSubtitle(t, play, c, ready, why, unmet)
	case f == media.PGS && c.externalPGS && ready:
		return deliver(DeliverExternal, pgsCodec, "")
	case f == media.PGS && c.externalPGS:
		return newPlay(ReasonSubtitlePreparing, nil)
	}
	return newPlay(why, unmet)
}

// textSubtitle decides a plain text track, or an ASS track a play can't
// keep styled: a WebVTT rendition of a stream, or a plain text file of its
// own once it's ready. The selected track can always be carried as plain
// WebVTT, except in a stream that's ready to burn it in.
func (in Input) textSubtitle(t TrackDecision, play Delivery, c subtitleCaps, ready bool, why Reason, unmet *Unmet) TrackDecision {
	deliver := func(d Delivery, codec media.Codec, r Reason) TrackDecision {
		t.Delivery, t.Codec, t.Reason = d, codec, r
		return t
	}
	switch {
	case play == DeliverStream && c.stream:
		return deliver(DeliverStream, webVTT, "")
	case c.externalText && (ready || t.Selected && play != DeliverFile):
		return deliver(DeliverExternal, webVTT, "")
	case !t.Selected && !ready:
		t.Reason = ReasonSubtitlePreparing
		return t
	case !t.Selected:
		return t
	case play == DeliverFile && !c.externalText:
		t = deliver(DeliverStream, webVTT, why)
		t.Unmet = unmet
		return t
	case play == DeliverFile || !ready:
		return deliver(DeliverStream, webVTT, ReasonSubtitlePreparing)
	case play == DeliverConversion:
		return deliver(DeliverStream, webVTT, ReasonSubtitleCodecUnsupported) // carried as a text track of the download's file
	}
	return deliver(DeliverBurn, "", ReasonSubtitleCodecUnsupported)
}

// withOthers completes a decision with every choice of the version. The
// selected tracks are as decided, and sub is the selected subtitle as the
// play shows it. Any other choice shows without a new play when the play
// already carries it, or the client reads it from the file or as a ready
// file of its own. While a subtitle is burned in, every other subtitle choice,
// off included, needs a new play, and so does every other subtitle track
// while the burn subtitles setting or a fallback would burn it in.
func (in Input) withOthers(d Decision, sub TrackDecision) Decision {
	selected := d.Tracks
	d.Tracks = nil
	for _, t := range in.Source.Tracks {
		if i := slices.IndexFunc(selected, func(s TrackDecision) bool { return s.Track == t.ID }); i >= 0 {
			d.Tracks = append(d.Tracks, selected[i])
			continue
		}
		other := TrackDecision{Type: t.Kind, Track: t.ID, Index: t.Index}
		if t.Kind == media.Audio {
			other = in.otherAudio(d.Delivery, t)
		}
		d.Tracks = append(d.Tracks, other)
	}
	burned := sub.Delivery == DeliverBurn
	for _, s := range in.Source.Subtitles {
		if sub.Selected && s.Track == sub.Track {
			d.Tracks = append(d.Tracks, sub)
			continue
		}
		t := TrackDecision{Type: media.Subtitle, Track: s.Track, Index: s.Index}
		switch {
		case in.Settings.BurnSubtitles:
			t.Reason = ReasonBurnSubtitlesEnabled // a new play burns it in
		case in.Fallback:
			t.Reason = ReasonFallback
		case !burned:
			t = in.subtitle(d.Delivery, s, false)
		}
		d.Tracks = append(d.Tracks, t)
	}
	off := TrackDecision{Type: media.Subtitle, Index: -1, Selected: in.Source.Subtitle == nil}
	if !burned {
		off.Delivery = DeliverNone
	}
	d.Tracks = append(d.Tracks, off)
	return d
}

// otherAudio says how an audio track other than the selected one shows. A
// client that selects tracks inside a file plays it there, during a play
// of the file, when it would play the track from the file. Anything else
// needs a new play.
func (in Input) otherAudio(play Delivery, a SourceTrack) TrackDecision {
	t := TrackDecision{Type: media.Audio, Track: a.ID, Index: a.Index}
	if play != DeliverFile || in.Target != TargetStream {
		return t
	}
	if !in.Caps.File.TrackSelection {
		t.Reason = ReasonTrackSelectionUnsupported
		return t
	}
	v := judgeAudio(in.Caps.Audio, a.Track, DeliverFile, containerNames(in.Source))
	if in.DialogueBoost {
		v.refuse(ReasonDialogueBoost)
	}
	if !v.ok {
		t.Reason, t.Unmet = v.why, v.unmet
		return t
	}
	t.Delivery, t.Codec = DeliverFile, a.Codec
	return t
}

// codecOf is the codec a subtitle format is delivered as.
func codecOf(f media.SubtitleFormat) media.Codec {
	switch f {
	case media.ASS:
		return "ass"
	case media.PGS:
		return pgsCodec
	}
	return webVTT
}

// downloadCaps are a client's capabilities for a download. Unless the
// download's audio is stereo, every audio entry for a file is raised to at
// least downloadChannels.
func downloadCaps(caps Capabilities, stereo bool) Capabilities {
	if stereo {
		return caps
	}
	caps.Audio = slices.Clone(caps.Audio)
	for i, a := range caps.Audio {
		if slices.Contains(a.Delivery, DeliverFile) {
			caps.Audio[i].MaxChannels = max(a.MaxChannels, downloadChannels)
		}
	}
	return caps
}

// stereo reports whether the audio a download sends is stereo AAC.
func (in Input) stereo() bool {
	return in.Target == TargetFile && in.Quality.Stereo && in.Source.Video != nil
}

// stereoAAC reports whether a track is already the audio a stereo download
// sends: AAC, in no more than two channels, at no more than stereoBitrate.
func stereoAAC(a media.Track) bool {
	return a.Codec == media.AAC && sourceChannels(a) <= 2 && a.Bitrate <= stereoBitrate
}

// audioBitrate is the bitrate of the audio a stream or a conversion sends,
// which comes out of a cap and sizes a download. It's the track's own
// bitrate when the track is copied and the container records one.
// Otherwise it's the bitrate of its transcode.
func (in Input) audioBitrate() int {
	a := in.Source.Audio
	if a == nil {
		return 0
	}
	if judgeAudio(in.Caps.Audio, a.Track, in.targetDelivery(), in.targetContainers()).ok && a.Bitrate > 0 && slices.Contains(hlsAudio, a.Codec) {
		return a.Bitrate
	}
	return in.audioTarget(a.Track).Bitrate
}

// aacChannels is how many channels a track transcoded to AAC keeps. It's
// never more than the source has, so mono and stereo stay as they are.
// It's at most 5.1, and no more than the client plays as AAC as the target
// delivers it. The client caps that number at what its audio output
// accepts.
func (in Input) aacChannels(a media.Track) int {
	n := min(sourceChannels(a), 6, maxChannels(in.Caps.Audio, media.AAC, in.targetDelivery()))
	if in.stereo() {
		return min(n, 2)
	}
	return n
}

// sourceChannels is a track's channels, taken as stereo when unknown.
func sourceChannels(a media.Track) int {
	if a.Channels == 0 {
		return 2
	}
	return a.Channels
}

// audioBitrateOf is the bitrate of an audio transcode: 64 kbps a channel
// for AAC, and Jellyfin's 384 kbps for stereo and 640 kbps for surround in
// Dolby Digital and Dolby Digital Plus.
func audioBitrateOf(codec media.Codec, channels int) int {
	switch {
	case codec == media.AAC:
		return 64_000 * channels
	case channels > 2:
		return 640_000
	}
	return 384_000
}

// containerNames are the names a client lists the source's container by.
// ffprobe reports MKV and WebM the same way ("matroska,webm"), so a
// Matroska file counts as WebM only when its codecs are ones WebM allows.
// MP4, MOV and M4A are one family, which every MP4 player plays, and
// MPEG-TS is ts, or m2ts for a Blu-ray file.
func containerNames(src Source) []string {
	var names []string
	for _, n := range strings.Split(src.Container, ",") {
		switch n {
		case "matroska":
			names = append(names, "mkv")
		case "webm":
			if webmCodecs(src) {
				names = append(names, "webm")
			}
		case "mov":
			names = append(names, "mp4")
			if src.Ext == ".mov" {
				names = append(names, "mov")
			}
		case "m4a":
			if src.Ext == ".m4a" || src.Ext == ".m4b" {
				names = append(names, "m4a")
			}
		case "mpegts":
			if src.Ext == ".m2ts" || src.Ext == ".mts" {
				names = append(names, "m2ts")
			} else {
				names = append(names, "ts")
			}
		case "3gp", "3g2", "mj2":
		default:
			names = append(names, n)
		}
	}
	slices.Sort(names)
	return slices.Compact(names)
}

func webmCodecs(src Source) bool {
	if src.Video != nil && !slices.Contains([]string{"vp8", "vp9", "av1"}, string(src.Video.Codec)) {
		return false
	}
	return src.Audio == nil || slices.Contains([]string{"opus", "vorbis"}, string(src.Audio.Codec))
}
