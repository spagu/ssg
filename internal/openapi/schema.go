package openapi

import (
	"slices"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// schemaPrefix is where named component schemas live.
const schemaPrefix = "#/components/schemas/"

// Schema is a render-ready JSON Schema subset. A reference to a component
// schema fills only Ref; renderers link to the named schema instead of
// inlining it, which keeps recursive schemas finite.
type Schema struct {
	Ref                  string // component schema name when this is a $ref, else ""
	Type                 string // "object", "string | null", ...
	Format               string
	Description          string
	Enum                 []any
	Default              any
	Example              any // "example", or the first of 3.1 "examples"
	Required             []string
	Properties           []Property // document order
	Items                *Schema
	AdditionalProperties *Schema // nil when absent or false; empty Schema when true
	OneOf                []*Schema
	AnyOf                []*Schema
	AllOf                []*Schema
	Deprecated           bool
	ReadOnly             bool
	WriteOnly            bool
	Minimum              *float64
	Maximum              *float64
	MinLength            *int
	MaxLength            *int
	Pattern              string
}

// Property is one named property of an object schema.
type Property struct {
	Name     string
	Required bool
	Schema   *Schema
}

// TypeString is a short human type for tables: the Ref name, "array of X",
// "string (date-time)", "oneOf A | B", "allOf A & B", "object" or "any".
func (s *Schema) TypeString() string {
	if s == nil {
		return ""
	}
	if rest, ok := strings.CutPrefix(s.Type, "array"); ok && s.Items != nil {
		return "array of " + s.Items.TypeString() + rest
	}
	switch {
	case s.Ref != "":
		return s.Ref
	case len(s.OneOf) > 0:
		return "oneOf " + joinTypes(s.OneOf, " | ")
	case len(s.AnyOf) > 0:
		return "anyOf " + joinTypes(s.AnyOf, " | ")
	case len(s.AllOf) > 0:
		return "allOf " + joinTypes(s.AllOf, " & ")
	case s.Type != "" && s.Format != "":
		return s.Type + " (" + s.Format + ")"
	case s.Type != "":
		return s.Type
	case len(s.Properties) > 0 || s.AdditionalProperties != nil:
		return "object"
	}
	return "any"
}

// joinTypes joins the TypeStrings of several schemas.
func joinTypes(list []*Schema, sep string) string {
	parts := make([]string, 0, len(list))
	for _, s := range list {
		parts = append(parts, s.TypeString())
	}
	return strings.Join(parts, sep)
}

// schema reads the schema object n located at at; nil when absent.
func (p *parser) schema(n *yaml.Node, at string) *Schema { return p.schemaAt(n, at, 0) }

// schemaAt is schema with a nesting-depth guard.
func (p *parser) schemaAt(n *yaml.Node, at string, depth int) *Schema {
	n = alias(n)
	if n == nil {
		return nil
	}
	if depth > maxDepth {
		p.diag(at, "schema nested deeper than %d", maxDepth)
		return nil
	}
	if n.Kind == yaml.ScalarNode && n.Value == "true" {
		return &Schema{} // 3.1 boolean schema / additionalProperties: true
	}
	if n.Kind != yaml.MappingNode {
		return nil
	}
	if ref, ok := refOf(n); ok {
		return p.schemaRef(n, ref, at, depth)
	}
	s := &Schema{
		Type:        schemaType(n),
		Format:      str(get(n, "format")),
		Description: str(get(n, "description")),
		Default:     toAny(get(n, "default")),
		Example:     toAny(get(n, "example")),
		Required:    strs(get(n, "required")),
		Deprecated:  flag(get(n, "deprecated")),
		ReadOnly:    flag(get(n, "readOnly")),
		WriteOnly:   flag(get(n, "writeOnly")),
		Minimum:     number(get(n, "minimum")),
		Maximum:     number(get(n, "maximum")),
		MinLength:   integer(get(n, "minLength")),
		MaxLength:   integer(get(n, "maxLength")),
		Pattern:     str(get(n, "pattern")),
	}
	for _, v := range items(get(n, "enum")) {
		s.Enum = append(s.Enum, toAny(v))
	}
	if examples := items(get(n, "examples")); s.Example == nil && len(examples) > 0 {
		s.Example = toAny(examples[0])
	}
	for _, e := range pairs(get(n, "properties")) {
		s.Properties = append(s.Properties, Property{
			Name:     e.key,
			Required: slices.Contains(s.Required, e.key),
			Schema:   p.schemaAt(e.value, pointer(at, "properties", e.key), depth+1),
		})
	}
	s.Items = p.schemaAt(get(n, "items"), at+"/items", depth+1)
	s.AdditionalProperties = p.schemaAt(get(n, "additionalProperties"), at+"/additionalProperties", depth+1)
	s.OneOf = p.schemaList(get(n, "oneOf"), at+"/oneOf", depth+1)
	s.AnyOf = p.schemaList(get(n, "anyOf"), at+"/anyOf", depth+1)
	s.AllOf = p.schemaList(get(n, "allOf"), at+"/allOf", depth+1)
	return s
}

// schemaRef turns a $ref into a named link (component schemas) or inlines
// any other local target; external refs are skipped with a diagnostic.
func (p *parser) schemaRef(n *yaml.Node, ref, at string, depth int) *Schema {
	if name, ok := strings.CutPrefix(ref, schemaPrefix); ok && !strings.Contains(name, "/") {
		name = unescape(name)
		if !p.hasComponent("schemas", name) {
			p.diag(at, "unknown schema %q", ref)
		}
		return &Schema{Ref: name}
	}
	target := p.deref(n, at)
	if target == nil {
		return nil
	}
	return p.schemaAt(target, at, depth+1)
}

// schemaList reads a oneOf/anyOf/allOf list, dropping unreadable entries.
func (p *parser) schemaList(n *yaml.Node, base string, depth int) []*Schema {
	var out []*Schema
	for i, item := range items(n) {
		if s := p.schemaAt(item, pointer(base, strconv.Itoa(i)), depth); s != nil {
			out = append(out, s)
		}
	}
	return out
}

// schemaType renders "type" (a string or a 3.1 list) plus 3.0 nullable.
func schemaType(n *yaml.Node) string {
	t := get(n, "type")
	typ := str(t)
	if list := strs(t); len(list) > 0 {
		typ = strings.Join(list, " | ")
	}
	if flag(get(n, "nullable")) && typ != "" && !slices.Contains(strings.Split(typ, " | "), "null") {
		typ += " | null"
	}
	return typ
}
