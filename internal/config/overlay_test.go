package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// project writes a config and, when overlay is not "", the overlay beside it.
func project(t *testing.T, name, cfg, overlay string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}
	if overlay != "" {
		if err := os.WriteFile(filepath.Join(dir, OverlayFileName), []byte(overlay), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return path
}

// TestNoOverlayLeavesDataUntouched is the byte-for-byte promise.
func TestNoOverlayLeavesDataUntouched(t *testing.T) {
	src := "title: A  # comment\nfeeds: [{path: /a.xml}]\n"
	path := project(t, ".ssg.yaml", src, "")
	out, err := applyOverlay(path, []byte(src), ".yaml")
	if err != nil || string(out) != src {
		t.Fatalf("got %q, %v", out, err)
	}
	if _, err := applyOverlay(project(t, "c.toml", "", ""), []byte("x"), ".toml"); err != nil {
		t.Errorf("a TOML config without an overlay is fine: %v", err)
	}
}

func TestOverlayMergeRules(t *testing.T) {
	cfg := `title: From file
port: 9000
marketing:
  logo: /file-logo.png
  og_image: /file-card.png
image_formats: [webp, avif]
mermaid_theme: dark
`
	overlay := `{"title": "From overlay", "marketing": {"logo": "/over.png"},
	  "image_formats": ["webp"], "mermaid_theme": null, "toc_depth": 4, "rate_limit": 2.5,
	  "story": {"enabled": true, "transition": "fade"}, "pretty_urls": true}`
	got, err := Load(project(t, ".ssg.yaml", cfg, overlay))
	if err != nil {
		t.Fatal(err)
	}
	if got.Title != "From overlay" || got.Port != 9000 {
		t.Errorf("scalars: title %q port %d", got.Title, got.Port)
	}
	if got.Marketing.Logo != "/over.png" || got.Marketing.OGImage != "/file-card.png" {
		t.Errorf("mappings merge key by key: %+v", got.Marketing)
	}
	if !reflect.DeepEqual(got.ImageFormats, []string{"webp"}) {
		t.Errorf("a list replaces a list: %v", got.ImageFormats)
	}
	if got.MermaidTheme != "" {
		t.Errorf("null removes the key: %q", got.MermaidTheme)
	}
	if got.TOCDepth != 4 || got.RateLimit != 2.5 {
		t.Errorf("numbers: toc_depth %d rate_limit %v", got.TOCDepth, got.RateLimit)
	}
	if !got.Story.Enabled || got.Story.Settings["transition"] != "fade" {
		t.Errorf("open object: %+v", got.Story)
	}
	if !got.PrettyURLs.Enabled() {
		t.Error("alternate form true accepted")
	}
}

// TestOverlayDoesNotEditASharedAnchor: merging into an alias copies it, so
// the anchor's other users keep their value.
func TestOverlayDoesNotEditASharedAnchor(t *testing.T) {
	cfg := "print: &tool\n  enabled: true\n  label: Shared\nlisten: *tool\n"
	got, err := Load(project(t, ".ssg.yaml", cfg, `{"listen": {"label": "Hear"}}`))
	if err != nil {
		t.Fatal(err)
	}
	if got.Listen.Label != "Hear" || got.Print.Label != "Shared" || !got.Listen.Enabled {
		t.Errorf("listen %+v print %+v", got.Listen, got.Print)
	}
}

func TestOverlayOnEmptyAndOddConfigs(t *testing.T) {
	got, err := Load(project(t, ".ssg.yaml", "", `{"title": "T", "marketing": {"logo": "/l.png"}}`))
	if err != nil || got.Title != "T" || got.Marketing.Logo != "/l.png" {
		t.Errorf("empty config: %+v %v", got, err)
	}
	// A scalar replaces a mapping-to-be: marketing is a string in the file.
	if _, err := applyOverlay(project(t, ".ssg.yaml", "x: 1\n", `{"marketing": {"logo": "/l"}}`), []byte("marketing: oops\n"), ".yaml"); err != nil {
		t.Errorf("non-mapping value is replaced: %v", err)
	}
	path := project(t, ".ssg.yaml", "", `{"title": "T"}`)
	if _, err := applyOverlay(path, []byte("- a\n"), ".yaml"); err == nil || !strings.Contains(err.Error(), "not a mapping") {
		t.Errorf("list config: %v", err)
	}
	if _, err := applyOverlay(path, []byte("a: [\n"), ".yaml"); err == nil {
		t.Error("broken YAML is an error")
	}
	if _, err := Load(project(t, ".ssg.toml", "title = \"x\"\n", `{"title": "T"}`)); err == nil || !strings.Contains(err.Error(), "YAML config only") {
		t.Errorf("TOML + overlay: %v", err)
	}
}

func TestOverlayFileErrors(t *testing.T) {
	cases := map[string]string{
		`{"title": `:                          "invalid JSON",
		`{"title": "a"} {}`:                   "more than one value",
		`["title"]`:                           "top level must be an object",
		`{"titel": "x"}`:                      "titel: unknown configuration key",
		`{"port": "80"}`:                      "port: expected a number, got text",
		`{"toc_depth": 2.5}`:                  "toc_depth: expected a whole number",
		`{"rate_limit": true}`:                "rate_limit: expected a number, got true/false",
		`{"webp": 1}`:                         "webp: expected true or false, got a number",
		`{"title": []}`:                       "title: expected text, got a list",
		`{"image_formats": "x"}`:              "image_formats: expected a list, got text",
		`{"image_formats": [null]}`:           "image_formats[0]: null is not a list element",
		`{"colors": []}`:                      "colors: expected an object, got a list",
		`{"marketing": 3}`:                    "marketing: expected an object, got a number",
		`{"check_links": "on"}`:               `check_links: "on" is not one of "", "warn", "strict"`,
		`{"story": 3}`:                        "story: expected an object (or bool), got a number",
		`{"pretty_urls": 1}`:                  `expected one of "off", "strip", "strip-slash" (or bool)`,
		`{"jwt_secret": "hunter2"}`:           `jwt_secret: a secret must be written as an environment reference ("$NAME" or "${NAME}")`,
		`{"server_users": ["admin:hunter2"]}`: `server_users[0]: a secret must be written`,
		`{"notifications": [{"headers": {"Authorization": "Bearer abc"}}]}`: `notifications[0].headers.Authorization: a secret`,
		`{"workers": [{"auth": {"token": "abc"}}]}`:                         "workers[0].auth.token: a secret",
		`{"include": ["more.yaml"]}`:                                        "include: unknown configuration key",
		`{"listen": {"enabled": null, "nope": 1}}`:                          "listen.nope: unknown configuration key",
	}
	for overlay, want := range cases {
		_, err := Load(project(t, ".ssg.yaml", "title: x\n", overlay))
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: got %v, want %q", overlay, err, want)
		}
		if err != nil && !strings.Contains(err.Error(), OverlayFileName) {
			t.Errorf("%s: error does not name the overlay: %v", overlay, err)
		}
	}
}

