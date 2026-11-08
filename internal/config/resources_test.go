package config

// resources: (#384) and the sitemap prefix: (#386) as the three file formats
// spell them.

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/spagu/ssg/internal/models"
)

func loadConfigFile(t *testing.T, name, body string) *Config {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	return cfg
}

func TestResourcesConfig(t *testing.T) {
	for name, c := range map[string]struct{ file, body string }{
		"yaml": {".ssg.yaml", "mem_limit: 1GiB\nresources:\n  cpu: 2\n  memory: 512MiB\n  gc_percent: 50\n"},
		"toml": {"ssg.toml", "mem_limit = \"1GiB\"\n[resources]\ncpu = 2\nmemory = \"512MiB\"\ngc_percent = 50\n"},
		"json": {"ssg.json", `{"mem_limit":"1GiB","resources":{"cpu":2,"memory":"512MiB","gc_percent":50}}`},
	} {
		cfg := loadConfigFile(t, c.file, c.body)
		r := cfg.Resources
		if r.CPU != 2 || r.Memory != "512MiB" || r.GCPercent == nil || *r.GCPercent != 50 || cfg.MemLimit != "1GiB" {
			t.Errorf("%s: %+v mem_limit=%q", name, r, cfg.MemLimit)
		}
	}
}

func TestSitemapPrefixConfig(t *testing.T) {
	want := []models.SitemapSpec{
		{Path: "/sitemap-bsd.xml", Prefix: models.PathList{"/openbsd/", "/netbsd/"}},
		{Path: "/sitemap-vuln.xml", Source: "vuln", Prefix: models.PathList{"/vuln/"}},
	}
	for name, c := range map[string]struct{ file, body string }{
		"yaml": {".ssg.yaml", "sitemaps:\n  - path: /sitemap-bsd.xml\n    prefix: [/openbsd/, /netbsd/]\n" +
			"  - path: /sitemap-vuln.xml\n    source: vuln\n    prefix: /vuln/\n"},
		"toml": {"ssg.toml", "[[sitemaps]]\npath = \"/sitemap-bsd.xml\"\nprefix = [\"/openbsd/\", \"/netbsd/\"]\n" +
			"[[sitemaps]]\npath = \"/sitemap-vuln.xml\"\nsource = \"vuln\"\nprefix = \"/vuln/\"\n"},
		"json": {"ssg.json", `{"sitemaps":[{"path":"/sitemap-bsd.xml","prefix":["/openbsd/","/netbsd/"]},` +
			`{"path":"/sitemap-vuln.xml","source":"vuln","prefix":"/vuln/"}]}`},
	} {
		if got := loadConfigFile(t, c.file, c.body).Sitemaps; !reflect.DeepEqual(got, want) {
			t.Errorf("%s: %+v", name, got)
		}
	}
}
