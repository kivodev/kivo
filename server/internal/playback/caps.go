package playback

import (
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"

	"github.com/mmcdole/kivo/internal/media"
)

// Capabilities are what a client plays. The client measures them where it
// runs and corrects them for its platform's known misreports. They're
// facts, never choices: the user's choices are Settings. Clients send them
// with every start, and nothing is stored per client.
//
// Each list holds entries. A track plays when any entry for its delivery
// matches it, and an entry matches when every field it sets holds. An
// unset field is no limit.
type Capabilities struct {
	File      *FileCaps // nil means the client plays no file
	Video     []VideoEntry
	Audio     []AudioEntry
	Subtitles []SubtitleEntry
}

// FileCaps say how a client plays a whole file.
type FileCaps struct {
	Containers []string // mkv, mp4, webm, ts, m2ts, mov, m4a, mp3, flac, ogg
	// TrackSelection means the client plays any audio track inside a file,
	// not only the default one.
	TrackSelection bool
}

// VideoEntry is one kind of video a client decodes.
type VideoEntry struct {
	Codec        media.Codec
	Delivery     []Delivery // file, stream
	Containers   []string   // for file delivery, the containers it decodes this in
	Tags         []string   // for file delivery, the codec tags it takes: hvc1, hev1, dvh1, dvhe
	Profiles     []string   // as ffprobe prints them
	MaxLevel     float64    // as the codec's specification writes it, such as 4.1
	MaxBitDepth  int
	MaxWidth     int
	MaxHeight    int
	MaxFramerate float64
	MaxBitrate   int   // the decoder's limit, in bits per second
	MaxRefFrames int   // zero means any
	Interlaced   *bool // nil means allowed
	Anamorphic   *bool // nil means allowed
	HDR          []media.DynamicRange
	DolbyVision  []DolbyVisionEntry
	// DoviFallback plays a Dolby Vision video whose profile isn't listed
	// by its base layer, when that layer's range is in HDR.
	DoviFallback bool
}

// DolbyVisionEntry is a Dolby Vision profile a client shows.
type DolbyVisionEntry struct {
	Profile int   // 5, 7, 8 or 10
	Compat  []int // the base layer compatibility IDs; empty means any
	EL      *bool // whether it plays an enhancement layer; nil means it does
}

// AudioEntry is one kind of audio a client plays.
type AudioEntry struct {
	Codec         media.Codec
	Delivery      []Delivery // file, stream
	Containers    []string   // for file delivery
	MaxChannels   int        // what reaches the output
	MaxSampleRate int
	MaxBitDepth   int
	Profiles      []string // as ffprobe prints them: LC, HE-AAC, Dolby TrueHD + Dolby Atmos
}

// SubtitleEntry is a subtitle codec a client shows, and how: read from the
// played file, as a WebVTT rendition of the stream, or as its own file.
type SubtitleEntry struct {
	Codec      media.Codec
	Delivery   []Delivery // file, stream, external
	Containers []string   // for file delivery, the containers it reads this from
}

// HDR10Plus is the HDR format a client may list for HDR10+ dynamic
// metadata. The probe reads only the transfer, so an HDR10+ video is
// described as HDR10, which matches a client that lists HDR10.
const HDR10Plus media.DynamicRange = "hdr10plus"

// Settings are the client's playback settings. They're kept on the client,
// not the server.
type Settings struct {
	DisableDirectPlay bool
	BurnSubtitles     bool
}

// The kinds of entry each say which codec they're for, and how it's
// delivered.
func (e VideoEntry) codec() (media.Codec, []Delivery)    { return e.Codec, e.Delivery }
func (e AudioEntry) codec() (media.Codec, []Delivery)    { return e.Codec, e.Delivery }
func (e SubtitleEntry) codec() (media.Codec, []Delivery) { return e.Codec, e.Delivery }

type entry interface {
	codec() (media.Codec, []Delivery)
}

// isFor reports whether an entry is for codec delivered as d.
func isFor(e entry, codec media.Codec, d Delivery) bool {
	c, ds := e.codec()
	return strings.EqualFold(string(c), string(codec)) && slices.Contains(ds, d)
}

