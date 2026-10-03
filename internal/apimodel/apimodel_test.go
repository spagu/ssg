package apimodel

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var update = flag.Bool("update", false, "rewrite testdata/api.golden.json")

// fixture is a small library: a class with a constructor, a method with an
// overload, a property, a generic function, a type alias, an enum, and a
// re-used type reference between modules.
func fixture() *API {
	lex := ModuleID("core", "./src/lex.js")
	idx := ModuleID("core", "src/index.ts")
	token := SymbolID(lex, "Token")
	lexer := SymbolID(lex, "Lexer")
	deprecated := "use Lexer.next"
	return &API{Packages: []*Package{{
		Name: "core", Version: "1.2.0", Readme: "# core",
		Modules: []*Module{
			{ID: idx, Path: "src/index", Symbols: []*Symbol{{
				ID: SymbolID(idx, "parse"), Name: "parse", Kind: KindFunction,
				TypeParams: []*TypeParam{{Name: "T", Constraint: Named("Token", token)}},
				Signatures: []*Signature{{
					TypeParams: []*TypeParam{{Name: "T", Constraint: Named("Token", token)}},
					Params: []*Param{
						{Name: "src", Type: Named("string", "")},
						{Name: "opts", Type: &TypeRef{Kind: TypeObject, Fields: []*Param{{Name: "strict", Type: Named("boolean", ""), Optional: true}}}, Optional: true},
					},
					Returns: Named("Promise", "", &TypeRef{Kind: TypeArray, Args: []*TypeRef{Named("T", "")}}),
				}},
				Flags:  Flags{Async: true, Stability: Beta},
				Source: &Source{File: "src/index.ts", Line: 12},
				Doc:    &Doc{Summary: "Parses source.", Examples: []string{"```js\nparse(\"a\")\n```"}, Tags: []Tag{{Name: "remarks", Text: "fast"}}},
			}}},
			{ID: lex, Path: "src/lex", Symbols: []*Symbol{
				{ID: token, Name: "Token", Kind: KindType, Type: &TypeRef{Kind: TypeUnion, Args: []*TypeRef{
					{Kind: TypeLiteral, Name: `"word"`}, {Kind: TypeLiteral, Name: `"space"`}}}},
				{ID: lexer, Name: "Lexer", Kind: KindClass, Implements: []*TypeRef{Named("Iterable", "", Named("Token", token))},
					Members: []*Symbol{
						{ID: MemberID(lexer, "next"), Name: "next", Kind: KindMethod, Signatures: []*Signature{
							{Returns: Named("Token", token)},
							{Params: []*Param{{Name: "n", Type: Named("number", "")}}, Returns: &TypeRef{Kind: TypeArray, Args: []*TypeRef{Named("Token", token)}}},
						}},
						{ID: MemberID(lexer, "constructor"), Name: "constructor", Kind: KindConstructor, Signatures: []*Signature{
							{Params: []*Param{{Name: "src", Type: Named("string", "")}}}}},
						{ID: MemberID(lexer, "pos"), Name: "pos", Kind: KindProperty, Type: Named("number", ""), Flags: Flags{Readonly: true}},
						{ID: MemberID(lexer, "old"), Name: "old", Kind: KindMethod, Doc: &Doc{Deprecated: &deprecated}},
					}},
				{ID: SymbolID(lex, "Mode"), Name: "Mode", Kind: KindEnum, Members: []*Symbol{
					{ID: MemberID(SymbolID(lex, "Mode"), "Strict"), Name: "Strict", Kind: KindEnumMember}}},
			}},
		},
	}}}
}

