package config

// The configuration schema (GO-177): every key ssg reads, built by reflection
// from the configuration types themselves, so a key cannot exist in the code
// and be missing from the schema. The per-key text — description, form group,
// class — lives beside it in schema_docs_*.go, and a test fails the build of
// any field that has none. `ssg config schema` prints it; designer-ui reads a
// generated twin of it (workers/designer-ui/src/config-schema.json).

import (
	"encoding/json"
	"reflect"
	"sort"
	"strings"
)

// SchemaVersion is the shape version of the JSON below. It changes only when
// a field of SchemaNode or Schema changes meaning, never when a key is added.
const SchemaVersion = 1

// Key classes. Every named key has exactly one; there is no default, so a key
// nobody classified is a failing test rather than a silent "plain".
const (
	// ClassPlain is an ordinary setting.
	ClassPlain = "plain"
	// ClassExecutes runs a command, deploys, changes what a server does,
	// names a program, file or directory the build reads, can send visitors
	// to another place (redirects, link rewrites, the domain), or weakens a
	// security guard. designer-ui asks a separate permission for these.
	ClassExecutes = "executes"
	// ClassSecret carries a credential: only a $VARIABLE reference may be
	// written, never a literal.
	ClassSecret = "secret"
)

// Secret formats: what a secret-class value must look like.
const (
	// SecretWhole is "$NAME" or "${NAME}" — the whole value.
	SecretWhole = "whole"
	// SecretLogin is "login:$NAME" (server_users).
	SecretLogin = "login"
	// SecretInline is any text holding a reference, e.g. "Bearer $TOKEN".
	SecretInline = "inline"
)

// SchemaNode is one key, or the anonymous element of a list or map.
//
// Path is dotted; "[]" stands for any list element and "*" for any map key:
// "feeds[].merchant.brand", "external_sources.sources.*.auth.token". Named
// keys (object fields) carry description, group and class; an anonymous
// element (items, values, alternate) carries its type and group and takes
// description and class from the key that owns it. Nested keys inherit the
// group of their top-level key: a form section holds a key with all of it.
type SchemaNode struct {
	Path         string        `json:"path"`
	Key          string        `json:"key,omitempty"`
	Type         string        `json:"type"` // bool | int | number | string | enum | list | map | object | any
	Enum         []string      `json:"enum,omitempty"`
	Default      any           `json:"default,omitempty"`
	Description  string        `json:"description,omitempty"`
	Group        string        `json:"group,omitempty"`
	Class        string        `json:"class,omitempty"`
	SecretFormat string        `json:"secretFormat,omitempty"`
	MCPWritable  bool          `json:"mcpWritable,omitempty"`
	Open         bool          `json:"open,omitempty"` // object: keys beyond fields are accepted, any value
	Items        *SchemaNode   `json:"items,omitempty"`
	Values       *SchemaNode   `json:"values,omitempty"`
	Fields       []*SchemaNode `json:"fields,omitempty"`
	Alternate    *SchemaNode   `json:"alternate,omitempty"` // a second accepted form: `story: true`, `prefix: /blog/`
}

// SchemaGroup is one tab section of the options form.
type SchemaGroup struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
}

// Schema is the whole document `ssg config schema` prints.
type Schema struct {
	SchemaVersion int           `json:"schemaVersion"`
	OverlayFile   string        `json:"overlayFile"`
	Groups        []SchemaGroup `json:"groups"`
	Keys          []*SchemaNode `json:"keys"`
	problems      []string      // fields without docs, docs without fields
	byPath        map[string]*SchemaNode
}

// BuildSchema reflects over Config and DefaultConfig.
func BuildSchema() *Schema {
	b := &schemaBuilder{used: map[string]bool{}}
	root := b.node(reflect.TypeOf(Config{}), "", reflect.ValueOf(*DefaultConfig()), "")
	s := &Schema{
		SchemaVersion: SchemaVersion,
		OverlayFile:   OverlayFileName,
		Groups:        schemaGroups,
		Keys:          root.Fields,
		problems:      b.problems,
		byPath:        map[string]*SchemaNode{},
	}
	for owner, docs := range schemaDocs {
		for key := range docs {
			if !b.used[owner+"."+key] {
				s.problems = append(s.problems, "documented but not a field: "+owner+"."+key)
			}
		}
	}
	s.problems = append(s.problems, schemaDocDuplicates...)
	sort.Strings(s.problems)
	walkSchema(s.Keys, func(n *SchemaNode) {
		if _, taken := s.byPath[n.Path]; n.Key != "" && !taken { // the key, not its alternate form
			s.byPath[n.Path] = n
		}
	})
	return s
}

