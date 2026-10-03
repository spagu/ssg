package dts

import (
	"strings"
	"testing"

	"github.com/spagu/ssg/internal/apimodel"
	"github.com/spagu/ssg/internal/apisource"
)

// TestMemberForms checks members whose names look like modifiers, property
// initializers, definite assignment, setters, parameter properties,
// destructured and defaulted parameters and lines without semicolons.
func TestMemberForms(t *testing.T) {
	x := extractFiles(t, map[string]string{
		"index.d.ts": `export declare class C {
    #secret: number
    static: boolean
    readonly
    get(key: string): string
    a!: number
    b = 1
    protected c
    set only(v: string)
    constructor(private readonly x: number, readonly: boolean, { a, b }: Opts, [first]: string[], n = 2)
    "quoted-name": string; 42: number
    m(this: C, s?: string): void { }
}
export interface Opts {
    a: "x"
    b: 1
    c: ` + "`t${string}`" + `
    d: Array<
        string
    >
    e: keyof
        Opts
}`,
	}, apisource.Config{})
	if s := x.sym("p/index#C.static"); s.Kind != apimodel.KindProperty || s.Flags.Static {
		t.Errorf("static = %+v", s)
	}
	x.sym("p/index#C.readonly")
	x.sym("p/index#C.a")
	x.sym("p/index#C.quoted-name")
	x.sym("p/index#C.42")
	if s := x.sym("p/index#C.get"); s.Kind != apimodel.KindMethod {
		t.Errorf("get = %+v", s)
	}
	if s := x.sym("p/index#C.b"); s.Type != nil {
		t.Errorf("an initialized property got a type: %+v", s.Type)
	}
	if s := x.sym("p/index#C.c"); s.Doc == nil || s.Doc.Tags[0].Name != "protected" {
		t.Errorf("protected member without a comment = %+v", s.Doc)
	}
	if s := x.sym("p/index#C.only"); s.Flags.Readonly || s.Type.Name != "string" {
		t.Errorf("setter = %+v", s)
	}
	params := x.sym("p/index#C.constructor").Signatures[0].Params
	names := []string{}
	for _, p := range params {
		names = append(names, p.Name)
	}
	if strings.Join(names, "|") != "x|readonly|{ a, b }|[first]|n" || !params[4].Optional || params[4].Default != "2" {
		t.Errorf("constructor params = %v %+v", names, params[4])
	}
	if p := x.sym("p/index#C.m").Signatures[0].Params; len(p) != 1 || !p[0].Optional {
		t.Errorf("method params = %+v", p)
	}
	x.missing("p/index#C.#secret")
	for _, name := range []string{"a", "b", "c", "d", "e"} {
		if s := x.sym("p/index#Opts." + name); s.Type == nil {
			t.Errorf("Opts.%s has no type", name)
		}
	}
	if len(x.diags) != 0 {
		t.Errorf("diagnostics: %v", x.diags)
	}
}

// TestSyntaxErrors checks each kind of syntax error becomes a diagnostic
// on the right line.
func TestSyntaxErrors(t *testing.T) {
	cases := map[string]string{
		"interface I { a? b }":            `expected ";" after a member`,
		"declare function f() x":          `expected ";"`,
		"export * from x;":                "expected a module specifier",
		"export * as ns './x';":           `expected "from"`,
		"declare const a = ;":             "expected a value",
		"declare const a: (x: [1) ;":      "unbalanced",
		"declare function f(a = ): void;": "expected a value",
		"declare function f(a: number;":   `expected ")"`,
		"declare function f(): A<B;":      "unclosed bracket",
		"declare class C { m(): void ":    "expected",
		"declare function f(): {":         "expected a type",
		"export = ;":                      "expected a name",
		"declare enum E { A B }":          `expected "}"`,
		"declare class C { m: x[ }":       "unbalanced",
		"export type T = | ;":             "type |",
	}
	for src, want := range cases {
		f, diags, err := parseFile("x.d.ts", src)
		if err != nil {
			t.Fatalf("%s: %v", src, err)
		}
		if f == nil || len(diags) == 0 || !strings.Contains(diags[0].Message, want) {
			if want == "type |" {
				continue // a type error is reported when the model is built
			}
			t.Errorf("%s: diagnostics %v, want %q", src, diags, want)
		}
	}
}

// TestTypeErrorReported checks a type tstype cannot parse is kept verbatim
// and reported with its line.
func TestTypeErrorReported(t *testing.T) {
	x := extractFiles(t, map[string]string{"index.d.ts": "\nexport type T = | ;"}, apisource.Config{})
	if s := x.sym("p/index#T"); s.Type.Kind != apimodel.TypeVerbatim {
		t.Errorf("type = %+v", s.Type)
	}
	if len(x.diags) != 1 || x.diags[0].Line != 2 || !strings.Contains(x.diags[0].Message, "type |") {
		t.Errorf("diagnostics: %v", x.diags)
	}
}

// TestBalanced checks an unclosed group at the end of a file.
func TestBalanced(t *testing.T) {
	_, diags, _ := parseFile("x.d.ts", "declare class C { [Symbol.iterator")
	if len(diags) == 0 || !strings.Contains(diags[0].Message, "unbalanced") {
		t.Errorf("diagnostics: %v", diags)
	}
}

// TestMiscForms checks a dotted `export =`, interface heritage, a
// documented protected member, private methods and an empty overview.
func TestMiscForms(t *testing.T) {
	x := extractFiles(t, map[string]string{
		"index.d.ts": `/** @packageDocumentation */
declare namespace ns {
    interface A {}
    interface B {}
    /** Both. */
    interface I extends A, B { e: E.Nope }
    enum E { One }
    class K {
        /** Kept. */
        protected x: number;
        #p(): void { }
        #q
    }
}
export = ns.I;`,
	}, apisource.Config{})
	s := x.sym("p/index#I")
	if s.Name != "I" || len(s.Extends) != 2 || s.Extends[0].Ref != "" {
		t.Errorf("default = %+v", s)
	}
	if x.pkg.Modules[0].Doc != nil {
		t.Errorf("an overview with only its tag = %+v", x.pkg.Modules[0].Doc)
	}
	k := extractFiles(t, map[string]string{"index.d.ts": `export declare class K {
    /** Kept. */
    protected x: number;
    #p(): void { }
    #q
}`}, apisource.Config{})
	if d := k.sym("p/index#K.x").Doc; d.Summary != "Kept." || d.Tags[0].Name != "protected" || len(k.sym("p/index#K").Members) != 1 {
		t.Errorf("protected = %+v", d)
	}
}
