package admintheme

import (
	"encoding/json"
	"regexp"
	"strings"
	"testing"
)

var always = func(string) bool { return true }

func full() Config {
	return Config{
		Name: "Anna's Kitchen", Logo: "/img/logo.svg", LogoDark: "/img/logo-dark.PNG", Main: "#1b3a4b", Accent: "#e07a5f",
		Font: "Georgia, serif", FontBody: `"Brand Sans", system-ui`,
		FontFiles: []FontFileConfig{{Family: "Brand Serif", Src: "/fonts/brand-serif.woff2", Weight: "400 700"}, {Family: "Brand Sans", Src: "/fonts/b.ttf", Style: "italic"}},
	}
}

func TestBuildFull(t *testing.T) {
	res, err := Build(".ssg.yaml", full(), always)
	if err != nil || len(res.Findings) != 0 {
		t.Fatal(err, res.Findings)
	}
	th := res.Theme
	if *th.Name != "Anna's Kitchen" || *th.Logo != "/img/logo.svg" || *th.LogoDark != "/img/logo-dark.PNG" || *th.Main != "#1b3a4b" || len(th.Derived) != 0 {
		t.Errorf("%+v", th)
	}
	if len(th.Light) != len(Themed) || len(th.Dark) != len(Themed) || len(th.Checks) == 0 || len(th.Adjustments) == 0 {
		t.Errorf("palette %v %v", th.Light, th.Adjustments)
	}
	if th.FontFiles[0].Format != "woff2" || th.FontFiles[1].Format != "truetype" || th.FontFiles[1].Weight != "normal" || th.FontFiles[1].Style != "italic" {
		t.Errorf("font files %+v", th.FontFiles)
	}
	css := string(res.CSS)
	for _, want := range []string{
		"@font-face {\n  font-family: \"Brand Serif\";\n  src: url(\"/fonts/brand-serif.woff2\") format(\"woff2\");\n  font-weight: 400 700;\n  font-style: normal;\n  font-display: swap;\n}",
		":root {\n  --font-display: \"Georgia\", \"Noto Serif\",", "\n  --font-ui: \"Brand Sans\", \"Noto Sans\",", "\n  --primary: #1b3a4b;\n",
		"@media (prefers-color-scheme: dark) {\n  :root:not([data-theme=\"light\"]) {\n    --primary: ", "\n:root[data-theme=\"dark\"] {\n  --primary: ",
	} {
		if !strings.Contains(css, want) {
			t.Errorf("theme.css lacks %q:\n%s", want, css)
		}
	}
	again, _ := Build(".ssg.yaml", full(), always)
	if string(again.CSS) != css {
		t.Error("two builds wrote different CSS")
	}
	a, _ := json.Marshal(res.Theme)
	b, _ := json.Marshal(again.Theme)
	if string(a) != string(b) {
		t.Error("two builds made different registry themes")
	}
}

// TestCSSOnlyDeclaresTokens: every line of theme.css is a comment, a
// selector or brace, or `name: value;` with a known name.
func TestCSSOnlyDeclaresTokens(t *testing.T) {
	res, _ := Build("f", full(), always)
	line := regexp.MustCompile(`^(?:\s*(?:/\*.*|\*.*|@font-face \{|:root \{|@media \(prefers-color-scheme: dark\) \{|:root:not\(\[data-theme="light"\]\) \{|:root\[data-theme="dark"\] \{|\}|)|\s+(--[a-z0-9-]+|font-family|src|font-weight|font-style|font-display): [^;{}@\\]+;)$`)
	for _, l := range strings.Split(string(res.CSS), "\n") {
		m := line.FindStringSubmatch(l)
		if m == nil {
			t.Errorf("unexpected line %q", l)
			continue
		}
		if name := strings.TrimPrefix(m[1], "--"); strings.HasPrefix(m[1], "--") && !IsThemed(name) && name != "font-display" && name != "font-ui" {
			t.Errorf("unknown token %q", m[1])
		}
	}
}

