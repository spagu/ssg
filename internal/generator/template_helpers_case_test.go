package generator

import (
	"html/template"
	"strings"
	"testing"

	"github.com/spagu/ssg/internal/models"
)

// TestCaseHelpers: a language switcher renders "EN PL RO" from the codes
// themselves, with no per-language branch in the theme (#280).
func TestCaseHelpers(t *testing.T) {
	g := &Generator{config: Config{Quiet: true}, siteData: &models.SiteData{}}
	for name, funcs := range map[string]template.FuncMap{
		"page":      g.buildTemplateFuncs(nil),
		"shortcode": g.shortcodeFuncMap(),
	} {
		tmpl := template.Must(template.New(name).Funcs(funcs).Parse(
			`{{range .}}{{upper .}} {{end}}|{{lower "PL"}}|{{title "hello wide world"}}`))
		var b strings.Builder
		if err := tmpl.Execute(&b, []string{"en", "pl", "ro"}); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if want := "EN PL RO |pl|Hello Wide World"; b.String() != want {
			t.Errorf("%s: got %q, want %q", name, b.String(), want)
		}
	}
}

// TestTitleIsLanguageNeutral: a template does not say which language a string
// is in, so title must not apply one language's special rules.
func TestTitleIsLanguageNeutral(t *testing.T) {
	if got := tmplTitle("ijssel łódź"); got != "Ijssel Łódź" {
		t.Errorf("got %q", got)
	}
}
