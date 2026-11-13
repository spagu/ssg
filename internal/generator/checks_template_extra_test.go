package generator

import (
	"strings"
	"testing"
)

// TestCheckTemplateExtraKeys: keys that left .Extra in 1.8.85 are reported
// with the field that holds them now; custom keys and record fields are not
// (#416).
func TestCheckTemplateExtraKeys(t *testing.T) {
	src := "<p>{{index .Post.Extra \"rating\"}}</p>\n" +
		"{{with index .Post.Extra \"schema\"}}{{.video.embedUrl}}{{end}}\n" +
		"{{if $p.Extra.alias_stubs}}x{{end}}\n" +
		"{{index .Extra \"title\"}} {{.Post.Extra.title}}\n"
	got := checkTemplateExtraKeys("single.html", src)
	if len(got) != 2 {
		t.Fatalf("findings = %+v", got)
	}
	want := []struct {
		line       int
		key, field string
		fragment   string
	}{
		{2, "schema", "Schema", "use .Post.Schema instead"},
		{3, "alias_stubs", "AliasStubs", "use $p.AliasStubs instead"},
	}
	for i, w := range want {
		f := got[i]
		if f.Line != w.line || f.Key != w.key || f.Field != w.field {
			t.Errorf("finding %d = %+v, want %+v", i, f, w)
		}
		if msg := f.Message(); !strings.Contains(msg, w.fragment) || !strings.HasPrefix(msg, "single.html:") {
			t.Errorf("message %d = %q", i, msg)
		}
	}
	if checkTemplateExtraKeys("clean.html", "{{index .Page.Extra \"rating\"}} {{.Page.Schema}}") != nil {
		t.Error("custom keys and page fields must not be reported")
	}
}