// SchemaJSON is the schema as indented JSON with a trailing newline — the
// bytes `ssg config schema` prints and the designer-ui twin holds.
func SchemaJSON() []byte {
	out, _ := json.MarshalIndent(BuildSchema(), "", "  ") // plain data: cannot fail
	return append(out, '\n')
}

// Node returns the named key at path ("marketing.logo"), or nil.
func (s *Schema) Node(path string) *SchemaNode { return s.byPath[path] }

// walkSchema visits every node, depth first, alternates and elements included.
func walkSchema(nodes []*SchemaNode, visit func(*SchemaNode)) {
	for _, n := range nodes {
		if n == nil {
			continue
		}
		visit(n)
		walkSchema(n.Fields, visit)
		walkSchema([]*SchemaNode{n.Items, n.Values, n.Alternate}, visit)
	}
}

// schemaBuilder carries what the walk records on the side.
type schemaBuilder struct {
	used     map[string]bool
	problems []string
}

// node builds the node for type t at path; def is the default value when it
// is known (a struct-field path from DefaultConfig), group the inherited one.
func (b *schemaBuilder) node(t reflect.Type, path string, def reflect.Value, group string) *SchemaNode {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
		def = derefValue(def)
	}
	n := &SchemaNode{Path: path, Group: group}
	if sh, ok := schemaShapes[t]; ok {
		t, n.Open, n.Enum = sh.t, sh.open, sh.enum
		def = reflect.Value{} // the default lives in the public type, not the shape
		if sh.alt != nil {
			n.Alternate = b.node(sh.alt, path, reflect.Value{}, group)
		}
	}
	switch t.Kind() {
	case reflect.Bool:
		n.Type = "bool"
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		n.Type = "int"
	case reflect.Float32, reflect.Float64:
		n.Type = "number"
	case reflect.String:
		n.Type = "string"
	case reflect.Slice, reflect.Array:
		n.Type = "list"
		n.Items = b.node(t.Elem(), path+"[]", reflect.Value{}, group)
	case reflect.Map:
		n.Type = "map"
		n.Values = b.node(t.Elem(), path+".*", reflect.Value{}, group)
	case reflect.Struct:
		n.Type = "object"
		n.Fields = b.fields(t, path, def, group)
	default: // interface{}: any value
		n.Type = "any"
	}
	if len(n.Enum) > 0 {
		n.Type = "enum"
	}
	if def.IsValid() && n.Type != "object" && n.Type != "list" && n.Type != "map" {
		n.Default = def.Interface()
	}
	return n
}

// fields builds the named keys of struct t.
func (b *schemaBuilder) fields(t reflect.Type, path string, def reflect.Value, group string) []*SchemaNode {
	owner := t.String()
	var out []*SchemaNode
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		key := strings.Split(f.Tag.Get("yaml"), ",")[0]
		if !f.IsExported() || key == "-" {
			continue
		}
		if key == "" {
			b.problems = append(b.problems, "field without a yaml tag: "+owner+"."+f.Name)
			continue
		}
		d, ok := schemaDocs[owner][key]
		b.used[owner+"."+key] = true
		childPath := joinPath(path, key)
		if !ok {
			b.problems = append(b.problems, "undocumented: "+childPath+" ("+owner+"."+key+")")
		}
		g := group
		if path == "" {
			g = d.group
		}
		var fdef reflect.Value
		if def.IsValid() {
			fdef = def.Field(i)
		}
		n := b.node(f.Type, childPath, fdef, g)
		n.Key, n.Description, n.Class = key, d.desc, d.class
		if n.Default == nil && d.unset != nil {
			n.Default = d.unset
		}
		n.SecretFormat, n.MCPWritable = d.secret, d.mcp
		if n.Class == ClassSecret && n.SecretFormat == "" {
			n.SecretFormat = SecretWhole
		}
		if len(d.enum) > 0 {
			n.Enum, n.Type = d.enum, "enum"
		}
		if d.alt != nil {
			n.Alternate = b.node(d.alt, childPath, reflect.Value{}, g)
		}
		b.checkDoc(childPath, n, d, ok)
		out = append(out, n)
	}
	return out
}

// checkDoc records a documented key whose entry is incomplete.
func (b *schemaBuilder) checkDoc(path string, n *SchemaNode, d keyDoc, documented bool) {
	if !documented {
		return
	}
	if d.desc == "" || d.class == "" || n.Group == "" {
		b.problems = append(b.problems, "incomplete entry (description, class or group): "+path)
	}
}

// joinPath appends key to a dotted path.
func joinPath(path, key string) string {
	if path == "" {
		return key
	}
	return path + "." + key
}

// derefValue follows a non-nil pointer; a nil one has no default.
func derefValue(v reflect.Value) reflect.Value {
	if !v.IsValid() || v.Kind() != reflect.Pointer || v.IsNil() {
		return reflect.Value{}
	}
	return v.Elem()
}
