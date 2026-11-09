package archtest

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"maps"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

const module = "github.com/mmcdole/kivo"

// fault is the package of errors, in the lowest layer, so every package
// may import it.
const fault = "fault"

// ffmpeg is the package that runs ffmpeg and ffprobe. Only cmd/kivo may
// import it. Components call it through interfaces of their own.
const ffmpeg = "media/ffmpeg"

// egress is the only package that sends a request or dials. Any other
// package that calls out does so through its client.
const egress = "egress"

// store is the only package that begins, commits or rolls back a
// transaction. Every other package writes in the *store.Tx that
// store.Write hands it, and reads in store.Read's *sql.Tx.
const store = "store"

// layer is one of the architecture's layers, with its components.
type layer struct {
	Name       string
	Components []string
}

// layers are the architecture's layers, lowest first, as its layer diagram
// draws them. A component may import any component in its own layer or a
// lower one, but never one in a higher layer. cmd/kivo sits above them all
// and may import any. A component may always import its own subpackages.
// Every package under internal except archtest is in a layer.
var layers = []layer{
	{"helpers", []string{"fault", "id", "events", "health", "keylock", "atomicfile", "egress"}},
	{"domain", []string{"store", "config", "media", "discovery", "tasks", "users", "auth", "catalog", "activity",
		"query", "metadata", "userstate", "release"}},
	{"features", []string{"scan", "identify", "previews", "subtitles", "markers", "transcode", "playback"}},
	{"api", []string{"api"}},
}

// outbound are the names in net/http and net that send a request or dial.
// Only egress uses them.
var outbound = map[string][]string{
	"net/http": {"Client", "Transport", "DefaultClient", "DefaultTransport", "Get", "Head", "Post", "PostForm", "NewRequest", "NewRequestWithContext"},
	"net":      {"Dial", "DialTimeout", "Dialer", "DialIP", "DialTCP", "DialUDP", "DialUnix"},
}

// pure lists the packages that decide only from what they're given, with
// the internal packages each may import. They reach no database, ffmpeg or
// files, so they import none of impure either.
var pure = map[string][]string{
	"markers/detect": {"id", "media"},
}

// impure are the standard packages that reach outside the program.
var impure = []string{"database/sql", "io/fs", "net", "os", "os/exec", "syscall"}

// owns lists what each component may write, following the architecture's
// component table. An entry is a whole table, or a column of another
// component's table, such as "work.refreshed_at", which only its owner
// updates. query owns nothing and may only read. Its statements are in its
// Go code, so its own TestReadOnly checks them.
var owns = map[string][]string{
	"tasks":    {"task"},
	"activity": {"activity"},
	"config":   {"setting"},
	"users":    {"user", "library_grant", "user_library", "user_home_row", "list"},
	"auth":     {"credential", "session", "api_key", "cast_token", "audit", "invite", "invite_library"},
	"catalog": {"library", "library_folder", "media_file", "media_track", "media_chapter", "media_attachment", "work", "work_library",
		"work_external_id", "media_version", "media_version_work", "person", "person_external_id", "credit", "genre", "work_genre",
		"tag", "work_tag", "company", "work_company", "collection", "collection_work", "collection_fts", "work_fts", "person_fts",
		"book", "book_series", "book_series_book"},
	"metadata":  {"provider_cache", "image", "artwork", "work.refreshed_at", "work.refresh_error"},
	"userstate": {"user_state", "play_event", "track_choice", "list_work"},
	"scan": {"scan_dir", "scan_folder", "media_file.probed", "media_file.probe_error",
		"library_folder.reachable", "library_folder.last_scan_at"},
	"identify":  {"match", "work_folder", "media_file.identified"},
	"markers":   {"media_marker", "marker_file"},
	"previews":  {"preview_file"},
	"subtitles": {"subtitle_file"},
	"playback":  {"download"},
}

// violation is one broken rule, printed as "file: message".
type violation struct {
	File, Msg string
}

func (v violation) String() string { return v.File + ": " + v.Msg }

// checkLayers reports components placed in more than one layer.
func checkLayers(layers []layer) []violation {
	var out []violation
	seen := map[string]string{}
	for _, l := range layers {
		for _, c := range l.Components {
			if was, ok := seen[c]; ok {
				out = append(out, violation{"layers", fmt.Sprintf("%s is in both %s and %s", c, was, l.Name)})
			}
			seen[c] = l.Name
		}
	}
	return out
}

