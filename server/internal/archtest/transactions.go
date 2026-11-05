package archtest

import (
	"fmt"
	"go/ast"
	"go/token"
	"slices"
)

// txControl are the database/sql methods that begin, commit or roll back a
// transaction.
var txControl = []string{"Begin", "BeginTx", "Commit", "Rollback"}

// checkTransactions reports each use of a method named in txControl in a
// non-test file outside store.
func checkTransactions(internalDir string) ([]violation, error) {
	var out []violation
	err := eachFile(internalDir, func(rel string, fset *token.FileSet, f *ast.File) {
		if component(rel) == store {
			return
		}
		ast.Inspect(f, func(n ast.Node) bool {
			sel, ok := n.(*ast.SelectorExpr)
			if ok && slices.Contains(txControl, sel.Sel.Name) {
				at := fmt.Sprintf("%s:%d", rel, fset.Position(sel.Pos()).Line)
				out = append(out, violation{at, fmt.Sprintf("%s controls a transaction: write through store.Write and read through store.Read", sel.Sel.Name)})
			}
			return true
		})
	})
	return out, err
}