func TestBuildPartial(t *testing.T) {
	if res, err := Build("f", Config{}, always); err != nil || res.Theme != nil || res.CSS != nil {
		t.Error("no admin_theme, no theme")
	}
	res, err := Build("f", Config{Name: " Kuchnia Anny "}, always)
	if err != nil || *res.Theme.Name != "Kuchnia Anny" || res.CSS != nil || res.Theme.Main != nil || res.Theme.Logo != nil {
		t.Errorf("name only keeps the shipped theme.css: %v %+v", err, res.Theme)
	}
	res, _ = Build("f", Config{Accent: "#e07a5f"}, always)
	if res.Theme.Derived[0] != "main" || res.CSS == nil {
		t.Errorf("accent only: %+v", res.Theme)
	}
	res, _ = Build("f", Config{FontBody: "Verdana"}, always)
	if css := string(res.CSS); !strings.Contains(css, "--font-ui: \"Verdana\"") || strings.Contains(css, "--primary") || strings.Contains(css, "@media") {
		t.Errorf("font only:\n%s", css)
	}
	// Overrides without brand colours: the shipped palette, one token changed.
	res, err = Build("f", Config{Tokens: map[string]map[string]string{"dark": {"focus": "#ffcc80"}}}, always)
	if err != nil || len(res.Findings) != 0 || res.Theme.Dark["focus"] != "#ffcc80" || res.Theme.Light["primary"] != "#00695c" {
		t.Errorf("override only: %v %v %v", err, res.Findings, res.Theme.Dark)
	}
}

// TestBuildOverrideFindings: an override that breaks a pair is a finding,
// one per broken pair, naming the scheme and both colours.
func TestBuildOverrideFindings(t *testing.T) {
	c := full()
	c.Tokens = map[string]map[string]string{"light": {"primary": "#ffff00", "c2": "#1b3a4b"}}
	res, err := Build("f", c, always)
	if err != nil || res.Theme.Light["primary"] != "#ffff00" {
		t.Fatal(err)
	}
	all := ""
	for _, f := range res.Findings {
		all += f.Key + " " + f.Detail + "\n"
	}
	for _, want := range []string{"tokens light: primary #ffff00 on surface #ffffff is 1.07:1, needs 4.5:1", "light: c1 #1b3a4b and c2 #1b3a4b sit side by side"} {
		if !strings.Contains(all, want) {
			t.Errorf("missing %q in\n%s", want, all)
		}
	}
}

func TestBuildMissingFiles(t *testing.T) {
	res, err := Build("f", full(), func(p string) bool { return p == "/img/logo.svg" })
	if err != nil || len(res.Findings) != 3 {
		t.Fatal(err, res.Findings)
	}
	if res.Findings[0].Key != "logo_dark" || !strings.Contains(res.Findings[0].Detail, "/img/logo-dark.PNG is not in the build output") || res.Findings[2].Key != "font_files[1].src" {
		t.Errorf("%+v", res.Findings)
	}
}

