package apidoc

import (
	"reflect"
	"testing"
)

// TestParseParams covers every @param form and its aliases.
func TestParseParams(t *testing.T) {
	tests := []struct {
		name, raw string
		want      ParamTag
	}{
		{"plain", "@param name the text", ParamTag{Name: "name", Text: "the text"}},
		{"typed", "@param {string} name the text", ParamTag{Name: "name", Type: "string", Text: "the text"}},
		{"optional", "@param {string} [name] text", ParamTag{Name: "name", Type: "string", Text: "text", Optional: true}},
		{"default", "@param {number} [n = 3] text", ParamTag{Name: "n", Type: "number", Text: "text", Optional: true, Default: "3"}},
		{"default brackets", "@param {Array} [a=[1]] x", ParamTag{Name: "a", Type: "Array", Text: "x", Optional: true, Default: "[1]"}},
		{"type optional", "@param {string=} name", ParamTag{Name: "name", Type: "string", Optional: true}},
		{"rest", "@param {...number} nums", ParamTag{Name: "nums", Type: "number", Rest: true}},
		{"rest name", "@arg ...nums all", ParamTag{Name: "nums", Rest: true, Text: "all"}},
		{"hyphen", "@argument {T} name - text", ParamTag{Name: "name", Type: "T", Text: "text"}},
		{"hyphen only", "@param name -", ParamTag{Name: "name"}},
		{"hyphen word", "@param name -1 is ok", ParamTag{Name: "name", Text: "-1 is ok"}},
		{"dotted", "@param {boolean} opts.strict be strict", ParamTag{Name: "opts.strict", Type: "boolean", Text: "be strict"}},
		{"nested braces", "@param {Object<string, {a: number}>} map m", ParamTag{Name: "map", Type: "Object<string, {a: number}>", Text: "m"}},
		{"multiline type", "@param {{\n *   a: number\n * }} o obj", ParamTag{Name: "o", Type: "{\n  a: number\n}", Text: "obj"}},
		{"unterminated brace", "@param {string name text", ParamTag{Name: "{string", Text: "name text"}},
		{"unterminated bracket", "@param [name text", ParamTag{Name: "[name", Text: "text"}},
		{"multiline text", "@param a first\n *   second", ParamTag{Name: "a", Text: "first\n  second"}},
		{"empty", "@param", ParamTag{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := Parse(tt.raw)
			if len(c.Params) != 1 || c.Params[0] != tt.want {
				t.Errorf("got %+v\nwant %+v", c.Params, tt.want)
			}
		})
	}
}

// TestParseTemplates covers @template and @typeParam.
func TestParseTemplates(t *testing.T) {
	tests := []struct {
		name, raw string
		want      []ParamTag
	}{
		{"single", "@template T", []ParamTag{{Name: "T"}}},
		{"list", "@template T, U the types", []ParamTag{{Name: "T", Text: "the types"}, {Name: "U", Text: "the types"}}},
		{"constraint", "@template {string} K", []ParamTag{{Name: "K", Type: "string"}}},
		{"typeParam", "@typeParam T - the item", []ParamTag{{Name: "T", Text: "the item"}}},
		{"lowercase", "@typeparam T", []ParamTag{{Name: "T"}}},
		{"empty", "@template", []ParamTag{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Parse(tt.raw).Templates
			if len(got) == 0 && len(tt.want) == 0 {
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %+v\nwant %+v", got, tt.want)
			}
		})
	}
}

// TestParseTypedefCallback covers scoping of @property, @param and @returns.
func TestParseTypedefCallback(t *testing.T) {
	raw := `
 * Top.
 * @param {string} a top param
 * @property {number} stray
 * @typedef {Object} Options Options for parse.
 *   More lines.
 *
 *   Body here.
 * @property {boolean} [strict=false] be strict
 * @prop {number} depth
 * @callback Done Called when done.
 * @param {Error} err the error
 * @returns {void}
 * @typedef Point
 * @property {number} x
 * @param late belongs to the comment
 * @returns {number} top result
 `
	c := Parse(raw)
	wantParams := []ParamTag{{Name: "a", Type: "string", Text: "top param"}, {Name: "late", Text: "belongs to the comment"}}
	if !reflect.DeepEqual(c.Params, wantParams) {
		t.Errorf("params %+v", c.Params)
	}
	if len(c.Doc.Tags) != 1 || c.Doc.Tags[0].Name != "property" || c.Doc.Tags[0].Text != "{number} stray" {
		t.Errorf("stray property %+v", c.Doc.Tags)
	}
	if len(c.Typedefs) != 2 {
		t.Fatalf("typedefs %+v", c.Typedefs)
	}
	opts := c.Typedefs[0]
	if opts.Name != "Options" || opts.Type != "Object" || opts.Doc.Summary != "Options for parse.\nMore lines." || opts.Doc.Body != "Body here." {
		t.Errorf("options %+v %+v", opts, opts.Doc)
	}
	wantProps := []ParamTag{{Name: "strict", Type: "boolean", Optional: true, Default: "false", Text: "be strict"}, {Name: "depth", Type: "number"}}
	if !reflect.DeepEqual(opts.Properties, wantProps) {
		t.Errorf("props %+v", opts.Properties)
	}
	if pt := c.Typedefs[1]; pt.Name != "Point" || pt.Type != "" || len(pt.Properties) != 1 {
		t.Errorf("point %+v", pt)
	}
	if len(c.Callbacks) != 1 {
		t.Fatalf("callbacks %+v", c.Callbacks)
	}
	cb := c.Callbacks[0]
	if cb.Name != "Done" || cb.Doc.Summary != "Called when done." || len(cb.Params) != 1 || cb.Params[0].Name != "err" {
		t.Errorf("callback %+v", cb)
	}
	if cb.Returns == nil || cb.Returns.Type != "void" {
		t.Errorf("callback returns %+v", cb.Returns)
	}
	if c.Returns == nil || c.Returns.Type != "number" || c.Doc.Returns != "top result" {
		t.Errorf("returns %+v %q", c.Returns, c.Doc.Returns)
	}
}
