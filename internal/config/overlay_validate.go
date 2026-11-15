package config

// Validation of `.ssg.designer.json` against the configuration schema: every
// key must exist, every value must have the key's type, and a secret key may
// hold only a $NAME reference. Problems are reported all at once, each with
// the path of the key, sorted, so one fix-and-retry round is enough.

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// envWholeRe is a whole-value reference, "$NAME" or "${NAME}" — the two
// spellings the loaders of secrets accept.
var envWholeRe = regexp.MustCompile(`^\$(?:\{[A-Za-z_][A-Za-z0-9_]*\}|[A-Za-z_][A-Za-z0-9_]*)$`)

// envRefRe finds a reference anywhere in a value ("Bearer $TOKEN").
var envRefRe = regexp.MustCompile(`\$(?:\{[A-Za-z_][A-Za-z0-9_]*\}|[A-Za-z_][A-Za-z0-9_]*)`)

// ValidateOverlay checks a decoded overlay (numbers as json.Number) against
// the schema and returns every problem joined, or nil.
func ValidateOverlay(overlay map[string]any) error {
	root := &SchemaNode{Type: "object", Fields: BuildSchema().Keys}
	var problems []string
	checkObject(root, overlay, "", "", &problems)
	checkThemeName(overlay, &problems)
	if len(problems) == 0 {
		return nil
	}
	sort.Strings(problems)
	return errors.New(strings.Join(problems, "\n"))
}

// checkThemeName: `template` is one folder inside templates_dir (GO-183).
// The theme folder is filepath.Join(templates_dir, template), and ssg parses
// its *.html and copies its css/, js/ and images/ into the public output, so a
// template holding a separator or "." / ".." would let the overlay — written
// by a holder of designer-ui:settings, where `template` is plain — point the
// build at any folder of the CI machine. templates_dir itself is "executes".
// designer-ui applies the same rule (src/overlay-validate.ts).
func checkThemeName(overlay map[string]any, problems *[]string) {
	name, ok := overlay["template"].(string)
	if !ok || name == "" {
		return
	}
	if strings.ContainsAny(name, `/\`) || name == "." || name == ".." {
		*problems = append(*problems, fmt.Sprintf("%s: template: a theme is one folder name inside templates_dir, never a path", OverlayFileName))
	}
}

// checkValue validates v against n; secret is the format of the secret key
// that owns this value ("" when none).
func checkValue(n *SchemaNode, v any, path, secret string, problems *[]string) {
	var primary []string
	checkPrimary(n, v, path, secret, &primary)
	if len(primary) == 0 {
		return
	}
	if n.Alternate != nil {
		var alt []string
		checkPrimary(n.Alternate, v, path, secret, &alt)
		if len(alt) == 0 {
			return
		}
	}
	*problems = append(*problems, primary...)
}

// checkPrimary validates v against the node's own type.
func checkPrimary(n *SchemaNode, v any, path, secret string, problems *[]string) {
	bad := func(want string) {
		*problems = append(*problems, fmt.Sprintf("%s: %s: expected %s, got %s", OverlayFileName, path, describeType(n, want), jsonKind(v)))
	}
	switch n.Type {
	case "any":
		return
	case "bool":
		if _, ok := v.(bool); !ok {
			bad("true or false")
		}
	case "int", "number":
		num, ok := v.(json.Number)
		if !ok {
			bad("a number")
			return
		}
		if _, err := num.Int64(); n.Type == "int" && err != nil {
			bad("a whole number")
		}
	case "string", "enum":
		str, ok := v.(string)
		if !ok {
			bad("text")
			return
		}
		checkString(n, str, path, secret, problems)
	case "list":
		list, ok := v.([]any)
		if !ok {
			bad("a list")
			return
		}
		for i, e := range list {
			if e == nil {
				*problems = append(*problems, fmt.Sprintf("%s: %s[%d]: null is not a list element", OverlayFileName, path, i))
				continue
			}
			checkValue(n.Items, e, fmt.Sprintf("%s[%d]", path, i), secret, problems)
		}
	case "map":
		m, ok := v.(map[string]any)
		if !ok {
			bad("an object")
			return
		}
		for k, e := range m {
			if e != nil {
				checkValue(n.Values, e, joinPath(path, k), secret, problems)
			}
		}
	default: // object
		m, ok := v.(map[string]any)
		if !ok {
			bad("an object")
			return
		}
		checkObject(n, m, path, secret, problems)
	}
}

// checkObject validates the keys of an object node; null removes a key and
// is always allowed.
func checkObject(n *SchemaNode, m map[string]any, path, secret string, problems *[]string) {
	byKey := make(map[string]*SchemaNode, len(n.Fields))
	for _, f := range n.Fields {
		byKey[f.Key] = f
	}
	for k, e := range m {
		f, known := byKey[k]
		p := joinPath(path, k)
		switch {
		case !known && n.Open:
			continue
		case !known:
			*problems = append(*problems, fmt.Sprintf("%s: %s: unknown configuration key", OverlayFileName, p))
		case e != nil:
			own := secret
			if f.Class == ClassSecret {
				own = f.SecretFormat
			}
			checkValue(f, e, p, own, problems)
		}
	}
}

// checkString applies an enum and the secret rule.
func checkString(n *SchemaNode, str, path, secret string, problems *[]string) {
	if len(n.Enum) > 0 && !contains(n.Enum, str) {
		*problems = append(*problems, fmt.Sprintf("%s: %s: %q is not one of %s", OverlayFileName, path, str, quoteAll(n.Enum)))
	}
	if secret == "" || str == "" || secretRefOK(secret, str) {
		return
	}
	*problems = append(*problems, fmt.Sprintf("%s: %s: a secret must be written as an environment reference (%s), never as a literal value",
		OverlayFileName, path, secretExample(secret)))
}

// secretRefOK reports whether a secret value is a reference of its format.
func secretRefOK(format, str string) bool {
	switch format {
	case SecretInline:
		return hasEnvRef(str)
	case SecretLogin:
		ref, ok := trimLogin(str)
		return ok && envWholeRe.MatchString(ref)
	}
	return envWholeRe.MatchString(str)
}

// secretExample shows the accepted form of a secret format.
func secretExample(format string) string {
	switch format {
	case SecretInline:
		return `text holding "$NAME", e.g. "Bearer $TOKEN"`
	case SecretLogin:
		return `"login:$NAME"`
	}
	return `"$NAME" or "${NAME}"`
}

// describeType names what a node accepts, alternate included.
func describeType(n *SchemaNode, want string) string {
	if len(n.Enum) > 0 {
		want = "one of " + quoteAll(n.Enum)
	}
	if n.Alternate != nil {
		return want + " (or " + n.Alternate.Type + ")"
	}
	return want
}

// jsonKind names the JSON type of a decoded value.
func jsonKind(v any) string {
	switch v.(type) {
	case bool:
		return "true/false"
	case json.Number:
		return "a number"
	case string:
		return "text"
	case []any:
		return "a list"
	case map[string]any:
		return "an object"
	}
	return "null"
}

// contains reports whether list holds s.
func contains(list []string, s string) bool {
	for _, e := range list {
		if e == s {
			return true
		}
	}
	return false
}

// quoteAll lists values quoted and comma-separated.
func quoteAll(values []string) string {
	q := make([]string, len(values))
	for i, v := range values {
		q[i] = fmt.Sprintf("%q", v)
	}
	return strings.Join(q, ", ")
}
