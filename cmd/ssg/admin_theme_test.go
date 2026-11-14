package main

import (
	"reflect"
	"testing"

	"github.com/spagu/ssg/internal/admintheme"
	"github.com/spagu/ssg/internal/config"
)

// TestAdminThemeOf (GO-152): admin_theme: reaches the build unchanged; an
// absent section stays nil.
func TestAdminThemeOf(t *testing.T) {
	if adminThemeOf(nil) != nil {
		t.Error("no admin_theme → nil")
	}
	in := &config.AdminThemeConfig{Name: "N", Logo: "/l.svg", LogoDark: "/d.svg", Main: "#123", Accent: "#456", Font: "Georgia", FontBody: "Verdana",
		FontFiles: []config.AdminFontFile{{Family: "F", Src: "/f.woff2", Weight: "400", Style: "italic"}},
		Tokens:    map[string]map[string]string{"dark": {"focus": "#ffcc80"}}}
	want := &admintheme.Config{Name: "N", Logo: "/l.svg", LogoDark: "/d.svg", Main: "#123", Accent: "#456", Font: "Georgia", FontBody: "Verdana",
		FontFiles: []admintheme.FontFileConfig{{Family: "F", Src: "/f.woff2", Weight: "400", Style: "italic"}},
		Tokens:    map[string]map[string]string{"dark": {"focus": "#ffcc80"}}}
	if got := adminThemeOf(in); !reflect.DeepEqual(got, want) {
		t.Errorf("%+v", got)
	}
	cfg := createGeneratorConfig(&config.Config{AdminTheme: in, CheckAdminTheme: "strict"})
	if cfg.AdminTheme == nil || cfg.CheckAdminTheme != "strict" {
		t.Error("createGeneratorConfig carries admin_theme and check_admin_theme")
	}
}
