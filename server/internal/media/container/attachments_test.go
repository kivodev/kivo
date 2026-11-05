package container

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"reflect"
	"testing"

	"github.com/mmcdole/kivo/internal/media"
)

// The attachments gen.sh put in attachments.mkv, with their SHA-256 sums.
var wantAttachments = []media.AttachedFile{
	{Filename: "font.ttf", MimeType: "application/x-truetype-font", Size: 36, Kind: media.AttachmentFont,
		SHA256: "e50562b91d93c17c2e795e02747574de6e63b103ccdd2c852f8f7246800ed6d0"},
	{Filename: "cover.png", MimeType: "image/png", Size: 33, Kind: media.AttachmentImage,
		SHA256: "4ffd8bb30991e3a6f28d1d03f1aedcd02ccf8e0cc16bb9e969e7bc2bda1ddf03"},
	{Filename: "notes.txt", MimeType: "text/plain", Size: 11, Kind: media.AttachmentOther,
		SHA256: "6751572f6149ec62801298b4b33e48c4683da9f4d49b64fb9e8a16a92e85d214"},
}

func TestAttachments(t *testing.T) {
	b := readFixture(t, "attachments.mkv")
	got, err := Attachments(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, wantAttachments) {
		t.Errorf("got  %+v\nwant %+v", got, wantAttachments)
	}
}

func TestCopyAttachment(t *testing.T) {
	b := readFixture(t, "attachments.mkv")
	for n, want := range wantAttachments {
		var out bytes.Buffer
		if err := CopyAttachment(bytes.NewReader(b), int64(len(b)), n, &out); err != nil {
			t.Fatal(err)
		}
		if h := sha256.Sum256(out.Bytes()); hex.EncodeToString(h[:]) != want.SHA256 {
			t.Errorf("attachment %d: %d bytes, not %s's", n, out.Len(), want.Filename)
		}
	}
	if err := CopyAttachment(bytes.NewReader(b), int64(len(b)), len(wantAttachments), io.Discard); !errors.Is(err, media.ErrSourceCorrupt) {
		t.Errorf("past the last: %v", err)
	}
	mp4 := readFixture(t, "h264.mp4")
	if err := CopyAttachment(bytes.NewReader(mp4), int64(len(mp4)), 0, io.Discard); !errors.Is(err, media.ErrSourceCorrupt) {
		t.Errorf("an MP4's: %v", err)
	}
}

// TestAttachmentsBySeekHead hides the Attachments element behind a
// Cluster, where only the SeekHead finds it, and then hides the SeekHead
// too.
func TestAttachmentsBySeekHead(t *testing.T) {
	b := readFixture(t, "attachments.mkv")
	m, err := openMKV(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		t.Fatal(err)
	}
	att, ok := m.seen[idAttachments]
	if !ok || len(m.seek[idAttachments]) == 0 {
		t.Fatalf("the fixture's Attachments: seen %v, SeekHead %v", m.seen, m.seek)
	}
	// The level-1 element just before the Attachments becomes a Cluster.
	var before int64
	for _, off := range m.seen {
		if off < att && off > before {
			before = off
		}
	}
	hidden := bytes.Clone(b)
	copy(hidden[before:], []byte{0x1f, 0x43, 0xb6, 0x75})
	got, err := Attachments(bytes.NewReader(hidden), int64(len(hidden)))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, wantAttachments) {
		t.Errorf("by the SeekHead: got %+v", got)
	}

	// With the SeekHead renamed to an unknown element, nothing finds them.
	copy(hidden[m.seen[idSeekHead]:], []byte{0x1b, 0x53, 0x86, 0x67})
	got, err = Attachments(bytes.NewReader(hidden), int64(len(hidden)))
	if err != nil || len(got) != 0 {
		t.Errorf("without a SeekHead: got %+v, %v", got, err)
	}
}

// TestAttachmentsSkipsBadOnes renames the second attachment's FileData,
// leaving it with no data. That attachment is left out and the others are
// kept.
func TestAttachmentsSkipsBadOnes(t *testing.T) {
	b := bytes.Clone(readFixture(t, "attachments.mkv"))
	data := []byte{0x46, 0x5c}
	first := bytes.Index(b, data)
	second := first + 2 + bytes.Index(b[first+2:], data)
	if first < 0 || second < first+2 {
		t.Fatal("the fixture has no second FileData")
	}
	copy(b[second:], []byte{0x46, 0x5d})
	got, err := Attachments(bytes.NewReader(b), int64(len(b)))
	if !errors.Is(err, media.ErrSourceCorrupt) {
		t.Errorf("err = %v, want ErrSourceCorrupt", err)
	}
	if want := []media.AttachedFile{wantAttachments[0], wantAttachments[2]}; !reflect.DeepEqual(got, want) {
		t.Errorf("got  %+v\nwant %+v", got, want)
	}
	// CopyAttachment counts attachments as Attachments lists them, so the
	// skipped one isn't counted.
	var out bytes.Buffer
	if err := CopyAttachment(bytes.NewReader(b), int64(len(b)), 1, &out); err != nil {
		t.Fatal(err)
	}
	if h := sha256.Sum256(out.Bytes()); hex.EncodeToString(h[:]) != wantAttachments[2].SHA256 {
		t.Errorf("attachment 1 after a skipped one: not %s", wantAttachments[2].Filename)
	}
}

func TestAttachmentsOverTheCap(t *testing.T) {
	defer func(n int) { maxAttachments = n }(maxAttachments)
	maxAttachments = 2
	b := readFixture(t, "attachments.mkv")
	got, err := Attachments(bytes.NewReader(b), int64(len(b)))
	if !errors.Is(err, media.ErrSourceCorrupt) {
		t.Errorf("err = %v, want ErrSourceCorrupt", err)
	}
	if !reflect.DeepEqual(got, wantAttachments[:2]) {
		t.Errorf("got %+v, want the first two", got)
	}
}

func TestAttachmentsOfOtherContainers(t *testing.T) {
	for _, name := range []string{"h264.mp4", "h264.mkv"} {
		b := readFixture(t, name)
		got, err := Attachments(bytes.NewReader(b), int64(len(b)))
		if err != nil || len(got) != 0 {
			t.Errorf("%s: got %+v, %v", name, got, err)
		}
	}
}

func TestAttachmentsTruncated(t *testing.T) {
	b := readFixture(t, "attachments.mkv")
	for _, n := range []int{100, len(b) / 2, len(b) - 10} {
		got, err := Attachments(bytes.NewReader(b[:n]), int64(n))
		if err != nil && !errors.Is(err, media.ErrSourceCorrupt) {
			t.Errorf("%d bytes: %v", n, err)
		}
		if len(got) > len(wantAttachments) || len(got) > 0 && !reflect.DeepEqual(got, wantAttachments[:len(got)]) {
			t.Errorf("%d bytes: got %+v, want a prefix of the attachments", n, got)
		}
	}
}

func TestFontType(t *testing.T) {
	for head, want := range map[string]string{
		"\x00\x01\x00\x00rest": "font/ttf", "OTTOrest": "font/otf", "ttcfrest": "font/collection",
		"wOFFrest": "font/woff", "wOF2rest": "font/woff2", "true": "font/ttf", "\x89PNG": "", "": "",
	} {
		if got := FontType([]byte(head)); got != want {
			t.Errorf("FontType(%q) = %q, want %q", head, got, want)
		}
	}
}
