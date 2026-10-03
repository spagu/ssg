package openapipage

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spagu/ssg/internal/openapi"
)

// maxDepth bounds nested property tables and generated examples: deeper
// structure is reached through the schema links.
const maxDepth = 3

// sampleDepth bounds generated examples, which recursive schemas would
// otherwise make endless.
const sampleDepth = 6

// typeOf is a schema's type for a table cell, with named schemas linked.
func (w *writer) typeOf(s *openapi.Schema) string {
	switch {
	case s == nil:
		return ""
	case s.Ref != "":
		return "[`" + s.Ref + "`](" + w.site.schemaHref(s.Ref) + ")"
	case s.Items != nil && s.Items.Ref != "":
		return "array of " + w.typeOf(s.Items)
	}
	return "`" + cell(s.TypeString()) + "`"
}

// describe is a property's or parameter's description with its constraints.
func describe(desc string, s *openapi.Schema, deprecated bool) string {
	parts := []string{oneLine(desc)}
	if deprecated || (s != nil && s.Deprecated) {
		parts = append([]string{"**Deprecated.**"}, parts...)
	}
	if s != nil {
		if len(s.Enum) > 0 {
			vals := make([]string, len(s.Enum))
			for i, v := range s.Enum {
				vals[i] = "`" + jsonText(v) + "`"
			}
			parts = append(parts, "One of "+strings.Join(vals, ", ")+".")
		}
		if s.Default != nil {
			parts = append(parts, "Default `"+jsonText(s.Default)+"`.")
		}
		parts = append(parts, limits(s)...)
		if s.ReadOnly {
			parts = append(parts, "Read-only.")
		}
		if s.WriteOnly {
			parts = append(parts, "Write-only.")
		}
	}
	return strings.TrimSpace(strings.Join(parts, " "))
}

// limits lists a schema's numeric and length bounds and pattern.
func limits(s *openapi.Schema) []string {
	var out []string
	if s.Minimum != nil {
		out = append(out, fmt.Sprintf("Minimum %v.", *s.Minimum))
	}
	if s.Maximum != nil {
		out = append(out, fmt.Sprintf("Maximum %v.", *s.Maximum))
	}
	if s.MinLength != nil {
		out = append(out, fmt.Sprintf("At least %d characters.", *s.MinLength))
	}
	if s.MaxLength != nil {
		out = append(out, fmt.Sprintf("At most %d characters.", *s.MaxLength))
	}
	if s.Pattern != "" {
		out = append(out, "Pattern `"+s.Pattern+"`.")
	}
	return out
}

// schema writes a named schema: description, type and properties.
func (w *writer) schema(s *openapi.Schema) {
	w.text(s.Description)
	w.line("Type: %s\n", w.typeOf(s))
	if d := describe("", s, false); d != "" {
		w.line("%s\n", d)
	}
	if len(s.Properties) > 0 {
		w.properties(s)
	}
}

// properties writes an object's properties as a table; inline objects and
// arrays of them continue as dotted names (owner.name, tags[].id).
func (w *writer) properties(s *openapi.Schema) {
	w.line("| Property | Type | Description |\n|---|---|---|")
	w.propertyRows("", s, 0)
	w.line("")
}

func (w *writer) propertyRows(prefix string, s *openapi.Schema, depth int) {
	for _, p := range s.Properties {
		name := "`" + prefix + p.Name + "`"
		if p.Required {
			name += " (required)"
		}
		desc := ""
		if p.Schema != nil {
			desc = p.Schema.Description
		}
		w.line("| %s | %s | %s |", name, w.typeOf(p.Schema), cell(describe(desc, p.Schema, false)))
		if depth+1 >= maxDepth || p.Schema == nil || p.Schema.Ref != "" {
			continue
		}
		switch {
		case len(p.Schema.Properties) > 0:
			w.propertyRows(prefix+p.Name+".", p.Schema, depth+1)
		case p.Schema.Items != nil && p.Schema.Items.Ref == "" && len(p.Schema.Items.Properties) > 0:
			w.propertyRows(prefix+p.Name+"[].", p.Schema.Items, depth+1)
		}
	}
}

// exampleText is a media type's example as text: the one the spec gives,
// else one made from the schema; "" when neither says anything.
func exampleText(mt openapi.MediaType, named map[string]*openapi.Schema) string {
	v := mt.Example
	if v == nil {
		v = sample(mt.Schema, named, 0)
	}
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok && !strings.Contains(mt.Type, "json") {
		return s
	}
	out, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return ""
	}
	return string(out)
}

// sample builds a value that fits a schema: its example, default or first
// enum value, else a placeholder of the right type.
func sample(s *openapi.Schema, named map[string]*openapi.Schema, depth int) any {
	if s == nil || depth > sampleDepth {
		return nil
	}
	if s.Ref != "" {
		return sample(named[s.Ref], named, depth+1)
	}
	switch {
	case s.Example != nil:
		return s.Example
	case s.Default != nil:
		return s.Default
	case len(s.Enum) > 0:
		return s.Enum[0]
	}
	for _, alt := range [][]*openapi.Schema{s.OneOf, s.AnyOf, s.AllOf} {
		if len(alt) > 0 {
			return sample(alt[0], named, depth+1)
		}
	}
	t, _, _ := strings.Cut(s.Type, " ")
	switch t {
	case "string":
		return placeholderString(s.Format)
	case "integer", "number":
		return 0
	case "boolean":
		return false
	case "array":
		if v := sample(s.Items, named, depth+1); v != nil {
			return []any{v}
		}
		return []any{}
	}
	if len(s.Properties) == 0 {
		return nil
	}
	obj := map[string]any{}
	for _, p := range s.Properties {
		if p.Schema != nil && p.Schema.ReadOnly {
			continue
		}
		if v := sample(p.Schema, named, depth+1); v != nil {
			obj[p.Name] = v
		}
	}
	return obj
}

// placeholderString is a string that looks like its format.
func placeholderString(format string) string {
	switch format {
	case "date":
		return "2026-01-31"
	case "date-time":
		return "2026-01-31T12:00:00Z"
	case "email":
		return "user@example.com"
	case "uri", "url":
		return "https://example.com/"
	case "uuid":
		return "00000000-0000-0000-0000-000000000000"
	}
	return "string"
}

// jsonText is a value as JSON, for enum values and defaults.
func jsonText(v any) string {
	out, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprint(v)
	}
	return string(out)
}
