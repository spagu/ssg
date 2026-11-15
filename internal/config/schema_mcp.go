package config

// The keys `ssg mcp` lets an assistant change through designer_config_set
// come from the schema (GO-177): an entry marked .assistant(). The kind tells
// the tool how a value is written; a mapping-shaped feature is a "switch",
// written to <key>.enabled.

import "sort"

// MCPKey is one key designer_config_set may write.
type MCPKey struct {
	Path        string
	Kind        string // bool | int | string | list | switch
	Description string
}

// MCPWritableKeys lists the assistant-writable keys, sorted by path.
func MCPWritableKeys() []MCPKey {
	var out []MCPKey
	walkSchema(BuildSchema().Keys, func(n *SchemaNode) {
		if n.MCPWritable {
			out = append(out, MCPKey{Path: n.Path, Kind: mcpKind(n.Type), Description: n.Description})
		}
	})
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}

// mcpKind maps a schema type to the way designer_config_set writes it.
func mcpKind(schemaType string) string {
	switch schemaType {
	case "enum":
		return "string"
	case "object":
		return "switch"
	}
	return schemaType
}
