package ffmpeg

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestNoFlagsInExportedNames checks that no option used in the golden
// command lines names an exported identifier of media or of this package.
// That way none of ffmpeg's vocabulary leaks through the interface.
func TestNoFlagsInExportedNames(t *testing.T) {
	goldens, err := filepath.Glob("testdata/*.golden")
	if err != nil || len(goldens) == 0 {
		t.Fatal("no golden files", err)
	}
	flags := map[string]bool{}
	for _, g := range goldens {
		b, err := os.ReadFile(g)
		if err != nil {
			t.Fatal(err)
		}
		for f := range strings.FieldsSeq(string(b)) {
			name, _, _ := strings.Cut(strings.TrimPrefix(f, "-"), ":")
			if strings.HasPrefix(f, "-") && len(name) > 3 {
				flags[strings.ToLower(strings.ReplaceAll(name, "_", ""))] = true
			}
		}
	}
	// "Profile" is the codecs' own word, as in H.264 High or Dolby Vision
	// profile 7, and requests and probe results have to carry it. The same
	// goes for "keyframe", which contains "frames".
	delete(flags, "profile")
	for _, dir := range []string{"..", "."} {
		for _, name := range exportedNames(t, dir) {
			for f := range flags {
				if strings.Contains(strings.ReplaceAll(strings.ToLower(name), "keyframe", ""), f) {
					t.Errorf("%s/%s carries ffmpeg's option %q", dir, name, f)
				}
			}
		}
	}
}

// exportedNames lists the exported identifiers, fields and methods declared
// in dir's non-test files.
func exportedNames(t *testing.T, dir string) []string {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	fset := token.NewFileSet()
	for _, path := range files {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(f, func(n ast.Node) bool {
			var ids []*ast.Ident
			switch d := n.(type) {
			case *ast.FuncDecl:
				ids = []*ast.Ident{d.Name}
			case *ast.TypeSpec:
				ids = []*ast.Ident{d.Name}
			case *ast.ValueSpec:
				ids = d.Names
			case *ast.Field:
				ids = d.Names
			}
			for _, id := range ids {
				if id.IsExported() {
					names = append(names, id.Name)
				}
			}
			return true
		})
	}
	if len(names) == 0 {
		t.Fatalf("no exported names in %s", dir)
	}
	return names
}
