package main

import (
	"os"
	"strings"
	"testing"
)

const spec = `
paths:
  /api/v1/works/{id}/children:
    parameters:
      - {$ref: "#/components/parameters/ID"}
    get:
      operationId: listWorkChildren
      summary: List seasons or episodes
  /api/v1/device-codes:
    post:
      operationId: createDeviceCode
      summary: Start device sign-in
      security: []
      requestBody:
        content:
          application/json: {}
  /api/v1/libraries:
    get:
      operationId: listLibraries
      summary: List libraries
      parameters:
        - {name: after, in: query}
        - {name: limit, in: query}
components:
  parameters:
    ID: {name: id, in: path, required: true}
  schemas:
    Resolution:
      type: string
      enum: [4k, 1080p, sd]
    Name:
      type: string
`

func TestEachOperationDescribesItsRequest(t *testing.T) {
	api, types, err := generate([]byte(spec))
	if err != nil {
		t.Fatal(err)
	}
	src := string(api) + string(types)
	for _, want := range []string{
		"namespace KivoApi\n",
		"    function listWorkChildren(id as string) as KivoApi.Request\n",
		`path: "/api/v1/works/" + KivoApi.pathEscape(id) + "/children", query: invalid, body: invalid, contentType: invalid, auth: true }`,
		"    function createDeviceCode(body as object) as KivoApi.Request\n",
		`body: body, contentType: "application/json", auth: false }`,
		"    ' Query: after, limit.\n    function listLibraries(query = invalid as KivoApi.ListLibrariesQuery) as KivoApi.Request\n",
		"    interface ListLibrariesQuery\n        optional after as dynamic\n        optional limit as dynamic\n    end interface\n",
		"    enum Resolution\n        _4k = \"4k\"\n        _1080p = \"1080p\"\n        sd = \"sd\"\n    end enum\n",
		"end namespace\n",
	} {
		if !strings.Contains(src, want) {
			t.Errorf("missing %q in\n%s", want, src)
		}
	}
	if strings.Contains(src, "enum Name") {
		t.Error("a string without values isn't an enum")
	}
}

func TestEnumMembersAreDistinctAndSafe(t *testing.T) {
	var b strings.Builder
	for _, c := range []struct{ in, out string }{{"next", "_next"}, {"a-b", "a_b"}, {"iPhone", "iPhone"}, {"", "_"}} {
		if got := identifier(c.in); got != c.out {
			b.WriteString(c.in + " became " + got + "; ")
		}
	}
	if b.Len() > 0 {
		t.Error(b.String())
	}
	_, types, err := generate([]byte("paths: {}\ncomponents:\n  schemas:\n    Case:\n      type: string\n      enum: [SD, sd]\n"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(types), "SD = \"SD\"\n        sd_2 = \"sd\"\n") {
		t.Errorf("case-insensitive names must differ:\n%s", types)
	}
}

func TestTheContractGenerates(t *testing.T) {
	data, err := os.ReadFile("../../openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	api, types, err := generate(data)
	if err != nil {
		t.Fatal(err)
	}
	src := string(api) + string(types)
	for _, want := range []string{
		"    function pollDeviceCode(body as KivoApi.DeviceCodePoll) as KivoApi.Request\n",
		"    function pollDeviceCodeAnswer(response as object) as KivoApi.Session\n",
		"    function startPlay(body as KivoApi.PlayCreate) as KivoApi.Request\n",
		"    function getImageUrl(base as string, id as string, query = invalid as KivoApi.GetImageQuery) as string\n",
		"    interface WorkDetail\n",
		"        roku = \"roku\"\n",
	} {
		if !strings.Contains(src, want) {
			t.Errorf("missing %q", want)
		}
	}
}

func TestADuplicateOperationIdIsRefused(t *testing.T) {
	dup := "paths:\n  /a:\n    get: {operationId: getA}\n  /b:\n    get: {operationId: geta}\n"
	if _, _, err := generate([]byte(dup)); err == nil {
		t.Error("operationIds that differ only in case collide in BrightScript")
	}
}
