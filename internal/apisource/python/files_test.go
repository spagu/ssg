package python

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spagu/ssg/internal/apisource"
)

func TestFindFiles(t *testing.T) {
	tests := []struct {
		name  string
		files map[string]string
		cfg   apisource.Config
		want  []string // module paths
	}{
		{"src layout", map[string]string{
			"src/pkg/__init__.py": "", "src/pkg/a.py": "", "src/pkg/sub/__init__.py": "", "src/pkg/sub/b.py": "",
			"src/pkg/tests/t.py": "", "src/pkg/test_a.py": "", "src/pkg/a_test.py": "", "src/pkg/conftest.py": "",
			"src/pkg/__pycache__/x.py": "", "src/pkg/.hidden/y.py": "", "setup.py": "", "docs/conf.py": "",
		}, apisource.Config{}, []string{"pkg", "pkg/a", "pkg/sub", "pkg/sub/b"}},
		{"flat layout", map[string]string{"pkg/__init__.py": "", "pkg/m.py": "", "venv/lib.py": "", "other.py": ""},
			apisource.Config{}, []string{"pkg", "pkg/m"}},
		{"single modules", map[string]string{"one.py": "", "two.py": "", "setup.py": "", "test_one.py": ""},
			apisource.Config{}, []string{"one", "two"}},
		{"src single module", map[string]string{"src/one.py": ""}, apisource.Config{}, []string{"one"}},
		{"entries", map[string]string{"a/__init__.py": "", "a/x.py": "", "b/__init__.py": "", "b/y.py": "", "c.py": ""},
			apisource.Config{Entries: []string{"./b", "c.py"}}, []string{"b", "b/y", "c"}},
		{"globs", map[string]string{"p/__init__.py": "", "p/keep.py": "", "p/gen/x.py": "", "p/drop.py": ""},
			apisource.Config{Include: []string{"p/*.py"}, Exclude: []string{"**/drop.py"}}, []string{"p", "p/keep"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.cfg.Name == "" {
				tt.cfg.Name = "n"
			}
			pkg, _ := extractTree(t, tt.files, tt.cfg)
			var got []string
			for _, m := range pkg.Modules {
				got = append(got, m.Path)
			}
			if !equal(got, tt.want) {
				t.Errorf("modules = %v, want %v", got, tt.want)
			}
		})
	}
}

// equal reports whether two string slices are equal.
func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestFindFilesDiagnostics(t *testing.T) {
	files := map[string]string{"p/__init__.py": "", "p.py": "x = 1\n"}
	_, diags := extractTree(t, files, apisource.Config{Entries: []string{"p", "p.py", "missing.py", "../up.py"}})
	for _, want := range []struct{ file, text string }{{"missing.py", "entry not found"}, {"../up.py", "entry not found"}, {"p/__init__.py", "already read from p.py"}} {
		found := false
		for _, d := range diags {
			found = found || (d.File == want.file && strings.Contains(d.Message, want.text))
		}
		if !found {
			t.Errorf("no %q diagnostic for %s in %v", want.text, want.file, diags)
		}
	}
}

func TestExtractErrors(t *testing.T) {
	if _, _, err := Extract(apisource.Config{}); err == nil {
		t.Error("no root: want an error")
	}
	if _, _, err := Extract(apisource.Config{Root: filepath.Join(t.TempDir(), "absent")}); err == nil {
		t.Error("missing root: want an error")
	}
	if _, _, err := Extract(apisource.Config{Root: writeTree(t, map[string]string{"README.md": "x"})}); err == nil {
		t.Error("no modules: want an error")
	}
}

func TestPackageMeta(t *testing.T) {
	tests := []struct {
		name          string
		files         map[string]string
		cfg           apisource.Config
		name0, ver    string
		readmePresent bool
	}{
		{"configured", map[string]string{"pyproject.toml": "[project]\nname = \"a\"\n", "x/__init__.py": ""}, apisource.Config{Name: "cfg"}, "cfg", "", false},
		{"poetry", map[string]string{"pyproject.toml": "[project]\ndynamic = [\"version\"]\n[tool.poetry]\nname = 'po'\nversion = '2.0'\nversion = '3'\n", "x/__init__.py": ""}, apisource.Config{}, "po", "2.0", false},
		{"pyproject odd values", map[string]string{"pyproject.toml": "[project]\n# c\nnoequals\nversion = 1\nname = \"open\n", "top/__init__.py": "", "README.rst": "rst"}, apisource.Config{}, "top", "", true},
		{"setup.cfg", map[string]string{"setup.cfg": "[metadata]\n; c\nname: cfgname\nversion = 0.3\n", "x/__init__.py": "", "README": "r"}, apisource.Config{}, "cfgname", "0.3", true},
		{"two packages", map[string]string{"a/__init__.py": "", "b/__init__.py": ""}, apisource.Config{}, "", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pkg, _ := extractTree(t, tt.files, tt.cfg)
			if tt.name0 != "" && pkg.Name != tt.name0 || pkg.Version != tt.ver {
				t.Errorf("name/version = %q/%q, want %q/%q", pkg.Name, pkg.Version, tt.name0, tt.ver)
			}
			if tt.name0 == "" && (pkg.Name == "a" || pkg.Name == "b") {
				t.Errorf("name = %q, want the root directory's", pkg.Name)
			}
			if (pkg.Readme != "") != tt.readmePresent || pkg.Language != "python" {
				t.Errorf("readme = %q, language = %q", pkg.Readme, pkg.Language)
			}
		})
	}
}

func TestDetect(t *testing.T) {
	tests := []struct {
		name  string
		files map[string]string
		want  bool
	}{
		{"pyproject", map[string]string{"pyproject.toml": ""}, true},
		{"setup.py", map[string]string{"setup.py": ""}, true},
		{"package dir", map[string]string{"pkg/__init__.py": ""}, true},
		{"src package", map[string]string{"src/pkg/__init__.py": ""}, true},
		{"root package", map[string]string{"__init__.py": ""}, true},
		{"javascript", map[string]string{"package.json": "{}", "index.js": ""}, false},
	}
	for _, tt := range tests {
		if got := Detect(writeTree(t, tt.files)); got != tt.want {
			t.Errorf("%s: Detect = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestRootPackageAndName(t *testing.T) {
	root := writeTree(t, map[string]string{"__init__.py": "X = 1\n"})
	pkg, _, err := Extract(apisource.Config{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	if pkg.Modules[0].Path != "__init__" || pkg.Name != "__init__" {
		t.Errorf("path/name = %q/%q", pkg.Modules[0].Path, pkg.Name)
	}
	if rootName(".") == "" || rootName(filepath.Join(os.TempDir(), "x")) != "x" {
		t.Error("rootName")
	}
}
