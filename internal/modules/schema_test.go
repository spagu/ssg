package modules

import (
	"slices"
	"strings"
	"testing"
)

// TestParseSchemaRefusesWhatItCannotHonour: a schema edit that needs a richer
// validator fails when the schema is loaded, never silently at validation.
func TestParseSchemaRefusesWhatItCannotHonour(t *testing.T) {
	cases := map[string]string{
		`[]`:                                    "not a JSON object",
		`{} {}`:                                 "not a JSON object",
		`{"oneOf": []}`:                         `unsupported keyword "oneOf" at #`,
		`{"$ref": "other.json#/x"}`:             "only local #/$defs references",
		`{"$ref": "#/$defs/none"}`:              "#/$defs/none is not defined",
		`{"pattern": "(?=x)"}`:                  "pattern at # is not RE2",
		`{"pattern": 3}`:                        "pattern at # has the wrong shape",
		`{"maxItems": 1.5}`:                     "maxItems at # has the wrong shape",
		`{"type": 1}`:                           "type at # has the wrong shape",
		`{"properties": {"a": 1}}`:              "#/properties/a is not a schema object",
		`{"properties": {"a": {"not": {}}}}`:    `unsupported keyword "not" at #/properties/a`,
		`{"$defs": {"a": []}}`:                  "#/$defs/a is not a schema object",
		`{"items": true}`:                       "#/items is not a schema object",
		`{"additionalProperties": {"enum": 1}}`: "enum at #/additionalProperties has the wrong shape",
		`{"$defs": {"a": {"$defs": {"b": {"if": {}}}}}}`:  `unsupported keyword "if" at #/$defs/a/$defs/b`,
		`{"title": "t", "$comment": "c", "required": {}}`: "required at # has the wrong shape",
	}
	for src, want := range cases {
		if _, err := ParseSchema([]byte(src)); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: err %v, want %q", src, err, want)
		}
	}
	if _, err := ParseSchema([]byte(`{"additionalProperties": false, "type": ["string", "null"]}`)); err != nil {
		t.Errorf("a boolean additionalProperties and a type list are fine: %v", err)
	}
}

// TestValidateKeywordSubset covers the keywords module.schema.json does not
// exercise today, with the TypeScript reader's messages.
func TestValidateKeywordSubset(t *testing.T) {
	check := func(schema, value string, want ...string) {
		t.Helper()
		s, err := ParseSchema([]byte(schema))
		if err != nil {
			t.Fatal(err)
		}
		v, _ := decodeJSON([]byte(value))
		if got := s.Validate(v); !slices.Equal(got, want) {
			t.Errorf("%s / %s: %q, want %q", schema, value, got, want)
		}
	}
	check(`{"type": "object", "properties": {"a": {}}, "additionalProperties": false}`, `{"a": 1, "b": 2}`, "manifest.b: is not allowed")
	check(`{"additionalProperties": true}`, `{"a": 1}`)
	check(`{"type": "number", "minimum": 1}`, `3`)
	check(`{"type": "number", "minimum": 1}`, `0.5`, "manifest: must be at least 1")
	check(`{"type": "integer"}`, `1e400`, "manifest: must be integer, not number")
	check(`{"type": "boolean"}`, `true`)
	check(`{"required": ["a"], "properties": {}, "additionalProperties": {}, "items": {}, "maxItems": 0}`, `"x"`)
	check(`{"minLength": 2, "maxLength": 0, "pattern": "^z$", "minimum": 5}`, `[1]`)
	check(`{"enum": ["a", 1]}`, `2`, `manifest: must be one of "a", 1`)
}

func TestTypeOf(t *testing.T) {
	for src, want := range map[string]string{`null`: "null", `[]`: "array", `{}`: "object", `""`: "string", `false`: "boolean", `2`: "integer", `2.0`: "integer", `2.5`: "number"} {
		v, _ := decodeJSON([]byte(src))
		if got := typeOf(v); got != want {
			t.Errorf("typeOf(%s) = %s, want %s", src, got, want)
		}
	}
}
