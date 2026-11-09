package api

import (
	"context"
	"fmt"

	"github.com/mmcdole/kivo/internal/api/internal/gen"
	"github.com/mmcdole/kivo/internal/query"
	"github.com/mmcdole/kivo/internal/users"
)

func (h *handlers) GetHome(ctx context.Context, req gen.GetHomeRequestObject) (gen.GetHomeResponseObject, error) {
	var kinds []query.RowKind
	if req.Params.Kind != nil {
		for _, k := range *req.Params.Kind {
			if !k.Valid() {
				return nil, invalid("unknown kind", field("kind", "invalid"))
			}
			kinds = append(kinds, query.RowKind(k))
		}
	}
	rows, allHidden, err := h.deps.Query.Home(ctx, callerOf(ctx).viewer, kinds...)
	if err != nil {
		return nil, err
	}
	out := gen.Home{Rows: each(rows, homeRow)}
	if allHidden {
		out.AllHidden = ptr(true)
	}
	return gen.GetHome200JSONResponse(out), nil
}

// homeRow converts a Home row.
func homeRow(r query.HomeRow) gen.HomeRow {
	return gen.HomeRow{ID: r.ID, Kind: gen.HomeRowKind(r.Kind), Items: summaries(r.Items), Library: libraryRef(r.Library), List: listRef(r.List)}
}

// listRef is a row's list, for a list's row.
func listRef(l query.ListRef) *gen.ListRef {
	if l.ID.IsZero() {
		return nil
	}
	kind := gen.ListKindManual
	if l.Smart {
		kind = gen.ListKindSmart
	}
	return &gen.ListRef{ID: l.ID, Name: l.Name, Kind: kind}
}

// libraryRef is a row's library, for a Recently Added row.
func libraryRef(l query.Library) *gen.LibraryRef {
	if l.ID.IsZero() {
		return nil
	}
	return &gen.LibraryRef{ID: l.ID, Name: l.Name, Kind: gen.LibraryKind(l.Kind)}
}

func (h *handlers) ListHomeRows(ctx context.Context, _ gen.ListHomeRowsRequestObject) (gen.ListHomeRowsResponseObject, error) {
	rows, err := h.homeRowInfos(ctx)
	if err != nil {
		return nil, err
	}
	return gen.ListHomeRows200JSONResponse(rows), nil
}

func (h *handlers) UpdateHomeRows(ctx context.Context, req gen.UpdateHomeRowsRequestObject) (gen.UpdateHomeRowsResponseObject, error) {
	v := callerOf(ctx).viewer
	changes := make([]users.HomeRow, len(req.Body.Items))
	seen := make(map[string]bool, len(changes))
	for i, c := range req.Body.Items {
		if !query.ValidRowID(c.ID) || seen[c.ID] {
			return nil, invalid("bad row id", field(fmt.Sprintf("items[%d].id", i), "invalid"))
		}
		seen[c.ID] = true
		changes[i] = users.HomeRow{ID: c.ID, Shown: c.Shown}
	}
	rows, err := h.deps.Query.ArrangedHomeRows(ctx, v, changes)
	if err != nil {
		return nil, err
	}
	if err := h.deps.Users.SetHomeRows(ctx, v, rows); err != nil {
		return nil, err
	}
	infos, err := h.homeRowInfos(ctx)
	if err != nil {
		return nil, err
	}
	return gen.UpdateHomeRows200JSONResponse(infos), nil
}

func (h *handlers) ResetHomeRows(ctx context.Context, _ gen.ResetHomeRowsRequestObject) (gen.ResetHomeRowsResponseObject, error) {
	v := callerOf(ctx).viewer
	if err := h.deps.Users.ResetHomeRows(ctx, v); err != nil {
		return nil, err
	}
	infos, err := h.homeRowInfos(ctx)
	if err != nil {
		return nil, err
	}
	return gen.ResetHomeRows200JSONResponse(infos), nil
}

// homeRowInfos reads the caller's Home layout.
func (h *handlers) homeRowInfos(ctx context.Context) (gen.HomeRowInfoList, error) {
	rows, err := h.deps.Query.HomeRows(ctx, callerOf(ctx).viewer)
	if err != nil {
		return gen.HomeRowInfoList{}, err
	}
	return gen.HomeRowInfoList{Items: each(rows, homeRowInfo)}, nil
}

// homeRowInfo converts a row of the layout.
func homeRowInfo(r query.HomeRowInfo) gen.HomeRowInfo {
	return gen.HomeRowInfo{ID: r.ID, Kind: gen.HomeRowKind(r.Kind), Shown: r.Shown, Library: libraryRef(r.Library), List: listRef(r.List)}
}
