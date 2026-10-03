package dts

import (
	"strings"
	"testing"

	"github.com/spagu/ssg/internal/apimodel"
	"github.com/spagu/ssg/internal/apisource"
)

// extracted is the result of extracting a package written in a test.
type extracted struct {
	t     *testing.T
	pkg   *apimodel.Package
	ix    *apimodel.Index
	diags []apisource.Diagnostic
}

// extractFiles writes files and extracts them as package "p", failing the
// test when the model does not validate.
func extractFiles(t *testing.T, files map[string]string, cfg apisource.Config) *extracted {
	t.Helper()
	cfg.Root = writeTree(t, files)
	if cfg.Name == "" {
		cfg.Name = "p"
	}
	pkg, diags, err := Extract(cfg)
	if err != nil {
		t.Fatal(err)
	}
	api := &apimodel.API{Packages: []*apimodel.Package{pkg}}
	if probs := apimodel.Validate(api); len(probs) > 0 {
		t.Errorf("model problems: %v", probs)
	}
	return &extracted{t: t, pkg: pkg, ix: apimodel.NewIndex(api), diags: diags}
}

// sym returns the symbol with this ID or fails the test.
func (x *extracted) sym(id string) *apimodel.Symbol {
	x.t.Helper()
	s, ok := x.ix.Symbol(id)
	if !ok {
		x.t.Fatalf("no symbol %s", id)
	}
	return s
}

// missing fails the test when a symbol with this ID exists.
func (x *extracted) missing(id string) {
	x.t.Helper()
	if _, ok := x.ix.Symbol(id); ok {
		x.t.Errorf("symbol %s should not be listed", id)
	}
}

// TestCommonJSExport checks `export =` makes the target the default export
// and `import x = require()` binds to it.
func TestCommonJSExport(t *testing.T) {
	x := extractFiles(t, map[string]string{
		"index.d.ts": `import other = require("./other");
import alias = other.Thing;
declare function main(o: other.Thing): main.Options;
declare namespace main {
    interface Options { a: number }
}
export = main;
export as namespace Main;`,
		"other.d.ts": `declare namespace o { class Thing {} }
export = o;`,
	}, apisource.Config{})
	s := x.sym("p/index#main")
	if s.Name != "main" || !s.Flags.Default || s.Kind != apimodel.KindFunction {
		t.Errorf("default = %+v", s)
	}
	if ref := s.Signatures[0].Returns.Ref; ref != "p/index#main.Options" {
		t.Errorf("return ref = %q", ref)
	}
	if ref := s.Signatures[0].Params[0].Type.Ref; ref != "" {
		t.Errorf("a type of an unlisted file resolved to %q", ref)
	}
}

// TestDefaultExports checks anonymous defaults and a default that is also
// exported by name.
func TestDefaultExports(t *testing.T) {
	x := extractFiles(t, map[string]string{
		"index.d.ts": `export default class {}
export * as all from "./b";`,
		"b.d.ts": `export declare function f(): void;
export default f;
export { f as g };`,
		"c.d.ts": "export default abstract class C {}",
		"f.d.ts": "export default async function () {}",
		"d.d.ts": "export default interface I {}",
		"e.d.ts": "declare const x: number;\nexport default { x };",
	}, apisource.Config{Entries: []string{"index.d.ts", "b.d.ts", "c.d.ts", "d.d.ts", "e.d.ts", "f.d.ts"}})
	if s := x.sym("p/index#default"); s.Name != "default" || s.Kind != apimodel.KindClass {
		t.Errorf("anonymous class = %+v", s)
	}
	if x.sym("p/b#f").Flags.Default || x.sym("p/b#g").Flags.Default || !x.sym("p/b#default").Flags.Default {
		t.Error("a default export whose name is taken is listed as default")
	}
	if s := x.sym("p/c#C"); !s.Flags.Abstract || !s.Flags.Default {
		t.Error("abstract default class")
	}
	x.sym("p/d#I")
	if s := x.sym("p/f#default"); s.Name != "default" || s.Kind != apimodel.KindFunction {
		t.Errorf("anonymous function = %+v", s)
	}
	x.sym("p/index#all.f")
	if len(x.pkg.Modules[4].Symbols) != 0 {
		t.Errorf("an object default export listed: %+v", x.pkg.Modules[4].Symbols)
	}
}

// TestImportsAndQualifiedNames checks the import forms and dotted type
// names through namespaces, file namespaces and enums.
func TestImportsAndQualifiedNames(t *testing.T) {
	x := extractFiles(t, map[string]string{
		"index.d.ts": `import "./side";
import type Def, * as ns from "./a";
import { type K as Kind, E } from "./a" with { type: "json" };
import json from "./data.json";
export declare function f(a: ns.A, b: Def, c: E.One, d: Kind, e: N.Inner.Deep, g: Missing.X): void;
export declare namespace N.Inner { interface Deep {} }
export { E, ns };`,
		"a.d.ts": `export interface A {}
export type K = string;
export declare enum E { One }
export default A;`,
		"side.d.ts": "declare global { interface Window { x: 1 } }\nexport {};",
	}, apisource.Config{})
	params := x.sym("p/index#f").Signatures[0].Params
	want := []string{"p/index#ns.A", "p/index#ns.A", "p/index#E.One", "p/index#ns.K", "p/index#N.Inner.Deep", ""}
	for i, w := range want {
		if params[i].Type.Ref != w {
			t.Errorf("param %s ref = %q, want %q", params[i].Name, params[i].Type.Ref, w)
		}
	}
}

// TestMergingAndScripts checks declaration merging and a script file
// without exports, whose declarations are all public.
func TestMergingAndScripts(t *testing.T) {
	x := extractFiles(t, map[string]string{
		"index.d.ts": `interface Box { a: string }
interface Box { b: number }
/** Makes boxes. */
declare function box(): Box;
declare namespace box { const size: number; }
declare namespace box { export const max: number; }
declare namespace Plain { let v: number }
declare namespace Plain { let w: number }
declare module "p/extra" { interface Extra {} }
declare module "p" { interface Merged {} }
declare module "other-shorthand";`,
	}, apisource.Config{})
	if n := len(x.sym("p/index#Box").Members); n != 2 {
		t.Errorf("merged interface has %d members", n)
	}
	x.sym("p/index#box.max")
	x.missing("p/index#box.size") // the second block exports explicitly
	x.sym("p/index#Plain.w")
	x.sym("p/extra#Extra")
	x.sym("p/index#Merged")
	if _, ok := x.ix.Module("p/other-shorthand"); ok {
		t.Error("a shorthand ambient module was listed")
	}
}

// TestRecovery checks a bad statement is reported and skipped, and the
// rest of the file still read.
func TestRecovery(t *testing.T) {
	x := extractFiles(t, map[string]string{
		"index.d.ts": `export declare function a(: number): void;
export declare const b: { x: (1 };
export interface C { m(: number): void; ok: string }
export declare function d(): void;
export declare class E { m(): void }}
foo bar;
export type F = ;
export declare function g(): Weird<;
export declare const h: number = 1 2;
export declare function i<T,>(x: T): T
export declare const z: number;`,
	}, apisource.Config{})
	x.sym("p/index#d")
	x.sym("p/index#z")
	x.sym("p/index#i")
	x.missing("p/index#a")
	if len(x.diags) < 5 {
		t.Errorf("diagnostics: %v", x.diags)
	}
	found := false
	for _, d := range x.diags {
		found = found || strings.Contains(d.Message, "type Weird<")
	}
	if found {
		t.Error("a type error escaped the statement")
	}
}
