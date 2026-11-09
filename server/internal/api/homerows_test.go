package api

import (
	"net/http"
	"testing"
)

func TestHomeAllHidden(t *testing.T) {
	e := newEnv(t, serving{})
	type homeJSON struct {
		Rows      []any `json:"rows"`
		AllHidden *bool `json:"allHidden"`
	}
	get := func() homeJSON { return decode[homeJSON](t, e.do("GET", "/api/v1/home", e.adminToken, nil)) }
	if h := get(); h.AllHidden != nil {
		t.Errorf("a Home with rows shown: %+v", h)
	}
	rows := decode[struct {
		Items []struct {
			ID string `json:"id"`
		} `json:"items"`
	}](t, e.do("GET", "/api/v1/home/rows", e.adminToken, nil)).Items
	var items []map[string]any
	for _, r := range rows {
		items = append(items, map[string]any{"id": r.ID, "shown": false})
	}
	if w := e.do("PATCH", "/api/v1/home/rows", e.adminToken, map[string]any{"items": items}); w.Code != http.StatusOK {
		t.Fatalf("hide all: %d %s", w.Code, w.Body.String())
	}
	if h := get(); len(h.Rows) != 0 || h.AllHidden == nil || !*h.AllHidden {
		t.Errorf("every row hidden: %+v", h)
	}
}

func TestHomeRowsValidation(t *testing.T) {
	e := newEnv(t, serving{})
	item := func(id string) map[string]any { return map[string]any{"id": id, "shown": true} }
	for _, ids := range [][]string{
		{"recentlyAdded:not-an-id"},
		{"sometimes"},
		{"continueWatching", "continueWatching"},
	} {
		var items []map[string]any
		for _, x := range ids {
			items = append(items, item(x))
		}
		if w := e.do("PATCH", "/api/v1/home/rows", e.adminToken, map[string]any{"items": items}); w.Code != http.StatusBadRequest {
			t.Errorf("%v: %d", ids, w.Code)
		}
	}
	// A well-formed id of a row that doesn't exist is ignored.
	gone := "recentlyAdded:01938b7e-0000-7000-8000-000000000000"
	w := e.do("PATCH", "/api/v1/home/rows", e.adminToken, map[string]any{"items": []map[string]any{item(gone), item("continueWatching")}})
	if w.Code != http.StatusOK {
		t.Fatalf("a dead id: %d %s", w.Code, w.Body.String())
	}
	got := decode[struct {
		Items []struct {
			ID    string `json:"id"`
			Shown bool   `json:"shown"`
		} `json:"items"`
	}](t, w)
	if len(got.Items) == 0 || got.Items[0].ID != "continueWatching" {
		t.Errorf("after a dead id: %+v", got)
	}
	if w := e.do("GET", "/api/v1/home?kind=nonsense", e.adminToken, nil); w.Code != http.StatusBadRequest {
		t.Errorf("an unknown kind: %d", w.Code)
	}
}
