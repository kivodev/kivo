package transcode

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
)

// withFonts returns s with its burned-in subtitle's fonts gathered in
// dir/fonts, because libass reads a subtitle's fonts from one folder. Each
// font is hard-linked where the disk allows, and copied otherwise. A font
// that can't be gathered is left out, and libass draws its text in a
// different font.
func (s Spec) withFonts(dir string) (Spec, error) {
	if s.BurnIn == nil || len(s.Fonts) == 0 {
		return s, nil
	}
	fonts := filepath.Join(dir, "fonts")
	if err := os.Mkdir(fonts, 0o755); err != nil {
		return s, err
	}
	for _, p := range s.Fonts {
		dst := filepath.Join(fonts, filepath.Base(p))
		if os.Link(p, dst) == nil {
			continue
		}
		if err := copyFile(p, dst); err != nil {
			slog.Warn("transcode: subtitle font left out", "font", p, "err", err)
		}
	}
	ref := *s.BurnIn
	ref.Fonts = fonts
	s.BurnIn = &ref
	return s, nil
}

func copyFile(src, dst string) error {
	r, err := os.Open(src)
	if err != nil {
		return err
	}
	defer r.Close()
	w, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(w, r); err != nil {
		w.Close()
		os.Remove(dst)
		return err
	}
	return w.Close()
}
