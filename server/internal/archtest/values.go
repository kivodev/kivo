package archtest

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"path"
	"slices"
	"strings"
)

// valuesOnly lists, for each component, the components whose values alone
// it may use: their types, constants and pure functions. It may import none
// of their subpackages. It may not name any of their functions that take
// one of databaseParams, or any of their types with a method that does. So
// nothing it names reaches the database. query reads only through its own
// statements.
var valuesOnly = map[string][]string{"query": {"catalog"}}

// databaseParams are the parameter types, by import path and name, that
// mark an operation as reaching the database.
var databaseParams = []string{"context.Context", "*database/sql.Tx", "*" + module + "/internal/store.Tx", "*" + module + "/internal/store.DB"}

// parsedFile is a parsed non-test Go file and its path relative to the
// internal directory.
type parsedFile struct {
	rel  string
	fset *token.FileSet
	f    *ast.File
}

// checkValuesOnly checks the components in rules. In their non-test files
// it reports each import of a values-only component's subpackage, and each
// use of a name of that component that reaches the database.
func checkValuesOnly(internalDir string, rules map[string][]string) ([]violation, error) {
	var files []parsedFile
	err := eachFile(internalDir, func(rel string, fset *token.FileSet, f *ast.File) {
		files = append(files, parsedFile{rel, fset, f})
	})
	if err != nil {
		return nil, err
	}
	reaching := map[string]map[string]string{} // why each name reaches the database, by component
	for _, used := range rules {
		for _, c := range used {
			if reaching[c] == nil {
				reaching[c] = databaseNames(files, c)
			}
		}
	}
	var out []violation
	for _, pf := range files {
		user := component(pf.rel)
		for _, c := range rules[user] {
			out = append(out, valueUses(pf, user, c, reaching[c])...)
		}
	}
	return out, nil
}

// databaseNames returns the exported names in component c's root package
// that reach the database, each with the reason. Those are its functions
// that take one of databaseParams, and its types with a method that does.
func databaseNames(files []parsedFile, c string) map[string]string {
	out := map[string]string{}
	for _, pf := range files {
		if path.Dir(pf.rel) != c {
			continue
		}
		imports := importPaths(pf.f)
		for _, d := range pf.f.Decls {
			fd, ok := d.(*ast.FuncDecl)
			if !ok || !fd.Name.IsExported() {
				continue
			}
			param, ok := databaseParam(fd.Type.Params, imports)
			switch {
			case !ok:
			case fd.Recv == nil:
				out[fd.Name.Name] = "takes a " + param
			case receiverName(fd.Recv) != "":
				out[receiverName(fd.Recv)] = fmt.Sprintf("has %s, which takes a %s", fd.Name.Name, param)
			}
		}
	}
	return out
}

// valueUses reports pf's imports of c's subpackages, and its uses of the
// names in reaching from c's root package. pf is a file of component user.
func valueUses(pf parsedFile, user, c string, reaching map[string]string) []violation {
	var out []violation
	local := map[string]bool{}
	for _, imp := range pf.f.Imports {
		rest, ok := internal(strings.Trim(imp.Path.Value, `"`))
		switch {
		case !ok || component(rest) != c:
		case rest != c:
			at := fmt.Sprintf("%s:%d", pf.rel, pf.fset.Position(imp.Pos()).Line)
			out = append(out, violation{at, fmt.Sprintf("%s may use only %s's values, not its subpackage %s", user, c, rest)})
		case imp.Name != nil:
			local[imp.Name.Name] = true
		default:
			local[c] = true
		}
	}
	ast.Inspect(pf.f, func(n ast.Node) bool {
		sel, ok := n.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		if pkg, ok := sel.X.(*ast.Ident); ok && local[pkg.Name] {
			if why, ok := reaching[sel.Sel.Name]; ok {
				at := fmt.Sprintf("%s:%d", pf.rel, pf.fset.Position(sel.Pos()).Line)
				out = append(out, violation{at, fmt.Sprintf("%s may use only %s's values, not %s.%s, which %s", user, c, c, sel.Sel.Name, why)})
			}
		}
		return true
	})
	return out
}

// importPaths maps each name a file gives its imports to the import path.
func importPaths(f *ast.File) map[string]string {
	out := map[string]string{}
	for _, imp := range f.Imports {
		p := strings.Trim(imp.Path.Value, `"`)
		name := path.Base(p)
		if imp.Name != nil {
			name = imp.Name.Name
		}
		out[name] = p
	}
	return out
}

// databaseParam returns the first of params whose type is one of
// databaseParams, as the source writes it.
func databaseParam(params *ast.FieldList, imports map[string]string) (string, bool) {
	for _, p := range params.List {
		if slices.Contains(databaseParams, typeName(p.Type, imports)) {
			return types.ExprString(p.Type), true
		}
	}
	return "", false
}

// typeName names a type expression of the form pkg.Name or *pkg.Name using
// its package's import path. It returns "" for any other form.
func typeName(e ast.Expr, imports map[string]string) string {
	if star, ok := e.(*ast.StarExpr); ok {
		if t := typeName(star.X, imports); t != "" {
			return "*" + t
		}
		return ""
	}
	sel, ok := e.(*ast.SelectorExpr)
	if !ok {
		return ""
	}
	pkg, ok := sel.X.(*ast.Ident)
	if !ok || imports[pkg.Name] == "" {
		return ""
	}
	return imports[pkg.Name] + "." + sel.Sel.Name
}

// receiverName returns the name of a method's receiver type, or "" for an
// unexported one.
func receiverName(recv *ast.FieldList) string {
	t := recv.List[0].Type
	if star, ok := t.(*ast.StarExpr); ok {
		t = star.X
	}
	if id, ok := t.(*ast.Ident); ok && id.IsExported() {
		return id.Name
	}
	return ""
}
