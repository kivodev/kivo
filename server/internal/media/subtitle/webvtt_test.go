package subtitle

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/encoding/unicode"

	"github.com/mmcdole/kivo/internal/media"
)

func TestSRT(t *testing.T) {
	src := "\xef\xbb\xbf1\r\n00:00:01,500 --> 00:00:03,000\r\n<i>Hello</i>\r\n<font color=\"red\">there</font>\r\n\r\n" +
		"2\r\n0:01:02.25 --> 0:01:04,000 X1:10 X2:20\r\nArrows --> are not timings\r\n\r\n" +
		"3\r\n00:00:05,000 --> 00:00:04,000\r\nbackwards, dropped\r\n"
	got, err := WebVTT([]byte(src), media.SRT)
	if err != nil {
		t.Fatal(err)
	}
	want := "WEBVTT\n\n00:00:01.500 --> 00:00:03.000\n<i>Hello</i>\nthere\n\n00:01:02.250 --> 00:01:04.000\nArrows -> are not timings\n"
	if string(got) != want {
		t.Fatalf("got\n%q\nwant\n%q", got, want)
	}
}

func TestASS(t *testing.T) {
	src := `[Script Info]
Title: x

[V4+ Styles]
Format: Name, Fontname
Style: Default,Arial

[Events]
Format: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text
Dialogue: 0,0:00:10.00,0:00:12.50,Default,,0,0,0,,{\an8\i1}Later, with commas\Nand a break
Dialogue: 0,0:00:02.10,0:00:03.00,Default,,0,0,0,,Earlier
Dialogue: 0,0:00:04.00,0:00:05.00,Default,,0,0,0,,{\p1}
Comment: 0,0:00:01.00,0:00:02.00,Default,,0,0,0,,not shown
`
	got, err := WebVTT([]byte(src), media.ASS)
	if err != nil {
		t.Fatal(err)
	}
	want := "WEBVTT\n\n00:00:02.100 --> 00:00:03.000\nEarlier\n\n00:00:10.000 --> 00:00:12.500 line:0\nLater, with commas\nand a break\n"
	if string(got) != want {
		t.Fatalf("got\n%q\nwant\n%q", got, want)
	}
}

func TestEncodings(t *testing.T) {
	srt := "1\n00:00:01,000 --> 00:00:02,000\nCafé déjà vu\n"
	latin1, _ := charmap.Windows1252.NewEncoder().String(srt)
	enc := func(order unicode.Endianness, bom unicode.BOMPolicy) string {
		s, _ := unicode.UTF16(order, bom).NewEncoder().String(srt)
		return s
	}
	for name, src := range map[string]string{
		"Windows-1252":              latin1,
		"UTF-16LE":                  enc(unicode.LittleEndian, unicode.UseBOM),
		"UTF-16BE":                  enc(unicode.BigEndian, unicode.UseBOM),
		"UTF-16LE without its mark": enc(unicode.LittleEndian, unicode.IgnoreBOM),
		"UTF-16BE without its mark": enc(unicode.BigEndian, unicode.IgnoreBOM),
		"CRLF":                      strings.ReplaceAll(srt, "\n", "\r\n"),
		"CR":                        strings.ReplaceAll(srt, "\n", "\r"),
	} {
		got, err := WebVTT([]byte(src), media.SRT)
		if err != nil || string(got) != "WEBVTT\n\n00:00:01.000 --> 00:00:02.000\nCafé déjà vu\n" {
			t.Errorf("%s: %q %v", name, got, err)
		}
	}
}

