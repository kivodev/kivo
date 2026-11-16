package playback

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/jpeg"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/image/webp"

	"github.com/mmcdole/kivo/internal/media"
)

// PreviewBIF names a play's previews as one BIF file, the format a Roku's
// video player shows while seeking: the sheets' frames, each as a JPEG.
const PreviewBIF = "previews.bif"

// bifMagic starts every BIF file.
var bifMagic = []byte{0x89, 'B', 'I', 'F', 0x0d, 0x0a, 0x1a, 0x0a}

// bifHeader is the header's length: the magic, the version, the frame
// count and the interval, then reserved bytes.
const bifHeader = 64

// bif makes the BIF file of the previews in dir: one frame every
// interval, cut from the sheets in order, as many as the video of
// duration has (all of them when it's zero), leaving out the last sheet's
// padding.
func bif(p media.Previews, dir string, duration time.Duration) ([]byte, error) {
	perSheet := p.Columns * p.Rows
	if perSheet <= 0 || p.Interval <= 0 {
		return nil, fmt.Errorf("previews: %d by %d frames every %v", p.Columns, p.Rows, p.Interval)
	}
	count := perSheet * p.Sheets
	if duration > 0 {
		count = min(count, int((duration+p.Interval-1)/p.Interval))
	}
	frames := make([][]byte, 0, count)
	for i, name := range p.SheetNames() {
		if len(frames) == count {
			break
		}
		sheet, err := readSheet(filepath.Join(dir, name))
		if err != nil {
			return nil, err
		}
		for j := 0; j < perSheet && len(frames) < count; j++ {
			x, y := j%p.Columns*p.Width, j/p.Columns*p.Height
			frame, err := jpegOf(sheet, image.Rect(x, y, x+p.Width, y+p.Height).Add(sheet.Bounds().Min))
			if err != nil {
				return nil, fmt.Errorf("previews: sheet %d frame %d: %w", i, j, err)
			}
			frames = append(frames, frame)
		}
	}
	var out bytes.Buffer
	out.Write(bifMagic)
	head := make([]byte, bifHeader-len(bifMagic))
	binary.LittleEndian.PutUint32(head[0:], 0) // version
	binary.LittleEndian.PutUint32(head[4:], uint32(len(frames)))
	binary.LittleEndian.PutUint32(head[8:], uint32(p.Interval.Milliseconds()))
	out.Write(head)
	// The index: each frame's number and where it starts, then an end entry.
	offset := bifHeader + 8*(len(frames)+1)
	entry := make([]byte, 8)
	for i, frame := range frames {
		binary.LittleEndian.PutUint32(entry[0:], uint32(i))
		binary.LittleEndian.PutUint32(entry[4:], uint32(offset))
		out.Write(entry)
		offset += len(frame)
	}
	binary.LittleEndian.PutUint32(entry[0:], 0xffffffff)
	binary.LittleEndian.PutUint32(entry[4:], uint32(offset))
	out.Write(entry)
	for _, frame := range frames {
		out.Write(frame)
	}
	return out.Bytes(), nil
}

func readSheet(path string) (image.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	img, err := webp.Decode(f)
	if err != nil {
		return nil, fmt.Errorf("previews: %s: %w", filepath.Base(path), err)
	}
	return img, nil
}

// jpegOf encodes the part r of img as a JPEG.
func jpegOf(img image.Image, r image.Rectangle) ([]byte, error) {
	sub, ok := img.(interface {
		SubImage(image.Rectangle) image.Image
	})
	if !ok {
		return nil, fmt.Errorf("a %T can't be cut", img)
	}
	var b bytes.Buffer
	if err := jpeg.Encode(&b, sub.SubImage(r), &jpeg.Options{Quality: 75}); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}
