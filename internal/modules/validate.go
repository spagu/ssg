package modules

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"
)

// Validate returns every place where value breaks the schema, as
// "<path>: <problem>" lines in sorted order; none means it conforms. The
// messages are the TypeScript reader's, word for word.
func (s *Schema) Validate(value any) []string {
	errs := s.validate(value, s.root, "manifest")
	slices.Sort(errs)
	return errs
}

// validate applies one schema node. ParseSchema has already refused anything
// unknown, so every key here is an annotation or a keyword handled below.
func (s *Schema) validate(value any, node map[string]any, at string) []string {
	var errs []string
	for _, key := range sortedKeys(node) {
		errs = append(errs, s.keyword(key, value, node[key], node, at)...)
	}
	return errs
}

// keyword checks value against one keyword of a schema node.
func (s *Schema) keyword(key string, value, rule any, node map[string]any, at string) []string {
	if annotations[key] {
		return nil
	}
	obj, isObj := value.(map[string]any)
	switch key {
	case "$ref":
		target, _ := s.resolve(rule) // resolvable: checked by ParseSchema
		return s.validate(value, target, at)
	case "type":
		return checkType(value, rule, at)
	case "enum":
		for _, allowed := range rule.([]any) {
			if jsonText(allowed) == jsonText(value) {
				return nil
			}
		}
		names := make([]string, 0, len(rule.([]any)))
		for _, allowed := range rule.([]any) {
			names = append(names, jsonText(allowed))
		}
		return []string{fmt.Sprintf("%s: must be one of %s", at, strings.Join(names, ", "))}
	case "required":
		var errs []string
		for _, k := range rule.([]any) {
			if _, ok := obj[fmt.Sprint(k)]; isObj && !ok {
				errs = append(errs, fmt.Sprintf("%s.%v: is required", at, k))
			}
		}
		return errs
	case "properties":
		var errs []string
		for _, k := range sortedKeys(asObject(rule)) {
			if v, ok := obj[k]; ok {
				errs = append(errs, s.validate(v, asObject(rule)[k].(map[string]any), at+"."+k)...)
			}
		}
		return errs
	case "additionalProperties":
		return s.additional(obj, rule, node, at)
	case "items":
		var errs []string
		list, _ := value.([]any)
		for i, v := range list {
			errs = append(errs, s.validate(v, rule.(map[string]any), fmt.Sprintf("%s[%d]", at, i))...)
		}
		return errs
	}
	return checkScalar(key, value, rule, at, s)
}

// additional checks the members properties does not name.
func (s *Schema) additional(obj map[string]any, rule any, node map[string]any, at string) []string {
	var errs []string
	known := asObject(node["properties"])
	for _, k := range sortedKeys(obj) {
		if _, ok := known[k]; ok {
			continue
		}
		if rule == false {
			errs = append(errs, fmt.Sprintf("%s.%s: is not allowed", at, k))
		} else if sub, ok := rule.(map[string]any); ok {
			errs = append(errs, s.validate(obj[k], sub, at+"."+k)...)
		}
	}
	return errs
}

// checkType: "number" also accepts an integer, as in JSON Schema.
func checkType(value, rule any, at string) []string {
	wanted := []string{fmt.Sprint(rule)}
	if list, ok := rule.([]any); ok {
		wanted = wanted[:0]
		for _, w := range list {
			wanted = append(wanted, fmt.Sprint(w))
		}
	}
	got := typeOf(value)
	for _, w := range wanted {
		if w == got || (w == "number" && got == "integer") {
			return nil
		}
	}
	return []string{fmt.Sprintf("%s: must be %s, not %s", at, strings.Join(wanted, " or "), got)}
}

// checkScalar handles the keywords that look at one value: string lengths
// (in code points, as [...s].length counts them), patterns, list sizes and
// numeric minimums. Each applies only to values of its own type.
func checkScalar(key string, value, rule any, at string, s *Schema) []string {
	n, _ := rule.(json.Number)
	limit, _ := n.Float64()
	str, isStr := value.(string)
	switch key {
	case "minLength":
		if isStr && float64(utf8.RuneCountInString(str)) < limit {
			return []string{at + ": must not be empty"}
		}
	case "maxLength":
		if isStr && float64(utf8.RuneCountInString(str)) > limit {
			return []string{fmt.Sprintf("%s: longer than %v characters", at, rule)}
		}
	case "pattern":
		if isStr && !s.patterns[rule.(string)].MatchString(str) {
			return []string{fmt.Sprintf("%s: %s does not match %s", at, jsonText(str), rule)}
		}
	case "maxItems":
		if list, ok := value.([]any); ok && float64(len(list)) > limit {
			return []string{fmt.Sprintf("%s: at most %v entries", at, rule)}
		}
	case "minimum":
		if v, ok := value.(json.Number); ok {
			if f, _ := v.Float64(); f < limit {
				return []string{fmt.Sprintf("%s: must be at least %v", at, rule)}
			}
		}
	}
	return nil
}
