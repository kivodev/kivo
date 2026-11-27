package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"
)

// TestMissingAccessLevelStopsGeneration removes getWork's x-kivo-access,
// then sets it to a value that is not a level. Either change makes
// readRoutes fail with an error naming the operation and its path.
func TestMissingAccessLevelStopsGeneration(t *testing.T) {
	spec, err := os.ReadFile("../../openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	const line = "      operationId: getWork\n"
	if bytes.Count(spec, []byte(line)) != 1 {
		t.Fatalf("spec has no single %q", line)
	}
	for name, replace := range map[string][2]string{
		"missing": {"      x-kivo-access: cast\n", ""},
		"unknown": {"      x-kivo-access: cast\n", "      x-kivo-access: owner\n"},
	} {
		at := bytes.Index(spec, []byte(line))
		rest := spec[at:]
		i := bytes.Index(rest, []byte(replace[0]))
		b := slices.Concat(spec[:at], rest[:i], []byte(replace[1]), rest[i+len(replace[0]):])
		_, err := readRoutes(b)
		msg := fmt.Sprint(err)
		if !errors.Is(err, errSpec) || !strings.Contains(msg, "getWork (GET /api/v1/works/{id})") ||
			name == "unknown" && !strings.Contains(msg, `"owner"`) {
			t.Errorf("%s: %v", name, err)
		}
	}
}

// TestNoOperationID refuses an operation without an operationId.
func TestNoOperationID(t *testing.T) {
	spec := []byte("paths:\n  /x:\n    get:\n      x-kivo-access: admin\n")
	if _, err := readRoutes(spec); !errors.Is(err, errSpec) || !strings.Contains(err.Error(), "GET /x has no operationId") {
		t.Errorf("err %v", err)
	}
}
