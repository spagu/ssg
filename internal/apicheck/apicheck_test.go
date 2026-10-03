package apicheck

import (
	"strings"
	"testing"

	"github.com/spagu/ssg/internal/apimodel"
)

func TestCheck(t *testing.T) {
	empty := ""
	dep := "use b"
	doc := &apimodel.Doc{Summary: "Documented."}
	api := &apimodel.API{Packages: []*apimodel.Package{{Name: "p", Modules: []*apimodel.Module{{ID: "p/m", Symbols: []*apimodel.Symbol{
		{ID: "p/m#bare", Name: "bare", Kind: apimodel.KindFunction, Source: &apimodel.Source{File: "m.js", Line: 12}},
		{ID: "p/m#good", Name: "good", Kind: apimodel.KindFunction, Doc: &apimodel.Doc{Summary: "Fine.", Deprecated: &dep,
			Examples: []string{"```js\ngood()\n```"}},
			Signatures: []*apimodel.Signature{{Params: []*apimodel.Param{{Name: "a", Doc: "the a"}}}}},
		{ID: "p/m#half", Name: "half", Kind: apimodel.KindFunction, Doc: &apimodel.Doc{Summary: "Half.", Deprecated: &empty,
			Examples: []string{"half()"}},
			Signatures: []*apimodel.Signature{{Params: []*apimodel.Param{{Name: "x"}}}}},
		{ID: "p/m#C", Name: "C", Kind: apimodel.KindClass, Doc: doc, Members: []*apimodel.Symbol{
			{ID: "p/m#C.constructor", Name: "constructor", Kind: apimodel.KindConstructor},
			{ID: "p/m#C.m", Name: "m", Kind: apimodel.KindMethod}}},
		{ID: "p/m#U", Name: "U", Kind: apimodel.KindClass, Members: []*apimodel.Symbol{
			{ID: "p/m#U.m", Name: "m", Kind: apimodel.KindMethod}}},
		{ID: "p/m#E", Name: "E", Kind: apimodel.KindEnum, Doc: doc, Members: []*apimodel.Symbol{
			{ID: "p/m#E.A", Name: "A", Kind: apimodel.KindEnumMember}}},
	}}}}}}
	var got []string
	for _, f := range Check(api) {
		got = append(got, f.Where+" "+f.Message)
	}
	want := []string{
		"m.js:12 bare: no description",
		"p/m#C.m m: no description",
		"p/m#U U: no description",
		"p/m#half half: @deprecated does not say what to use instead",
		"p/m#half half: an @example is not a code block",
		"p/m#half half: parameter x has no description",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("findings:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func TestItoa(t *testing.T) {
	if itoa(0) != "0" || itoa(-3) != "0" || itoa(1203) != "1203" {
		t.Error("itoa")
	}
}
