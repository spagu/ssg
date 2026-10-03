package js

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/spagu/ssg/internal/apisource"
)

func TestEntryCandidates(t *testing.T) {
	tests := []struct {
		name       string
		configured []string
		pj         packageJSON
		want       []string
	}{
		{"configured", []string{"a.js"}, packageJSON{Main: "m.js"}, []string{"a.js"}},
		{"module", nil, packageJSON{Module: "esm.js", Main: "m.js"}, []string{"esm.js"}},
		{"main", nil, packageJSON{Main: "lib/index"}, []string{"lib/index"}},
		{"fallback", nil, packageJSON{}, []string{"index.js"}},
		{"exports", nil, packageJSON{Exports: json.RawMessage(`"./e.js"`), Main: "m.js"}, []string{"./e.js"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := entryCandidates(tt.configured, &tt.pj); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestExportEntries(t *testing.T) {
	tests := []struct {
		raw  string
		want []string
	}{
		{``, nil},
		{`"./index.js"`, []string{"./index.js"}},
		{`"./lib/index"`, []string{"./lib/index"}},
		{`"./index.d.ts"`, nil},
		{`{"import": "./esm.mjs", "require": "./cjs.cjs"}`, []string{"./esm.mjs"}},
		{`{"node": {"import": "./n.js"}, "default": "./d.js"}`, []string{"./d.js"}},
		{`{"import": {"types": "./t.d.ts", "default": "./i.js"}}`, []string{"./i.js"}},
		{`["./x.d.ts", "./a.js"]`, []string{"./a.js"}},
		{`["./x.d.ts"]`, nil},
		{`{"types": "./t.d.ts"}`, nil},
		{`{"import": "./x.d.ts", "default": "./d.js"}`, []string{"./d.js"}},
		{`42`, nil},
		{`{".": "./a.js", "./b": {"module": "./b.js"}, "./c/*": "./c/*.js", "./a": "./a.js", "./pkg.json": "./package.json"}`,
			[]string{"./a.js", "./b.js"}},
	}
	for _, tt := range tests {
		if got := exportEntries(json.RawMessage(tt.raw)); !reflect.DeepEqual(got, tt.want) {
			t.Errorf("exportEntries(%s) = %v, want %v", tt.raw, got, tt.want)
		}
	}
}

func TestExtractConfigErrors(t *testing.T) {
	if _, _, err := Extract(apisource.Config{}); err == nil {
		t.Error("no root: want an error")
	}
	if _, _, err := Extract(apisource.Config{Root: filepath.Join(t.TempDir(), "missing")}); err == nil {
		t.Error("missing root: want an error")
	}
	root := writeTree(t, map[string]string{"package.json": `{"main": "gone.js"}`})
	_, diags, err := Extract(apisource.Config{Root: root})
	if err == nil || !hasDiag(diags, apisource.Error, "entry point not found") {
		t.Errorf("no entry: err=%v diags=%v", err, diags)
	}
}

func TestExtractEntries(t *testing.T) {
	pkg, diags := extractTree(t, map[string]string{
		"package.json": `{"version": "2.0.0", "main": "lib/main"}`,
		"lib/main.js":  "export const a = 1;",
		"other.mjs":    "export const b = 2;",
		"readme.md":    "hello",
	}, apisource.Config{Entries: []string{"./lib/main", "lib/main.js", "../escape.js", "/abs.js", "other.mjs", "nope.js"}})
	if pkg.Version != "2.0.0" || pkg.Readme != "hello" {
		t.Errorf("version %q readme %q", pkg.Version, pkg.Readme)
	}
	if len(pkg.Modules) != 2 || pkg.Modules[0].ID != "p/lib/main" || pkg.Modules[1].Path != "other" {
		t.Errorf("modules: %v", pkg.Modules)
	}
	n := 0
	for _, d := range diags {
		if d.Message == "entry point not found" {
			n++
		}
	}
	if n != 3 {
		t.Errorf("want 3 missing entries, got %v", diags)
	}
}

func TestPackageNameAndJSON(t *testing.T) {
	root := writeTree(t, map[string]string{"package.json": `{"name":`, "index.js": "export const a = 1;"})
	pkg, diags, err := Extract(apisource.Config{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	if pkg.Name != filepath.Base(root) {
		t.Errorf("name = %q, want the directory name", pkg.Name)
	}
	if !hasDiag(diags, apisource.Error, "cannot read package.json") {
		t.Errorf("diags = %v", diags)
	}
	named, _, _ := Extract(apisource.Config{Root: writeTree(t, map[string]string{"package.json": `{"name": "fromjson"}`, "index.js": ""})})
	if named.Name != "fromjson" {
		t.Errorf("name = %q", named.Name)
	}
	if got := packageName(apisource.Config{Root: "."}, &packageJSON{}); got == "." || got == "" {
		t.Errorf("relative root name = %q", got)
	}
}

func TestExtractUnreadable(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root reads every file")
	}
	root := writeTree(t, map[string]string{"index.js": "export * from './locked.js';", "locked.js": "export const x = 1;"})
	if err := os.Chmod(filepath.Join(root, "locked.js"), 0); err != nil {
		t.Fatal(err)
	}
	_, diags, err := Extract(apisource.Config{Root: root, Name: "p"})
	if err != nil || !hasDiag(diags, apisource.Error, "cannot read the file") {
		t.Errorf("err=%v diags=%v", err, diags)
	}
}

func TestErrorHelpers(t *testing.T) {
	plain := errors.New("plain")
	if errorLine(plain) != 0 || errorMessage(plain) != "plain" || unwrapPath(plain) != "plain" {
		t.Error("plain errors pass through")
	}
	_, err := os.Open(filepath.Join(t.TempDir(), "x"))
	if msg := unwrapPath(err); strings.Contains(msg, "x") {
		t.Errorf("unwrapPath kept the path: %q", msg)
	}
}
