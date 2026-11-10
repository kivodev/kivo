package playback

import (
	"context"
	"errors"

	"github.com/mmcdole/kivo/internal/catalog"
	"github.com/mmcdole/kivo/internal/id"
	"github.com/mmcdole/kivo/internal/users"
)

// planner plans a play and a download the same way. It chooses a work's
// version and tracks, decides how each is delivered, chooses the encoder,
// builds the transcode, and reads which subtitles are prepared. It holds
// no play and no download. Service and Downloads share one.
type planner struct {
	deps Deps

	accelFailure acceleratorFailure
}

// visible reports whether a viewer may see a work.
func (s *planner) visible(ctx context.Context, v users.Viewer, work id.ID) (bool, error) {
	err := s.deps.Catalog.Visible(ctx, v, work)
	if errors.Is(err, catalog.ErrNotFound) {
		return false, nil
	}
	return err == nil, err
}
