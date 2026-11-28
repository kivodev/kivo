package main

import (
	"bytes"
	"fmt"
	"sort"
	"strings"
	"unicode"

	"gopkg.in/yaml.v3"
)

// model is an object schema of the contract, as a BrighterScript interface.
type model struct {
	name   string
	fields []field
}

// field is a property of a model: its name, its BrighterScript type and
// whether the server may leave it out.
type field struct {
	name, typ string
	optional  bool
}

// models returns an interface per object schema, with allOf flattened and
// an inline object property as an interface of its own, named after its
// model and property (WorkUpdate's book is WorkUpdateBook). A oneOf schema
// has no interface: its one use is the live events socket, which the Roku
// app doesn't open.
func models(root *yaml.Node) []model {
	return newModeler(root).all()
}

// newModeler makes the interfaces of every object schema; an operation's
// query and inline bodies add theirs as they're met.
func newModeler(root *yaml.Node) *modeler {
	schemas := lookup(lookup(root, "components"), "schemas")
	g := &modeler{schemas: schemas, taken: map[string]bool{}, inline: map[*yaml.Node]string{}}
	if schemas == nil {
		return g
	}
	for i := 0; i+1 < len(schemas.Content); i += 2 {
		g.taken[strings.ToLower(schemas.Content[i].Value)] = true
	}
	for i := 0; i+1 < len(schemas.Content); i += 2 {
		name, s := schemas.Content[i].Value, schemas.Content[i+1]
		if g.isObject(s) {
			g.object(name, s)
		}
	}
	return g
}

// all returns the interfaces made, by name.
func (g *modeler) all() []model {
	out := append([]model(nil), g.out...)
	sort.SliceStable(out, func(i, j int) bool { return out[i].name < out[j].name })
	return out
}

// query adds an interface for an operation's query parameters, named after
// it (listLibraries' is ListLibrariesQuery), and returns its name.
func (g *modeler) query(op string, params []queryParam) string {
	name := g.nested(pascal(op) + "Query")
	m := model{name: name}
	for _, p := range params {
		m.fields = append(m.fields, field{name: p.name, typ: g.typeOf(name, p.name, p.schema), optional: !p.required})
	}
	g.out = append(g.out, m)
	return name
}

// schemaType is the type of a request or answer body: a named schema's
// interface, or the type of an inline one.
func (g *modeler) schemaType(op, role string, s *yaml.Node) string {
	if s == nil {
		return "object"
	}
	t := g.typeOf(pascal(op), role, s)
	if t == "dynamic" || strings.Contains(t, " or ") {
		return "object"
	}
	return t
}

type modeler struct {
	schemas *yaml.Node
	taken   map[string]bool
	inline  map[*yaml.Node]string // an inline object's interface, made once
	out     []model
}

// isObject reports whether a schema becomes an interface: an object, or an
// allOf of objects.
func (g *modeler) isObject(s *yaml.Node) bool {
	if lookup(s, "oneOf") != nil || lookup(s, "anyOf") != nil {
		return false
	}
	return scalar(s, "type") == "object" || lookup(s, "properties") != nil || lookup(s, "allOf") != nil
}

func (g *modeler) object(name string, s *yaml.Node) {
	m := model{name: name}
	props, required := g.flatten(name, s, map[string]bool{})
	for _, p := range props {
		m.fields = append(m.fields, field{name: p.name, typ: g.typeOf(p.owner, p.name, p.schema), optional: !required[p.name]})
	}
	g.out = append(g.out, m)
}

// property is a schema's property, with the schema that declares it,
// which names an inline object's interface.
type property struct {
	name, owner string
	schema      *yaml.Node
}

// flatten returns a schema's properties in order, allOf's parts first, and
// the names it requires.
func (g *modeler) flatten(owner string, s *yaml.Node, seen map[string]bool) ([]property, map[string]bool) {
	required := map[string]bool{}
	var props []property
	add := func(ps []property, req map[string]bool) {
		for _, p := range ps {
			replaced := false
			for i := range props {
				if props[i].name == p.name {
					props[i], replaced = p, true
				}
			}
			if !replaced {
				props = append(props, p)
			}
		}
		for k := range req {
			required[k] = true
		}
	}
	if all := lookup(s, "allOf"); all != nil {
		for _, part := range all.Content {
			partOwner := owner
			if ref := scalar(part, "$ref"); ref != "" {
				target := refName(ref)
				if seen[target] {
					continue
				}
				seen[target] = true
				part, partOwner = lookup(g.schemas, target), target
			}
			add(g.flatten(partOwner, part, seen))
		}
	}
	var own []property
	if p := lookup(s, "properties"); p != nil {
		for i := 0; i+1 < len(p.Content); i += 2 {
			own = append(own, property{p.Content[i].Value, owner, p.Content[i+1]})
		}
	}
	req := map[string]bool{}
	if r := lookup(s, "required"); r != nil {
		for _, n := range r.Content {
			req[n.Value] = true
		}
	}
	add(own, req)
	return props, required
}