func TestOverlaySecretReferencesPass(t *testing.T) {
	overlay := `{"jwt_secret": "$JWT", "mddb": {"api_key": "${MDDB_KEY}"}, "tts": {"api_key": ""},
	  "server_users": ["admin:$ADMIN_PW"], "colors": {"primary": null},
	  "notifications": [{"url": "https://example.com/hook", "headers": {"Authorization": "Bearer $TOKEN"}}],
	  "variables": {"anything": [1, {"a": true}]}, "sitemaps": [{"prefix": "/blog/"}],
	  "languages": [{"code": "en", "name": "English"}], "build_report": true, "check_links": "warn",
	  "listen": null}`
	got, err := Load(project(t, ".ssg.yaml", "listen: {enabled: true}\n", overlay))
	if err != nil {
		t.Fatal(err)
	}
	if got.JWTSecret != "$JWT" || got.Mddb.APIKey != "${MDDB_KEY}" || got.Listen.Enabled {
		t.Errorf("got %+v", got)
	}
	if len(got.LanguageConfigs) != 1 || got.BuildReport != DefaultBuildReport {
		t.Errorf("alternate forms: languages %+v report %q", got.LanguageConfigs, got.BuildReport)
	}
}

func TestOverlayReadErrorAndPath(t *testing.T) {
	if OverlayPath("") != "" {
		t.Error("no config, no overlay")
	}
	path := project(t, ".ssg.yaml", "title: x\n", "")
	if err := os.Mkdir(OverlayPath(path), 0o750); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Error("an unreadable overlay is an error, never skipped")
	}
}

func TestOverlayKeys(t *testing.T) {
	path := project(t, ".ssg.yaml", "", `{"title": "T", "marketing": {"logo": "/l"}, "colors": {}}`)
	keys, err := OverlayKeys(path)
	if err != nil || strings.Join(keys, ",") != "colors,marketing.logo,title" {
		t.Errorf("keys %v, %v", keys, err)
	}
	if keys, err := OverlayKeys(project(t, ".ssg.yaml", "", "")); err != nil || len(keys) != 0 {
		t.Errorf("no overlay: %v %v", keys, err)
	}
	if _, err := OverlayKeys(project(t, ".ssg.yaml", "", "{")); err == nil {
		t.Error("a broken overlay is reported")
	}
}

func TestOverlayHelpers(t *testing.T) {
	if normalizeNumbers("x") != "x" {
		t.Error("strings pass through")
	}
	if jsonKind(nil) != "null" {
		t.Error("null")
	}
	if secretExample(SecretInline) == secretExample(SecretWhole) || secretExample(SecretLogin) == secretExample(SecretWhole) {
		t.Error("formats are told apart")
	}
	if secretRefOK(SecretLogin, "no-colon") || !secretRefOK(SecretInline, "x ${A} y") {
		t.Error("secretRefOK")
	}
}
