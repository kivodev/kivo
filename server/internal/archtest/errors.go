package archtest

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// checkSentinels reports package-level Err variables under internalDir that
// aren't made with fault.New or fault.Coded. It also reports error
// messages, wraps included, that start with the package's name. A sentinel
// names the condition, such as "not found". A wrap says what failed, such
// as "read keyframes of %s". The edge that logs the error says which
// operation it was.
func checkSentinels(internalDir string) ([]violation, error) {
	var out []violation
	err := eachFile(internalDir, func(rel string, fset *token.FileSet, f *ast.File) {
		for _, vs := range packageVars(f) {
			out = append(out, plainSentinels(rel, vs)...)
		}
		prefix := f.Name.Name + ": "
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			if msg, ok := message(call); ok && strings.HasPrefix(msg, prefix) {
				out = append(out, violation{fmt.Sprintf("%s:%d", rel, fset.Position(call.Pos()).Line), fmt.Sprintf("error message %q starts with %q: say what failed, not where", msg, prefix)})
			}
			return true
		})
	})
	return out, err
}

// message returns the message of the error that call makes. That is the
// message given to a fault or errors.New, or the format of fmt.Errorf.
func message(call *ast.CallExpr) (string, bool) {
	switch callee(call) {
	case "fault.New", "fault.Coded":
		return literal(call, len(call.Args)-1)
	case "errors.New", "fmt.Errorf":
		return literal(call, 0)
	}
	return "", false
}

func packageVars(f *ast.File) []*ast.ValueSpec {
	var out []*ast.ValueSpec
	for _, d := range f.Decls {
		if gd, ok := d.(*ast.GenDecl); ok && gd.Tok == token.VAR {
			for _, s := range gd.Specs {
				out = append(out, s.(*ast.ValueSpec))
			}
		}
	}
	return out
}

func plainSentinels(rel string, vs *ast.ValueSpec) []violation {
	var out []violation
	for i, name := range vs.Names {
		if sentinelName.MatchString(name.Name) && (i >= len(vs.Values) || !isFaultCall(vs.Values[i])) {
			out = append(out, violation{rel, fmt.Sprintf("%s is not a fault: make it with fault.New or fault.Coded", name.Name)})
		}
	}
	return out
}

func isFaultCall(e ast.Expr) bool {
	call, ok := e.(*ast.CallExpr)
	return ok && (callee(call) == "fault.New" || callee(call) == "fault.Coded")
}

// checkDeferredWraps reports a deferred closure that wraps the error its
// function returns. A wrap belongs inline, at the call that failed, and
// says what the caller doesn't know. A recover that turns a panic into an
// error wraps nothing, so it is allowed.
func checkDeferredWraps(internalDir string) ([]violation, error) {
	var out []violation
	err := eachFile(internalDir, func(rel string, fset *token.FileSet, f *ast.File) {
		ast.Inspect(f, func(n ast.Node) bool {
			d, ok := n.(*ast.DeferStmt)
			if !ok {
				return true
			}
			lit, ok := d.Call.Fun.(*ast.FuncLit)
			if !ok {
				return true
			}
			ast.Inspect(lit.Body, func(n ast.Node) bool {
				as, ok := n.(*ast.AssignStmt)
				if !ok || len(as.Rhs) != 1 {
					return true
				}
				if call, ok := as.Rhs[0].(*ast.CallExpr); ok && callee(call) == "fmt.Errorf" && wraps(call) {
					out = append(out, violation{fmt.Sprintf("%s:%d", rel, fset.Position(as.Pos()).Line), "a deferred closure wraps the returned error: wrap inline, at the call that failed"})
				}
				return true
			})
			return false
		})
	})
	return out, err
}

// wraps reports whether a fmt.Errorf call wraps an error.
func wraps(call *ast.CallExpr) bool {
	format, ok := literal(call, 0)
	return ok && slices.Contains(verbsOf(format), 'w')
}

