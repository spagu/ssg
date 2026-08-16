package mddb

// Structured frontmatter through mddb's flat meta model (#154, tradik/mddb#187).
//
// mddb stores meta as map<string, repeated string> — deliberately flat, and
// consistent across its whole API. A site whose frontmatter is not flat (a
// `faq:` list of {question, answer}, a `schema:` object — ordinary for recipe,
// FAQ and product pages) therefore has only one way through: encode the value
// as JSON in a single meta string.
//
// ssg used to hand whatever arrived straight to the template, which broke that
// arrangement from both ends:
//
//   - a producer that JSON-encoded correctly still lost, because the template
//     received a JSON *string* and `{{range .faq}}` cannot walk one;
//   - a producer that stringified with Go's %v stored `map[answer:… question:…]`,
//     and the render failed on EVERY post with "can't evaluate field question in
//     type interface {}" — an error that points at the theme, not at the field
//     or the document that carries the bad value.
//
// So: JSON round-trips, and a Go-stringified value is named rather than
// rendered. A value that is neither reaches the template exactly as before.

import (
	"encoding/json"
	"strings"
)

// decodeStructuredMeta returns the structured value a meta entry carries.
//
// A string that parses as a JSON object or array becomes that object or array,
// which is what lets structured frontmatter survive a flat store. Anything
// else — a plain string, a number, a list of strings — is returned unchanged,
// so every existing mddb-backed site sees exactly what it saw before.
func decodeStructuredMeta(value interface{}) interface{} {
	s, ok := value.(string)
	if !ok {
		return value
	}
	trimmed := strings.TrimSpace(s)
	if len(trimmed) < 2 {
		return value
	}
	// Only objects and arrays: decoding a bare number or a quoted string would
	// change the type of ordinary text for no gain.
	if trimmed[0] != '{' && trimmed[0] != '[' {
		return value
	}
	var decoded interface{}
	if err := json.Unmarshal([]byte(trimmed), &decoded); err != nil {
		return value
	}
	return decoded
}

// looksGoStringified reports whether a meta value is the printed form of a Go
// map or struct rather than content anyone authored. Such a value cannot be
// recovered — the information was lost before it was stored — so the only
// useful thing left is to say which field and which document carry it.
func looksGoStringified(value interface{}) bool {
	s, ok := value.(string)
	if !ok {
		return false
	}
	s = strings.TrimSpace(s)
	// A JSON value is data, however map-like it reads.
	if json.Valid([]byte(s)) {
		return false
	}
	// `map[` is what fmt prints for a map and for anything holding one — a
	// slice of maps comes out as `[map[…] map[…]]`. Nothing an author types
	// looks like that, so the test is exact rather than heuristic: no ordinary
	// title, URL or sentence is ever reported.
	return strings.Contains(s, "map[") && strings.HasSuffix(s, "]")
}

// StructuredMetaWarning describes one meta field that arrived as a stringified
// Go value, so the caller can report the document and the field together.
type StructuredMetaWarning struct {
	Key   string
	Value string
}

// Message is the line a build prints for the finding.
func (w StructuredMetaWarning) Message() string {
	value := w.Value
	if len(value) > 60 {
		value = value[:60] + "…"
	}
	return "meta field " + w.Key + " looks like a stringified Go value (" + value + ") — " +
		"mddb stores meta as flat strings, so structured fields must be JSON-encoded by the producer"
}
