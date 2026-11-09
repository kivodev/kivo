package api

import (
	"net/http"
	"testing"
)

func TestWorkFieldsAndValues(t *testing.T) {
	e := newEnv(t, serving{})
	fields := decode[struct {
		Items []struct {
			Field string `json:"field"`
			Ops   []struct {
				Op string `json:"op"`
			} `json:"ops"`
		} `json:"items"`
	}](t, e.do("GET", "/api/v1/work-fields", e.adminToken, nil))
	if len(fields.Items) != 25 {
		t.Fatalf("%d fields, want 25: %+v", len(fields.Items), fields.Items)
	}
	if w := e.do("GET", "/api/v1/work-fields/nope/values", e.adminToken, nil); w.Code != http.StatusNotFound {
		t.Errorf("an unknown field: %d", w.Code)
	}
	if w := e.do("GET", "/api/v1/work-fields/rootWork/values?ids=nope", e.adminToken, nil); w.Code != http.StatusBadRequest {
		t.Errorf("a malformed id: %d", w.Code)
	} else if p := decode[map[string]any](t, w); p["errors"].([]any)[0].(map[string]any)["field"] != "ids" {
		t.Errorf("a malformed id names %v", p["errors"])
	}
	w := e.do("POST", "/api/v1/works/query", e.adminToken, map[string]any{"query": map[string]any{
		"kinds": []string{"movie"},
		"where": map[string]any{"match": "all", "rules": []map[string]any{{"field": "year", "op": "nope", "values": []string{"1"}}}},
	}})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("an invalid rule: %d %s", w.Code, w.Body.String())
	}
	p := decode[map[string]any](t, w)
	errs, ok := p["errors"].([]any)
	if !ok || errs[0].(map[string]any)["field"] != "/query/where/rules/0/op" {
		t.Errorf("errors %v", p["errors"])
	}
}
