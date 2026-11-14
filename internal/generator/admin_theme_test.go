package generator

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spagu/ssg/internal/admintheme"
	"github.com/spagu/ssg/internal/modules"
)

var coreTemplate = filepath.Join(templates, "core")

func themeCfg(theme *admintheme.Config) Config {
	return Config{Workers: []WorkerConfig{{Name: "core", Dir: coreTemplate}}, AdminTheme: theme}
}

func readRegistry(t *testing.T, out string) modules.Registry {
	t.Helper()
	var reg modules.Registry
	data, err := os.ReadFile(filepath.Join(out, "functions", modules.RegistryFile))
	if err != nil || json.Unmarshal(data, &reg) != nil {
		t.Fatalf("registry: %v %s", err, data)
	}
	return reg
}

// TestAdminThemeWithCore (GO-152): the theme reaches the registry and
// core/theme.css replaces the shipped empty file — core's own, no clash.
func TestAdminThemeWithCore(t *testing.T) {
	cfg := themeCfg(&admintheme.Config{Name: "Anna's Kitchen", Main: "#1b3a4b", Accent: "#e07a5f", Logo: "/core/img/mark.svg"})
	out, printed, err := registryBuild(t, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(printed, "admin theme valid") || !strings.Contains(printed, "core/theme.css") {
		t.Errorf("output:\n%s", printed)
	}
	reg := readRegistry(t, out)
	if reg.Theme == nil || *reg.Theme.Name != "Anna's Kitchen" || reg.Theme.Light["primary"] != "#1b3a4b" {
		t.Fatalf("theme %+v", reg.Theme)
	}
	css := mustRead(t, filepath.Join(out, "core", "theme.css"))
	if !strings.Contains(css, "--nav-bg: #1b3a4b;") {
		t.Errorf("theme.css:\n%s", css)
	}
	// A second build into the same output, as a non-clean rebuild does it:
	// the generated theme.css from the first is a leftover, not a clash.
	g := &Generator{config: cfg}
	g.config.OutputDir, g.config.ConfigPath = out, "site.yaml"
	g.snapshotWorkerPublic()
	if _, err := capture(t, g.generateWorkerFiles); err != nil {
		t.Fatalf("rebuild: %v", err)
	}
	if mustRead(t, filepath.Join(out, "core", "theme.css")) != css {
		t.Error("the rebuild wrote another theme.css")
	}
}

// TestCoreWithoutAdminTheme: the shipped empty theme.css, a null theme.
func TestCoreWithoutAdminTheme(t *testing.T) {
	out, printed, err := registryBuild(t, themeCfg(nil))
	if err != nil || strings.Contains(printed, "admin") {
		t.Fatalf("%v\n%s", err, printed)
	}
	shipped, _ := os.ReadFile(filepath.Join(coreTemplate, "public", "core", "theme.css"))
	if mustRead(t, filepath.Join(out, "core", "theme.css")) != string(shipped) || readRegistry(t, out).Theme != nil {
		t.Error("without admin_theme the shipped file and a null theme")
	}
	// A name alone: in the registry, theme.css as shipped.
	out, _, err = registryBuild(t, themeCfg(&admintheme.Config{Name: "Kuchnia"}))
	if err != nil || mustRead(t, filepath.Join(out, "core", "theme.css")) != string(shipped) || *readRegistry(t, out).Theme.Name != "Kuchnia" {
		t.Errorf("name only: %v", err)
	}
}

func TestAdminThemeFindings(t *testing.T) {
	cfg := themeCfg(&admintheme.Config{Logo: "/img/missing.svg"})
	_, printed, err := registryBuild(t, cfg)
	if err != nil || !strings.Contains(printed, "admin_theme in site.yaml → admin_theme.logo: /img/missing.svg is not in the build output") {
		t.Errorf("warn by default: %v\n%s", err, printed)
	}
	cfg.CheckAdminTheme = "strict"
	if _, _, err := registryBuild(t, cfg); err == nil || !strings.Contains(err.Error(), "1 admin theme problem(s)") {
		t.Errorf("strict: %v", err)
	}
	cfg.CheckAdminTheme = "off"
	if _, printed, err := registryBuild(t, cfg); err != nil || strings.Contains(printed, "admin_theme") {
		t.Errorf("off: %v\n%s", err, printed)
	}
	cfg.CheckAdminTheme = "warn"
	if _, _, err := registryBuild(t, cfg); err != nil {
		t.Errorf("warn: %v", err)
	}
}

// TestAdminThemeWithoutCore: validated anyway (a bad value is an error), and
// otherwise a finding that it has no effect; nothing is written.
func TestAdminThemeWithoutCore(t *testing.T) {
	theme := &admintheme.Config{Main: "#1b3a4b"}
	out, printed, err := registryBuild(t, Config{AdminTheme: theme})
	if err != nil || !strings.Contains(printed, "admin_theme: is set, but no worker in workers: is the core worker") {
		t.Errorf("%v\n%s", err, printed)
	}
	if _, err := os.Stat(filepath.Join(out, "core")); err == nil {
		t.Error("nothing is written without core")
	}
	comments := Config{Workers: []WorkerConfig{{Name: "comments", Dir: filepath.Join(templates, "comments")}}, AdminTheme: theme, CheckAdminTheme: "strict"}
	if _, _, err := registryBuild(t, comments); err == nil {
		t.Error("strict fails on an admin_theme without core")
	}
	_, _, err = registryBuild(t, Config{AdminTheme: &admintheme.Config{Main: "red"}})
	if err == nil || !strings.Contains(err.Error(), `site.yaml: admin_theme.main "red" is not a hex colour`) {
		t.Errorf("a bad value is a configuration error: %v", err)
	}
	_, _, err = registryBuild(t, themeCfg(&admintheme.Config{Logo: "https://example.com/l.svg"}))
	if err == nil || !strings.Contains(err.Error(), "admin_theme.logo") {
		t.Errorf("with core too: %v", err)
	}
}

func TestAdminThemeWriteError(t *testing.T) {
	g := &Generator{config: Config{OutputDir: t.TempDir(), Quiet: true}}
	if err := g.writeAdminThemeCSS(nil); err != nil {
		t.Error("nil writes nothing")
	}
	mustWrite(t, filepath.Join(g.config.OutputDir, "core"), "a file where the directory goes")
	if err := g.writeAdminThemeCSS([]byte("x")); err == nil || !strings.Contains(err.Error(), "admin theme:") {
		t.Errorf("parent: %v", err)
	}
	g.config.OutputDir = t.TempDir()
	mustMkdir(t, filepath.Join(g.config.OutputDir, "core", "theme.css"))
	if err := g.writeAdminThemeCSS([]byte("x")); err == nil || !strings.Contains(err.Error(), "admin theme:") {
		t.Errorf("write: %v", err)
	}
}
