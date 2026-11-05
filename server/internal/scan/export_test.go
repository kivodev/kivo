package scan

import (
	"context"

	"github.com/mmcdole/kivo/internal/id"
)

// Behind lists the libraries with files a scan hasn't caught up with.
func (s *Scanner) Behind(ctx context.Context) ([]id.ID, error) { return s.q.Behind(ctx) }