// A cue's position comes from WebVTT cue settings or an ASS alignment tag,
// which SubRip files carry too. Anything else on the timing line is left
// out.
func TestCuePositions(t *testing.T) {
	for _, tc := range []struct {
		format   media.SubtitleFormat
		src      string
		text     string
		settings string
	}{
		{media.VTT, "WEBVTT\n\n00:01.000 --> 00:02.000 align:start line:10% region:r bogus size:50%\nHi\n", "Hi", "align:start line:10% size:50%"},
		{media.SRT, "1\n00:00:01,000 --> 00:00:02,000 X1:10 X2:20 Y1:5 Y2:9\nHi\n", "Hi", ""},
		{media.SRT, "1\n00:00:01,000 --> 00:00:02,000\n{\\an8}{\\i1}Top\n", "Top", "line:0"},
		{media.SRT, "1\n00:00:01,000 --> 00:00:02,000\n{\\an1}\nBottom left\n", "Bottom left", "align:left"},
		{media.ASS, "[Events]\nFormat: Start, End, Text\nDialogue: 0:00:01.00,0:00:02.00,{\\an9\\b1}Top right\n", "Top right", "line:0 align:right"},
		{media.ASS, "[Events]\nFormat: Start, End, Text\nDialogue: 0:00:01.00,0:00:02.00,{\\an8}Moved{\\an5} mid\n", "Moved mid", "line:50%,center"},
	} {
		want := []media.Cue{{Start: time.Second, End: 2 * time.Second, Text: tc.text, Settings: tc.settings}}
		if got, err := Cues([]byte(tc.src), tc.format); err != nil || !slices.Equal(got, want) {
			t.Errorf("%q: %+v %v", tc.src, got, err)
		}
	}
}

// Cue text keeps WebVTT's tags and character references and escapes the
// rest of what WebVTT reads as markup, so no text is lost or misread.
func TestCueTextIsEscaped(t *testing.T) {
	for _, tc := range []struct {
		format    media.SubtitleFormat
		src, want string
	}{
		{media.SRT, "1\n00:00:01,000 --> 00:00:02,000\n<i>Rock</i> & roll <3 &amp; <v Bob>hi</v> <c.yellow>x</c> <00:01.500>y\n",
			"<i>Rock</i> &amp; roll &lt;3 &amp; <v Bob>hi</v> <c.yellow>x</c> <00:01.500>y"},
		{media.SRT, "1\n00:00:01,000 --> 00:00:02,000\n<I>The Molly Mae.\n", "<i>The Molly Mae."},
		{media.SRT, "1\n00:00:01,000 --> 00:00:02,000\n<I>a</I> <B>b</B> <U>c</U>\n", "<i>a</i> <b>b</b> <u>c</u>"},
		{media.SRT, "1\n00:00:01,000 --> 00:00:02,000\n<font color=\"#ffff00\">Yellow</font> <SPAN>x</SPAN>\n", "Yellow x"},
		{media.SRT, "1\n00:00:01,000 --> 00:00:02,000\nCut off</fo\n", "Cut off"},
		{media.SRT, "1\n00:00:01,000 --> 00:00:02,000\n<b>Unclosed <b\n", "<b>Unclosed"},
		{media.SRT, "1\n00:00:01,000 --> 00:00:02,000\n<SPAN class=x and the rest\n", "class=x and the rest"},
		{media.SRT, "1\n00:00:01,000 --> 00:00:02,000\n<font color=red>\nOnly line\n", "Only line"},
		{media.SRT, "1\n00:00:01,000 --> 00:00:02,000\nx < 5, <3 & <> <\n", "x &lt; 5, &lt;3 &amp; &lt;> &lt;"},
		{media.ASS, "[Events]\nFormat: Start, End, Text\nDialogue: 0:00:01.00,0:00:02.00,a < b & <i>c</i>\n", "a &lt; b &amp; &lt;i>c&lt;/i>"},
	} {
		got, err := Cues([]byte(tc.src), tc.format)
		if err != nil || len(got) != 1 || got[0].Text != tc.want {
			t.Errorf("%q: %+v %v", tc.src, got, err)
		}
	}
}

