// Command brsgen writes the Roku app's API description from openapi.yaml:
// a BrighterScript function per operation that returns the request to
// send (its method, path, query, body and whether it carries the session's
// token), an enum per named string enum and an interface per object schema
// of the contract. The app's request task sends the requests; the
// interfaces only type what ParseJson gives, for the compiler.
//
//	brsgen <openapi.yaml> <output.bs>
package main

import (
	"fmt"
	"os"
	"path/filepath"
)

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "usage: brsgen <openapi.yaml> <output.bs>")
		os.Exit(2)
	}
	spec, err := os.ReadFile(os.Args[1])
	if err != nil {
		fail(err)
	}
	out, err := generate(spec)
	if err != nil {
		fail(err)
	}
	if err := os.MkdirAll(filepath.Dir(os.Args[2]), 0o755); err != nil {
		fail(err)
	}
	if err := os.WriteFile(os.Args[2], out, 0o644); err != nil {
		fail(err)
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "brsgen:", err)
	os.Exit(1)
}
