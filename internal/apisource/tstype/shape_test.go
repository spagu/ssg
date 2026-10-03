package tstype

import (
	"reflect"
	"strings"
	"testing"

	"github.com/spagu/ssg/internal/apimodel"
)

// node shorthands keep the expected trees readable.
func name(n string, args ...*apimodel.TypeRef) *apimodel.TypeRef {
	return apimodel.Named(n, "", args...)
}

func of(kind apimodel.TypeKind, args ...*apimodel.TypeRef) *apimodel.TypeRef {
	return &apimodel.TypeRef{Kind: kind, Args: args}
}

func lit(s string) *apimodel.TypeRef  { return &apimodel.TypeRef{Kind: apimodel.TypeLiteral, Name: s} }
func verb(s string) *apimodel.TypeRef { return &apimodel.TypeRef{Kind: apimodel.TypeVerbatim, Name: s} }

func fn(params []*apimodel.Param, ret *apimodel.TypeRef) *apimodel.TypeRef {
	return &apimodel.TypeRef{Kind: apimodel.TypeFunction, Signature: &apimodel.Signature{Params: params, Returns: ret}}
}

// TestParseShape checks the tree, not only its rendering, for key cases.
func TestParseShape(t *testing.T) {
	ctor := fn([]*apimodel.Param{{Name: "x", Type: name("A")}}, name("B"))
	ctor.Name = "new"
	generic := fn([]*apimodel.Param{{Name: "x", Type: name("T")}}, name("T"))
	generic.Signature.TypeParams = []*apimodel.TypeParam{{Name: "T", Constraint: name("Base"), Default: name("D")}}
	cases := []struct {
		in   string
		want *apimodel.TypeRef
	}{
		{"Map<K, V>", name("Map", name("K"), name("V"))},
		{"A | B & C", of(apimodel.TypeUnion, name("A"), of(apimodel.TypeIntersection, name("B"), name("C")))},
		{"readonly string[][]", of(apimodel.TypeArray, of(apimodel.TypeArray, name("string")))},
		{"[a: A, B]", of(apimodel.TypeTuple, name("A"), name("B"))},
		{"'on' | -2", of(apimodel.TypeUnion, lit("'on'"), lit("-2"))},
		{"null", name("null")},
		{"?T", of(apimodel.TypeUnion, name("T"), name("null"))},
		{"new (x: A) => B", ctor},
		{"<T extends Base = D>(x: T) => T", generic},
		{"(a?: A, ...r: R[]) => void", fn([]*apimodel.Param{
			{Name: "a", Type: name("A"), Optional: true},
			{Name: "r", Type: of(apimodel.TypeArray, name("R")), Rest: true},
		}, name("void"))},
		{"{ a?: A; [k: string]: V }", &apimodel.TypeRef{Kind: apimodel.TypeObject, Fields: []*apimodel.Param{
			{Name: "a", Type: name("A"), Optional: true},
			{Name: "[k: string]", Type: name("V")},
		}}},
		{"keyof T", verb("keyof T")},
		{"A extends B\n  ? C\n  : D", verb("A extends B ? C : D")},
		{"`x-${string}`", verb("`x-${string}`")},
		{"`x-y`", lit("`x-y`")},
	}
	for _, c := range cases {
		got, err := Parse(c.in, nil)
		if err != nil {
			t.Errorf("Parse(%q): %v", c.in, err)
			continue
		}
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("Parse(%q) = %#v, want %#v", c.in, got, c.want)
		}
	}
}

// TestParseErrors checks that bad input yields the trimmed source verbatim
// together with an error.
func TestParseErrors(t *testing.T) {
	for _, in := range []string{
		"", "   ", "A |", "Map<K", "Map<>", "(A", "[A, B", "{ a: A", "{ a: A b: B }",
		"A extends B ? C", "A extends B C", "'open", "`open", "`a${b", "`a${'x}`",
		"A # B", "A B", "() =>", "(a: A", "{ -[K in T]: V }", "{ [K in T }",
		"typeof x.", "x is", "!", "{ ? }", "{ [a: A }", "{ [x }", "{ [x", "function(", "-", ")",
		"<T>(x: T)", "{ (x: A }", "import('x').Foo",
	} {
		got, err := Parse(in, nil)
		if err == nil {
			t.Errorf("Parse(%q): expected an error, got %q", in, got)
			continue
		}
		want := verb(strings.TrimSpace(in))
		if !reflect.DeepEqual(got, want) {
			t.Errorf("Parse(%q) = %#v, want verbatim %#v", in, got, want)
		}
	}
}

// TestParseTooDeep checks the nesting bound.
func TestParseTooDeep(t *testing.T) {
	in := ""
	for range maxDepth + 1 {
		in += "("
	}
	if _, err := Parse(in+"A", nil); err == nil {
		t.Fatal("expected a nesting error")
	}
}
