package transcode

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/mmcdole/kivo/internal/media"
)

// ConvertedFile is one part of a download's conversion, as it's kept.
type ConvertedFile struct {
	Path   string
	Size   int64
	SHA256 string // the hash of its bytes, in lowercase hex
}

// Download converts a spec's parts in turn, each to one MP4 file. The
// files go in a folder named name under cache/downloads/, which stays
// until DeleteDownload. Whatever an earlier attempt left there is
// replaced. The conversion isn't paced, so it runs as fast as the
// accelerator or the CPU allows, and it shares nothing with plays. A
// part's file appears only once it's complete, and is hashed then so a
// client can check what it fetched. reached is told how far the
// conversion has got on the parts' joined timeline. Errors are mapped the
// same way as a stream's job errors, and an accelerator failure before any
// output becomes ErrAcceleratorFailed. A failed or cancelled conversion
// leaves nothing behind.
func (t *Transcoder) Download(ctx context.Context, name string, s Spec, reached func(time.Duration)) ([]ConvertedFile, error) {
	if len(s.Parts) == 0 {
		return nil, fmt.Errorf("%w: no parts", ErrUnsupported)
	}
	dir, err := t.dl.named(name)
	if err != nil {
		return nil, err
	}
	if s, err = s.withFonts(dir); err != nil {
		os.RemoveAll(dir)
		return nil, err
	}
	var text []media.TextOut
	for _, t := range s.Text {
		text = append(text, media.TextOut{Track: t.Track, Path: t.Path, Title: t.Title, Language: t.Language})
	}
	var out []ConvertedFile
	var at time.Duration
	produced := false
	for i, f := range s.Parts {
		file, err := t.convert(ctx, filepath.Join(dir, strconv.Itoa(i)+".mp4"), media.ConvertRequest{
			Path: f.Path, VideoTrack: s.VideoTrack, Source: s.Source, Video: s.Video, Audio: s.Audio,
			BurnIn: s.BurnIn, Text: text, Accelerator: s.Accelerator,
			Reached: func(d time.Duration) {
				produced = true
				reached(at + min(d, f.Duration))
			},
		})
		if err != nil {
			os.RemoveAll(dir)
			return nil, s.classify(err, produced)
		}
		out = append(out, file)
		at += f.Duration
		reached(at)
	}
	if s.BurnIn != nil && s.BurnIn.Fonts != "" {
		os.RemoveAll(s.BurnIn.Fonts) // the fonts were gathered for the job, and aren't kept
	}
	return out, nil
}

// convert runs req into a temporary file, renames it to dst once it's
// complete, and hashes it.
func (t *Transcoder) convert(ctx context.Context, dst string, req media.ConvertRequest) (ConvertedFile, error) {
	req.Dst = dst + ".tmp"
	if err := t.deps.Media.Convert(ctx, req); err != nil {
		return ConvertedFile{}, err
	}
	if err := os.Rename(req.Dst, dst); err != nil {
		return ConvertedFile{}, err
	}
	return hashFile(dst)
}

// hashFile reads a file's size and SHA-256.
func hashFile(path string) (ConvertedFile, error) {
	f, err := os.Open(path)
	if err != nil {
		return ConvertedFile{}, err
	}
	defer f.Close()
	h := sha256.New()
	n, err := io.Copy(h, f)
	if err != nil {
		return ConvertedFile{}, err
	}
	return ConvertedFile{Path: path, Size: n, SHA256: hex.EncodeToString(h.Sum(nil))}, nil
}

// DeleteDownload deletes a download's conversion, if there is one.
func (t *Transcoder) DeleteDownload(name string) error {
	if !validName(name) {
		return fmt.Errorf("invalid download name %q", name)
	}
	return os.RemoveAll(filepath.Join(t.dl.dir, name))
}

// Downloads lists the names of the conversions kept in cache/downloads/,
// so the prune can delete those no download needs.
func (t *Transcoder) Downloads() ([]string, error) {
	entries, err := os.ReadDir(t.dl.dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		out = append(out, e.Name())
	}
	return out, nil
}

// validName reports whether a download name stays inside
// cache/downloads/.
func validName(name string) bool {
	return name != "" && name != "." && name != ".." && !strings.ContainsAny(name, `/\`)
}
