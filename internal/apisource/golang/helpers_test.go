package golang

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spagu/ssg/internal/apisource"
)

func TestRef(t *testing.T) {
	b := &modBuilder{
		ix: &index{fset: token.NewFileSet(), types: map[string]map[string]string{
			"m":       {"T": "p/m#T"},
			"m/lexer": {"Token": "p/lexer#Token"},
		}},
		d: &pkgDir{importPath: "m", imports: map[string]map[string]string{"": {"lx": "m/lexer"}}},
	}
	tests := []struct {
		expr string
		tps  typeParamSet
		want string
	}{
		{"T", nil, "p/m#T"},
		{"T", typeParamSet{"T": true}, ""},
		{"*T", nil, "p/m#T"},
		{"[]T", nil, "p/m#T"},
		{"[4]*T", nil, "p/m#T"},
		{"map[string]T", nil, "p/m#T"},
		{"chan T", nil, "p/m#T"},
		{"(T)", nil, "p/m#T"},
		{"T[int]", nil, "p/m#T"},
		{"T[int, string]", nil, "p/m#T"},
		{"lx.Token", nil, "p/lexer#Token"},
		{"io.Reader", nil, ""},
		{"a.b.C", nil, ""},
		{"func() T", nil, ""},
		{"string", nil, ""},
	}
	for _, tt := range tests {
		e, err := parser.ParseExpr(tt.expr)
		if err != nil {
			t.Fatalf("%s: %v", tt.expr, err)
		}
		if got := b.ref(e, tt.tps); got != tt.want {
			t.Errorf("ref(%s) = %q, want %q", tt.expr, got, tt.want)
		}
	}
}

func TestChooseName(t *testing.T) {
	file := func(name string) *ast.File { return &ast.File{Name: ast.NewIdent(name)} }
	tests := []struct {
		names []string
		want  string
	}{
		{nil, ""},
		{[]string{"a"}, "a"},
		{[]string{"main", "lib"}, "lib"},
		{[]string{"lib", "main"}, "lib"},
		{[]string{"b", "a"}, "a"},
		{[]string{"main", "main", "lib"}, "main"},
	}
	for _, tt := range tests {
		var files []*ast.File
		for _, n := range tt.names {
			files = append(files, file(n))
		}
		if got := chooseName(files); got != tt.want {
			t.Errorf("chooseName(%v) = %q, want %q", tt.names, got, tt.want)
		}
	}
}

func TestSortDiags(t *testing.T) {
	got := sortDiags([]apisource.Diagnostic{
		{File: "b.go", Line: 1, Message: "x"},
		{File: "a.go", Line: 2, Message: "x"},
		{File: "a.go", Line: 2, Message: "a"},
		{File: "a.go", Line: 1, Message: "z"},
	})
	var s []string
	for _, d := range got {
		s = append(s, d.String())
	}
	want := "a.go:1: z|a.go:2: a|a.go:2: x|b.go:1: x"
	if strings.Join(s, "|") != want {
		t.Errorf("sortDiags = %v, want %s", s, want)
	}
}

func TestTruncate(t *testing.T) {
	long := strings.Repeat("é", maxValue+5)
	if got := truncate(long); len([]rune(got)) != maxValue || !strings.HasSuffix(got, "…") {
		t.Errorf("truncate kept %d runes: %q", len([]rune(got)), got)
	}
	if got := truncate("a(\n\tb)"); got != "a( b)" {
		t.Errorf("truncate = %q", got)
	}
}

// writeFiles creates files under a temporary root.
func writeFiles(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for name, body := range files {
		p := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestExtractUnreadable(t *testing.T) {
	root := writeFiles(t, map[string]string{
		"go.mod":      "go 1.27\n",
		"lib.go":      "package lib\n\n// A is a.\nfunc A() {}\n",
		"locked/x.go": "package locked\n",
	})
	if err := os.Symlink(filepath.Join(root, "nowhere.go"), filepath.Join(root, "dangling.go")); err != nil {
		t.Skip("no symlinks:", err)
	}
	locked := filepath.Join(root, "locked")
	if err := os.Chmod(locked, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(locked, 0o750) }) // #nosec G302 -- restore a test directory
	pkg, diags := extract(t, apisource.Config{Root: root})
	if want := filepath.Base(root); pkg.Name != want {
		t.Errorf("name = %q, want the directory name %q (go.mod without module)", pkg.Name, want)
	}
	files := map[string]bool{}
	for _, d := range diags {
		files[d.File] = true
	}
	if !files["dangling.go"] {
		t.Errorf("diagnostics = %v, want one for dangling.go", diags)
	}
	if os.Geteuid() != 0 && !files["locked"] {
		t.Errorf("diagnostics = %v, want one for locked", diags)
	}
}
