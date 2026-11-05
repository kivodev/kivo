package api

import (
	"context"
	"errors"
	"io"
	"mime/multipart"

	"github.com/mmcdole/kivo/internal/api/internal/gen"
	"github.com/mmcdole/kivo/internal/metadata"
)

func (h *handlers) ListWorkArtwork(ctx context.Context, req gen.ListWorkArtworkRequestObject) (gen.ListWorkArtworkResponseObject, error) {
	all, err := h.deps.Metadata.Artwork(ctx, callerOf(ctx).viewer, req.ID, string(deref(req.Params.Type)))
	if err != nil {
		return nil, err
	}
	return gen.ListWorkArtwork200JSONResponse{Items: each(all, artwork)}, nil
}

func (h *handlers) UploadWorkArtwork(ctx context.Context, req gen.UploadWorkArtworkRequestObject) (gen.UploadWorkArtworkResponseObject, error) {
	kind, data, err := readUpload(req.Body)
	if err != nil {
		return nil, err
	}
	a, err := h.deps.Metadata.UploadArtwork(ctx, callerOf(ctx).viewer, req.ID, kind, data)
	if err != nil {
		return nil, err
	}
	return gen.UploadWorkArtwork201JSONResponse(artwork(a)), nil
}

// readUpload reads an artwork upload's type and file. It refuses a file
// over the size limit without reading the rest of it.
func readUpload(r *multipart.Reader) (kind string, data []byte, err error) {
	for {
		part, err := r.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return "", nil, invalid("the multipart body is malformed")
		}
		switch part.FormName() {
		case "type":
			b, err := io.ReadAll(io.LimitReader(part, 64))
			if err != nil {
				return "", nil, invalid("the multipart body is malformed")
			}
			kind = string(b)
		case "file":
			if data, err = io.ReadAll(io.LimitReader(part, metadata.MaxUpload+1)); err != nil {
				return "", nil, invalid("the multipart body is malformed")
			}
			if len(data) > metadata.MaxUpload {
				return "", nil, invalid("the image is over 25 MB", field("/file", "tooLarge"))
			}
		}
		part.Close()
	}
	switch {
	case kind == "":
		return "", nil, invalid("the artwork's type is required", field("/type", "required"))
	case len(data) == 0:
		return "", nil, invalid("an image file is required", field("/file", "required"))
	}
	return kind, data, nil
}

func (h *handlers) SelectWorkArtwork(ctx context.Context, req gen.SelectWorkArtworkRequestObject) (gen.SelectWorkArtworkResponseObject, error) {
	a, err := h.deps.Metadata.SelectArtwork(ctx, callerOf(ctx).viewer, req.ID, req.ArtworkID)
	if err != nil {
		return nil, err
	}
	return gen.SelectWorkArtwork200JSONResponse(artwork(a)), nil
}

// artwork is an image to choose, shown through the image route by its ID.
func artwork(a metadata.Artwork) gen.Artwork {
	return gen.Artwork{
		ID: a.ID, Type: gen.ArtworkType(a.Kind), Source: gen.ArtworkSource(a.Source), Provider: optAs[gen.ArtworkProvider](a.Provider), Language: opt(a.Language),
		Width: opt(a.Width), Height: opt(a.Height), Selected: a.Selected,
		Image: gen.ImageRef{ID: a.ID, Blurhash: opt(a.Blurhash), Width: opt(a.Width), Height: opt(a.Height)},
	}
}
