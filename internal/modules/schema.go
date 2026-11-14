// Package modules reads worker module manifests (module.json, GO-143) and
// assembles the module registry a site build hands to the `core` worker
// (GO-144).
//
// The manifest format has one definition, workers/module.schema.json. Like
// the TypeScript reader (workers/module-manifest.ts), this package interprets
// the small subset of JSON Schema that file uses rather than carrying a schema
// library, and refuses any other keyword when the schema is loaded, so a
// schema edit that needs a richer validator fails loudly in both languages
// instead of being silently ignored. The two readers produce the same
// messages; internal/modules/testdata/manifests pins them for both.
package modules

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strings"
)

// Schema is a parsed module.schema.json, checked to use only the keywords
// this reader interprets and with every pattern compiled once.
type Schema struct {
	root     map[string]any
	patterns map[string]*regexp.Regexp
}

// annotations only document the schema and never reject a value.
var annotations = map[string]bool{"$schema": true, "$id": true, "$comment": true, "$defs": true, "title": true, "description": true}

// keywords are the validating keywords this reader understands.
var keywords = map[string]bool{
	"$ref": true, "type": true, "enum": true, "required": true, "properties": true, "additionalProperties": true,
	"items": true, "maxItems": true, "minLength": true, "maxLength": true, "pattern": true, "minimum": true,
}

// ParseSchema reads a schema and refuses it when it uses a keyword, a
// reference or a pattern this reader cannot honour.
func ParseSchema(data []byte) (*Schema, error) {
	v, _ := decodeJSON(data)
	root, ok := v.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("module.schema.json: not a JSON object")
	}
	s := &Schema{root: root, patterns: map[string]*regexp.Regexp{}}
	if err := s.check(root, "#"); err != nil {
		return nil, err
	}
	return s, nil
}

// decodeJSON parses JSON keeping numbers as json.Number, so an integer and a
// fraction stay distinguishable. ok is false when data is not one JSON value.
func decodeJSON(data []byte) (v any, ok bool) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	if dec.Decode(&v) != nil || dec.More() {
		return nil, false
	}
	return v, true
}

// check walks one schema node, refusing what validate would not understand.
func (s *Schema) check(node map[string]any, at string) error {
	for _, key := range sortedKeys(node) {
		rule := node[key]
		switch {
		case key == "$defs":
			for _, name := range sortedKeys(asObject(rule)) {
				if err := s.checkSub(asObject(rule)[name], at+"/$defs/"+name); err != nil {
					return err
				}
			}
		case annotations[key]:
		case !keywords[key]:
			return fmt.Errorf("module.schema.json: unsupported keyword %q at %s", key, at)
		case key == "$ref":
			if _, err := s.resolve(rule); err != nil {
				return err
			}
		case key == "properties":
			for _, name := range sortedKeys(asObject(rule)) {
				if err := s.checkSub(asObject(rule)[name], at+"/properties/"+name); err != nil {
					return err
				}
			}
		case key == "items", key == "additionalProperties" && rule != true && rule != false:
			if err := s.checkSub(rule, at+"/"+key); err != nil {
				return err
			}
		case key == "pattern":
			src, _ := rule.(string)
			re, err := regexp.Compile(src)
			if err != nil {
				return fmt.Errorf("module.schema.json: pattern at %s is not RE2: %w", at, err)
			}
			s.patterns[src] = re
		}
		if !ruleShapeOK(key, rule) {
			return fmt.Errorf("module.schema.json: %s at %s has the wrong shape", key, at)
		}
	}
	return nil
}

// ruleShapes is the JSON type each keyword's value must have, so validate can
// rely on it.
var ruleShapes = map[string]string{
	"enum": "array", "required": "array", "pattern": "string",
	"maxItems": "integer", "minLength": "integer", "maxLength": "integer", "minimum": "integer",
}

// ruleShapeOK reports whether a keyword's value has the shape validate expects.
func ruleShapeOK(key string, rule any) bool {
	if key == "type" {
		return typeOf(rule) == "string" || typeOf(rule) == "array"
	}
	want, ok := ruleShapes[key]
	return !ok || typeOf(rule) == want
}

// checkSub checks a node that must itself be a schema object.
func (s *Schema) checkSub(v any, at string) error {
	node, ok := v.(map[string]any)
	if !ok {
		return fmt.Errorf("module.schema.json: %s is not a schema object", at)
	}
	return s.check(node, at)
}

// resolve follows a local "#/$defs/<name>" reference.
func (s *Schema) resolve(ref any) (map[string]any, error) {
	name, ok := strings.CutPrefix(fmt.Sprint(ref), "#/$defs/")
	if !ok {
		return nil, fmt.Errorf("module.schema.json: only local #/$defs references are supported (%v)", ref)
	}
	target, ok := asObject(s.root["$defs"])[name].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("module.schema.json: %v is not defined", ref)
	}
	return target, nil
}

// typeOf names a decoded JSON value the way JSON Schema does; a whole number
// is an "integer", as Number.isInteger says in the TypeScript reader.
func typeOf(v any) string {
	switch x := v.(type) {
	case nil:
		return "null"
	case []any:
		return "array"
	case map[string]any:
		return "object"
	case string:
		return "string"
	case bool:
		return "boolean"
	case json.Number:
		if f, err := x.Float64(); err == nil && f == math.Trunc(f) {
			return "integer"
		}
	}
	return "number"
}

// asObject returns v as an object, or an empty one.
func asObject(v any) map[string]any {
	m, _ := v.(map[string]any)
	return m
}

// sortedKeys lists an object's keys in a stable order, so messages come out
// the same on every run.
func sortedKeys[T any](m map[string]T) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// jsonText renders a value as JSON.stringify would, for messages.
func jsonText(v any) string {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
	return strings.TrimSuffix(b.String(), "\n")
}
