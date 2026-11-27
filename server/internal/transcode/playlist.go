package transcode

import (
	"cmp"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/mmcdole/kivo/internal/media"
)

// Segment and playlist names are produced and parsed only in this file.
// The api package passes the URL tail through untouched. The names are
// flat (no "/"), so each fits in one path segment.
const (
	// PlaylistName is a stream's multivariant playlist, where a client
	// starts. It names the media playlists: the video's, the audio's when
	// it's apart from the video, and each text track's.
	PlaylistName  = "index.m3u8"
	videoPlaylist = "main.m3u8"
	audioPlaylist = "audio.m3u8"
	segmentExt    = ".m4s"
	textExt       = ".vtt"
	// audioPrefix begins the names of the audio's segments and init
	// segments, which are otherwise named as the video's.
	audioPrefix = "audio-"
)

// rendition is one of a stream's media playlists: the video's, the
// audio's, or text track k's as rendition(k).
type rendition int

const (
	videoRendition rendition = -1
	audioRendition rendition = -2
)

func segmentName(i int) string { return strconv.Itoa(i) + segmentExt }

// audioName is the name of the audio's segment i, which covers the same
// span as video segment i: audio-<i>.m4s.
func audioName(i int) string { return audioPrefix + segmentName(i) }

// IsAudio reports whether a segment's name is the audio's, apart from the
// video.
func IsAudio(name string) bool { return strings.HasPrefix(name, audioPrefix) }

// audioInitName is the name of the audio's init segment for part p:
// audio-init.mp4, then audio-init-1.mp4 and so on.
func audioInitName(p int) string { return audioPrefix + initName(p) }

// textPlaylist is the name of text track k's media playlist.
func textPlaylist(k int) string { return "text-" + strconv.Itoa(k) + ".m3u8" }

// textName is the name of text track k's segment i, a WebVTT segment that
// covers the same span as video segment i: t<k>-<i>.vtt.
func textName(k, i int) string { return "t" + strconv.Itoa(k) + "-" + strconv.Itoa(i) + textExt }

// initName is the name of part p's init segment: init.mp4 for the first
// part, then init-1.mp4 and so on.
func initName(p int) string {
	if p == 0 {
		return "init.mp4"
	}
	return "init-" + strconv.Itoa(p) + ".mp4"
}

// parseInit returns the part an init segment's name is for. It returns
// false if name isn't an init segment's name.
func parseInit(name string) (int, bool) {
	if name == "init.mp4" {
		return 0, true
	}
	n, ok := strings.CutPrefix(name, "init-")
	if !ok {
		return 0, false
	}
	if n, ok = strings.CutSuffix(n, ".mp4"); !ok {
		return 0, false
	}
	p, err := strconv.Atoi(n)
	if err != nil || p < 1 || strconv.Itoa(p) != n {
		return 0, false
	}
	return p, true
}

// parseAudioInit returns the part an audio init segment's name is for. It
// returns false if name isn't an audio init segment's name.
func parseAudioInit(name string) (int, bool) {
	n, ok := strings.CutPrefix(name, audioPrefix)
	if !ok {
		return 0, false
	}
	return parseInit(n)
}

// parseSegment returns the index in a segment name, and the rendition it's
// a segment of. ok is false if name isn't a segment name.
func parseSegment(name string) (i int, r rendition, ok bool) {
	if n, ok := strings.CutSuffix(name, segmentExt); ok {
		r := videoRendition
		if a, ok := strings.CutPrefix(n, audioPrefix); ok {
			n, r = a, audioRendition
		}
		i, ok := index(n)
		return i, r, ok
	}
	n, ok := strings.CutSuffix(name, textExt)
	if !ok {
		return 0, 0, false
	}
	k, n, ok := strings.Cut(strings.TrimPrefix(n, "t"), "-")
	text, okK := index(k)
	i, okI := index(n)
	if !ok || !okK || !okI || !strings.HasPrefix(name, "t") {
		return 0, 0, false
	}
	return i, rendition(text), true
}

// index parses a name's number, written without a sign or leading zeros.
func index(n string) (int, bool) {
	i, err := strconv.Atoi(n)
	return i, err == nil && i >= 0 && strconv.Itoa(i) == n
}

// renderPlaylists writes every playlist of a stream up front, so a client
// can seek anywhere before far segments exist. Those are the multivariant
// playlist, the video's media playlist, the audio's when it's apart from
// the video, and each text track's. URIs are relative, which keeps the
// play's ID out of the playlists.
func renderPlaylists(s Spec, plan []span) map[string][]byte {
	out := map[string][]byte{PlaylistName: renderMultivariant(s), videoPlaylist: renderPlaylist(plan, videoRendition)}
	if s.splitAudio() {
		out[audioPlaylist] = renderPlaylist(plan, audioRendition)
	}
	for k := range s.Text {
		out[textPlaylist(k)] = renderPlaylist(plan, rendition(k))
	}
	return out
}

// sdhCharacteristics mark a subtitle rendition for the deaf and hard of
// hearing.
const sdhCharacteristics = "public.accessibility.transcribes-spoken-dialog,public.accessibility.describes-music-and-sound"

