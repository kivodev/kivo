package subtitles

import (
	"context"
	"errors"
	"os"
	"path/filepath"

	"github.com/mmcdole/kivo/internal/health"
	"github.com/mmcdole/kivo/internal/id"
)

// complete reports whether a file's subtitles are extracted at a
// generation, and every font the result lists is in the store.
func (s *Service) complete(file id.ID, gen int64) (bool, error) {
	if !s.results.Has(file, gen) {
		return false, nil
	}
	names, err := readFontsList(filepath.Join(s.results.Path(file, gen), fontsList))
	switch {
	case errors.Is(err, os.ErrNotExist):
		return true, nil // extracted, and no fonts are needed
	case err != nil:
		return false, err
	}
	return s.allFontsStored(names), nil
}

// Health implements health.Reporter. It reports a full cache disk, and
// OpenSubtitles being down or refusing the key or the account.
func (s *Service) Health(ctx context.Context) []health.Problem {
	return append(s.room.Health(ctx), s.openSubtitles.health(s.deps.Settings.Get())...)
}