// TestRoundTripGolden: the fixture encodes to the golden file, and decoding
// it and encoding again gives the same bytes.
func TestRoundTripGolden(t *testing.T) {
	data, err := Encode(fixture())
	if err != nil {
		t.Fatal(err)
	}
	golden := filepath.Join("testdata", "api.golden.json")
	if *update {
		if err := os.WriteFile(golden, data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatalf("%v (run go test -update once)", err)
	}
	if !bytes.Equal(data, want) {
		t.Errorf("encoding changed:\n%s", data)
	}
	back, err := Decode(data)
	if err != nil {
		t.Fatal(err)
	}
	again, _ := Encode(back)
	if !bytes.Equal(again, data) {
		t.Error("decode then encode must give the same bytes")
	}
	if strings.Contains(string(data), `<`) {
		t.Error("type parameters must stay readable, not HTML-escaped")
	}
}

// TestEncodeIsOrderIndependent: shuffling packages, modules, symbols and
// members does not change a byte.
func TestEncodeIsOrderIndependent(t *testing.T) {
	a, b := fixture(), fixture()
	m := b.Packages[0].Modules
	m[0], m[1] = m[1], m[0]
	syms := m[0].Symbols
	syms[0], syms[2] = syms[2], syms[0]
	b.Packages = append([]*Package{{Name: "zz", Modules: []*Module{}}}, b.Packages...)
	a.Packages = append(a.Packages, &Package{Name: "zz", Modules: []*Module{}})
	ea, _ := Encode(a)
	eb, _ := Encode(b)
	if !bytes.Equal(ea, eb) {
		t.Error("the order sources were met in must not show in api.json")
	}
}

func TestDecodeErrors(t *testing.T) {
	if _, err := Decode([]byte("{")); err == nil {
		t.Error("invalid JSON must fail")
	}
	if _, err := Decode([]byte(`{"schema": 99, "packages": []}`)); err == nil || !strings.Contains(err.Error(), "schema 99") {
		t.Errorf("another schema must be refused by number, got %v", err)
	}
}

func TestIDs(t *testing.T) {
	for in, want := range map[string]string{
		"./src/parser/lex.js": "core/src/parser/lex", `src\a.d.ts`: "core/src/a",
		"index.mjs": "core/index", "types.d.cts": "core/types", "plain": "core/plain",
	} {
		if got := ModuleID("core", in); got != want {
			t.Errorf("ModuleID(%q) = %q, want %q", in, got, want)
		}
	}
	id := MemberID(SymbolID("core/lex", "Lexer"), "next")
	if id != "core/lex#Lexer.next" || ModuleOf(id) != "core/lex" || Anchor(id) != "Lexer.next" {
		t.Errorf("id = %q, module %q, anchor %q", id, ModuleOf(id), Anchor(id))
	}
	for id, want := range map[string]string{"m#$": "symbol", "m#a b<c>": "a-b-c", "m": ""} {
		if got := Anchor(id); got != want {
			t.Errorf("Anchor(%q) = %q, want %q", id, got, want)
		}
	}
}

func TestTypeStrings(t *testing.T) {
	fn := &TypeRef{Kind: TypeFunction, Signature: &Signature{Params: []*Param{{Name: "xs", Type: Named("T", ""), Rest: true}}, Returns: Named("void", "")}}
	for _, tc := range []struct {
		t    *TypeRef
		want string
	}{
		{nil, "unknown"},
		{Named("Map", "", Named("string", ""), Named("number", "")), "Map<string, number>"},
		{&TypeRef{Kind: TypeIntersection, Args: []*TypeRef{Named("A", ""), Named("B", "")}}, "A & B"},
		{&TypeRef{Kind: TypeArray, Args: []*TypeRef{{Kind: TypeUnion, Args: []*TypeRef{Named("A", ""), Named("B", "")}}}}, "(A | B)[]"},
		{&TypeRef{Kind: TypeArray}, "unknown[]"},
		{&TypeRef{Kind: TypeTuple, Args: []*TypeRef{Named("A", ""), Named("B", "")}}, "[A, B]"},
		{fn, "(...xs: T) => void"},
		{&TypeRef{Kind: TypeFunction}, "() => unknown"},
		{&TypeRef{Kind: TypeVerbatim, Name: "T extends U ? X : Y"}, "T extends U ? X : Y"},
	} {
		if got := tc.t.String(); got != tc.want {
			t.Errorf("String() = %q, want %q", got, tc.want)
		}
	}
	sig := fixture().Packages[0].Modules[0].Symbols[0].Signatures[0]
	if got := sig.Declaration("parse"); got != "parse<T extends Token>(src: string, opts?: { strict?: boolean }): Promise<T[]>" {
		t.Errorf("Declaration = %q", got)
	}
	if got := (&Signature{}).Declaration("f"); got != "f(): unknown" {
		t.Errorf("empty Declaration = %q", got)
	}
}

func TestIndex(t *testing.T) {
	a := fixture()
	ix := NewIndex(a)
	if ix.Len() != 9 {
		t.Errorf("Len = %d, want 9", ix.Len())
	}
	if _, ok := ix.Module("core/src/lex"); !ok {
		t.Error("module not indexed")
	}
	if s, ok := ix.Symbol("core/src/lex#Lexer.next"); !ok || s.Kind != KindMethod {
		t.Error("member not indexed")
	}
	if ix.Parent("core/src/lex#Lexer.next") != "core/src/lex#Lexer" || ix.Parent("core/src/lex#Lexer") != "" {
		t.Error("Parent")
	}
}

func TestValidate(t *testing.T) {
	if probs := Validate(fixture()); len(probs) != 0 {
		t.Fatalf("the fixture is valid, got %v", probs)
	}
	a := fixture()
	mod := a.Packages[0].Modules[1]
	mod.Symbols = append(mod.Symbols,
		&Symbol{ID: "", Name: "x", Kind: KindVariable},
		&Symbol{ID: "core/src/lex#Token", Name: "Token", Kind: KindType},
		&Symbol{ID: "other/mod#Stray", Name: "Stray", Kind: "gadget",
			Type:    Named("Ghost", "core/src/lex#Ghost"),
			Members: []*Symbol{{ID: "elsewhere#m", Name: "m", Kind: KindMethod}}})
	var got []string
	for _, p := range Validate(a) {
		got = append(got, p.String())
	}
	joined := strings.Join(got, "\n")
	for _, want := range []string{"(empty): an ID is empty", "core/src/lex#Token: the ID is used twice",
		`other/mod#Stray: is not under "core/src/lex"`, `other/mod#Stray: unknown kind "gadget"`,
		`other/mod#Stray: refers to "core/src/lex#Ghost"`, `elsewhere#m: is not under "other/mod#Stray"`} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing problem %q in:\n%s", want, joined)
		}
	}
}

