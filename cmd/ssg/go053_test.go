package main

import (
	"strings"
	"testing"

	"github.com/spagu/ssg/internal/config"
)

// GO-053: every value-taking flag accepts both "--flag=value" and
// "--flag value"; the space form must never leak the value into positionals.
func TestValueFlagsBothFormsParity(t *testing.T) {
	cases := []struct {
		name  string
		flag  string
		value string
		check func(cfg *config.Config) string
	}{
		{"deploy", "--deploy", "cloudflare", func(c *config.Config) string { return c.Deploy }},
		{"deploy-project", "--deploy-project", "my-site", func(c *config.Config) string { return c.DeployProject }},
		{"host", "--host", "0.0.0.0", func(c *config.Config) string { return c.Host }},
		{"tls-domain", "--tls-domain", "a.com,b.com", func(c *config.Config) string { return c.TLSDomain }},
		{"timezone", "--timezone", "Europe/Warsaw", func(c *config.Config) string { return c.Timezone }},
		{"default-language", "--default-language", "pl", func(c *config.Config) string { return c.DefaultLanguage }},
		{"static-dir", "--static-dir", "assets", func(c *config.Config) string { return c.StaticDir }},
		{"mem-limit", "--mem-limit", "512MiB", func(c *config.Config) string { return c.MemLimit }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			equalCfg := config.DefaultConfig()
			parseFlags([]string{tc.flag + "=" + tc.value}, equalCfg)
			spaceCfg := config.DefaultConfig()
			parseFlags([]string{tc.flag, tc.value}, spaceCfg)
			if got := tc.check(equalCfg); got != tc.value {
				t.Fatalf("%s=%s: got %q", tc.flag, tc.value, got)
			}
			if got := tc.check(spaceCfg); got != tc.value {
				t.Errorf("%s %s (space form): got %q, want %q", tc.flag, tc.value, got, tc.value)
			}
		})
	}
}

func TestValueFlagsSpaceFormInts(t *testing.T) {
	cfg := config.DefaultConfig()
	parseFlags([]string{"--paginate", "10", "--feed-items", "5", "--max-conns", "77", "--toc-depth", "4"}, cfg)
	if cfg.Paginate != 10 || cfg.FeedItems != 5 || cfg.MaxConns != 77 || cfg.TOCDepth != 4 {
		t.Errorf("space-form int flags not applied: paginate=%d feedItems=%d maxConns=%d tocDepth=%d",
			cfg.Paginate, cfg.FeedItems, cfg.MaxConns, cfg.TOCDepth)
	}
}

func TestValueFlagsSpaceFormMddbURL(t *testing.T) {
	cfg := config.DefaultConfig()
	parseFlags([]string{"--mddb-url", "http://localhost:8080"}, cfg)
	if cfg.Mddb.URL != "http://localhost:8080" || !cfg.Mddb.Enabled {
		t.Errorf("--mddb-url space form: url=%q enabled=%v", cfg.Mddb.URL, cfg.Mddb.Enabled)
	}
}

func TestPositionalArgsSkipAllValueFlags(t *testing.T) {
	args := []string{"src", "tmpl", "example.com", "--deploy", "cloudflare", "--paginate", "10", "--zip"}
	got := positionalArgsOf(args)
	if len(got) != 3 || got[0] != "src" || got[1] != "tmpl" || got[2] != "example.com" {
		t.Errorf("positionalArgsOf = %v, want [src tmpl example.com]", got)
	}
}

func TestValueFlagAtEndConsumesNothing(t *testing.T) {
	cfg := config.DefaultConfig()
	if skip := parseSeparateValueFlags([]string{"--deploy"}, 0, cfg); skip != 0 {
		t.Errorf("trailing value flag skip = %d, want 0", skip)
	}
	if cfg.Deploy != "" {
		t.Errorf("trailing value flag must not set a value, got %q", cfg.Deploy)
	}
}

func TestValueFlagsCoversEqualTables(t *testing.T) {
	known := valueFlags()
	var cfg config.Config
	for prefix := range stringEqualFlags(&cfg) {
		bare := strings.TrimSuffix(prefix, "=")
		if !known[bare] {
			t.Errorf("valueFlags() missing %s from stringEqualFlags", bare)
		}
	}
	for _, name := range extraEqualValueFlags {
		if !known[name] {
			t.Errorf("valueFlags() missing %s from extraEqualValueFlags", name)
		}
	}
}
