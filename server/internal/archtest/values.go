package archtest

import (
	"fmt"
	"go/ast"
	"go/token"
	"slices"
	"strings"
)

// catalogDatabase are catalog's names that reach the database. Every read
// and write in catalog goes through a Catalog, made by New, or a Batch.
var catalogDatabase = []string{"Catalog", "New", "Batch"}

// checkValuesOnly reports query's non-test files that import a catalog
// subpackage or name one of catalogDatabase. query uses catalog's values
// and reads only through its own statements.
func checkValuesOnly(internalDir string) ([]violation, error) {
	var out []violation
	err := eachFile(internalDir, func(rel string, fset *token.FileSet, f *ast.File) {
		if component(rel) != "query" {
			return
		}
		report := func(n ast.Node, msg string) {
			out = append(out, violation{fmt.Sprintf("%s:%d", rel, fset.Position(n.Pos()).Line), msg})
		}
		local := ""
		for _, imp := range f.Imports {
			switch rest, ok := internal(strings.Trim(imp.Path.Value, `"`)); {
			case !ok || component(rest) != "catalog":
			case rest != "catalog":
				report(imp, "query may use only catalog's values, not its subpackage "+rest)
			case imp.Name != nil:
				local = imp.Name.Name
			default:
				local = "catalog"
			}
		}
		ast.Inspect(f, func(n ast.Node) bool {
			if sel, ok := n.(*ast.SelectorExpr); ok && local != "" {
				if pkg, ok := sel.X.(*ast.Ident); ok && pkg.Name == local && slices.Contains(catalogDatabase, sel.Sel.Name) {
					report(sel, "query may use only catalog's values, not catalog."+sel.Sel.Name+", which reaches the database")
				}
			}
			return true
		})
	})
	return out, err
}
