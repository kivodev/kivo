package transcode

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

// Segment and playlist names are produced and parsed only in this file.
// The api package passes the URL tail through untouched. The names are
// flat (no "/"), so each fits in one path segment.
const (
	// PlaylistName is a stream's multivariant playlist, where a client
	// starts. It names the media playlists: the video's, and each text
	// track's.
	PlaylistName  = "index.m3u8"
	videoPlaylist = "main.m3u8"
	segmentExt    = ".m4s"
	textExt       = ".vtt"
)

func segmentName(i int) string { return strconv.Itoa(i) + segmentExt }

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

// parseSegment returns the index in a segment name, and the text track
// it's a segment of, or -1 for the video's. ok is false if name isn't a
// segment name.
func parseSegment(name string) (i, text int, ok bool) {
	if n, ok := strings.CutSuffix(name, segmentExt); ok {
		i, ok := index(n)
		return i, -1, ok
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
	return i, text, true
}

// index parses a name's number, written without a sign or leading zeros.
func index(n string) (int, bool) {
	i, err := strconv.Atoi(n)
	return i, err == nil && i >= 0 && strconv.Itoa(i) == n
}

// renderPlaylists writes every playlist of a stream up front, so a client
// can seek anywhere before far segments exist. Those are the multivariant
// playlist, the video's media playlist, and each text track's. URIs are
// relative, which keeps the play's ID out of the playlists.
func renderPlaylists(s Spec, plan []span) map[string][]byte {
	out := map[string][]byte{PlaylistName: renderMultivariant(s), videoPlaylist: renderPlaylist(plan, -1)}
	for k := range s.Text {
		out[textPlaylist(k)] = renderPlaylist(plan, k)
	}
	return out
}

// sdhCharacteristics mark a subtitle rendition for the deaf and hard of
// hearing.
const sdhCharacteristics = "public.accessibility.transcribes-spoken-dialog,public.accessibility.describes-music-and-sound"

// renderMultivariant names the stream's single variant, with its codecs
// when they're known, and each text track as a subtitle rendition. A
// rendition's STABLE-RENDITION-ID is its track's ID, which clients select
// it by, and only the track the play shows is the default.
func renderMultivariant(s Spec) []byte {
	var b strings.Builder
	b.WriteString("#EXTM3U\n#EXT-X-VERSION:7\n#EXT-X-INDEPENDENT-SEGMENTS\n")
	inf := "#EXT-X-STREAM-INF:BANDWIDTH=" + strconv.Itoa(s.bandwidth())
	if c := s.codecs(); c != "" {
		inf += ",CODECS=" + quoted(c)
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

// renderPlaylist writes a media playlist from the plan. It writes the
// video's playlist, or text track k's when k isn't -1. All cover the same
// spans. Each part after the first begins with a discontinuity and, for
// video, its own init segment.
func renderPlaylist(plan []span, k int) []byte {
	var longest time.Duration
	for _, s := range plan {
		longest = max(longest, s.Duration)
	}
	name, initMap := segmentName, func(p int) string { return fmt.Sprintf("#EXT-X-MAP:URI=%q\n", initName(p)) }
	if k >= 0 {
		name, initMap = func(i int) string { return textName(k, i) }, func(int) string { return "" }
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
