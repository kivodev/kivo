package main

import (
	"bytes"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// operation is one route of the contract, as the app's request needs it.
type operation struct {
	id, summary, method, path string
	pathParams                []string // in the order the path names them
	query                     []queryParam
	body                      string // the request body's media type; empty without one
	bodySchema                *yaml.Node
	answerSchema              *yaml.Node // a JSON answer's schema, when it has one
	media                     string     // a success answer's media type when it isn't JSON
	public                    bool
	queryType, bodyType       string // set as the interfaces are made
	answerType                string
}

// queryParam is a query parameter of an operation.
type queryParam struct {
	name     string
	required bool
	schema   *yaml.Node
}

// enum is a named string enum of the contract.
type enum struct {
	name   string
	values []string
}

var methods = []string{"get", "put", "post", "patch", "delete", "head"}

// generate returns the BrighterScript sources for the spec: the
// operations, and the types (enums and interfaces) they and the app use.
// The types compile to nothing, so any file may import them.
func generate(spec []byte) (api, types []byte, err error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(spec, &doc); err != nil {
		return nil, nil, err
	}
	if len(doc.Content) == 0 {
		return nil, nil, errors.New("empty spec")
	}
	root := doc.Content[0]
	ops, err := operations(root)
	if err != nil {
		return nil, nil, err
	}
	g := newModeler(root)
	for i := range ops {
		op := &ops[i]
		if len(op.query) > 0 {
			op.queryType = g.query(op.id, op.query)
		}
		if op.body != "" {
			op.bodyType = "object"
			if op.body == "application/json" {
				op.bodyType = g.schemaType(op.id, "Body", op.bodySchema)
			}
		}
		if op.answerSchema != nil {
			op.answerType = g.schemaType(op.id, "Answer", op.answerSchema)
		}
	}
	api, types = render(ops, enums(root), g.all())
	return api, types, nil
}

func operations(root *yaml.Node) ([]operation, error) {
	params := lookup(lookup(root, "components"), "parameters")
	responses := lookup(lookup(root, "components"), "responses")
	paths := lookup(root, "paths")
	if paths == nil {
		return nil, errors.New("no paths")
	}
	var ops []operation
	seen := map[string]bool{}
	for i := 0; i+1 < len(paths.Content); i += 2 {
		path, item := paths.Content[i].Value, paths.Content[i+1]
		shared := lookup(item, "parameters")
		for _, m := range methods {
			node := lookup(item, m)
			if node == nil {
				continue
			}
			op, err := parseOperation(path, m, node, shared, params, responses)
			if err != nil {
				return nil, err
			}
			if seen[strings.ToLower(op.id)] {
				return nil, fmt.Errorf("operationId %s twice", op.id)
			}
			seen[strings.ToLower(op.id)] = true
			ops = append(ops, op)
		}
	}
	return ops, nil
}

var pathParam = regexp.MustCompile(`\{([^}]+)\}`)

func parseOperation(path, method string, node, shared, params, responses *yaml.Node) (operation, error) {
	op := operation{
		id:      scalar(node, "operationId"),
		summary: scalar(node, "summary"),
		method:  strings.ToUpper(method),
		path:    path,
	}
	if op.id == "" {
		return op, fmt.Errorf("%s %s has no operationId", op.method, path)
	}
	for _, m := range pathParam.FindAllStringSubmatch(path, -1) {
		op.pathParams = append(op.pathParams, m[1])
	}
	for _, list := range []*yaml.Node{shared, lookup(node, "parameters")} {
		if list == nil {
			continue
		}
		for _, p := range list.Content {
			p = resolve(p, params)
			if scalar(p, "in") == "query" {
				op.query = append(op.query, queryParam{scalar(p, "name"), scalar(p, "required") == "true", lookup(p, "schema")})
			}
		}
	}
	if body := lookup(lookup(node, "requestBody"), "content"); body != nil && len(body.Content) > 0 {
		op.body = body.Content[0].Value
		op.bodySchema = lookup(body.Content[1], "schema")
	}
	op.answerSchema, op.media = success(lookup(node, "responses"), responses)
	if security := lookup(node, "security"); security != nil && security.Kind == yaml.SequenceNode && len(security.Content) == 0 {
		op.public = true
	}
	return op, nil
}

// success returns the schema of an operation's first 2xx JSON answer, or,
// when that answer isn't JSON, its media type.
func success(list, shared *yaml.Node) (*yaml.Node, string) {
	if list == nil {
		return nil, ""
	}
	for i := 0; i+1 < len(list.Content); i += 2 {
		code := list.Content[i].Value
		if len(code) != 3 || code[0] != '2' {
			continue
		}
		r := list.Content[i+1]
		if ref := scalar(r, "$ref"); ref != "" {
			r = lookup(shared, strings.TrimPrefix(ref, "#/components/responses/"))
		}
		content := lookup(r, "content")
		if content == nil || len(content.Content) == 0 {
			return nil, ""
		}
		if json := lookup(content, "application/json"); json != nil {
			return lookup(json, "schema"), ""
		}
		return nil, content.Content[0].Value
	}
	return nil, ""
}

// resolve follows a parameter's $ref into components.parameters.
func resolve(p, params *yaml.Node) *yaml.Node {
	ref := scalar(p, "$ref")
	if ref == "" {
		return p
	}
	if r := lookup(params, strings.TrimPrefix(ref, "#/components/parameters/")); r != nil {
		return r
	}
	return p
}

func enums(root *yaml.Node) []enum {
	schemas := lookup(lookup(root, "components"), "schemas")
	if schemas == nil {
		return nil
	}
	var out []enum
	for i := 0; i+1 < len(schemas.Content); i += 2 {
		values := lookup(schemas.Content[i+1], "enum")
		if values == nil || scalar(schemas.Content[i+1], "type") != "string" {
			continue
		}
		e := enum{name: schemas.Content[i].Value}
		for _, v := range values.Content {
			e.values = append(e.values, v.Value)
		}
		out = append(out, e)
	}
	return out
}

func render(ops []operation, enums []enum, models []model) (api, types []byte) {
	var a, t bytes.Buffer
	a.WriteString(header)
	a.WriteString("import \"pkg:/source/generated/types.bs\"\n\nnamespace KivoApi\n")
	a.WriteString(helpers)
	for _, op := range ops {
		writeOperation(&a, op)
	}
	a.WriteString("end namespace\n")
	t.WriteString(typesHeader)
	t.WriteString("namespace KivoApi\n")
	t.WriteString(request)
	for _, e := range enums {
		writeEnum(&t, e)
	}
	for _, m := range models {
		writeModel(&t, m)
	}
	t.WriteString("end namespace\n")
	return a.Bytes(), t.Bytes()
}

const header = `' Generated from server/internal/api/openapi.yaml by make generate in
' server/ (internal/api/internal/brsgen). Don't edit.
'
' A function per operation returns the request to send, typed by its
' query, path and body; the app's request task sends it. One with a JSON
' answer has an Answer function that gives the response's body as its
' type, and a GET whose answer isn't JSON (an image, a stream, subtitles)
' has a Url function for the node that fetches it. The types are in
' types.bs.

`

const typesHeader = `' Generated from server/internal/api/openapi.yaml by make generate in
' server/ (internal/api/internal/brsgen). Don't edit.
'
' The API's types: an enum per named string enum and an interface per
' object schema, describing the JSON both ways. They exist only for the
' compiler (an answer is still the object ParseJson gives), so they cost
' nothing at run time and any file may import them.

`

const request = `
    ' A request to send: its method, its path from the server's address, its
    ' query and JSON body, the body's media type, and whether it carries the
    ' session's token.
    interface Request
        method as string
        path as string
        query as dynamic
        body as dynamic
        contentType as dynamic
        auth as boolean
    end interface
`

const helpers = `
    ' A path parameter, escaped for the URL.
    function pathEscape(value as string) as string
        return CreateObject("roUrlTransfer").Escape(value)
    end function

    ' A query's fields as a URL query, each escaped; an array's values each
    ' make a pair of their own. Empty without fields.
    function queryString(query as dynamic) as string
        if query = invalid or query.Count() = 0 then return ""
        transfer = CreateObject("roUrlTransfer")
        pairs = []
        for each key in query
            values = query[key]
            if type(values) <> "roArray" then values = [values]
            for each value in values
                if value <> invalid then pairs.Push(transfer.Escape(key) + "=" + transfer.Escape(value.ToStr()))
            end for
        end for
        if pairs.Count() = 0 then return ""
        return "?" + pairs.Join("&")
    end function
`

func writeOperation(b *bytes.Buffer, op operation) {
	var args, names []string
	for _, p := range op.pathParams {
		args = append(args, identifier(p)+" as string")
		names = append(names, identifier(p))
	}
	if op.body != "" {
		args = append(args, "body as "+qualified(op.bodyType))
	}
	if len(op.query) > 0 {
		args = append(args, "query = invalid as "+qualified(op.queryType))
	}
	fmt.Fprintf(b, "\n    ' %s: %s %s.\n", op.summary, op.method, op.path)
	if len(op.query) > 0 {
		var q []string
		for _, p := range op.query {
			q = append(q, p.name)
		}
		fmt.Fprintf(b, "    ' Query: %s.\n", strings.Join(q, ", "))
	}
	fmt.Fprintf(b, "    function %s(%s) as KivoApi.Request\n", op.id, strings.Join(args, ", "))
	query, body, media := "invalid", "invalid", "invalid"
	if len(op.query) > 0 {
		query = "query"
	}
	if op.body != "" {
		body, media = "body", quote(op.body)
	}
	fmt.Fprintf(b, "        return { method: %s, path: %s, query: %s, body: %s, contentType: %s, auth: %t }\n",
		quote(op.method), pathExpr(op.path), query, body, media, !op.public)
	b.WriteString("    end function\n")
	if op.answerType != "" {
		fmt.Fprintf(b, "\n    ' %s's answer, as its type.\n", op.id)
		fmt.Fprintf(b, "    function %sAnswer(response as object) as %s\n", op.id, qualified(op.answerType))
		b.WriteString("        return response.body\n    end function\n")
	}
	if op.media != "" && op.method == "GET" {
		urlArgs := append([]string{"base as string"}, args...)
		call := strings.Join(names, ", ")
		if len(op.query) > 0 {
			if call != "" {
				call += ", "
			}
			call += "query"
		}
		fmt.Fprintf(b, "\n    ' %s's URL from the server address [base], for a node that fetches its %s itself.\n", op.id, op.media)
		fmt.Fprintf(b, "    function %sUrl(%s) as string\n", op.id, strings.Join(urlArgs, ", "))
		fmt.Fprintf(b, "        request = KivoApi.%s(%s)\n", op.id, call)
		b.WriteString("        return base + request.path + KivoApi.queryString(request.query)\n    end function\n")
	}
}

// pathExpr is the path as a BrighterScript expression, each parameter escaped.
func pathExpr(path string) string {
	var parts []string
	rest := path
	for _, m := range pathParam.FindAllStringSubmatchIndex(path, -1) {
		offset := len(path) - len(rest)
		if lit := rest[:m[0]-offset]; lit != "" {
			parts = append(parts, quote(lit))
		}
		parts = append(parts, "KivoApi.pathEscape("+identifier(path[m[2]:m[3]])+")")
		rest = path[m[1]:]
	}
	if rest != "" {
		parts = append(parts, quote(rest))
	}
	return strings.Join(parts, " + ")
}

func writeEnum(b *bytes.Buffer, e enum) {
	fmt.Fprintf(b, "\n    enum %s\n", e.name)
	used := map[string]bool{}
	for _, v := range e.values {
		name := identifier(v)
		for n := 2; used[strings.ToLower(name)]; n++ {
			name = fmt.Sprintf("%s_%d", identifier(v), n)
		}
		used[strings.ToLower(name)] = true
		fmt.Fprintf(b, "        %s = %s\n", name, quote(v))
	}
	b.WriteString("    end enum\n")
}

// reserved are BrightScript's reserved words, which can't name a parameter or an enum member.
var reserved = map[string]bool{
	"and": true, "box": true, "createobject": true, "dim": true, "each": true, "else": true, "elseif": true, "end": true,
	"endfunction": true, "endif": true, "endsub": true, "endwhile": true, "eval": true, "exit": true, "exitwhile": true,
	"false": true, "for": true, "function": true, "getglobalaa": true, "getlastruncompileerror": true,
	"getlastrunruntimeerror": true, "goto": true, "if": true, "invalid": true, "let": true, "line_num": true, "m": true,
	"next": true, "not": true, "objfun": true, "or": true, "pos": true, "print": true, "rem": true, "return": true,
	"run": true, "step": true, "stop": true, "sub": true, "tab": true, "then": true, "to": true, "true": true,
	"type": true, "while": true, "mod": true, "in": true, "as": true,
}

var nonIdentifier = regexp.MustCompile(`[^A-Za-z0-9_]`)

// identifier makes a value a BrightScript name: other characters become
// underscores, and a leading digit or a reserved word gets one in front.
func identifier(v string) string {
	name := nonIdentifier.ReplaceAllString(v, "_")
	if name == "" || (name[0] >= '0' && name[0] <= '9') || reserved[strings.ToLower(name)] {
		name = "_" + name
	}
	return name
}

func quote(s string) string {
	return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
}

// lookup returns a mapping's value for key, or nil.
func lookup(n *yaml.Node, key string) *yaml.Node {
	if n == nil || n.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(n.Content); i += 2 {
		if n.Content[i].Value == key {
			return n.Content[i+1]
		}
	}
	return nil
}

func scalar(n *yaml.Node, key string) string {
	if v := lookup(n, key); v != nil && v.Kind == yaml.ScalarNode {
		return v.Value
	}
	return ""
}
