package config

import (
	"os"
	"path/filepath"
	"testing"
)

// TestModulesConfig (GO-144): modules: and check_modules: load from YAML,
// TOML and JSON alike.
func TestModulesConfig(t *testing.T) {
	dir := t.TempDir()
	files := map[string]string{
		"a.yaml": "check_modules: strict\nmodules:\n  - id: newsletter-ui\n    url: /newsletter-admin\n    dir: workers/newsletter-ui\n",
		"a.toml": "check_modules = \"strict\"\n[[modules]]\nid = \"newsletter-ui\"\nurl = \"/newsletter-admin\"\ndir = \"workers/newsletter-ui\"\n",
		"a.json": `{"check_modules": "strict", "modules": [{"id": "newsletter-ui", "url": "/newsletter-admin", "dir": "workers/newsletter-ui"}]}`,
	}
	want := ModuleConfig{ID: "newsletter-ui", URL: "/newsletter-admin", Dir: "workers/newsletter-ui"}
	for name, body := range files {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		cfg, err := Load(p)
		if err != nil {
			t.Fatal(err)
		}
		if cfg.CheckModules != "strict" || len(cfg.Modules) != 1 || cfg.Modules[0] != want {
			t.Errorf("%s: %q %+v", name, cfg.CheckModules, cfg.Modules)
		}
	}
	if DefaultConfig().CheckModules != "" || DefaultConfig().Modules != nil {
		t.Error("defaults: no modules, check_modules unset (the build reads unset as warn)")
	}
}