// typeOf is a property's BrighterScript type. ParseJson gives a whole
// number as an Integer or, past its range, a LongInteger, and any other
// number as a Float or Double, so the types say so.
func (g *modeler) typeOf(owner, prop string, s *yaml.Node) string {
	if s == nil {
		return "dynamic"
	}
	if ref := scalar(s, "$ref"); ref != "" {
		name := refName(ref)
		target := lookup(g.schemas, name)
		if target == nil {
			return "dynamic"
		}
		if g.isObject(target) {
			return name
		}
		if lookup(target, "enum") != nil && scalar(target, "type") == "string" {
			return name
		}
		if lookup(target, "oneOf") != nil || lookup(target, "anyOf") != nil {
			return "object"
		}
		return g.typeOf(owner, prop, target)
	}
	if lookup(s, "allOf") != nil && lookup(s, "properties") == nil {
		// A reference wrapped to add a description.
		if parts := lookup(s, "allOf").Content; len(parts) == 1 {
			return g.typeOf(owner, prop, parts[0])
		}
	}
	switch scalar(s, "type") {
	case "string":
		return "string"
	case "boolean":
		return "boolean"
	case "integer":
		if scalar(s, "format") == "int64" {
			return "integer or longinteger"
		}
		return "integer"
	case "number":
		return "integer or float or double"
	case "array":
		item := g.typeOf(owner, prop, lookup(s, "items"))
		if strings.Contains(item, " or ") || item == "dynamic" {
			return "object"
		}
		return item + "[]"
	case "object":
		if lookup(s, "properties") == nil {
			return "object"
		}
		return g.inlineObject(owner, prop, s)
	}
	if lookup(s, "properties") != nil || lookup(s, "allOf") != nil {
		return g.inlineObject(owner, prop, s)
	}
	return "dynamic"
}

// inlineObject is the interface of an inline object, made the first time
// it's met.
func (g *modeler) inlineObject(owner, prop string, s *yaml.Node) string {
	if name, ok := g.inline[s]; ok {
		return name
	}
	name := g.nested(owner + pascal(prop))
	g.inline[s] = name
	g.object(name, s)
	return name
}

// nested is a free name for an inline object's interface.
func (g *modeler) nested(name string) string {
	candidate := name
	for n := 2; g.taken[strings.ToLower(candidate)]; n++ {
		candidate = fmt.Sprintf("%s%d", name, n)
	}
	g.taken[strings.ToLower(candidate)] = true
	return candidate
}

func refName(ref string) string {
	return strings.TrimPrefix(ref, "#/components/schemas/")
}

func pascal(s string) string {
	r := []rune(identifier(s))
	if len(r) > 0 {
		r[0] = unicode.ToUpper(r[0])
	}
	return string(r)
}

func writeModel(b *bytes.Buffer, m model) {
	fmt.Fprintf(b, "\n    interface %s\n", m.name)
	for _, f := range m.fields {
		opt := ""
		if f.optional {
			opt = "optional "
		}
		fmt.Fprintf(b, "        %s%s as %s\n", opt, f.name, qualified(f.typ))
	}
	b.WriteString("    end interface\n")
}

var builtin = map[string]bool{
	"string": true, "boolean": true, "integer": true, "longinteger": true, "float": true, "double": true,
	"object": true, "dynamic": true,
}

// qualified names a generated type with its namespace, as the 0.x compiler
// needs to find an interface in a signature: Summary[] is KivoApi.Summary[].
func qualified(t string) string {
	parts := strings.Split(t, " or ")
	for i, p := range parts {
		base := strings.TrimSuffix(p, "[]")
		if !builtin[base] {
			parts[i] = "KivoApi." + p
		}
	}
	return strings.Join(parts, " or ")
}
