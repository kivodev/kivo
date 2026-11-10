package subtitle

import (
	"bytes"
	"cmp"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/encoding/unicode"

	"github.com/mmcdole/kivo/internal/media"
)

// ErrFormat means the format is one WebVTT doesn't convert.
var ErrFormat = errors.New("unsupported format")

// maxSize caps how much is read. A feature film's subtitles are well under 1 MB.
const maxSize = 16 << 20

// WebVTT converts a text subtitle file in SRT, VTT, ASS or SSA to WebVTT.
// Text in UTF-16 with a byte-order mark is converted to UTF-8, and so is
// text that isn't valid UTF-8, most often Windows-1252. Styling beyond
// italics, bold and underline is dropped.
func WebVTT(src []byte, format media.SubtitleFormat) ([]byte, error) {
	text, err := readText(src, format)
	if err != nil {
		return nil, err
	}
	if format == media.VTT {
		return []byte(text), nil
	}
	var b bytes.Buffer
	b.WriteString("WEBVTT\n")
	writeCues(&b, parse(text, format))
	return b.Bytes(), nil
}

// Cues reads a text subtitle file's cues. It accepts any format and
// encoding that WebVTT converts. It is for a stream that carries the file
// in its subtitle rendition. Cue times are the file's own.
func Cues(src []byte, format media.SubtitleFormat) ([]media.Cue, error) {
	text, err := readText(src, format)
	if err != nil {
		return nil, err
	}
	return parse(text, format), nil
}

// readText returns a text subtitle file of a format WebVTT converts, as
// UTF-8 with Unix line endings.
func readText(src []byte, format media.SubtitleFormat) (string, error) {
	if !slices.Contains([]media.SubtitleFormat{media.VTT, media.SRT, media.ASS}, format) {
		return "", fmt.Errorf("%w: %s", ErrFormat, format)
	}
	if len(src) > maxSize {
		return "", fmt.Errorf("%d bytes is too large", len(src))
	}
	text, err := decode(src)
	if err != nil {
		return "", err
	}
	text = strings.ReplaceAll(strings.ReplaceAll(text, "\r\n", "\n"), "\r", "\n")
	if format == media.VTT && !strings.HasPrefix(text, "WEBVTT") {
		return "", errors.New("not WebVTT")
	}
	return text, nil
}

// parse reads the cues of text that readText returned. WebVTT's cue blocks
// are read the same way as SubRip's.
func parse(text string, format media.SubtitleFormat) []media.Cue {
	if format == media.ASS {
		return parseASS(text)
	}
	return parseSRT(text)
}