// layerOf returns the index of the layer c is in.
func layerOf(layers []layer, c string) (int, bool) {
	for i, l := range layers {
		if slices.Contains(l.Components, c) {
			return i, true
		}
	}
	return 0, false
}

// checkCycles reports cycles among components, based on how their files
// import each other. The compiler forbids cycles among packages, but not
// among components, whose subpackages could close a cycle.
func checkCycles(files []goFile) []violation {
	imports := map[string][]string{}
	for _, f := range files {
		from := component(f.Path)
		for _, imp := range f.Imports {
			if rest, ok := internal(imp); ok && component(rest) != from && !slices.Contains(imports[from], component(rest)) {
				imports[from] = append(imports[from], component(rest))
			}
		}
	}
	var out []violation
	const (
		unvisited = iota
		visiting
		done
	)
	state := map[string]int{}
	var path []string
	var visit func(string)
	visit = func(c string) {
		switch state[c] {
		case visiting:
			i := slices.Index(path, c)
			out = append(out, violation{"imports", "cycle: " + strings.Join(append(path[i:], c), " → ")})
			return
		case done:
			return
		}
		state[c] = visiting
		path = append(path, c)
		for _, d := range slices.Sorted(slices.Values(imports[c])) {
			visit(d)
		}
		path = path[:len(path)-1]
		state[c] = done
	}
	for _, c := range slices.Sorted(maps.Keys(imports)) {
		visit(c)
	}
	return out
}

// goFile is one non-test Go file under the internal directory. It holds the
// file's path and package directory, relative to the internal directory,
// and its import paths.
type goFile struct {
	Path, Pkg string
	Imports   []string
}

// readFiles parses the imports of every non-test Go file under internalDir.
// Test files may import anything, since they use other packages' test
// helpers.
func readFiles(internalDir string) ([]goFile, error) {
	var out []goFile
	err := walkGo(internalDir, parser.ImportsOnly, func(rel string, _ *token.FileSet, f *ast.File) {
		gf := goFile{Path: rel, Pkg: path.Dir(rel)}
		for _, imp := range f.Imports {
			gf.Imports = append(gf.Imports, strings.Trim(imp.Path.Value, `"`))
		}
		out = append(out, gf)
	})
	return out, err
}

// walkGo parses every non-test Go file under internalDir with mode, and
// calls fn with its path relative to internalDir.
func walkGo(internalDir string, mode parser.Mode, fn func(rel string, fset *token.FileSet, f *ast.File)) error {
	fset := token.NewFileSet()
	return filepath.WalkDir(internalDir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
			return nil
		}
		f, err := parser.ParseFile(fset, p, nil, mode)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(internalDir, p)
		fn(filepath.ToSlash(rel), fset, f)
		return nil
	})
}

// internal returns an import path's path under internal/, if it has one.
func internal(imp string) (string, bool) {
	return strings.CutPrefix(imp, module+"/internal/")
}

// checkImports reports imports of a component in a higher layer than the
// importer's, imports of media/ffmpeg from outside it, and components in
// no layer.
func checkImports(files []goFile, layers []layer) []violation {
	var out []violation
	unplaced := map[string]bool{}
	for _, f := range files {
		from := component(f.Path)
		if from == "archtest" {
			continue
		}
		at, placed := layerOf(layers, from)
		if !placed {
			if !unplaced[from] {
				out = append(out, violation{"layers", fmt.Sprintf("%s is in no layer", from)})
			}
			unplaced[from] = true
			continue
		}
		for _, imp := range f.Imports {
			if v, ok := checkImport(f.Path, imp, layers, at); ok {
				out = append(out, v)
			}
		}
	}
	return out
}

// checkImport checks one import by a file of a component in layer at. It
// reports an import of media/ffmpeg from outside it, or of a component in a
// higher layer.
func checkImport(file, imp string, layers []layer, at int) (violation, bool) {
	rest, ok := internal(imp)
	if !ok {
		return violation{}, false
	}
	if rest == ffmpeg && !strings.HasPrefix(file, ffmpeg+"/") {
		return violation{file, fmt.Sprintf("only cmd/kivo may import %s", imp)}, true
	}
	if above, ok := layerOf(layers, component(rest)); ok && above > at {
		return violation{file, fmt.Sprintf("%s, in %s, may not import %s, in %s", component(file), layers[at].Name, imp, layers[above].Name)}, true
	}
	return violation{}, false
}

