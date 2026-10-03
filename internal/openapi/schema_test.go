package openapi

import (
	"reflect"
	"testing"
)

func TestTypeString(t *testing.T) {
	str := &Schema{Type: "string"}
	cases := []struct {
		name string
		s    *Schema
		want string
	}{
		{"nil", nil, ""},
		{"ref", &Schema{Ref: "Pet"}, "Pet"},
		{"array", &Schema{Type: "array", Items: &Schema{Ref: "Pet"}}, "array of Pet"},
		{"nullableArray", &Schema{Type: "array | null", Items: str}, "array of string | null"},
		{"arrayNoItems", &Schema{Type: "array"}, "array"},
		{"format", &Schema{Type: "string", Format: "date-time"}, "string (date-time)"},
		{"oneOf", &Schema{OneOf: []*Schema{{Ref: "A"}, {Ref: "B"}}}, "oneOf A | B"},
		{"anyOf", &Schema{AnyOf: []*Schema{str, {Type: "integer"}}}, "anyOf string | integer"},
		{"allOf", &Schema{AllOf: []*Schema{{Ref: "A"}, {Ref: "B"}}}, "allOf A & B"},
		{"properties", &Schema{Properties: []Property{{Name: "a"}}}, "object"},
		{"additional", &Schema{AdditionalProperties: &Schema{}}, "object"},
		{"any", &Schema{}, "any"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.s.TypeString(); got != tc.want {
				t.Errorf("TypeString() = %q, want %q", got, tc.want)
			}
		})
	}
}

// componentSchema parses one component schema "S" given as a YAML flow value.
func componentSchema(t *testing.T, value string) *Schema {
	t.Helper()
	spec, _ := mustParse(t, head+"components:\n  schemas:\n    S: "+value+"\n")
	return spec.Schemas[0].Schema
}

func TestSchemaFields(t *testing.T) {
	cases := []struct {
		name, value string
		want        *Schema
	}{
		{"nullable", "{type: string, nullable: true}", &Schema{Type: "string | null"}},
		{"nullableNoType", "{nullable: true}", &Schema{}},
		{"typeList", "{type: [integer, 'null'], nullable: true}", &Schema{Type: "integer | null"}},
		{"booleanTrue", "true", &Schema{}},
		{"booleanFalse", "false", nil},
		{"badNumbers", "{minimum: x, minLength: 1.5}", &Schema{}},
		{"exampleWins", "{example: 1, examples: [2]}", &Schema{Example: 1}},
		{"enumMixed", "{enum: [1, a, null]}", &Schema{Enum: []any{1, "a", nil}}},
		{"allOf", "{allOf: [{$ref: '#/components/schemas/S'}, 7]}", &Schema{AllOf: []*Schema{{Ref: "S"}}}},
		{"anyOf", "{anyOf: [{type: string}]}", &Schema{AnyOf: []*Schema{{Type: "string"}}}},
		{"localRef", "{$ref: '#/components/schemas/S/properties/x'}", nil},
		{"defaultObject", "{default: {a: [1]}}", &Schema{Default: map[string]any{"a": []any{1}}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := componentSchema(t, tc.value); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("schema = %#v, want %#v", got, tc.want)
			}
		})
	}
}

func TestSchemaInlineLocalRef(t *testing.T) {
	spec, diags := mustParse(t, head+`
components:
  schemas:
    A: {type: object, properties: {x: {type: integer}}}
    B: {$ref: '#/components/schemas/A/properties/x'}
    C: {$ref: '#/components/schemas/A~1x'}
`)
	if got := spec.Schemas[1].Schema; got == nil || got.Type != "integer" {
		t.Errorf("inlined = %+v", got)
	}
	if spec.Schemas[2].Schema.Ref != "A/x" || len(diags) != 1 {
		t.Errorf("escaped ref = %+v, diags %v", spec.Schemas[2].Schema, diags)
	}
}
