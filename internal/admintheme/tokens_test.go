package admintheme

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var coreDir = filepath.Join("..", "..", "workers", "core")

// block returns the custom properties set in the block that opens with
// selector in tokens.css.
func block(t *testing.T, css, selector string) map[string]string {
	t.Helper()
	start := strings.Index(css, selector+" {")
	if start < 0 {
		t.Fatalf("no %s block", selector)
	}
	body := css[start : start+strings.Index(css[start:], "}")]
	out := map[string]string{}
	for _, m := range regexp.MustCompile(`--([\w-]+):\s*(#[0-9a-fA-F]{6})`).FindAllStringSubmatch(body, -1) {
		out[m[1]] = strings.ToLower(m[2])
	}
	return out
}

// TestShippedIsTokensCSS: Shipped is tokens.css, token for token, so the
// pairs are checked against the colours the panels really have — and every
// themed name is a token there (a theme cannot set a name no panel reads).
func TestShippedIsTokensCSS(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(coreDir, "public", "core", "tokens.css"))
	if err != nil {
		t.Fatal(err)
	}
	css := string(data)
	for scheme, selector := range map[string]string{"light": ":root", "dark": `:root[data-theme="dark"]`} {
		got := block(t, css, selector)
		if len(got) != len(Shipped[scheme]) {
			t.Errorf("%s: tokens.css has %d colours, Shipped %d", scheme, len(got), len(Shipped[scheme]))
		}
		for k, v := range got {
			if Shipped[scheme][k] != v {
				t.Errorf("%s --%s: tokens.css %s, Shipped %s", scheme, k, v, Shipped[scheme][k])
			}
		}
	}
	for _, k := range Themed {
		if !strings.Contains(css, "--"+k+":") {
			t.Errorf("--%s is not in tokens.css", k)
		}
	}
	shipped := map[string]map[string]RGB{}
	for s, m := range Shipped {
		shipped[s] = map[string]RGB{}
		for k, v := range m {
			shipped[s][k] = MustHex(v)
		}
	}
	if p := Verify(shipped); len(p) > 0 {
		t.Errorf("the shipped palette fails its own pairs: %v", p)
	}
}

// demoThemes are the three looks core's dev server shows (test/dev-server.mjs
// ?theme=…): a dark navy with a coral accent, a very light yellow with grey
// (the colours that need adjusting most), and one colour alone.
var demoThemes = map[string]Config{
	"harbour": {Name: "Harbour Kitchen", Logo: "/dev/logo.svg", LogoDark: "/dev/logo-dark.svg", Main: "#1b3a4b", Accent: "#e07a5f", Font: "Georgia, serif"},
	"lemon":   {Name: "Lemon Grove Studio", Main: "#fff59d", Accent: "#9e9e9e", FontBody: `"Trebuchet MS", sans-serif`},
	"plum":    {Name: "Plum & Pine", Logo: "/dev/missing-logo.svg", Main: "#6a1b9a"},
}

// TestDemoThemeFixtures: the dev server's demo themes are what this package
// computes. After an intended change, rewrite them with
//
//	UPDATE_THEME_FIXTURES=1 go test ./internal/admintheme/ -run DemoTheme
func TestDemoThemeFixtures(t *testing.T) {
	dir := filepath.Join(coreDir, "test", "fixtures", "themes")
	for name, c := range demoThemes {
		res, err := Build(".ssg.yaml", c, always)
		if err != nil {
			t.Fatal(err)
		}
		var data bytes.Buffer
		enc := json.NewEncoder(&data)
		enc.SetEscapeHTML(false) // as the registry is written (modules.Encode)
		enc.SetIndent("", "  ")
		_ = enc.Encode(res.Theme)
		files := map[string][]byte{name + ".json": data.Bytes(), name + ".css": res.CSS}
		for file, want := range files {
			path := filepath.Join(dir, file)
			if os.Getenv("UPDATE_THEME_FIXTURES") == "1" {
				if err := os.WriteFile(path, want, 0o644); err != nil { // #nosec G306 -- a test fixture in the repository
					t.Fatal(err)
				}
				continue
			}
			got, err := os.ReadFile(path) // #nosec G304 -- a test fixture in the repository
			if err != nil || string(got) != string(want) {
				t.Errorf("%s is out of date: UPDATE_THEME_FIXTURES=1 go test ./internal/admintheme/ -run DemoTheme", path)
			}
		}
	}
}
