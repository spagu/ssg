package js

import (
	"testing"

	"github.com/spagu/ssg/internal/apimodel"
	"github.com/spagu/ssg/internal/apisource"
)

func TestDefaultExports(t *testing.T) {
	tests := []struct {
		name, src, want string
		kind            apimodel.Kind
	}{
		{"anonymous function", "export default function () {}", "default", apimodel.KindFunction},
		{"anonymous class", "export default class {}", "default", apimodel.KindClass},
		{"arrow", "/** Doubles. */\nexport default (x) => x * 2;", "default", apimodel.KindFunction},
		{"value", "export default 42;", "default", apimodel.KindVariable},
		{"named class", "export default class Parser {}", "Parser", apimodel.KindClass},
		{"identifier", "function run() {}\nexport default run;", "run", apimodel.KindFunction},
		{"alias", "const cfg = {};\nexport { cfg as default };", "cfg", apimodel.KindVariable},
		{"name taken", "export function go() {}\nexport default function go2() {}\nexport { go as go2 };", "default", apimodel.KindFunction},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, _ := extractIndex(t, tt.src)
			s := symbolNamed(t, m, tt.want)
			if s.Kind != tt.kind || !s.Flags.Default {
				t.Errorf("kind %s default %v", s.Kind, s.Flags.Default)
			}
		})
	}
}

func TestImportReexports(t *testing.T) {
	pkg, _ := extractTree(t, map[string]string{
		"index.js": `import { a as b } from './x.js';
import * as ns from './x.js';
import d from './x.js';
import { gone } from './x.js';
import ext from 'external';
export { b, ns, d, gone, ext };
export { missing } from './nowhere.js';
export * from './nowhere.js';
export * as lost from './nowhere.js';
export const { destructured } = {};`,
		"x.js": "/** A. */\nexport const a = 1;\nexport default class D {}",
	}, apisource.Config{})
	m := pkg.Modules[0]
	if s := symbolNamed(t, m, "b"); s.Source.File != "x.js" || s.Doc.Summary != "A." {
		t.Errorf("b: %+v", s)
	}
	if s := symbolNamed(t, m, "ns.a"); s.ID != "p/index#ns.a" {
		t.Errorf("ns.a id %s", s.ID)
	}
	if s := symbolNamed(t, m, "d"); s.Kind != apimodel.KindClass || s.Flags.Default {
		t.Errorf("d: %+v", s)
	}
	for _, n := range []string{"gone", "ext", "missing", "lost", "destructured"} {
		if hasSymbol(m, n) {
			t.Errorf("%s should not be documented", n)
		}
	}
}

func TestIncludeExclude(t *testing.T) {
	files := map[string]string{
		"index.js":        "export * from './src/a.js';\nexport * from './src/b.js';\nexport * from './vendor/c.js';",
		"src/a.js":        "export const a = 1;",
		"src/b.js":        "export const b = 1;",
		"vendor/c.js":     "export const c = 1;",
		"src/dir.js/x.js": "",
	}
	pkg, _ := extractTree(t, files, apisource.Config{Include: []string{"src/**"}, Exclude: []string{"**/b.js"}})
	m := pkg.Modules[0]
	if !hasSymbol(m, "a") || hasSymbol(m, "b") || hasSymbol(m, "c") {
		t.Errorf("symbols %v", names(m.Symbols))
	}
}

func TestSpecifierResolution(t *testing.T) {
	pkg, _ := extractTree(t, map[string]string{
		"index.js":      "export * from './lib';\nexport * from './util';\nexport * from './dir.js';\nexport * from '../out.js';",
		"lib/index.mjs": "export const fromIndex = 1;",
		"util.cjs":      "exports.fromCjs = 1;",
		"dir.js/a.js":   "",
	}, apisource.Config{})
	m := pkg.Modules[0]
	if !hasSymbol(m, "fromIndex") || !hasSymbol(m, "fromCjs") {
		t.Errorf("symbols %v", names(m.Symbols))
	}
}

func TestCycles(t *testing.T) {
	pkg, _ := extractTree(t, map[string]string{
		"index.js": "export * from './a.js';\nexport * as me from './index.js';\nexport { loop } from './a.js';",
		"a.js":     "export * from './index.js';\nexport { loop } from './index.js';\nexport const a = 1;",
	}, apisource.Config{})
	m := pkg.Modules[0]
	if !hasSymbol(m, "a") || hasSymbol(m, "loop") {
		t.Errorf("symbols %v", names(m.Symbols))
	}
	if me := symbolNamed(t, m, "me"); len(me.Members) != 0 {
		t.Errorf("a namespace of itself lists %v", names(me.Members))
	}
}

func TestSyntaxErrorEntry(t *testing.T) {
	pkg, diags := extractTree(t, map[string]string{"index.js": "export const = ;", "ok.js": "export const ok = 1;"},
		apisource.Config{Entries: []string{"index.js", "ok.js"}})
	if len(pkg.Modules) != 1 || !hasDiag(diags, apisource.Error, "syntax error") {
		t.Errorf("modules %d diags %v", len(pkg.Modules), diags)
	}
}

func TestModuleDoc(t *testing.T) {
	m, _ := extractIndex(t, "/** @file Tools for text. */\n\n/** @module\n * @since 2 */\nexport const a = 1;")
	if m.Doc == nil || m.Doc.Summary != "Tools for text." {
		t.Errorf("doc %+v", m.Doc)
	}
	m, _ = extractIndex(t, "/**\n * About.\n * @module x\n * @see y\n */\nexport const a = 1;")
	if m.Doc == nil || m.Doc.Summary != "About." || len(m.Doc.See) != 1 || len(m.Doc.Tags) != 0 {
		t.Errorf("doc %+v", m.Doc)
	}
}
