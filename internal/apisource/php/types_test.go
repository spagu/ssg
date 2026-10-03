package php

import (
	"strings"
	"testing"

	"github.com/spagu/ssg/internal/apimodel"
)

func TestTypeRef(t *testing.T) {
	sc := newScope(`Acme\Lib`)
	sc.uses["alias"] = `Acme\Other\Thing`
	r := &resolver{byName: map[string]string{
		`acme\lib\doc`: "p/Acme/Lib#Doc", `acme\other\thing`: "p/Acme/Other#Thing", `acme\lib\base`: "p/Acme/Lib#Base",
		`acme\lib\sub\x`: "p/Acme/Lib/Sub#X",
	}}
	ctx := typeCtx{scope: sc, self: "p/Acme/Lib#Self", parent: "Base"}
	tests := []struct{ in, want string }{
		{"", "unknown"},
		{"int", "int"},
		{"?Doc", "Doc→p/Acme/Lib#Doc | null"},
		{"Doc|int|null", "Doc→p/Acme/Lib#Doc | int | null"},
		{"Doc&Alias", "Doc→p/Acme/Lib#Doc & Alias→p/Acme/Other#Thing"},
		{"(Doc&Alias)|null", "(Doc→p/Acme/Lib#Doc & Alias→p/Acme/Other#Thing) | null"},
		{`\Acme\Lib\Doc`, `Acme\Lib\Doc→p/Acme/Lib#Doc`},
		{`namespace\Doc`, `namespace\Doc→p/Acme/Lib#Doc`},
		{`Sub\X`, `Sub\X→p/Acme/Lib/Sub#X`},
		{`Alias\Inner`, `Alias\Inner`},
		{"self", "self→p/Acme/Lib#Self"},
		{"static", "static→p/Acme/Lib#Self"},
		{"$this", "$this→p/Acme/Lib#Self"},
		{"parent", "parent→p/Acme/Lib#Base"},
		{"Doc[]", "Doc→p/Acme/Lib#Doc[]"},
		{"array<string, Doc>", "array<string, Doc→p/Acme/Lib#Doc>"},
		{"list<int>[]", "list<int>[]"},
		{"'on'|'off'", "'on' | 'off'"},
		{"-1|2", "-1 | 2"},
		{"array{a: int|string}", "verbatim:array{a: int|string}"},
		{"callable(int): bool", "verbatim:callable(int): bool"},
		{"Foo<int", "verbatim:Foo<int"},
		{"Unknown", "Unknown"},
	}
	for _, tt := range tests {
		if got := render(r.typeRef(tt.in, ctx)); got != tt.want {
			t.Errorf("%q: got %q, want %q", tt.in, got, tt.want)
		}
	}
	if got := render(r.typeRef("parent", typeCtx{scope: sc})); got != "parent" {
		t.Errorf("parent outside a subclass = %q", got)
	}
}

func TestScopeResolve(t *testing.T) {
	global := newScope("")
	global.uses["x"] = `Vendor\X`
	tests := []struct {
		sc         *scope
		name, want string
	}{
		{global, "Foo", "Foo"},
		{global, `\Foo\Bar`, `Foo\Bar`},
		{global, "X", `Vendor\X`},
		{global, `X\Y`, `Vendor\X\Y`},
		{global, `namespace\Foo`, "Foo"},
		{newScope(`\A\B\`), "C", `A\B\C`},
	}
	for _, tt := range tests {
		if got := tt.sc.resolve(tt.name); got != tt.want {
			t.Errorf("%s: %q, want %q", tt.name, got, tt.want)
		}
	}
	if modulePath("") != "global" || modulePath(`A\B`) != "A/B" {
		t.Error("modulePath")
	}
}

// render prints a type like TypeRef.String, with "→ref" after a name that
// points at a symbol, parentheses around a nested compound and "verbatim:"
// before a type kept as written.
func render(t *apimodel.TypeRef) string {
	if t == nil {
		return "unknown"
	}
	join := func(sep string) string {
		parts := make([]string, len(t.Args))
		for i, a := range t.Args {
			parts[i] = render(a)
			if a.Kind == apimodel.TypeUnion || a.Kind == apimodel.TypeIntersection {
				parts[i] = "(" + parts[i] + ")"
			}
		}
		return strings.Join(parts, sep)
	}
	switch t.Kind {
	case apimodel.TypeUnion:
		return join(" | ")
	case apimodel.TypeIntersection:
		return join(" & ")
	case apimodel.TypeArray:
		return render(t.Args[0]) + "[]"
	case apimodel.TypeVerbatim:
		return "verbatim:" + t.Name
	}
	s := t.Name
	if t.Ref != "" {
		s += "→" + t.Ref
	}
	if len(t.Args) > 0 {
		s += "<" + join(", ") + ">"
	}
	return s
}