// TestBuildRefuses: each key's errors name the file and the key.
func TestBuildRefuses(t *testing.T) {
	cases := map[string]func(*Config){
		`site.yaml: admin_theme.name must be plain text`:                          func(c *Config) { c.Name = "   " },
		`admin_theme.name is 81 characters long`:                                  func(c *Config) { c.Name = strings.Repeat("ä", 81) },
		`admin_theme.name contains the control character U+000A`:                  func(c *Config) { c.Name = "a\nb" },
		`admin_theme.name contains the control character U+202E`:                  func(c *Config) { c.Name = "a\u202eb" },
		`admin_theme.name must be plain text`:                                     func(c *Config) { c.Name = "a\xffb" },
		`admin_theme.logo "https://cdn.example.com/l.svg" must be a file on this`: func(c *Config) { c.Logo = "https://cdn.example.com/l.svg" },
		`admin_theme.logo_dark "//example.com/l.svg"`:                             func(c *Config) { c.LogoDark = "//example.com/l.svg" },
		`admin_theme.logo "/img/../x.svg"`:                                        func(c *Config) { c.Logo = "/img/../x.svg" },
		`admin_theme.logo "/img/logo.gif" is not an image the panels can show`:    func(c *Config) { c.Logo = "/img/logo.gif" },
		`admin_theme.main "#1b3a4bff" has an alpha channel`:                       func(c *Config) { c.Main = "#1b3a4bff" },
		`admin_theme.accent "coral" is not a hex colour`:                          func(c *Config) { c.Accent = "coral" },
		`admin_theme.font contains the character U+000A`:                          func(c *Config) { c.Font = "Georgia\n}" },
		`admin_theme.font_body "x;y" is not a font family`:                        func(c *Config) { c.FontBody = "x;y" },
		`admin_theme.font_files lists 9 files; 8 at most`:                         func(c *Config) { c.FontFiles = make([]FontFileConfig, 9) },
		`admin_theme.font_files[0].family`:                                        func(c *Config) { c.FontFiles[0].Family = "serif" },
		`admin_theme.font_files[1].src "/fonts/b.svg" is not a font file`:         func(c *Config) { c.FontFiles[1].Src = "/fonts/b.svg" },
		`admin_theme.font_files[0].src "/fonts/a b.woff2" must be a file`:         func(c *Config) { c.FontFiles[0].Src = "/fonts/a b.woff2" },
		`admin_theme.font_files[0].weight "heavy"`:                                func(c *Config) { c.FontFiles[0].Weight = "heavy" },
		`admin_theme.font_files[0].style "slanted"`:                               func(c *Config) { c.FontFiles[0].Style = "slanted" },
		`admin_theme.tokens.sepia is not a scheme`:                                func(c *Config) { c.Tokens = map[string]map[string]string{"sepia": {}} },
		`admin_theme.tokens.light.danger is not a token a theme can set`:          func(c *Config) { c.Tokens = map[string]map[string]string{"light": {"danger": "#000"}} },
		`admin_theme.tokens.dark.primary "#0000" has an alpha channel`:            func(c *Config) { c.Tokens = map[string]map[string]string{"dark": {"primary": "#0000"}} },
	}
	for want, change := range cases {
		c := full()
		change(&c)
		if _, err := Build("site.yaml", c, always); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("want %q, got %v", want, err)
		}
	}
}

// TestBuildHostileValues: every hostile string in every free-text field is
// refused or comes out harmless — never part of the CSS beyond a quoted
// font name of letters, and in JSON only as an escaped string.
func TestBuildHostileValues(t *testing.T) {
	values := append([]string{"</script><script>alert(1)</script>", "a\"b", "x\\y", "}{", "*/ :root { --x: 1 } /*", "javascript:alert(1)", "/img/x.svg?a=b", "/img/x.svg#y", "/img/%2e%2e/x.svg", "/img\\x.svg", "/.hidden/x.svg", "/img/x.svg/", "data:image/svg+xml,<svg/>"}, hostile...)
	fields := map[string]func(*Config, string){
		"name": func(c *Config, v string) { c.Name = v }, "logo": func(c *Config, v string) { c.Logo = v }, "logo_dark": func(c *Config, v string) { c.LogoDark = v },
		"main": func(c *Config, v string) { c.Main = v }, "accent": func(c *Config, v string) { c.Accent = v }, "font": func(c *Config, v string) { c.Font = v },
		"font_body": func(c *Config, v string) { c.FontBody = v }, "family": func(c *Config, v string) { c.FontFiles[0].Family = v },
		"src": func(c *Config, v string) { c.FontFiles[0].Src = v }, "weight": func(c *Config, v string) { c.FontFiles[0].Weight = v },
		"style": func(c *Config, v string) { c.FontFiles[0].Style = v }, "token": func(c *Config, v string) { c.Tokens = map[string]map[string]string{"light": {"focus": v}} },
		"token name": func(c *Config, v string) { c.Tokens = map[string]map[string]string{"light": {v: "#000"}} },
	}
	for field, set := range fields {
		for _, v := range values {
			if v == "" {
				continue // an empty value is an absent one
			}
			c := full()
			set(&c, v)
			res, err := Build("f", c, always)
			if err != nil {
				continue
			}
			if field != "name" {
				t.Errorf("%s %q was accepted", field, v)
				continue
			}
			if strings.Contains(string(res.CSS), v) {
				t.Errorf("the name reached theme.css: %q", v)
			}
			if data, err := json.Marshal(res.Theme); err != nil || !json.Valid(data) {
				t.Errorf("name %q: %v", v, err)
			}
		}
	}
}

func TestCSSNil(t *testing.T) {
	if CSS(nil) != nil {
		t.Error("no theme, no CSS")
	}
}
