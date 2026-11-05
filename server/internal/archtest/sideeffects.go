package archtest

import (
	"fmt"
	"go/ast"
	"go/token"
	"slices"
)

// sideEffects maps the name of each method that carries out a change's
// side effects to where it should be called from instead. The service that
// owns a change fires its side effects once the change commits.
var sideEffects = map[string]string{
	"Publish": "the service that made the change publishes its event",
}

// noSideEffects are the components that may name none of the sideEffects.
// api's handlers only parse, call the service that owns the change, and
// convert.
var noSideEffects = []string{"api"}

// checkSideEffects reports each use of a method or function named in
// effects, in a non-test file of a component listed in in.
func checkSideEffects(internalDir string, in []string, effects map[string]string) ([]violation, error) {
	var out []violation
	err := eachFile(internalDir, func(rel string, fset *token.FileSet, f *ast.File) {
		c := component(rel)
		if !slices.Contains(in, c) {
			return
		}
		ast.Inspect(f, func(n ast.Node) bool {
			var name *ast.Ident
			switch n := n.(type) {
			case *ast.SelectorExpr:
				name = n.Sel
			case *ast.CallExpr:
				name, _ = n.Fun.(*ast.Ident)
			}
			if name == nil {
				return true
			}
			if why, ok := effects[name.Name]; ok {
				at := fmt.Sprintf("%s:%d", rel, fset.Position(name.Pos()).Line)
				out = append(out, violation{at, fmt.Sprintf("%s may not call %s: %s", c, name.Name, why)})
			}
			return true
		})
	})
	return out, err
}