// checkMessages reports error construction under internalDir that breaks
// the message rules:
//   - An error made from a sentinel puts it first, as "%w: detail", or
//     "%w: %w" with a cause. It never uses errors.Join.
//   - A cause is formatted with %w.
//   - Every message starts in lower case, has no trailing period and
//     doesn't say "failed to".
func checkMessages(internalDir string) ([]violation, error) {
	var out []violation
	err := eachFile(internalDir, func(rel string, fset *token.FileSet, f *ast.File) {
		report := func(n ast.Node, msg string) {
			out = append(out, violation{fmt.Sprintf("%s:%d", rel, fset.Position(n.Pos()).Line), msg})
		}
		ast.Inspect(f, func(n ast.Node) bool {
			if call, ok := n.(*ast.CallExpr); ok {
				checkCall(call, report)
			}
			return true
		})
	})
	return out, err
}

func checkCall(call *ast.CallExpr, report func(ast.Node, string)) {
	switch callee(call) {
	case "errors.New":
		checkMessage(call, 0, report)
	case "fault.New", "fault.Coded":
		checkMessage(call, len(call.Args)-1, report)
	case "errors.Join":
		if slices.ContainsFunc(call.Args, isSentinel) {
			report(call, `an error with its cause is fmt.Errorf("%w: %w"), not errors.Join`)
		}
	case "fmt.Errorf":
		if format, ok := literal(call, 0); ok {
			checkMessage(call, 0, report)
			checkCause(call, format, report)
		}
	}
}

// isSentinel reports whether e names a sentinel error, Err or pkg.Err.
func isSentinel(e ast.Expr) bool {
	if sel, ok := e.(*ast.SelectorExpr); ok {
		e = sel.Sel
	}
	id, ok := e.(*ast.Ident)
	return ok && sentinelName.MatchString(id.Name)
}

var sentinelName = regexp.MustCompile(`^Err[A-Z]`)

func callee(call *ast.CallExpr) string {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return ""
	}
	pkg, ok := sel.X.(*ast.Ident)
	if !ok {
		return ""
	}
	return pkg.Name + "." + sel.Sel.Name
}

// literal returns a call's argument i when it's a string literal.
func literal(call *ast.CallExpr, i int) (string, bool) {
	if i < 0 || i >= len(call.Args) {
		return "", false
	}
	bl, ok := call.Args[i].(*ast.BasicLit)
	if !ok || bl.Kind != token.STRING {
		return "", false
	}
	s, err := strconv.Unquote(bl.Value)
	return s, err == nil
}

// checkMessage reports the message in argument i of call if it starts in
// upper case, ends with a period or says "failed to".
func checkMessage(call *ast.CallExpr, i int, report func(ast.Node, string)) {
	msg, ok := literal(call, i)
	if !ok {
		return
	}
	if r, _ := utf8.DecodeRuneInString(msg); unicode.IsUpper(r) {
		report(call, fmt.Sprintf("error message %q starts in upper case", msg))
	}
	if strings.HasSuffix(msg, ".") {
		report(call, fmt.Sprintf("error message %q ends with a period", msg))
	}
	if strings.Contains(msg, "failed to") {
		report(call, fmt.Sprintf("error message %q says \"failed to\"", msg))
	}
}

// checkCause reports an error argument formatted with a verb other than %w.
// An argument counts as an error by its name: err, or a name ending in Err
// or err.
func checkCause(call *ast.CallExpr, format string, report func(ast.Node, string)) {
	verbs := verbsOf(format)
	for i, arg := range call.Args[1:] {
		id, ok := arg.(*ast.Ident)
		if !ok || i >= len(verbs) || verbs[i] == 'w' {
			continue
		}
		if name := strings.ToLower(id.Name); strings.HasSuffix(name, "err") {
			report(call, fmt.Sprintf("the error %s is formatted with %%%c, not %%w", id.Name, verbs[i]))
		}
	}
}

// verbsOf lists a format's verbs in order, skipping %%.
func verbsOf(format string) []rune {
	var out []rune
	for i := 0; i < len(format); i++ {
		if format[i] != '%' {
			continue
		}
		i++
		for i < len(format) && strings.ContainsRune("+-# 0123456789.*[]", rune(format[i])) {
			i++
		}
		if i < len(format) && format[i] != '%' {
			out = append(out, rune(format[i]))
		}
	}
	return out
}

// eachFile parses every non-test, hand-written Go file under internalDir.
func eachFile(internalDir string, fn func(rel string, fset *token.FileSet, f *ast.File)) error {
	return walkGo(internalDir, parser.ParseComments, func(rel string, fset *token.FileSet, f *ast.File) {
		if !ast.IsGenerated(f) {
			fn(rel, fset, f)
		}
	})
}
