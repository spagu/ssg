package js

import (
	"testing"

	"github.com/spagu/ssg/internal/apimodel"
	"github.com/spagu/ssg/internal/apisource"
)

func TestClassMembers(t *testing.T) {
	m, _ := extractIndex(t, `
/**
 * @abstract
 * @template T
 */
export class Box extends mixin(Base) {
  static {}
  static size = 1;
  size = 'x';
  [Symbol.iterator]() {}
  'quoted'() {}
  /** @hidden */ hidden() {}
  /** @type {T} */
  get value() { return 1; }
  /** @param {T} v */
  set only(v) {}
  set value(v) {}
  get value() { return 2; }
  #secret() {}
  plain = 1
  next = this.plain
  next2() {}
}
export const Expr = class extends ns.Base {
  /** @returns {Box<string>} */
  get box() { return null; }
};`)
	box := symbolNamed(t, m, "Box")
	if !box.Flags.Abstract || len(box.TypeParams) != 1 || box.Extends[0].Kind != apimodel.TypeVerbatim {
		t.Errorf("box: %+v", box)
	}
	want := map[string]apimodel.Kind{
		"size": apimodel.KindProperty, "[Symbol.iterator]": apimodel.KindMethod, "quoted": apimodel.KindMethod,
		"value": apimodel.KindAccessor, "only": apimodel.KindAccessor, "plain": apimodel.KindProperty,
		"next": apimodel.KindProperty, "next2": apimodel.KindMethod,
	}
	got := map[string]apimodel.Kind{}
	for _, mem := range box.Members {
		got[mem.Name] = mem.Kind
	}
	for n, k := range want {
		if got[n] != k {
			t.Errorf("member %s: %s, want %s (have %v)", n, got[n], k, got)
		}
	}
	if _, ok := got["hidden"]; ok {
		t.Error("hidden member listed")
	}
	if v := symbolNamed(t, m, "Box.value"); v.Flags.Readonly || v.Type == nil || v.Type.Ref != "" {
		t.Errorf("value: %+v type %v", v.Flags, v.Type)
	}
	if o := symbolNamed(t, m, "Box.only"); o.Type == nil || o.Type.Name != "T" {
		t.Errorf("only: %v", o.Type)
	}
	if s := symbolNamed(t, m, "Box.size"); !s.Flags.Static {
		t.Error("the first size is static; the instance one is a duplicate and dropped")
	}
	if n := symbolNamed(t, m, "Box.next2"); n.Source == nil || n.Source.Line != 22 {
		t.Errorf("next2 source %+v", n.Source)
	}
	expr := symbolNamed(t, m, "Expr")
	if expr.Kind != apimodel.KindClass || expr.Extends[0].Name != "ns.Base" {
		t.Errorf("Expr: %+v", expr)
	}
	if b := symbolNamed(t, m, "Expr.box"); b.Type == nil || b.Type.Ref != "p/index#Box" || !b.Flags.Readonly {
		t.Errorf("box accessor: %+v", b)
	}
}

func TestSignatures(t *testing.T) {
	m, diags := extractIndex(t, `
/**
 * @param {Object} opts - Settings.
 * @param {boolean} opts.strict - Nested.
 * @param {string=} name
 * @param {...number} nums
 * @returns {Promise<<>}
 */
export function* gen({ strict }, name, ...nums) {}
/** @type {string} */
export var label = 'x', other = 2, flag = true, nothing = null, neg = -1;
export let a = 1, b
export const fn = function named() {};
export const obj = { /** Run. */ run() {} };`)
	g := symbolNamed(t, m, "gen")
	ps := g.Signatures[0].Params
	if len(ps) != 3 || ps[0].Name != "opts" || ps[0].Doc != "Settings." || !ps[1].Optional || !ps[2].Rest {
		t.Errorf("params %+v %+v %+v", ps[0], ps[1], ps[2])
	}
	if g.Doc == nil || g.Doc.Tags[len(g.Doc.Tags)-1].Name != "generator" {
		t.Errorf("generator not noted: %+v", g.Doc)
	}
	if g.Signatures[0].Returns == nil || g.Signatures[0].Returns.Kind != apimodel.TypeVerbatim ||
		!hasDiag(diags, apisource.Warning, "cannot read the type") {
		t.Errorf("bad type: %v %v", g.Signatures[0].Returns, diags)
	}
	types := map[string]string{"label": "string", "other": "number", "flag": "boolean", "nothing": "unknown", "neg": "unknown"}
	for n, want := range types {
		if got := symbolNamed(t, m, n).Type.String(); got != want {
			t.Errorf("%s: %s, want %s", n, got, want)
		}
	}
	if !hasSymbol(m, "a") || !hasSymbol(m, "b") || symbolNamed(t, m, "a").Flags.Readonly {
		t.Errorf("let names: %v", names(m.Symbols))
	}
	if symbolNamed(t, m, "fn").Kind != apimodel.KindFunction || symbolNamed(t, m, "obj").Kind != apimodel.KindVariable {
		t.Error("function expression and object literal kinds")
	}
}

func TestCommonJS(t *testing.T) {
	pkg, diags := extractTree(t, map[string]string{
		"index.js": "export * as a from './a.cjs';\nexport * as b from './b.cjs';\nexport * as c from './c.cjs';",
		"a.cjs":    "class Store {}\n/** Store. */\nmodule.exports = Store;",
		"b.cjs":    "/** Anon. */\nmodule.exports = class {};\nmodule.exports.extra = 1;\nexports.deep.x = 2;\nfoo.bar = 3;\nexports.y += 1;",
		"c.cjs":    "const o = {};\nmodule.exports = { [key]: 1, ...o, 'q': 2, #bad: 1 };",
	}, apisource.Config{})
	m := pkg.Modules[0]
	if s := symbolNamed(t, m, "a.Store"); !s.Flags.Default || s.Kind != apimodel.KindClass {
		t.Errorf("a.Store: %+v", s)
	}
	if s := symbolNamed(t, m, "b.default"); s.Kind != apimodel.KindClass || s.Doc == nil {
		t.Errorf("b.default: %+v", s)
	}
	if b := symbolNamed(t, m, "b"); len(b.Members) != 2 {
		t.Errorf("b members %v", names(b.Members))
	}
	n := 0
	for _, d := range diags {
		if d.Message == cjsWarning {
			n++
		}
	}
	if n != 2 {
		t.Errorf("want one CommonJS warning per file, got %v", diags)
	}
}

func TestPropertyNameForms(t *testing.T) {
	m, _ := extractIndex(t, "module.exports = { [k]: 1, ...o, 'q': 2, get g() { return 1; }, [m]() {} };")
	if got := names(m.Symbols); len(got) != 2 || !hasSymbol(m, "q") || !hasSymbol(m, "g") {
		t.Errorf("symbols %v", got)
	}
}
