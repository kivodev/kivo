package api

import (
	"net/http"
	"testing"
)

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