// decode returns src as UTF-8 text without a byte-order mark.
func decode(src []byte) (string, error) {
	var out []byte
	var err error
	switch order, unmarked := utf16Order(src); {
	case bytes.HasPrefix(src, []byte{0xEF, 0xBB, 0xBF}):
		return string(src[3:]), nil
	case bytes.HasPrefix(src, []byte{0xFF, 0xFE}), bytes.HasPrefix(src, []byte{0xFE, 0xFF}):
		out, err = unicode.UTF16(unicode.LittleEndian, unicode.UseBOM).NewDecoder().Bytes(src)
	case unmarked:
		out, err = unicode.UTF16(order, unicode.IgnoreBOM).NewDecoder().Bytes(src)
	case utf8.Valid(src):
		return string(src), nil
	default:
		out, err = charmap.Windows1252.NewDecoder().Bytes(src)
	}
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// utf16Order recognizes UTF-16 without a byte-order mark by its zero
// bytes. Text in a Latin script has a zero in every other byte.
func utf16Order(src []byte) (unicode.Endianness, bool) {
	n := min(len(src), 4096) &^ 1
	even, odd := 0, 0
	for i := 0; i < n; i += 2 {
		if src[i] == 0 {
			even++
		}
		if src[i+1] == 0 {
			odd++
		}
	}
	switch {
	case odd > n/4:
		return unicode.LittleEndian, true
	case even > n/4:
		return unicode.BigEndian, true
	}
	return unicode.LittleEndian, false
}

var (
	srtTiming = regexp.MustCompile(`^\s*(?:(\d+):)?(\d{1,2}):(\d{1,2})[,.](\d{1,3})\s*-->\s*(?:(\d+):)?(\d{1,2}):(\d{1,2})[,.](\d{1,3})`)
	// srtOverride is an ASS override block in SubRip text, such as the
	// {\an8} ffmpeg writes for a cue at the top.
	srtOverride = regexp.MustCompile(`\{\\[^}]*\}`)
	cueSetting  = regexp.MustCompile(`^(vertical|line|position|size|align):\S+$`)
)

// parseSRT reads SubRip. Each block has an optional counter, a timing line
// and text, and blocks are separated by blank lines and ordered by start.
// As in WebVTT, the hours may be left out and cue settings may follow the
// timing.
func parseSRT(text string) []media.Cue {
	var out []media.Cue
	lines := strings.Split(text, "\n")
	for i := 0; i < len(lines); i++ {
		m := srtTiming.FindStringSubmatch(lines[i])
		if m == nil {
			continue
		}
		c := media.Cue{Start: hms(m[1], m[2], m[3], m[4]), End: hms(m[5], m[6], m[7], m[8])}
		var settings []string
		for _, f := range strings.Fields(lines[i][len(m[0]):]) {
			if cueSetting.MatchString(f) {
				settings = append(settings, f)
			}
		}
		c.Settings = strings.Join(settings, " ")
		var body []string
		for i++; i < len(lines) && strings.TrimSpace(lines[i]) != ""; i++ {
			body = append(body, lines[i])
		}
		t, at := overrides(strings.Join(body, "\n"), srtOverride)
		c.Text = cueLines(escape(t))
		c.Settings = cmp.Or(at, c.Settings)
		if c.Text != "" && c.End > c.Start {
			out = append(out, c)
		}
	}
	slices.SortStableFunc(out, func(a, b media.Cue) int { return cmp.Compare(a.Start, b.Start) })
	return out
}

// hms reads hours, minutes, seconds and a fraction of a second.
func hms(h, m, s, frac string) time.Duration {
	n := func(x string) time.Duration { v, _ := strconv.Atoi(x); return time.Duration(v) }
	ms := n((frac + "00")[:3])
	return n(h)*time.Hour + n(m)*time.Minute + n(s)*time.Second + ms*time.Millisecond
}

var (
	assOverride = regexp.MustCompile(`\{[^}]*\}`)
	assTime     = regexp.MustCompile(`^(\d+):(\d{2}):(\d{2})\.(\d{1,3})$`)
	alignTag    = regexp.MustCompile(`\\an([1-9])`)
	// cueMarkup is a WebVTT tag or timestamp at the start of a string.
	cueMarkup = regexp.MustCompile(`^(?:</?(?:[ibuc]|v|lang|ruby|rt)(?:[.\s][^<>]*)?>|<[\d:.]+>)`)
	// upperTag is SubRip's italics, bold or underline in capitals.
	upperTag = regexp.MustCompile(`^</?[IBU]>`)
	// otherTag is any other tag, or the name of a tag cut off before its ">".
	otherTag = regexp.MustCompile(`^</?[A-Za-z][A-Za-z0-9]*(?:\s[^<>]*>|>)?`)
	// charRef is an HTML character reference at the start of a string.
	charRef = regexp.MustCompile(`^&(?:#\d+|#[xX][0-9a-fA-F]+|[a-zA-Z]+\d*);`)
)

// alignments are the cue settings for ASS's \an1 to \an9, laid out as a
// numeric keypad: bottom, middle and top rows, each left, centre and right.
var alignments = [9]string{
	"align:left", "", "align:right",
	"line:50%,center align:left", "line:50%,center", "line:50%,center align:right",
	"line:0 align:left", "line:0", "line:0 align:right",
}

// overrides removes text's ASS override blocks. It returns the cue settings
// for the last alignment tag in them, if any.
func overrides(text string, block *regexp.Regexp) (string, string) {
	settings := ""
	text = block.ReplaceAllStringFunc(text, func(b string) string {
		if m := alignTag.FindAllStringSubmatch(b, -1); m != nil {
			settings = alignments[m[len(m)-1][1][0]-'1']
		}
		return ""
	})
	return text, settings
}

// cueLines returns text's lines without the blank lines, which WebVTT would
// read as the cue's end. It also breaks up any "-->", which WebVTT would
// read as a timing.
func cueLines(text string) string {
	var lines []string
	for l := range strings.SplitSeq(strings.ReplaceAll(text, "-->", "->"), "\n") {
		if l = strings.TrimSpace(l); l != "" {
			lines = append(lines, l)
		}
	}
	return strings.Join(lines, "\n")
}

// escape turns text into WebVTT cue text. It keeps WebVTT's tags,
// timestamps and character references. It lowercases SubRip's <I>, <B> and
// <U>. It drops any other tag, closed or cut off, such as <font> or <SPAN>.
// It escapes any other "<" or "&".
func escape(text string) string {
	var b strings.Builder
	for i := 0; i < len(text); {
		rest, n := text[i:], 1
		switch text[i] {
		case '<':
			if m := upperTag.FindString(rest); m != "" {
				b.WriteString(strings.ToLower(m))
				i += len(m)
				continue
			}
			if n = len(cueMarkup.FindString(rest)); n == 0 {
				if m := otherTag.FindString(rest); m != "" {
					i += len(m)
				} else {
					b.WriteString("&lt;")
					i++
				}
				continue
			}
		case '&':
			if n = len(charRef.FindString(rest)); n == 0 {
				b.WriteString("&amp;")
				i++
				continue
			}
		}
		b.WriteString(rest[:n])
		i += n
	}
	return b.String()
}

// parseASS reads the Dialogue events of an ASS or SSA script, with fields
// in the order its Format line gives. It drops override tags and drawing,
// but keeps an \an alignment as cue settings. Events are sorted by start,
// as WebVTT needs.
func parseASS(text string) []media.Cue {
	var out []media.Cue
	var fields []string
	inEvents := false
	plain := strings.NewReplacer(`\N`, "\n", `\n`, "\n", `\h`, " ", "&", "&amp;", "<", "&lt;")
	for line := range strings.SplitSeq(text, "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "["):
			inEvents = strings.EqualFold(line, "[Events]")
		case !inEvents:
		case strings.HasPrefix(line, "Format:"):
			fields = nil
			for f := range strings.SplitSeq(strings.TrimPrefix(line, "Format:"), ",") {
				fields = append(fields, strings.ToLower(strings.TrimSpace(f)))
			}
		case strings.HasPrefix(line, "Dialogue:") && len(fields) > 0:
			values := strings.SplitN(strings.TrimPrefix(line, "Dialogue:"), ",", len(fields))
			if len(values) != len(fields) {
				continue
			}
			get := func(name string) string {
				if i := slices.Index(fields, name); i >= 0 {
					return strings.TrimSpace(values[i])
				}
				return ""
			}
			start, ok1 := assStamp(get("start"))
			end, ok2 := assStamp(get("end"))
			t, settings := overrides(values[len(values)-1], assOverride)
			t = cueLines(plain.Replace(t))
			if ok1 && ok2 && end > start && t != "" {
				out = append(out, media.Cue{Start: start, End: end, Text: t, Settings: settings})
			}
		}
	}
	slices.SortStableFunc(out, func(a, b media.Cue) int { return cmp.Compare(a.Start, b.Start) })
	return out
}

func assStamp(s string) (time.Duration, bool) {
	m := assTime.FindStringSubmatch(s)
	if m == nil {
		return 0, false
	}
	return hms(m[1], m[2], m[3], m[4]), true
}

// ReadSRT reads the complete SubRip blocks at the start of b, each ended
// by a blank line. A transcode writes a text track this way while it reads
// the source. ReadSRT returns the cues and how many bytes they took. Any
// remaining bytes are a block still being written.
func ReadSRT(b []byte) ([]media.Cue, int) {
	n := 0
	for _, sep := range []string{"\n\n", "\r\n\r\n"} {
		if i := bytes.LastIndex(b, []byte(sep)); i >= 0 {
			n = max(n, i+len(sep))
		}
	}
	if n == 0 {
		return nil, 0
	}
	text, err := decode(b[:n])
	if err != nil {
		return nil, n
	}
	return parseSRT(strings.ReplaceAll(text, "\r\n", "\n")), n
}

// Segment writes one segment of an HLS WebVTT rendition. It holds the cues
// showing between from and to, cut to that span. Cue times are on the
// source's timeline. X-TIMESTAMP-MAP ties them to a transcode's media
// timestamps, which media.TimestampOffset shifts.
//
// A cue showing across a boundary is in each segment it shows in, as RFC
// 8216 asks. It is cut, so no two segments hold it at the same moment.
// Whole copies aren't used, because AVPlayer doesn't always merge them. It
// shows each segment's copy at once while another cue overlaps it, as
// rolling captions do. A cue that exactly repeats another in the same
// segment is left out. So is a cue that rounds to no time at all, or that
// falls before zero, where WebVTT has no time.
func Segment(cues []media.Cue, from, to time.Duration) []byte {
	var in []media.Cue
	seen := map[media.Cue]bool{}
	for _, c := range cues {
		c.Start, c.End = max(c.Start, from, 0), min(c.End, to)
		if c.End.Milliseconds() > c.Start.Milliseconds() && !seen[c] {
			seen[c] = true
			in = append(in, c)
		}
	}
	slices.SortStableFunc(in, func(a, b media.Cue) int { return cmp.Compare(a.Start, b.Start) })
	var b bytes.Buffer
	fmt.Fprintf(&b, "WEBVTT\nX-TIMESTAMP-MAP=MPEGTS:%d,LOCAL:00:00:00.000\n", media.TimestampOffset*90_000/time.Second)
	writeCues(&b, in)
	return b.Bytes()
}

func writeCues(b *bytes.Buffer, cues []media.Cue) {
	for _, c := range cues {
		settings := ""
		if c.Settings != "" {
			settings = " " + c.Settings
		}
		fmt.Fprintf(b, "\n%s --> %s%s\n%s\n", stamp(c.Start), stamp(c.End), settings, c.Text)
	}
}

// stamp formats a WebVTT timestamp, hh:mm:ss.ttt.
func stamp(d time.Duration) string {
	ms := d.Milliseconds()
	return fmt.Sprintf("%02d:%02d:%02d.%03d", ms/3_600_000, ms/60_000%60, ms/1000%60, ms%1000)
}
