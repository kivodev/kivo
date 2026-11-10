package api

import (
	"context"

	"github.com/mmcdole/kivo/internal/api/internal/gen"
	"github.com/mmcdole/kivo/internal/scan"
)

// ListDirectories browses the server's folders for the library folder
// picker, from where it starts when no path is given.
func (h *handlers) ListDirectories(_ context.Context, req gen.ListDirectoriesRequestObject) (gen.ListDirectoriesResponseObject, error) {
	dirs, err := h.directories(deref(req.Params.Path))
	if err != nil {
		return nil, err
	}
	items := make([]gen.Directory, len(dirs))
	for i, d := range dirs {
		items[i] = gen.Directory{Name: d.Name, Path: d.Path}
	}
	return gen.ListDirectories200JSONResponse{Items: items}, nil
}

// directories lists the directories inside the one at path. An empty path
// lists where the picker starts: / and the mounts that could hold media.
func (h *handlers) directories(path string) ([]scan.Directory, error) {
	if path == "" {
		return scan.PickerStarts(), nil
	}
	dirs, found, err := scan.Directories(path)
	if err == nil && !found {
		return nil, errNotFound
	}
	return dirs, err
}