// streams reports whether the client plays HLS: whether any of its video
// entries is for a stream.
func (c Capabilities) streams() bool {
	return slices.ContainsFunc(c.Video, func(e VideoEntry) bool { return slices.Contains(e.Delivery, DeliverStream) })
}

// shows reports whether the client shows a subtitle codec delivered as d.
func (c Capabilities) shows(codec media.Codec, d Delivery) bool {
	return slices.ContainsFunc(c.Subtitles, func(e SubtitleEntry) bool { return isFor(e, codec, d) })
}

// readsFromFile judges whether a client reads a subtitle codec from a
// played file in containers. Only entries for file delivery count, and an
// entry that lists containers matches only a file in one of them.
func (c Capabilities) readsFromFile(codec media.Codec, containers []string) verdict {
	return judge(c.Subtitles, codec, DeliverFile, ReasonSubtitleCodecUnsupported, func(e SubtitleEntry) []check {
		return []check{inContainers(true, e.Containers, containers)}
	})
}

// Unmet is the capability a track failed: the field, the value the client
// declared, and the track's own. Lists are joined with commas.
type Unmet struct {
	Field  string
	Limit  string // empty when the client declared nothing for the track
	Actual string
}

// verdict says whether a track plays unchanged, and why not if it doesn't.
type verdict struct {
	ok    bool
	why   Reason
	unmet *Unmet
}

// refuse rejects a track that would play unchanged, giving why as the
// reason. A track already rejected keeps its own reason.
func (v *verdict) refuse(why Reason) {
	if v.ok {
		*v = verdict{why: why}
	}
}

// failed is a verdict for a failed check of a declared field.
func failed(why Reason, field, limit, actual string) verdict {
	return verdict{why: why, unmet: &Unmet{Field: field, Limit: limit, Actual: actual}}
}

// holds is the verdict of a field that holds.
var holds = verdict{ok: true}

// check is one field of an entry.
type check func() verdict

// judge returns the verdict of the first entry for codec delivered as d
// that matches, or else of the one that got furthest through its checks.
// An entry's checks run in order and stop at the first that fails. With no
// entry for the codec and delivery, the verdict is why, or
// streamCodecUnsupported when an entry lists the codec for a file, since
// then only the stream can't carry it.
func judge[E entry](entries []E, codec media.Codec, d Delivery, why Reason, checks func(E) []check) verdict {
	best, depth := codecVerdict(entries, codec, d, why), -1
	for _, e := range entries {
		if !isFor(e, codec, d) {
			continue
		}
		passed := 0
		fail := holds
		for _, c := range checks(e) {
			if fail = c(); !fail.ok {
				break
			}
			passed++
		}
		if fail.ok {
			return holds
		}
		if passed > depth {
			best, depth = fail, passed
		}
	}
	return best
}

// videoCase is a video track as a client would receive it: as delivered,
// in which containers, and at what bitrate.
type videoCase struct {
	track      media.Track
	delivery   Delivery
	containers []string // the file's container names, for file delivery
	bitrate    int      // the video's bitrate, or the file's when the video's isn't known
}

// judgeVideo judges a video track against a client's video entries.
func judgeVideo(entries []VideoEntry, vc videoCase) verdict {
	return judge(entries, vc.track.Codec, vc.delivery, ReasonVideoCodecUnsupported, func(e VideoEntry) []check { return videoChecks(e, vc) })
}

