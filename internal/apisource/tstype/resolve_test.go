package tstype

import (
	"slices"
	"testing"

	"github.com/spagu/ssg/internal/apimodel"
)

// refs collects "name=ref" for every TypeName in the tree, in source order.
func refs(t *apimodel.TypeRef) []string {
	if t == nil {
		return nil
	}
	var out []string
	if t.Kind == apimodel.TypeName {
		out = append(out, t.Name+"="+t.Ref)
	}
	for _, a := range t.Args {
		out = append(out, refs(a)...)
	}
	for _, f := range t.Fields {
		out = append(out, refs(f.Type)...)
	}
	if s := t.Signature; s != nil {
		for _, tp := range s.TypeParams {
			out = append(out, refs(tp.Constraint)...)
		}
		for _, p := range s.Params {
			out = append(out, refs(p.Type)...)
		}
		out = append(out, refs(s.Returns)...)
	}
	return out
}

// TestResolve checks that names resolve, built-ins do not, generics resolve
// by their bare name, and type parameters shadow outer names.
func TestResolve(t *testing.T) {
	resolve := func(name string) string { return "sym:" + name }
	cases := []struct {
		in   string
		want []string
	}{
		{"Token | string | null", []string{"Token=sym:Token", "string=", "null="}},
		{"Map<K, ns.V>", []string{"Map=sym:Map", "K=sym:K", "ns.V=sym:ns.V"}},
		{"Array.<T>", []string{"Array=sym:Array", "T=sym:T"}},
		{"<T>(x: T, y: U) => T", []string{"T=", "U=sym:U", "T="}},
		{"<T extends Base<T>>(x: T.Key) => void", []string{"Base=sym:Base", "T=", "T.Key=", "void="}},
		{"[(<T>() => T), T]", []string{"T=", "T=sym:T"}},
		{"{ m<T>(x: T): T; n: T }", []string{"T=", "T=", "T=sym:T"}},
	}
	for _, c := range cases {
		got, err := Parse(c.in, resolve)
		if err != nil {
			t.Errorf("Parse(%q): %v", c.in, err)
			continue
		}
		if r := refs(got); !slices.Equal(r, c.want) {
			t.Errorf("Parse(%q) refs = %v, want %v", c.in, r, c.want)
		}
	}
}

// TestResolveNil checks that a nil resolver leaves every Ref empty.
func TestResolveNil(t *testing.T) {
	got, err := Parse("Token<Other>", nil)
	if err != nil {
		t.Fatal(err)
	}
	if r := refs(got); !slices.Equal(r, []string{"Token=", "Other="}) {
		t.Errorf("refs = %v", r)
	}
}

// FuzzParse checks that no input panics and that a failure always comes
// with a verbatim node.
func FuzzParse(f *testing.F) {
	for _, s := range []string{
		"Map<K, V>", "(a: A) => B", "{ a?: A; [k: string]: V }", "`a${B}`",
		"A extends B ? C : D", "?function(string=): !Array.<*>", "[x: A, ...B[]]",
		"{ [K in keyof T]-?: T[K] }", "new <T>(x: T) => T", "(((",
	} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, in string) {
		got, err := Parse(in, func(string) string { return "x" })
		if got == nil {
			t.Fatalf("Parse(%q) returned nil", in)
		}
		if err != nil && got.Kind != apimodel.TypeVerbatim {
			t.Fatalf("Parse(%q) failed with a %s node", in, got.Kind)
		}
		_ = got.String()
	})
}
