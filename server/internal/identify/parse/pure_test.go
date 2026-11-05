package parse

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// TestPure keeps parse pure. Its code imports nothing that does I/O and
// never reads the clock.
func TestPure(t *testing.T) {
	allowed := map[string]bool{
		"fmt": true, "regexp": true, "strings": true, "time": true, "unicode": true, "unicode/utf8": true,
		"golang.org/x/text/cases": true, "golang.org/x/text/unicode/norm": true,
	}
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	for _, path := range files {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, imp := range f.Imports {
			if p, _ := strconv.Unquote(imp.Path.Value); !allowed[p] {
				t.Errorf("%s imports %s", path, p)
			}
		}
		ast.Inspect(f, func(n ast.Node) bool {
			if sel, ok := n.(*ast.SelectorExpr); ok {
				if x, ok := sel.X.(*ast.Ident); ok && x.Name == "time" && sel.Sel.Name == "Now" {
					t.Errorf("%s calls time.Now", fset.Position(sel.Pos()))
				}
			}
			return true
		})
	}
}