// videoChecks are a video entry's fields in the order they're checked.
func videoChecks(e VideoEntry, vc videoCase) []check {
	v, file := vc.track, vc.delivery == DeliverFile
	return []check{
		inContainers(file, e.Containers, vc.containers),
		listed(file, ReasonVideoCodecTagUnsupported, "tags", e.Tags, v.Tag),
		listed(true, ReasonVideoProfileUnsupported, "profiles", e.Profiles, v.Profile),
		allowed(ReasonVideoInterlacedUnsupported, "interlaced", e.Interlaced, v.Interlaced),
		atMost(ReasonVideoLevelUnsupported, "maxLevel", e.MaxLevel, v.SpecLevel()),
		atMost(ReasonVideoFramerateUnsupported, "maxFramerate", e.MaxFramerate, math.Round(v.FrameRate*1000)/1000),
		atMost(ReasonVideoBitrateUnsupported, "maxBitrate", e.MaxBitrate, vc.bitrate),
		atMost(ReasonVideoRefFramesUnsupported, "maxRefFrames", e.MaxRefFrames, v.References),
		atMost(ReasonVideoBitDepthUnsupported, "maxBitDepth", e.MaxBitDepth, v.BitDepth),
		atMost(ReasonVideoResolutionUnsupported, "maxWidth", e.MaxWidth, v.Width),
		atMost(ReasonVideoResolutionUnsupported, "maxHeight", e.MaxHeight, v.Height),
		allowed(ReasonVideoAnamorphicUnsupported, "anamorphic", e.Anamorphic, v.Anamorphic),
		func() verdict {
			switch {
			case hdrShows(e, v):
				return holds
			case v.DolbyVisionProfile > 0:
				return failed(ReasonVideoHdrUnsupported, "dolbyVision", dolbyVisionList(e.DolbyVision), dolbyVisionName(v))
			}
			return failed(ReasonVideoHdrUnsupported, "hdr", list(e.HDR), string(v.Range))
		},
	}
}

// inContainers checks that a file's containers include one an entry
// lists. It holds when it doesn't apply or the entry lists none.
func inContainers(applies bool, declared, actual []string) check {
	return func() verdict {
		if applies && len(declared) > 0 && !overlaps(declared, actual) {
			return failed(ReasonContainerUnsupported, "containers", list(declared), list(actual))
		}
		return holds
	}
}

// listed checks that a track's value is in a declared list. It holds when
// it doesn't apply, the list is empty, or the value isn't known.
func listed(applies bool, why Reason, field string, declared []string, actual string) check {
	return func() verdict {
		if applies && len(declared) > 0 && actual != "" && !containsFold(declared, actual) {
			return failed(why, field, list(declared), actual)
		}
		return holds
	}
}

// allowed checks a declared allowance: a false one fails a track that has
// the property.
func allowed(why Reason, field string, declared *bool, actual bool) check {
	return func() verdict {
		if declared != nil && !*declared && actual {
			return failed(why, field, "false", "true")
		}
		return holds
	}
}

// atMost checks a declared ceiling. A zero ceiling is none.
func atMost[N int | float64](why Reason, field string, ceiling, actual N) check {
	return func() verdict {
		if ceiling > 0 && actual > ceiling {
			return failed(why, field, number(float64(ceiling)), number(float64(actual)))
		}
		return holds
	}
}

// hdrShows reports whether a video entry shows a video's dynamic range. A
// Dolby Vision video matches when its profile is listed, or with
// DoviFallback when its base layer does. SDR always matches.
func hdrShows(e VideoEntry, v media.Track) bool {
	if v.DolbyVisionProfile > 0 {
		if slices.ContainsFunc(e.DolbyVision, func(d DolbyVisionEntry) bool { return d.shows(v) }) {
			return true
		}
		base, ok := dolbyVisionBase(v)
		return ok && e.DoviFallback && rangeShows(e.HDR, base)
	}
	return rangeShows(e.HDR, v.Range)
}

func (d DolbyVisionEntry) shows(v media.Track) bool {
	return d.Profile == v.DolbyVisionProfile &&
		(len(d.Compat) == 0 || slices.Contains(d.Compat, v.DolbyVisionCompat)) &&
		(d.EL == nil || *d.EL || !v.DolbyVisionEL)
}

// rangeShows reports whether a client listing ranges shows r. SDR always
// shows.
func rangeShows(ranges []media.DynamicRange, r media.DynamicRange) bool {
	return r == "" || r == media.SDR || slices.Contains(ranges, r)
}

