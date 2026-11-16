package main

import (
	"bytes"
	"flag"
	"os"
	"testing"

	"gopkg.in/yaml.v3"
)

var update = flag.Bool("update", false, "rewrite testdata/models.golden.bs")

// The interfaces for a small spec match testdata/models.golden.bs: allOf
// flattened, an inline object named after its model and property, int64
// and number as the types ParseJson gives, and no interface for a oneOf.
func TestModelsMatchTheGoldenFile(t *testing.T) {
	spec, err := os.ReadFile("testdata/models.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(spec, &doc); err != nil {
		t.Fatal(err)
	}
	var got bytes.Buffer
	for _, m := range models(doc.Content[0]) {
		writeModel(&got, m)
	}
	if *update {
		if err := os.WriteFile("testdata/models.golden.bs", got.Bytes(), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile("testdata/models.golden.bs")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got.Bytes(), want) {
		t.Errorf("models differ from testdata/models.golden.bs (go test -update to rewrite):\n%s", got.String())
	}
}