func TestCuesComeInOrderOfTheirStart(t *testing.T) {
	src := "2\n00:00:05,000 --> 00:00:06,000\nSecond\n\n1\n00:00:01,000 --> 00:00:09,000\nFirst\n\n3\n00:00:05,000 --> 00:00:07,000\nThird\n"
	got, err := Cues([]byte(src), media.SRT)
	if err != nil || len(got) != 3 || got[0].Text != "First" || got[1].Text != "Second" || got[2].Text != "Third" {
		t.Fatalf("%+v %v", got, err)
	}
}

func TestVTTPassesThroughAndOthersAreRefused(t *testing.T) {
	if got, err := WebVTT([]byte("WEBVTT\r\n\r\n00:01.000 --> 00:02.000\r\nhi\r\n"), media.VTT); err != nil || string(got) != "WEBVTT\n\n00:01.000 --> 00:02.000\nhi\n" {
		t.Fatalf("vtt: %q %v", got, err)
	}
	if _, err := WebVTT([]byte("x"), media.PGS); !errors.Is(err, ErrFormat) {
		t.Fatalf("sup: %v", err)
	}
}

func TestCuesReadEachTextFormat(t *testing.T) {
	for format, c := range map[media.SubtitleFormat]struct {
		src      string
		settings string
	}{
		media.SRT: {"1\r\n00:00:01,000 --> 00:00:02,500\r\nHi\r\n", ""},
		media.VTT: {"WEBVTT\n\nNOTE hours left out\n\n00:01.000 --> 00:02.500 align:start\nHi\n", "align:start"},
		media.ASS: {"[Events]\nFormat: Start, End, Text\nDialogue: 0:00:01.00,0:00:02.50,{\\an8}Hi\n", "line:0"},
	} {
		want := []media.Cue{{Start: time.Second, End: 2500 * time.Millisecond, Text: "Hi", Settings: c.settings}}
		if got, err := Cues([]byte(c.src), format); err != nil || !slices.Equal(got, want) {
			t.Errorf("%s: %+v %v", format, got, err)
		}
	}
	if _, err := Cues([]byte("x"), media.PGS); !errors.Is(err, ErrFormat) {
		t.Fatalf("sup: %v", err)
	}
}

func TestReadSRTTakesWholeBlocks(t *testing.T) {
	// This is how a transcode writes it, with two whole cues and a third
	// still being written.
	src := "1\n00:00:01,000 --> 00:00:02,000\n<i>One</i>\n\n2\n00:00:03,000 --> 00:00:04,500\nTwo\nlines\n\n3\n00:00:05,000 --> 00:00:0"
	cues, n := ReadSRT([]byte(src))
	want := []media.Cue{
		{Start: time.Second, End: 2 * time.Second, Text: "<i>One</i>"},
		{Start: 3 * time.Second, End: 4500 * time.Millisecond, Text: "Two\nlines"},
	}
	if !slices.Equal(cues, want) || src[n:] != "3\n00:00:05,000 --> 00:00:0" {
		t.Fatalf("got %+v, rest %q", cues, src[n:])
	}
	if cues, n := ReadSRT([]byte("1\n00:00:01,000 --> 00:00:0")); cues != nil || n != 0 {
		t.Fatalf("a partial block: %+v %d", cues, n)
	}
	if cues, n := ReadSRT([]byte("1\r\n00:00:01,000 --> 00:00:02,000\r\nCRLF\r\n\r\n")); len(cues) != 1 || cues[0].Text != "CRLF" || n != 42 {
		t.Fatalf("CRLF: %+v %d", cues, n)
	}
}

func FuzzWebVTT(f *testing.F) {
	f.Add([]byte("1\n00:00:01,000 --> 00:00:02,000\nhi\n"), "srt")
	f.Add([]byte("[Events]\nFormat: Start, End, Text\nDialogue: 0:00:01.00,0:00:02.00,hi\n"), "ass")
	f.Fuzz(func(t *testing.T, src []byte, format string) {
		_, _ = WebVTT(src, media.SubtitleFormat(format))
	})
}