// dolbyVisionBase is the range of a Dolby Vision video's base layer, and
// whether it has one a player can show without Dolby Vision. Profile 5 has
// none, and profile 7's is HDR10. Otherwise the compatibility ID says: 1 and
// 6 are HDR10, 2 is SDR and 4 is HLG. A profile 8 video probed before the
// probe read the ID falls back to the range of its transfer.
func dolbyVisionBase(v media.Track) (media.DynamicRange, bool) {
	switch {
	case v.DolbyVisionProfile == 5:
		return "", false
	case v.DolbyVisionProfile == 7:
		return media.HDR10, true
	}
	switch v.DolbyVisionCompat {
	case 1, 6:
		return media.HDR10, true
	case 2:
		return media.SDR, true
	case 4:
		return media.HLG, true
	case 0:
		return v.Range, v.DolbyVisionProfile == 8
	}
	return "", false
}

// dolbyVisionName is a Dolby Vision video's profile as it's written, such
// as "8.1".
func dolbyVisionName(v media.Track) string {
	if v.DolbyVisionCompat > 0 {
		return fmt.Sprintf("%d.%d", v.DolbyVisionProfile, v.DolbyVisionCompat)
	}
	return strconv.Itoa(v.DolbyVisionProfile)
}

func dolbyVisionList(entries []DolbyVisionEntry) string {
	var out []string
	for _, d := range entries {
		out = append(out, strconv.Itoa(d.Profile))
	}
	return strings.Join(out, ",")
}

// judgeAudio judges an audio track against a client's audio entries.
func judgeAudio(entries []AudioEntry, a media.Track, d Delivery, containers []string) verdict {
	return judge(entries, a.Codec, d, ReasonAudioCodecUnsupported, func(e AudioEntry) []check {
		return []check{
			inContainers(d == DeliverFile, e.Containers, containers),
			listed(true, ReasonAudioProfileUnsupported, "profiles", e.Profiles, a.Profile),
			atMost(ReasonAudioSampleRateUnsupported, "maxSampleRate", e.MaxSampleRate, a.SampleRate),
			atMost(ReasonAudioBitDepthUnsupported, "maxBitDepth", e.MaxBitDepth, a.BitDepth),
			func() verdict {
				if a.Channels > e.MaxChannels {
					return failed(ReasonAudioChannelsUnsupported, "maxChannels", strconv.Itoa(e.MaxChannels), strconv.Itoa(a.Channels))
				}
				return holds
			},
		}
	})
}

// codecVerdict is the verdict for a codec no entry lists for delivery d.
func codecVerdict[E entry](entries []E, codec media.Codec, d Delivery, why Reason) verdict {
	var listed []string
	inFile := false
	for _, e := range entries {
		c, ds := e.codec()
		if slices.Contains(ds, d) {
			listed = append(listed, string(c))
		}
		inFile = inFile || isFor(e, codec, DeliverFile)
	}
	if d == DeliverStream && inFile {
		why = ReasonStreamCodecUnsupported
	}
	slices.Sort(listed)
	return failed(why, "codec", strings.Join(slices.Compact(listed), ","), string(codec))
}

// maxChannels is the most channels of a codec a client plays delivered as
// d. It's zero if the client doesn't play the codec so.
func maxChannels(entries []AudioEntry, codec media.Codec, d Delivery) int {
	n := 0
	for _, e := range entries {
		if isFor(e, codec, d) {
			n = max(n, e.MaxChannels)
		}
	}
	return n
}

func overlaps(a, b []string) bool {
	return slices.ContainsFunc(a, func(s string) bool { return containsFold(b, s) })
}

func containsFold(xs []string, s string) bool {
	return slices.ContainsFunc(xs, func(x string) bool { return strings.EqualFold(x, s) })
}

// list joins a declared list for an Unmet.
func list[T ~string](xs []T) string {
	out := make([]string, len(xs))
	for i, x := range xs {
		out[i] = string(x)
	}
	return strings.Join(out, ",")
}

// number writes a declared number for an Unmet, such as "4.1" or "23.976".
func number(f float64) string { return strconv.FormatFloat(f, 'f', -1, 64) }