// checkSubpackages reports api files importing another component's
// subpackage. api must reach every other component through its root
// package.
func checkSubpackages(files []goFile) []violation {
	var out []violation
	for _, f := range files {
		if component(f.Path) != "api" {
			continue
		}
		for _, imp := range f.Imports {
			rest, ok := internal(imp)
			if !ok || component(rest) == "api" || rest == component(rest) {
				continue
			}
			out = append(out, violation{f.Path, fmt.Sprintf("api may import only %s's root package, not %s", component(rest), imp)})
		}
	}
	return out
}

// checkPure reports files of a pure package that import an internal
// package outside its list, or an impure standard package or one under it.
// It also reports pure packages with no files.
func checkPure(files []goFile, pure map[string][]string) []violation {
	var out []violation
	seen := map[string]bool{}
	for _, f := range files {
		allowed, ok := pure[f.Pkg]
		if !ok {
			continue
		}
		seen[f.Pkg] = true
		for _, imp := range f.Imports {
			rest, in := internal(imp)
			if in && !slices.Contains(allowed, rest) && rest != fault ||
				slices.ContainsFunc(impure, func(p string) bool { return imp == p || strings.HasPrefix(imp, p+"/") }) {
				out = append(out, violation{f.Path, fmt.Sprintf("pure package %s may not import %s", f.Pkg, imp)})
			}
		}
	}
	for _, p := range slices.Sorted(maps.Keys(pure)) {
		if !seen[p] {
			out = append(out, violation{"pure", fmt.Sprintf("%s has no files", p)})
		}
	}
	return out
}

// checkOutbound reports each use of a name in names in a non-test file
// outside egress. names is keyed by import path.
func checkOutbound(internalDir string, names map[string][]string) ([]violation, error) {
	var out []violation
	err := eachFile(internalDir, func(rel string, fset *token.FileSet, f *ast.File) {
		if path.Dir(rel) == egress {
			return
		}
		local := map[string]string{} // the import path of each package in names, by the file's name for it
		for _, imp := range f.Imports {
			p := strings.Trim(imp.Path.Value, `"`)
			if _, ok := names[p]; !ok {
				continue
			}
			name := path.Base(p)
			if imp.Name != nil {
				name = imp.Name.Name
			}
			local[name] = p
		}
		ast.Inspect(f, func(n ast.Node) bool {
			sel, ok := n.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			pkg, ok := sel.X.(*ast.Ident)
			if !ok {
				return true
			}
			if p, in := local[pkg.Name]; in && slices.Contains(names[p], sel.Sel.Name) {
				at := fmt.Sprintf("%s:%d", rel, fset.Position(sel.Pos()).Line)
				out = append(out, violation{at, fmt.Sprintf("%s.%s calls out: call out through %s", p, sel.Sel.Name, egress)})
			}
			return true
		})
	})
	return out, err
}

func component(rel string) string {
	c, _, _ := strings.Cut(rel, "/")
	return c
}

var (
	lineComment  = regexp.MustCompile(`--[^\n]*`)
	blockComment = regexp.MustCompile(`(?s)/\*.*?\*/`)
	writeTarget  = regexp.MustCompile(`(?i)\b(insert(?:\s+or\s+\w+)?\s+into|replace\s+into|update(?:\s+or\s+\w+)?|delete\s+from)\s+["\x60\[]?(\w+)["\x60\]]?`)
	columnList   = regexp.MustCompile(`^\s*\(([^)]*)\)`)
	setKw        = regexp.MustCompile(`(?i)\bset\b`)
	identifier   = regexp.MustCompile(`\w+`)
	statementKw  = regexp.MustCompile(`(?i)^\s*(\w+)`)
	readOnlyKw   = []string{"select", "with", "values", "explain"}
	forbiddenKw  = regexp.MustCompile(`(?i)\b(insert|update|delete|replace|create|drop|alter|pragma|attach|vacuum)\b`)
)

