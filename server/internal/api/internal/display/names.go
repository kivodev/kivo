package display

import (
	"cmp"
	"strconv"
	"strings"

	"github.com/mmcdole/kivo/internal/api/internal/gen"
)

const separator = " · "

// NameTracks sets each track's Name and FullName. Names tell apart the
// tracks of one type in ts, as a picker lists them.
func NameTracks(ts []gen.Track) {
	byType := map[gen.TrackType][]*gen.Track{}
	var types []gen.TrackType
	for i := range ts {
		t := &ts[i]
		if _, seen := byType[t.Type]; !seen {
			types = append(types, t.Type)
		}
		byType[t.Type] = append(byType[t.Type], t)
		t.FullName = fullName(*t)
	}
	for _, typ := range types {
		nameAll(byType[typ])
	}
}

// named is one type's tracks with the parts of their names.
type named struct {
	tracks []*gen.Track
	parts  [][]string
}

// nameAll names one type's tracks. A name is built from the track's facts
// alone, never its title. Tracks whose name and title are both the same
// add their codec, then where a subtitle file lives, and are numbered last.
func nameAll(ts []*gen.Track) {
	n := named{tracks: ts, parts: make([][]string, len(ts))}
	for i, t := range ts {
		n.parts[i] = nameParts(*t)
	}
	for _, fact := range []func(gen.Track) string{codecFact, sourceFact} {
		for _, group := range n.alike() {
			n.tellApart(group, fact)
		}
	}
	for _, group := range n.alike() {
		for k, i := range group {
			n.parts[i] = append(n.parts[i], strconv.Itoa(k+1))
		}
	}
	for i, t := range ts {
		t.Name = strings.Join(n.parts[i], separator)
	}
}

// alike is the groups of tracks, by index, whose name and title are both
// the same, in order.
func (n named) alike() [][]int {
	groups := map[string][]int{}
	var keys []string
	for i, t := range n.tracks {
		k := strings.Join(n.parts[i], separator) + "\n" + strings.TrimSpace(deref(t.Title))
		if _, seen := groups[k]; !seen {
			keys = append(keys, k)
		}
		groups[k] = append(groups[k], i)
	}
	var out [][]int
	for _, k := range keys {
		if len(groups[k]) > 1 {
			out = append(out, groups[k])
		}
	}
	return out
}

// tellApart adds fact to the names of group's tracks, where it differs
// among them.
func (n named) tellApart(group []int, fact func(gen.Track) string) {
	values := map[string]bool{}
	for _, i := range group {
		values[fact(*n.tracks[i])] = true
	}
	if len(values) < 2 {
		return
	}
	for _, i := range group {
		if v := fact(*n.tracks[i]); v != "" {
			n.parts[i] = append(n.parts[i], v)
		}
	}
}

// codecFact tells tracks apart by codec, where a name doesn't show it already.
func codecFact(t gen.Track) string {
	if premiumAudio(t) != "" || t.Codec == "" {
		return ""
	}
	return codec(t.Codec)
}

// sourceFact tells subtitles apart by where a subtitle file lives.
func sourceFact(t gen.Track) string {
	if t.Subtitle == nil || t.Subtitle.Source == gen.SubtitleSourceEmbedded {
		return ""
	}
	return tracks.Sources[string(t.Subtitle.Source)]
}

// fullName is a track in full: a video's codec, profile, resolution and HDR
// ("HEVC Main 10 4K Dolby Vision"); audio's language, flags, codec or
// premium format and channels and Default ("English · E-AC-3 5.1 ·
// Default"); a subtitle's language, flags, codec and where it lives
// ("English · SDH · SRT · External"); then the title as written.
func fullName(t gen.Track) string {
	if t.Video != nil {
		return videoName(t, true)
	}
	parts := baseName(t)
	if a := t.Audio; a != nil {
		parts = append(parts, cmp.Or(premiumAudio(t), codec(t.Codec))+" "+layout(a.Layout, a.Channels))
	}
	if s := t.Subtitle; s != nil {
		parts = append(parts, codec(t.Codec))
		if source := tracks.Sources[string(s.Source)]; source != "" {
			parts = append(parts, source)
		}
	}
	if t.Default {
		parts = append(parts, tracks.Default)
	}
	if title := strings.TrimSpace(deref(t.Title)); title != "" {
		parts = append(parts, "“"+title+"”")
	}
	return strings.Join(parts, separator)
}

