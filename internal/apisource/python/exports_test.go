package python

import (
	"testing"

	"github.com/spagu/ssg/internal/apisource"
)

func TestReExports(t *testing.T) {
	files := map[string]string{
		"p/__init__.py":     "from .a import *\nfrom .b import (Thing as Alias, Other,)\nfrom . import a\nfrom .loop import loop\n__all__ = ['Alias', 'Starred', 'a', 'loop', 'Other', 'Nope']\n__all__ += ['Thing']\n",
		"p/a.py":            "__all__ = ('Starred',)\nclass Starred: ...\nclass Hidden: ...\n",
		"p/b.py":            "from .a import Starred\nfrom ..outside import Far\nclass Thing:\n    def f(self, s: Starred, o: Far) -> 'Thing': ...\n",
		"p/loop.py":         "from .loop2 import loop\n",
		"p/loop2.py":        "from .loop import loop\n",
		"p/sub/__init__.py": "from ...p.b import Thing\nfrom .. import a\n__all__ = foo()\n",
		"p/sub/deep.py":     "from ..b import Thing\nfrom . import *\nfrom os import path\nfrom .missing import *\nx: Thing\n__all__ = ['x', 'Thing', 'path']\n",
	}
	pkg, diags := extractTree(t, files, apisource.Config{Name: "p"})
	init := moduleNamed(t, pkg, "p")
	if got := names(init.Symbols); !equal(got, []string{"Alias"}) {
		t.Errorf("p exports = %v", got)
	}
	for _, want := range []string{"Nope", "loop", "Other", "Thing"} {
		if !hasDiag(diags, apisource.Warning, "p/__init__.py", "lists "+want) {
			t.Errorf("no warning for %s: %v", want, diags)
		}
	}
	if hasDiag(diags, apisource.Warning, "p/__init__.py", "lists a,") {
		t.Error("a submodule in __all__ is not a missing name")
	}
	thing := symbolNamed(t, moduleNamed(t, pkg, "p/b"), "Thing")
	f := thing.Members[0].Signatures[0]
	if f.Params[0].Type.Ref != "p/p/a#Starred" || f.Params[1].Type.Ref != "" || f.Returns.Ref != "p/p/b#Thing" {
		t.Errorf("refs = %q %q %q", f.Params[0].Type.Ref, f.Params[1].Type.Ref, f.Returns.Ref)
	}
	if alias := symbolNamed(t, init, "Alias"); alias.ID != "p/p#Alias" || alias.Code != "class Thing" || alias.Source.File != "p/b.py" {
		t.Errorf("Alias = %s %q %s", alias.ID, alias.Code, alias.Source.File)
	}
	if got := names(moduleNamed(t, pkg, "p/sub").Symbols); len(got) != 0 {
		t.Errorf("p/sub (non-literal __all__) = %v", got)
	}
	if got := names(moduleNamed(t, pkg, "p/sub/deep").Symbols); !equal(got, []string{"x"}) {
		t.Errorf("p/sub/deep = %v", got)
	}
}

// A private module is read for its definitions and shown only through the
// public modules that re-export them.
func TestPrivateModules(t *testing.T) {
	pkg, _ := extractTree(t, map[string]string{
		"p/__init__.py": "from ._impl import Engine\n__all__ = ['Engine']\n",
		"p/_impl.py":    "class Engine: ...\n",
	}, apisource.Config{Name: "p"})
	if len(pkg.Modules) != 1 || !equal(names(moduleNamed(t, pkg, "p").Symbols), []string{"Engine"}) {
		t.Errorf("modules = %d", len(pkg.Modules))
	}
}

func TestResolveRelative(t *testing.T) {
	tests := []struct {
		pkg    string
		level  int
		module string
		want   string
	}{
		{"a/b", 0, "x/y", "x/y"},
		{"a/b", 1, "", "a/b"},
		{"a/b", 1, "c", "a/b/c"},
		{"a/b", 2, "c", "a/c"},
		{"a/b", 3, "c", "c"},
		{"a", 2, "", ""},
	}
	for _, tt := range tests {
		if got := resolveRelative(tt.pkg, tt.level, tt.module); got != tt.want {
			t.Errorf("resolveRelative(%q, %d, %q) = %q, want %q", tt.pkg, tt.level, tt.module, got, tt.want)
		}
	}
}