// checkWrites scans each component's queries/*.sql under internalDir. A
// component may write a table it owns, except the columns another
// component owns in it. It may update the columns it owns in another
// component's table, and nothing else there. A component that owns nothing
// may only read.
func checkWrites(internalDir string, owns map[string][]string) ([]violation, error) {
	columnOwner := map[string]string{}
	for comp, owned := range owns {
		for _, o := range owned {
			if strings.Contains(o, ".") {
				columnOwner[o] = comp
			}
		}
	}
	var out []violation
	files, err := filepath.Glob(filepath.Join(internalDir, "*", "queries", "*.sql"))
	if err != nil {
		return nil, err
	}
	for _, p := range files {
		rel, _ := filepath.Rel(internalDir, p)
		rel = filepath.ToSlash(rel)
		comp := component(rel)
		b, err := os.ReadFile(p)
		if err != nil {
			return nil, err
		}
		sql := blockComment.ReplaceAllString(lineComment.ReplaceAllString(string(b), ""), "")
		owned, ownsAny := owns[comp]
		for stmt := range strings.SplitSeq(sql, ";") {
			if strings.TrimSpace(stmt) == "" {
				continue
			}
			if !ownsAny {
				kw := strings.ToLower(statementKw.FindStringSubmatch(stmt)[1])
				if !slices.Contains(readOnlyKw, kw) || forbiddenKw.MatchString(stmt) {
					out = append(out, violation{rel, fmt.Sprintf("%s owns no tables and may only read: %q", comp, firstLine(stmt))})
				}
				continue
			}
			for _, w := range writes(stmt) {
				if msg := checkWrite(comp, owned, columnOwner, w); msg != "" {
					out = append(out, violation{rel, msg})
				}
			}
		}
	}
	return out, nil
}

// tableWrite is a table that a statement writes, and the columns it names.
// A delete, or an insert that names no columns, has none.
type tableWrite struct {
	table   string
	update  bool
	columns []string
}

// checkWrite returns why comp may not make write w, or "" if it may.
func checkWrite(comp string, owned []string, columnOwner map[string]string, w tableWrite) string {
	if slices.Contains(owned, w.table) {
		for _, c := range w.columns {
			if o, ok := columnOwner[w.table+"."+c]; ok && o != comp {
				return fmt.Sprintf("%s may not write %s.%s, which %s owns", comp, w.table, c, o)
			}
		}
		return ""
	}
	if !w.update {
		return fmt.Sprintf("%s may not write table %s", comp, w.table)
	}
	for _, c := range w.columns {
		if !slices.Contains(owned, w.table+"."+c) {
			return fmt.Sprintf("%s may not write %s.%s", comp, w.table, c)
		}
	}
	return ""
}

// writes lists the writes of one statement. An upsert's DO UPDATE SET adds
// its columns to those of its insert.
func writes(stmt string) []tableWrite {
	var out []tableWrite
	for _, m := range writeTarget.FindAllStringSubmatchIndex(stmt, -1) {
		verb := strings.ToLower(stmt[m[2]:m[3]])
		table := strings.ToLower(stmt[m[4]:m[5]])
		rest := stmt[m[1]:]
		switch {
		case strings.HasPrefix(verb, "update") && table == "set": // ON CONFLICT … DO UPDATE SET
			if len(out) > 0 {
				out[len(out)-1].columns = append(out[len(out)-1].columns, setColumns(rest)...)
			}
		case strings.HasPrefix(verb, "update"):
			w := tableWrite{table: table, update: true}
			if loc := setKw.FindStringIndex(rest); loc != nil {
				w.columns = setColumns(rest[loc[1]:])
			}
			out = append(out, w)
		case strings.HasPrefix(verb, "delete"):
			out = append(out, tableWrite{table: table})
		default:
			w := tableWrite{table: table}
			if c := columnList.FindStringSubmatch(rest); c != nil {
				w.columns = identifier.FindAllString(strings.ToLower(c[1]), -1)
			}
			out = append(out, w)
		}
	}
	return out
}

// setColumns returns the columns a SET clause assigns. It reads s up to
// the clause's end, which is a WHERE, FROM or RETURNING outside
// parentheses.
func setColumns(s string) []string {
	var cols []string
	depth, start := 0, 0
	assign := func(end int) {
		lhs, _, _ := strings.Cut(s[start:end], "=")
		cols = append(cols, identifier.FindAllString(strings.ToLower(lhs), -1)...)
	}
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '(':
			depth++
		case ')':
			depth--
		case ',':
			if depth == 0 {
				assign(i)
				start = i + 1
			}
		}
		if depth == 0 && (i == 0 || !isWord(s[i-1])) && clauseEnd(s[i:]) {
			assign(i)
			return cols
		}
	}
	assign(len(s))
	return cols
}

// clauseEnd reports whether s starts with a keyword that ends a SET clause.
func clauseEnd(s string) bool {
	for _, kw := range []string{"where", "from", "returning"} {
		if len(s) >= len(kw) && strings.EqualFold(s[:len(kw)], kw) && (len(s) == len(kw) || !isWord(s[len(kw)])) {
			return true
		}
	}
	return false
}

func isWord(c byte) bool {
	return c == '_' || '0' <= c && c <= '9' || 'a' <= c && c <= 'z' || 'A' <= c && c <= 'Z'
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}
