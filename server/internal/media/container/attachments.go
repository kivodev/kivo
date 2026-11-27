package container

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/mmcdole/kivo/internal/media"
)

// Limits on how much work one file's attachments can cause the reader.
const (
	maxNameField = 4 << 10 // a file name or media type
	sniffSize    = 512     // what http.DetectContentType looks at
)

// maxAttachments is how many of one file's attachments are kept. It is a
// variable so tests can lower it.
var maxAttachments = 4096

// Attachments lists the files an MKV carries besides its tracks, in the
// order the container lists them. It hashes each file's bytes and uses
// them to tell fonts and images from the rest. It finds the Attachments
// element before the first Cluster or where a SeekHead points, and reads
// nothing else of the media. Other containers have no attachments.
//
// A malformed attachment, such as one with no data, is skipped and the rest
// are kept. Past maxAttachments, only the first ones are kept. An
// Attachments element that can't be read gives none. In each of these cases
// it returns what was kept, with an error wrapping media.ErrSourceCorrupt
// that says what was left out. A failed read returns nothing and
// media.ErrSourceUnreadable.
func Attachments(r io.ReaderAt, size int64) (out []media.AttachedFile, err error) {
	defer func() {
		if p := recover(); p != nil {
			out, err = nil, corrupt("%v", p)
		}
	}()
	files, skipped := attachedFiles(r, size)
	if errors.Is(skipped, media.ErrSourceUnreadable) {
		return nil, skipped
	}
	for _, f := range files {
		a, err := attachedFile(r, f)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, skipped
}

// CopyAttachments parses an MKV's attachments once and calls fn for each
// index in indices, in order, with a reader over that attachment's bytes.
// Indices count from 0 in the order [Attachments] lists them, skipping what
// [Attachments] skips. Source read errors from data wrap
// media.ErrSourceUnreadable.
func CopyAttachments(r io.ReaderAt, size int64, indices []int, fn func(n int, data io.Reader) error) (err error) {
	defer func() {
		if p := recover(); p != nil {
			err = corrupt("%v", p)
		}
	}()
	files, err := attachedFiles(r, size)
	if errors.Is(err, media.ErrSourceUnreadable) {
		return err
	}
	// Attachments that were left out, as reported in err, aren't counted.
	for _, n := range indices {
		if n < 0 || n >= len(files) {
			return corrupt("no attachment %d of %d", n, len(files))
		}
		d := files[n].data
		if err := fn(n, unreadableReader{io.NewSectionReader(r, d.data, d.end-d.data)}); err != nil {
			return err
		}
	}
	return nil
}

// unreadableReader wraps non-EOF read errors with media.ErrSourceUnreadable
// so callers can distinguish source failures from destination write errors.
type unreadableReader struct{ r io.Reader }

func (u unreadableReader) Read(p []byte) (int, error) {
	n, err := u.r.Read(p)
	if err != nil && !errors.Is(err, io.EOF) {
		return n, unreadable(err)
	}
	return n, err
}

// parsedAttachment is an AttachedFile whose header fields and FileData
// range have already been located.
type parsedAttachment struct {
	info media.AttachedFile
	data element
}

// attachedFiles finds an MKV's well-formed AttachedFile elements, in order.
// Other containers have none. A malformed element is skipped. The list
// stops at maxAttachments or at an element it can't read past. An
// Attachments element that can't be read gives none. Each of these problems
// is reported in the returned error, which wraps media.ErrSourceCorrupt. A
// failed read returns nothing and an error wrapping
// media.ErrSourceUnreadable.
func attachedFiles(r io.ReaderAt, size int64) ([]parsedAttachment, error) {
	if size < 8 {
		return nil, nil
	}
	magic, err := readAt(r, 0, 4)
	if err != nil {
		return nil, err
	}
	if !bytes.Equal(magic, ebmlMagic) {
		return nil, nil
	}
	m, err := openMKV(r, size)
	if err != nil {
		return nil, err
	}
	off, ok := m.at(idAttachments)
	if !ok || off < m.base || off >= m.end {
		return nil, nil
	}
	e, err := readElement(r, off, m.end)
	if err != nil {
		return nil, err
	}
	if e.id != idAttachments || e.end == unknownSize {
		return nil, nil // a stale SeekHead entry
	}
	var (
		out     []parsedAttachment
		skipped []error
	)
	errFull := corrupt("mkv: more than %d attachments; the rest left out", maxAttachments)
	err = each(r, e, func(f element) error {
		if f.id != idAttachedFile {
			return nil
		}
		if len(out) == maxAttachments {
			return errFull
		}
		a, data, err := attachedFileFields(r, f)
		switch {
		case errors.Is(err, media.ErrSourceCorrupt):
			skipped = append(skipped, err)
		case err != nil:
			return err
		default:
			out = append(out, parsedAttachment{info: a, data: data})
		}
		return nil
	})
	switch {
	case errors.Is(err, media.ErrSourceUnreadable):
		return nil, err
	case err != nil:
		skipped = append(skipped, err) // a broken element ends the list, keeping what came before
	}
	return out, errors.Join(skipped...)
}

// attachedFile hashes and sniffs a parsed AttachedFile's data without
// loading it all into memory.
func attachedFile(r io.ReaderAt, f parsedAttachment) (media.AttachedFile, error) {
	a, data := f.info, f.data
	a.Size = data.end - data.data
	h := sha256.New()
	head := make([]byte, min(a.Size, sniffSize))
	if _, err := r.ReadAt(head, data.data); err != nil && err != io.EOF {
		return a, unreadable(err)
	}
	if _, err := io.Copy(h, io.NewSectionReader(r, data.data, a.Size)); err != nil {
		return a, unreadable(err)
	}
	a.SHA256 = hex.EncodeToString(h.Sum(nil))
	a.Kind = sniffKind(head)
	return a, nil
}

// attachedFileFields reads an AttachedFile's name and declared type, and
// finds its data.
func attachedFileFields(r io.ReaderAt, f element) (media.AttachedFile, element, error) {
	var a media.AttachedFile
	var data *element
	err := each(r, f, func(c element) error {
		switch c.id {
		case idFileName, idFileMediaType:
			if c.end-c.data > maxNameField {
				return corrupt("mkv: attachment field %x is %d bytes", c.id, c.end-c.data)
			}
			b, err := readAt(r, c.data, c.end-c.data)
			if err != nil {
				return err
			}
			s := strings.TrimRight(string(b), "\x00")
			if c.id == idFileName {
				a.Filename = s
			} else {
				a.MimeType = s
			}
		case idFileData:
			data = &c
		}
		return nil
	})
	if err != nil {
		return a, element{}, err
	}
	if data == nil {
		return a, element{}, corrupt("mkv: attachment %q has no data", a.Filename)
	}
	return a, *data, nil
}

// each calls fn for each child of a master element. It reads only the
// children's headers.
func each(r io.ReaderAt, parent element, fn func(element) error) error {
	for off := parent.data; off < parent.end; {
		c, err := readElement(r, off, parent.end)
		if err != nil {
			return err
		}
		if c.end == unknownSize {
			return corrupt("mkv: element %x of unknown size in %x", c.id, parent.id)
		}
		if err := fn(c); err != nil {
			return err
		}
		off = c.end
	}
	return nil
}

// sniffKind tells a font or an image from its first bytes.
func sniffKind(head []byte) media.AttachmentKind {
	if FontType(head) != "" {
		return media.AttachmentFont
	}
	if strings.HasPrefix(http.DetectContentType(head), "image/") {
		return media.AttachmentImage
	}
	return media.AttachmentOther
}

// fontMagic lists the first four bytes of each font format, with the
// format's media type.
var fontMagic = []struct {
	magic string
	typ   string
}{
	{"\x00\x01\x00\x00", "font/ttf"},
	{"true", "font/ttf"}, // Apple's TrueType
	{"OTTO", "font/otf"},
	{"ttcf", "font/collection"},
	{"wOFF", "font/woff"},
	{"wOF2", "font/woff2"},
}

// FontType returns the media type of the font whose data starts with head,
// or "" when it isn't one.
func FontType(head []byte) string {
	for _, f := range fontMagic {
		if bytes.HasPrefix(head, []byte(f.magic)) {
			return f.typ
		}
	}
	return ""
}
