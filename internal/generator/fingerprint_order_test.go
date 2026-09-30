package generator

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestFingerprintESModuleImportOrder is #309: presets.js sorts after the
// playground.js that imports it, and was left unhashed in the import.
func TestFingerprintESModuleImportOrder(t *testing.T) {
	out := t.TempDir()
	js := filepath.Join(out, "js")
	mustWrite(t, filepath.Join(js, "playground.js"),
		"import { transform } from \"./playground-core.js\";\nimport { presets } from \"./presets.js\";\n")
	mustWrite(t, filepath.Join(js, "playground-core.js"), "export const transform = 1;\n")
	mustWrite(t, filepath.Join(js, "presets.js"), "export const presets = [];\n")
	// CSS two levels deep: a.css → b.css → c.css, each with one @import.
	css := filepath.Join(out, "css")
	mustWrite(t, filepath.Join(css, "a.css"), "@import \"b.css\";\n")
	mustWrite(t, filepath.Join(css, "b.css"), "@import \"c.css\";\n")
	mustWrite(t, filepath.Join(css, "c.css"), "p{color:red}\n")
	mustWrite(t, filepath.Join(out, "index.html"), `<script type="module" src="/js/playground.js"></script><link rel="stylesheet" href="/css/a.css">`)

	g := &Generator{config: Config{OutputDir: out, Fingerprint: true, Quiet: true}}
	if err := g.fingerprintAssets(); err != nil {
		t.Fatal(err)
	}
	unhashed := regexp.MustCompile(`["/](playground-core|presets|playground|a|b|c)\.(js|css)["']`)
	for _, dir := range []string{js, css} {
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range entries {
			data, _ := os.ReadFile(filepath.Join(dir, e.Name()))
			if m := unhashed.FindString(string(data)); m != "" {
				t.Errorf("%s still names an unhashed asset: %s\n%s", e.Name(), m, data)
			}
			// Every reference must name a file that exists.
			for _, ref := range regexp.MustCompile(`[\w-]+\.[0-9a-f]{8}\.(?:js|css)`).FindAllString(string(data), -1) {
				if _, err := os.Stat(filepath.Join(dir, ref)); err != nil {
					t.Errorf("%s references %s, which does not exist", e.Name(), ref)
				}
			}
		}
	}
}

// TestOrderAssetsByReferencesCycle: a cycle cannot be ordered; its files come
// last, in input order, and are reported.
func TestOrderAssetsByReferencesCycle(t *testing.T) {
	dir := t.TempDir()
	a, b, c := filepath.Join(dir, "a.js"), filepath.Join(dir, "b.js"), filepath.Join(dir, "c.js")
	mustWrite(t, a, `import "./b.js";`)
	mustWrite(t, b, `import "./a.js";`)
	mustWrite(t, c, `export {};`)
	ordered, cycle, err := orderAssetsByReferences([]string{a, b, c})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(ordered, ",") != strings.Join([]string{c, a, b}, ",") {
		t.Errorf("ordered = %v", ordered)
	}
	if len(cycle) != 2 {
		t.Errorf("cycle = %v, want a and b", cycle)
	}
	g := &Generator{config: Config{OutputDir: dir}}
	if got, err := g.fingerprintOrder([]string{a, b, c}); err != nil || len(got) != 3 {
		t.Errorf("fingerprintOrder = %v, %v", got, err)
	}
	if _, _, err := orderAssetsByReferences([]string{filepath.Join(dir, "missing.js")}); err == nil {
		t.Error("an unreadable asset must be reported")
	}
}