// TestSchemaDescribesEveryField: every JSON key the model writes is described
// in docs/api-model.schema.json, so the published contract cannot drift from
// the code without this failing.
func TestSchemaDescribesEveryField(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "docs", "api-model.schema.json"))
	if err != nil {
		t.Fatal(err)
	}
	var schema any
	if err := json.Unmarshal(raw, &schema); err != nil {
		t.Fatalf("the schema is not JSON: %v", err)
	}
	described := map[string]bool{}
	var collect func(v any)
	collect = func(v any) {
		switch x := v.(type) {
		case map[string]any:
			if props, ok := x["properties"].(map[string]any); ok {
				for k := range props {
					described[k] = true
				}
			}
			for _, c := range x {
				collect(c)
			}
		case []any:
			for _, c := range x {
				collect(c)
			}
		}
	}
	collect(schema)
	data, _ := Encode(fixture())
	var doc any
	_ = json.Unmarshal(data, &doc)
	var check func(v any)
	check = func(v any) {
		switch x := v.(type) {
		case map[string]any:
			for k, c := range x {
				if !described[k] {
					t.Errorf("api.json writes %q, which the schema does not describe", k)
				}
				check(c)
			}
		case []any:
			for _, c := range x {
				check(c)
			}
		}
	}
	check(doc)
}