// videoName is a video's codec, its profile when withProfile, its
// resolution and HDR: "HEVC Main 10 4K Dolby Vision".
func videoName(t gen.Track, withProfile bool) string {
	v := t.Video
	out := []string{codec(t.Codec)}
	if withProfile && v.Profile != nil {
		out = append(out, *v.Profile)
	}
	if v.Resolution != nil {
		out = append(out, resolution(string(*v.Resolution)))
	}
	if v.Hdr != nil {
		out = append(out, hdr(string(*v.Hdr)))
	}
	return strings.Join(out, " ")
}

// VersionName is a version's name: its edition, then its video's
// resolution and HDR ("Final Cut · 4K HDR", "1080p", "4K Dolby Vision"),
// or Audio without video.
func VersionName(v gen.Version) string {
	var parts []string
	if v.Edition != nil && *v.Edition != "" {
		parts = append(parts, *v.Edition)
	}
	if q := quality(v); q != "" {
		parts = append(parts, q)
	}
	return strings.Join(parts, separator)
}

// quality is a version's resolution and HDR, or Audio without video.
func quality(v gen.Version) string {
	if v.Resolution == nil {
		for _, t := range v.Tracks {
			if t.Type == gen.TrackTypeVideo {
				return ""
			}
		}
		return tracks.AudioOnly
	}
	q := resolution(string(*v.Resolution))
	if v.Hdr != nil {
		q += " " + versionHDR(string(*v.Hdr))
	}
	return q
}

// baseName is the language, marked as the original beside a dub, and the
// flags that matter: "English", "Japanese [Original]", "English · SDH".
// Forced only matters for subtitles, so audio never says it.
func baseName(t gen.Track) []string {
	lang := language(t.Language)
	if deref(t.Original) {
		lang += " [" + tracks.Flags.Original + "]"
	}
	out := []string{lang}
	if deref(t.HearingImpaired) {
		out = append(out, tracks.Flags.HearingImpaired)
	}
	if deref(t.Forced) && t.Type == gen.TrackTypeSubtitle {
		out = append(out, tracks.Flags.Forced)
	}
	if deref(t.Commentary) {
		out = append(out, tracks.Flags.Commentary)
	}
	return out
}

// nameParts are a name's parts before anything tells it from another
// track's: a video's codec, resolution and HDR ("HEVC 4K HDR10"); else the
// language, the flags, then audio's channels and premium format, or where
// a downloaded subtitle came from.
func nameParts(t gen.Track) []string {
	if t.Video != nil {
		return []string{videoName(t, false)}
	}
	parts := baseName(t)
	if a := t.Audio; a != nil {
		parts = append(parts, layout(a.Layout, a.Channels))
		if p := premiumAudio(t); p != "" {
			parts = append(parts, p)
		}
	}
	if t.Subtitle != nil && t.Subtitle.Source == gen.SubtitleSourceDownloaded {
		parts = append(parts, tracks.DownloadedFrom)
	}
	return parts
}

// premiumAudio is a lossless or premium audio format, which a name always
// shows: "TrueHD Atmos", "DTS-HD MA", "FLAC"; "" for any other.
func premiumAudio(t gen.Track) string {
	if t.Audio == nil {
		return ""
	}
	if p := t.Audio.Profile; p != nil {
		if v, ok := media.PremiumAudio[t.Codec+" "+string(*p)]; ok {
			return v
		}
	}
	return media.PremiumAudio[t.Codec]
}

func deref[T any](p *T) T {
	var zero T
	if p == nil {
		return zero
	}
	return *p
}
