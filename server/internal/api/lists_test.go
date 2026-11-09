package api

import (
	"net/http"
	"testing"
)

func TestLists(t *testing.T) {
	e := newEnv(t, serving{})
	type listJSON struct {
		ID      string `json:"id"`
		Name    string `json:"name"`
		Kind    string `json:"kind"`
		BuiltIn bool   `json:"builtIn"`
	}
	all := decode[struct {
		Items []listJSON `json:"items"`
	}](t, e.do("GET", "/api/v1/lists", e.adminToken, nil)).Items
	if len(all) != 1 || !all[0].BuiltIn || all[0].Name != "My List" || all[0].Kind != "manual" {
		t.Fatalf("a new user's lists: %+v", all)
	}
	myList := all[0].ID
	created := decode[listJSON](t, e.do("POST", "/api/v1/lists", e.adminToken, map[string]any{"name": "Date Night"}))
	if created.Name != "Date Night" || created.BuiltIn {
		t.Errorf("created %+v", created)
	}
	if w := e.do("POST", "/api/v1/lists", e.adminToken, map[string]any{"name": "date night"}); w.Code != http.StatusConflict {
		t.Errorf("a taken name: %d %s", w.Code, w.Body.String())
	} else if p := decode[map[string]any](t, w); p["code"] != "nameTaken" {
		t.Errorf("a taken name: %v", p)
	}
	if w := e.do("PATCH", "/api/v1/lists/"+myList, e.adminToken, map[string]any{"name": "Other"}); w.Code != http.StatusConflict {
		t.Errorf("renaming My List: %d", w.Code)
	}
	if w := e.do("DELETE", "/api/v1/lists/"+myList, e.adminToken, nil); w.Code != http.StatusConflict {
		t.Errorf("deleting My List: %d", w.Code)
	}
	if w := e.do("GET", "/api/v1/lists/"+created.ID, e.nobodyToken, nil); w.Code != http.StatusNotFound {
		t.Errorf("another user's list: %d", w.Code)
	}
	if w := e.do("POST", "/api/v1/lists/"+created.ID+"/works", e.nobodyToken, map[string]any{"workId": e.heat.Work.String()}); w.Code != http.StatusNotFound {
		t.Errorf("adding to another user's list: %d", w.Code)
	}
	if w := e.do("POST", "/api/v1/lists/"+myList+"/works", e.adminToken, map[string]any{"workId": "01938b7e-0000-7000-8000-000000000000"}); w.Code != http.StatusNotFound {
		t.Errorf("adding a work that doesn't exist: %d", w.Code)
	}
	detail := decode[map[string]any](t, e.do("GET", "/api/v1/works/"+e.heat.Work.String(), e.adminToken, nil))
	if ids, ok := detail["listIds"].([]any); !ok || len(ids) != 0 {
		t.Errorf("listIds before: %v", detail["listIds"])
	}
	if w := e.do("POST", "/api/v1/lists/"+myList+"/works", e.adminToken, map[string]any{"workId": e.heat.Work.String()}); w.Code != http.StatusNoContent {
		t.Fatalf("adding: %d", w.Code)
	}
	detail = decode[map[string]any](t, e.do("GET", "/api/v1/works/"+e.heat.Work.String(), e.adminToken, nil))
	if ids, ok := detail["listIds"].([]any); !ok || len(ids) != 1 || ids[0] != myList {
		t.Errorf("listIds after: %v", detail["listIds"])
	}
	if w := e.do("DELETE", "/api/v1/lists/"+myList+"/works/"+e.heat.Work.String(), e.adminToken, nil); w.Code != http.StatusNoContent {
		t.Errorf("removing: %d", w.Code)
	}
	if w := e.do("DELETE", "/api/v1/lists/"+created.ID, e.adminToken, nil); w.Code != http.StatusNoContent {
		t.Errorf("deleting: %d", w.Code)
	}
	if w := e.do("GET", "/api/v1/lists/"+created.ID, e.adminToken, nil); w.Code != http.StatusNotFound {
		t.Errorf("a deleted list: %d", w.Code)
	}
}