// renderMultivariant names the stream's single variant, with its codecs
// when they're known, its audio as an audio rendition when it's apart from
// the video, and each text track as a subtitle rendition. A subtitle
// rendition's STABLE-RENDITION-ID is its track's ID, which clients select
// it by, and only the track the play shows is the default.
func renderMultivariant(s Spec) []byte {
	var b strings.Builder
	b.WriteString("#EXTM3U\n#EXT-X-VERSION:7\n#EXT-X-INDEPENDENT-SEGMENTS\n")
	inf := "#EXT-X-STREAM-INF:BANDWIDTH=" + strconv.Itoa(s.bandwidth())
	if c := s.codecs(); c != "" {
		inf += ",CODECS=" + quoted(c)
	}
	if s.splitAudio() {
		writeAudioRendition(&b, s.Audio[0], s.AudioTitle)
		inf += `,AUDIO="audio"`
	}
	names := map[string]int{}
	for k, t := range s.Text {
		// Renditions in a group need names of their own.
		name := t.Title
		if names[t.Title]++; names[t.Title] > 1 {
			name += " " + strconv.Itoa(names[t.Title])
		}
		b.WriteString(`#EXT-X-MEDIA:TYPE=SUBTITLES,GROUP-ID="text",NAME=` + quoted(name))
		if t.Language != "" {
			b.WriteString(",LANGUAGE=" + quoted(t.Language))
		}
		b.WriteString(",DEFAULT=" + yesNo(t.Default) + ",AUTOSELECT=YES,FORCED=" + yesNo(t.Forced))
		if t.SDH {
			b.WriteString(",CHARACTERISTICS=" + quoted(sdhCharacteristics))
		}
		if t.ID != "" {
			b.WriteString(",STABLE-RENDITION-ID=" + quoted(t.ID))
		}
		b.WriteString(",URI=" + quoted(textPlaylist(k)) + "\n")
	}
	if len(s.Text) > 0 {
		inf += `,SUBTITLES="text"`
	}
	b.WriteString(inf + "\n" + videoPlaylist + "\n")
	return []byte(b.String())
}

// writeAudioRendition writes the stream's audio as the one rendition of
// its group, named title.
func writeAudioRendition(b *strings.Builder, a media.AudioOut, title string) {
	b.WriteString(`#EXT-X-MEDIA:TYPE=AUDIO,GROUP-ID="audio",NAME=` + quoted(cmp.Or(title, "Audio")))
	if a.Language != "" {
		b.WriteString(",LANGUAGE=" + quoted(a.Language))
	}
	b.WriteString(",DEFAULT=YES,AUTOSELECT=YES")
	channels := a.Out.Channels
	if a.Out.Codec == media.Copy || channels == 0 {
		channels = a.SourceChannels
	}
	if channels > 0 {
		b.WriteString(",CHANNELS=" + quoted(strconv.Itoa(channels)))
	}
	b.WriteString(",URI=" + quoted(audioPlaylist) + "\n")
}

func yesNo(b bool) string {
	if b {
		return "YES"
	}
	return "NO"
}

// quoted returns s as an HLS quoted string. Double quotes and line breaks
// are dropped, because a quoted string can't hold them.
func quoted(s string) string {
	return `"` + strings.Map(func(r rune) rune {
		if r == '"' || r == '\r' || r == '\n' {
			return -1
		}
		return r
	}, s) + `"`
}

// bandwidth is the stream's peak bitrate for the multivariant playlist.
// For encoded video it's the video and audio bitrates added up. For
// copied video it's the source's own bitrate. Players with only one
// rendition to choose from barely use it.
func (s Spec) bandwidth() int {
	b := 0
	if s.encoded() {
		b = s.Video.Bitrate
		for _, a := range s.Audio {
			b += a.Out.Bitrate
		}
	} else {
		for _, f := range s.Parts {
			b = max(b, f.Bitrate)
		}
	}
	return max(b, 1)
}

// renderPlaylist writes rendition r's media playlist from the plan. All
// cover the same spans. Each part after the first begins with a
// discontinuity and, for video and audio, its own init segment.
func renderPlaylist(plan []span, r rendition) []byte {
	var longest time.Duration
	for _, s := range plan {
		longest = max(longest, s.Duration)
	}
	mapOf := func(init func(int) string) func(int) string {
		return func(p int) string { return fmt.Sprintf("#EXT-X-MAP:URI=%q\n", init(p)) }
	}
	var name, initMap func(int) string
	switch r {
	case videoRendition:
		name, initMap = segmentName, mapOf(initName)
	case audioRendition:
		name, initMap = audioName, mapOf(audioInitName)
	default:
		name, initMap = func(i int) string { return textName(int(r), i) }, func(int) string { return "" }
	}
	var b strings.Builder
	b.WriteString("#EXTM3U\n#EXT-X-VERSION:7\n#EXT-X-PLAYLIST-TYPE:VOD\n#EXT-X-INDEPENDENT-SEGMENTS\n")
	fmt.Fprintf(&b, "#EXT-X-TARGETDURATION:%d\n", int(math.Ceil(longest.Seconds())))
	b.WriteString(initMap(0))
	for i, s := range plan {
		if i > 0 && s.Part != plan[i-1].Part {
			b.WriteString("#EXT-X-DISCONTINUITY\n" + initMap(s.Part))
		}
		fmt.Fprintf(&b, "#EXTINF:%.3f,\n%s\n", s.Duration.Seconds(), name(i))
	}
	b.WriteString("#EXT-X-ENDLIST\n")
	return []byte(b.String())
}
