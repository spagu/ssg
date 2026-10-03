package apidoc

import (
	"reflect"
	"testing"

	"github.com/spagu/ssg/internal/apimodel"
)

// strp returns a pointer to s.
func strp(s string) *string { return &s }

// TestParseDescription covers summary/body splitting and gutter handling.
func TestParseDescription(t *testing.T) {
	tests := []struct {
		name, raw     string
		summary, body string
	}{
		{"empty", "", "", ""},
		{"blank", "   \n * \n ", "", ""},
		{"single line", " Short. ", "Short.", ""},
		{"gutter", "\n * First line\n * continues.\n *\n * Second para.\n *\n * Third.\n ", "First line\ncontinues.", "Second para.\n\nThird."},
		{"crlf", "\r\n * One.\r\n *\r\n * Two.\r\n ", "One.", "Two."},
		{"lone cr", "\r * One.\r *\r * Two.\r", "One.", "Two."},
		{"tabs", "\n\t* One.\n\t*\n\t*\tTwo.\n", "One.", "Two."},
		{"no gutter", "\n   One.\n\n   Two\n     indented\n", "One.", "Two\n  indented"},
		{"fence keeps indent", "\n * Sum.\n *\n * ```js\n *   if (a) {\n *\n *     b()\n *   }\n * ```\n", "Sum.", "```js\n  if (a) {\n\n    b()\n  }\n```"},
		{"summary is fence", "\n * ```\n * a\n *\n * b\n * ```\n *\n * rest", "```\na\n\nb\n```", "rest"},
		{"bold not gutter", "\n **bold** text", "**bold** text", ""},
		{"gutter at", "\n *@since 1", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := Parse(tt.raw)
			if c == nil || c.Doc == nil {
				t.Fatal("nil comment or doc")
			}
			if c.Doc.Summary != tt.summary || c.Doc.Body != tt.body {
				t.Errorf("got summary %q body %q, want %q %q", c.Doc.Summary, c.Doc.Body, tt.summary, tt.body)
			}
		})
	}
}

// TestParseDocTags covers the tags that fill Doc fields.
func TestParseDocTags(t *testing.T) {
	tests := []struct {
		name, raw string
		want      apimodel.Doc
	}{
		{"returns", "@returns {string} the name", apimodel.Doc{Returns: "the name"}},
		{"return no type", "@return - it", apimodel.Doc{Returns: "it"}},
		{"throws", "@throws {TypeError} when bad\n@exception {RangeError}\n@throws plain\n@throws", apimodel.Doc{Throws: []string{"when bad", "RangeError", "plain"}}},
		{"deprecated text", "@deprecated use b", apimodel.Doc{Deprecated: strp("use b")}},
		{"deprecated empty", "@deprecated", apimodel.Doc{Deprecated: strp("")}},
		{"since see default", "@since 1.2\n@see a\n@see b\n@default 3", apimodel.Doc{Since: "1.2", See: []string{"a", "b"}, Default: "3"}},
		{"defaultValue", "@defaultValue `x`", apimodel.Doc{Default: "`x`"}},
		{"summary override", "Desc.\n\nMore.\n@summary Other.", apimodel.Doc{Summary: "Other.", Body: "More."}},
		{"remarks", "S.\n\nB.\n@remarks R1\n line\n@remarks\n@remarks R2", apimodel.Doc{Summary: "S.", Body: "B.\n\nR1\n line\n\nR2"}},
		{"remarks only", "S.\n@remarks R", apimodel.Doc{Summary: "S.", Body: "R"}},
		{"unknown tags", "@author Ann\n@license MIT\n@category io\n@x", apimodel.Doc{Tags: []apimodel.Tag{{Name: "author", Text: "Ann"}, {Name: "license", Text: "MIT"}, {Name: "category", Text: "io"}, {Name: "x"}}}},
		{"link kept", "See {@link Foo}.", apimodel.Doc{Summary: "See {@link Foo}."}},
		{"at mid line", "Mail a@b.c here", apimodel.Doc{Summary: "Mail a@b.c here"}},
		{"not a tag", "@1 x", apimodel.Doc{Summary: "@1 x"}},
		{"single line tag", " @since 2 ", apimodel.Doc{Since: "2"}},
		{"public ignored", "@public", apimodel.Doc{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := *Parse(tt.raw).Doc
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %+v\nwant %+v", got, tt.want)
			}
		})
	}
}

