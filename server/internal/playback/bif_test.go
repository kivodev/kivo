package playback

import (
	"bytes"
	"encoding/binary"
	"image/jpeg"
	"testing"
	"time"

	"github.com/mmcdole/kivo/internal/media"
)

// testdata/previews/sheet-0.webp is one sheet of 2 by 2 frames, 8 pixels
// square: red, green, blue and white.
func TestBIFHasAFrameEveryIntervalOfTheVideo(t *testing.T) {
	p := media.Previews{Interval: 10 * time.Second, Width: 8, Height: 8, Columns: 2, Rows: 2, Sheets: 1}
	b, err := bif(p, "testdata/previews", 25*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(b[:8], bifMagic) {
		t.Fatalf("magic %x", b[:8])
	}
	le := binary.LittleEndian
	if version, count, interval := le.Uint32(b[8:]), le.Uint32(b[12:]), le.Uint32(b[16:]); version != 0 || count != 3 || interval != 10000 {
		t.Fatalf("version %d, %d frames every %d ms; want 0, 3 (25 s at 10 s, the padding left out), 10000", version, count, interval)
	}
	want := [][3]uint32{{255, 0, 0}, {0, 255, 0}, {0, 0, 255}}
	for i := range 3 {
		at := bifHeader + 8*i
		number, start, end := le.Uint32(b[at:]), le.Uint32(b[at+4:]), le.Uint32(b[at+12:])
		if number != uint32(i) {
			t.Fatalf("entry %d numbers frame %d", i, number)
		}
		img, err := jpeg.Decode(bytes.NewReader(b[start:end]))
		if err != nil {
			t.Fatalf("frame %d: %v", i, err)
		}
		if size := img.Bounds().Size(); size.X != 8 || size.Y != 8 {
			t.Fatalf("frame %d is %v", i, size)
		}
		r, g, bl, _ := img.At(4, 4).RGBA()
		got := [3]uint32{r >> 8, g >> 8, bl >> 8}
		for c := range 3 {
			if d := int(got[c]) - int(want[i][c]); d > 24 || d < -24 {
				t.Fatalf("frame %d's middle is %v, want about %v", i, got, want[i])
			}
		}
	}
	end := bifHeader + 8*3
	if le.Uint32(b[end:]) != 0xffffffff || int(le.Uint32(b[end+4:])) != len(b) {
		t.Fatalf("the end entry: %x %d of %d", le.Uint32(b[end:]), le.Uint32(b[end+4:]), len(b))
	}
}

func TestBIFOfAnUnknownDurationHasEveryFrame(t *testing.T) {
	p := media.Previews{Interval: 5 * time.Second, Width: 8, Height: 8, Columns: 2, Rows: 2, Sheets: 1}
	b, err := bif(p, "testdata/previews", 0)
	if err != nil {
		t.Fatal(err)
	}
	if count := binary.LittleEndian.Uint32(b[12:]); count != 4 {
		t.Fatalf("%d frames, want 4", count)
	}
}

func TestBIFOfAMissingSheetFails(t *testing.T) {
	p := media.Previews{Interval: 10 * time.Second, Width: 8, Height: 8, Columns: 2, Rows: 2, Sheets: 2}
	if _, err := bif(p, "testdata/previews", 0); err == nil {
		t.Fatal("no error for the missing second sheet")
	}
}
