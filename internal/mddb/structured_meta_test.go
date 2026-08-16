package mddb

// Structured frontmatter through a flat meta store (#154, tradik/mddb#187).

import (
	"strings"
	"testing"
)

// TestDecodeStructuredMetaRoundTrips: a producer that JSON-encodes a
// structured field gets that shape back, which is what makes mddb usable as a
// content source for recipe/FAQ/product pages at all.
func TestDecodeStructuredMetaRoundTrips(t *testing.T) {
	faq := `[{"question":"How long?","answer":"20 minutes"},{"question":"Freezable?","answer":"Yes"}]`
	got := decodeStructuredMeta(faq)

	list, ok := got.([]interface{})
	if !ok {
		t.Fatalf("a JSON array must decode to a slice, got %T", got)
	}
	if len(list) != 2 {
		t.Fatalf("entries = %d, want 2", len(list))
	}
	first, ok := list[0].(map[string]interface{})
	if !ok {
		t.Fatalf("entry type = %T", list[0])
	}
	if first["question"] != "How long?" {
		t.Errorf("question = %v", first["question"])
	}

	obj := decodeStructuredMeta(`{"@type":"Recipe","cookTime":"PT20M"}`)
	m, ok := obj.(map[string]interface{})
	if !ok || m["@type"] != "Recipe" {
		t.Fatalf("a JSON object must decode: %#v", obj)
	}
}

// TestDecodeStructuredMetaLeavesEverythingElse: every value an existing
// mddb-backed site already stores must arrive exactly as it does today.
func TestDecodeStructuredMetaLeavesEverythingElse(t *testing.T) {
	cases := []interface{}{
		"A plain title",
		"Recipes: quick dinners",       // a colon is not structure
		"[not json",                    // looks like an array, is not
		"{still not json",              //
		"42",                           // a bare number stays the string it was
		`"quoted"`,                     // a JSON string is still just text
		"",                             //
		[]string{"a", "b"},             // already a list
		map[string]interface{}{"a": 1}, // already structured
		7,
	}
	for _, in := range cases {
		got := decodeStructuredMeta(in)
		switch v := in.(type) {
		case string:
			if got != v {
				t.Errorf("decodeStructuredMeta(%q) = %#v, want it unchanged", v, got)
			}
		default:
			if got == nil {
				t.Errorf("non-string value must survive: %#v", in)
			}
		}
	}
}

// TestLooksGoStringified: the shape a producer leaves behind when it formats a
// map instead of encoding it — and the shapes that must NOT be mistaken for it,
// because a false positive would warn about ordinary titles.
func TestLooksGoStringified(t *testing.T) {
	stringified := []string{
		"map[answer:20 minutes question:How long?]",
		"map[]",
		"[map[answer:Yes question:Freezable?]]",
	}
	for _, s := range stringified {
		if !looksGoStringified(s) {
			t.Errorf("%q must be recognised as a printed Go value", s)
		}
	}

	fine := []string{
		"A plain title",
		"Recipes: quick dinners",
		`[{"question":"How long?"}]`, // JSON, however map-like it reads
		`{"@type":"Recipe"}`,
		"https://example.com/a:b",
		"",
		"[]",
	}
	for _, s := range fine {
		if looksGoStringified(s) {
			t.Errorf("%q must NOT be reported", s)
		}
	}
	if looksGoStringified(42) {
		t.Error("a non-string value cannot be a stringified one")
	}
}

// TestStructuredMetaWarningsNameTheField: the value cannot be recovered, so
// the field and a readable excerpt are what the operator gets — pointed at the
// data rather than at the template that failed on it.
func TestStructuredMetaWarningsNameTheField(t *testing.T) {
	doc := &Document{Key: "chicken-soup", Metadata: map[string]any{
		"title":  "Chicken soup",
		"faq":    "map[answer:20 minutes question:How long?]",
		"schema": "map[@type:Recipe]",
	}}
	warnings := doc.StructuredMetaWarnings()
	if len(warnings) != 2 {
		t.Fatalf("warnings = %+v, want the two broken fields", warnings)
	}
	// Sorted, so two builds report the same way.
	if warnings[0].Key != "faq" || warnings[1].Key != "schema" {
		t.Fatalf("order = %s, %s", warnings[0].Key, warnings[1].Key)
	}
	msg := warnings[0].Message()
	for _, want := range []string{"faq", "stringified Go value", "JSON-encoded"} {
		if !strings.Contains(msg, want) {
			t.Errorf("message missing %q: %s", want, msg)
		}
	}
	// A long value is truncated rather than dumped into the log.
	long := &Document{Key: "k", Metadata: map[string]any{
		"faq": "map[" + strings.Repeat("a", 200) + "]",
	}}
	if m := long.StructuredMetaWarnings()[0].Message(); !strings.Contains(m, "…") {
		t.Errorf("a long value must be truncated: %s", m)
	}
	// A healthy document says nothing.
	ok := &Document{Key: "k", Metadata: map[string]any{
		"title": "Fine", "faq": `[{"question":"q"}]`,
	}}
	if w := ok.StructuredMetaWarnings(); len(w) != 0 {
		t.Errorf("nothing to report, got %+v", w)
	}
}

// TestToPageDecodesStructuredMeta: the whole point, end to end — a JSON-encoded
// faq reaches the template as a list a theme can range over.
func TestToPageDecodesStructuredMeta(t *testing.T) {
	doc := &Document{Key: "soup", Content: "Body", Metadata: map[string]any{
		"title": "Soup",
		"faq":   `[{"question":"How long?","answer":"20 minutes"}]`,
		"note":  "a plain string",
	}}
	page, err := doc.ToPage()
	if err != nil {
		t.Fatal(err)
	}
	faq, ok := page.Extra["faq"].([]interface{})
	if !ok {
		t.Fatalf("faq reached the template as %T — a theme cannot range over that", page.Extra["faq"])
	}
	entry, ok := faq[0].(map[string]interface{})
	if !ok || entry["question"] != "How long?" {
		t.Fatalf("faq entry = %#v", faq[0])
	}
	if page.Extra["note"] != "a plain string" {
		t.Errorf("an ordinary field must be untouched: %#v", page.Extra["note"])
	}
}