// TestParseExamples covers @example wrapping, captions and fences with @.
func TestParseExamples(t *testing.T) {
	tests := []struct {
		name, raw string
		want      []string
	}{
		{"bare", "@example\n *   foo()\n *     bar()", []string{"```js\n  foo()\n    bar()\n```"}},
		{"same line", "@example foo()", []string{"```js\nfoo()\n```"}},
		{"fenced", "@example\n * ```ts\n * @Component()\n * class A {}\n * ```\n * @since 1", []string{"```ts\n@Component()\nclass A {}\n```"}},
		{"caption", "@example <caption>Basic</caption>\n * foo()", []string{"*Basic*\n\n```js\nfoo()\n```"}},
		{"caption same line", "@example <caption>C</caption> foo()", []string{"*C*\n\n```js\nfoo()\n```"}},
		{"caption only", "@example <caption>C</caption>", []string{"*C*"}},
		{"caption unterminated", "@example <caption>C\nfoo()", []string{"```js\n<caption>C\nfoo()\n```"}},
		{"caption fenced", "@example <caption>C</caption>\n~~~\nx\n~~~", []string{"*C*\n\n~~~\nx\n~~~"}},
		{"empty dropped", "@example\n@example a", []string{"```js\na\n```"}},
		{"two", "@example a\n@example b", []string{"```js\na\n```", "```js\nb\n```"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := Parse(tt.raw)
			if !reflect.DeepEqual(c.Doc.Examples, tt.want) {
				t.Errorf("got %q\nwant %q", c.Doc.Examples, tt.want)
			}
		})
	}
	if c := Parse("@example\n```\n@x\n```"); c.Doc.Since != "" || len(c.Doc.Tags) != 0 {
		t.Errorf("@ inside fence parsed as tag: %+v", c.Doc)
	}
}

// TestParseModifiers covers every modifier tag.
func TestParseModifiers(t *testing.T) {
	tests := []struct {
		raw  string
		want Modifiers
	}{
		{"@internal", Modifiers{Internal: true}},
		{"@private", Modifiers{Internal: true, Private: true}},
		{"@hidden", Modifiers{Hidden: true}},
		{"@ignore reason", Modifiers{Hidden: true}},
		{"@beta", Modifiers{Stability: apimodel.Beta}},
		{"@alpha", Modifiers{Stability: apimodel.Alpha}},
		{"@experimental", Modifiers{Stability: apimodel.Experimental}},
		{"@readonly\n@abstract\n@override", Modifiers{Readonly: true, Abstract: true, Override: true}},
		{"@public", Modifiers{}},
	}
	for _, tt := range tests {
		t.Run(tt.raw, func(t *testing.T) {
			if got := Parse(tt.raw).Mods; got != tt.want {
				t.Errorf("got %+v want %+v", got, tt.want)
			}
		})
	}
}

// TestParseTypeTags covers @type, @returns and @throws types.
func TestParseTypeTags(t *testing.T) {
	c := Parse("@type {Object<string, {a: number}>}\n@returns {Promise<void>} done\n@throws {Error} bad")
	if c.Type != "Object<string, {a: number}>" {
		t.Errorf("type %q", c.Type)
	}
	if c.Returns == nil || *c.Returns != (TypeTag{Type: "Promise<void>", Text: "done"}) {
		t.Errorf("returns %+v", c.Returns)
	}
	if !reflect.DeepEqual(c.Throws, []TypeTag{{Type: "Error", Text: "bad"}}) {
		t.Errorf("throws %+v", c.Throws)
	}
	if got := Parse("@type string").Type; got != "string" {
		t.Errorf("bare type %q", got)
	}
	if got := Parse("@type {unterminated").Type; got != "{unterminated" {
		t.Errorf("unterminated type %q", got)
	}
}

// TestTypedefHyphen: "@typedef {T} Name - text" and "@callback Name - text"
// drop the separator, like @param does.
func TestTypedefHyphen(t *testing.T) {
	c := Parse("@typedef {Object} Opts - the options\n@callback Done - called at the end")
	if c.Typedefs[0].Doc.Summary != "the options" || c.Callbacks[0].Doc.Summary != "called at the end" {
		t.Errorf("summaries = %q, %q", c.Typedefs[0].Doc.Summary, c.Callbacks[0].Doc.Summary)
	}
}
