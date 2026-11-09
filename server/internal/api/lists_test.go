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
	if len(all) != 1 || !all[0].BuiltIn || all[0].Name != "Watchlist" || all[0].Kind != "manual" {
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
	smart := map[string]any{"name": "Comedies", "smart": map[string]any{"query": map[string]any{"kinds": []string{"movie"}, "where": map[string]any{"match": "all", "rules": []map[string]any{{"field": "year", "op": "gte", "values": []string{"1990"}}}}}}}
	w := e.do("POST", "/api/v1/lists", e.adminToken, smart)
	if w.Code != http.StatusCreated {
		t.Fatalf("a smart list: %d %s", w.Code, w.Body.String())
	}
	made := decode[map[string]any](t, w)
	id := made["id"].(string)
	if made["kind"] != "smart" || made["smart"].(map[string]any)["query"].(map[string]any)["version"] != float64(1) {
		t.Errorf("made: %v", made)
	}
	if w := e.do("POST", "/api/v1/lists/"+id+"/works", e.adminToken, map[string]any{"workId": e.heat.Work.String()}); w.Code != http.StatusConflict {
		t.Errorf("adding to a smart list: %d", w.Code)
	}
	bad := map[string]any{"name": "Bad", "smart": map[string]any{"query": map[string]any{"kinds": []string{"movie"}, "where": map[string]any{"match": "all", "rules": []map[string]any{{"field": "nope", "op": "eq"}}}}}}
	if w := e.do("POST", "/api/v1/lists", e.adminToken, bad); w.Code != http.StatusBadRequest {
		t.Errorf("bad rules: %d %s", w.Code, w.Body.String())
	} else if p := decode[map[string]any](t, w); p["errors"].([]any)[0].(map[string]any)["field"] != "/smart/query/where/rules/0/field" {
		t.Errorf("bad rules: %v", p["errors"])
	}
	if w := e.do("PATCH", "/api/v1/lists/"+created.ID, e.adminToken, map[string]any{"smart": smart["smart"]}); w.Code != http.StatusConflict {
		t.Errorf("rules on a manual list: %d %s", w.Code, w.Body.String())
	}
	if w := e.do("DELETE", "/api/v1/lists/"+created.ID, e.adminToken, nil); w.Code != http.StatusNoContent {
		t.Errorf("deleting: %d", w.Code)
	}
	if w := e.do("GET", "/api/v1/lists/"+created.ID, e.adminToken, nil); w.Code != http.StatusNotFound {
		t.Errorf("a deleted list: %d", w.Code)
	}
}
