package main

// One config, several hosts (#439): `--domain` on the command line, and
// `$NAME`/`${NAME}` in `domain:`, both settled before anything derives a URL.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spagu/ssg/internal/config"
	"github.com/spagu/ssg/internal/migrate"
)

// TestResolveDomain: precedence, both reference spellings, and every way a
// domain can resolve to something that is not a host.
func TestResolveDomain(t *testing.T) {
	t.Setenv("SSG_TEST_SITE_DOMAIN", "prod.example.com")
	t.Setenv("SSG_TEST_EMPTY_DOMAIN", "")
	t.Setenv("SSG_TEST_BAD_DOMAIN", "not a host")

	tests := []struct {
		name    string
		file    string // what `domain:` (or the positional argument) said
		args    []string
		want    string
		wantErr []string // substrings of the error; nil when it resolves
	}{
		{"no reference is unchanged", "example.com", nil, "example.com", nil},
		{"flag overrides the file", "example.com", []string{"--domain=staging.example.com"}, "staging.example.com", nil},
		{"flag in the space form", "example.com", []string{"--domain", "staging.example.com"}, "staging.example.com", nil},
		{"the last flag wins", "example.com", []string{"--domain=a.example.com", "--domain=b.example.com"}, "b.example.com", nil},
		{"braced reference", "${SSG_TEST_SITE_DOMAIN}", nil, "prod.example.com", nil},
		{"bare reference", "$SSG_TEST_SITE_DOMAIN", nil, "prod.example.com", nil},
		{"the flag is normalised like the file", "example.com", []string{"--domain=https://staging.example.com/"}, "staging.example.com", nil},
		{"the flag wins over an unset reference", "${SSG_TEST_UNSET_DOMAIN}", []string{"--domain=staging.example.com"}, "staging.example.com", nil},
		{"unset variable", "${SSG_TEST_UNSET_DOMAIN}", nil, "", []string{"$SSG_TEST_UNSET_DOMAIN", "not set"}},
		{"empty variable", "$SSG_TEST_EMPTY_DOMAIN", nil, "", []string{"$SSG_TEST_EMPTY_DOMAIN", "empty"}},
		{"variable holding no host", "$SSG_TEST_BAD_DOMAIN", nil, "", []string{"$SSG_TEST_BAD_DOMAIN", "not a host"}},
		{"invalid flag", "example.com", []string{"--domain=exa mple.com"}, "", []string{"--domain", "exa mple.com"}},
		{"empty flag", "example.com", []string{"--domain="}, "", []string{"--domain"}},
		// A literal that was never checked before stays unchecked: existing
		// configs must build exactly as they did.
		{"a literal keeps the old leniency", "https://", nil, "", nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := config.DefaultConfig()
			cfg.Quiet = true
			cfg.Domain = tc.file
			err := resolveDomain(cfg, tc.args)
			if tc.wantErr == nil {
				if err != nil || cfg.Domain != tc.want {
					t.Fatalf("resolveDomain = %q, %v; want %q", cfg.Domain, err, tc.want)
				}
				return
			}
			if err == nil {
				t.Fatalf("resolveDomain = %q, want an error", cfg.Domain)
			}
			for _, want := range tc.wantErr {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q lacks %q", err, want)
				}
			}
		})
	}
}

// TestDomainFlagIsAValueFlag: parsed in both spellings, never reported as
// unknown, and its value never mistaken for a positional argument.
func TestDomainFlagIsAValueFlag(t *testing.T) {
	if !valueFlags()["--domain"] {
		t.Fatal("--domain is not in the value-flag table")
	}
	cfg := config.DefaultConfig()
	out := captureStderr(t, func() { parseFlags([]string{"--domain", "staging.example.com"}, cfg) })
	if cfg.Domain != "staging.example.com" {
		t.Errorf("parseFlags left domain %q", cfg.Domain)
	}
	if strings.Contains(out, "Unknown option") {
		t.Errorf("--domain reported as unknown: %q", out)
	}
	if got := positionalArgsOf([]string{"--domain", "staging.example.com", "site"}); len(got) != 1 || got[0] != "site" {
		t.Errorf("positional = %q", got)
	}
}

// TestDomainFlagBeatsThePositionalDomain: the flag is the explicit override,
// so it wins over the third positional argument as it wins over the file.
func TestDomainFlagBeatsThePositionalDomain(t *testing.T) {
	args := []string{"site", "simple", "example.com", "--domain=staging.example.com"}
	cfg := config.DefaultConfig()
	cfg.Quiet = true
	parseFlags(args, cfg)
	validateRequiredFields(args, cfg)
	if err := resolveDomain(cfg, args); err != nil || cfg.Domain != "staging.example.com" {
		t.Fatalf("domain = %q, %v", cfg.Domain, err)
	}
}

// domainProject is a buildable project whose config file names `domain`, so
// the build is driven exactly as `ssg --config site.yaml` drives it.
func domainProject(t *testing.T, domain string) []string {
	t.Helper()
	rcovProject(t)
	// The canonical tag the way the shipped themes write it.
	page := `{{define "page.html"}}<html><head><link rel="canonical" href="{{getCanonical .Page .Domain}}"></head><body><p>x</p></body></html>{{end}}`
	if err := os.WriteFile(filepath.Join("templates", "simple", "page.html"), []byte(page), 0o600); err != nil {
		t.Fatal(err)
	}
	body := "source: site\ntemplate: simple\ndomain: " + domain + "\nseo: true\n"
	if err := os.WriteFile("site.yaml", []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return []string{"--config=site.yaml", "--quiet"}
}

// assertBuiltFor checks the two places a wrong host hurts most: the sitemap
// search engines read, and the canonical tag of a page.
func assertBuiltFor(t *testing.T, host string) {
	t.Helper()
	for _, rel := range []string{"sitemap.xml", filepath.Join("home", "index.html")} {
		data, err := os.ReadFile(filepath.Join("output", rel)) // #nosec G304 -- test output
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(data), "https://"+host+"/") {
			t.Errorf("%s does not use https://%s/:\n%s", rel, host, data)
		}
		if host != "example.com" && strings.Contains(string(data), "https://example.com/") {
			t.Errorf("%s still names the file's domain", rel)
		}
	}
	page, _ := os.ReadFile(filepath.Join("output", "home", "index.html")) // #nosec G304 -- test output
	if !strings.Contains(string(page), `rel="canonical" href="https://`+host+`/`) {
		t.Errorf("canonical does not use %s:\n%s", host, page)
	}
}

// TestBuildWithTheDomainFromEachSource: the same config built three ways, and
// every derived URL follows the host that won.
func TestBuildWithTheDomainFromEachSource(t *testing.T) {
	t.Run("file", func(t *testing.T) {
		args := domainProject(t, "example.com")
		if code, done := run(args); !done || code != 0 {
			t.Fatalf("run = %d, %v", code, done)
		}
		assertBuiltFor(t, "example.com")
	})
	t.Run("flag", func(t *testing.T) {
		args := domainProject(t, "example.com")
		if code, done := run(append(args, "--domain=staging.example.com")); !done || code != 0 {
			t.Fatalf("run = %d, %v", code, done)
		}
		assertBuiltFor(t, "staging.example.com")
	})
	t.Run("environment", func(t *testing.T) {
		t.Setenv("SSG_TEST_SITE_DOMAIN", "prod.example.com")
		args := domainProject(t, "${SSG_TEST_SITE_DOMAIN}")
		if code, done := run(args); !done || code != 0 {
			t.Fatalf("run = %d, %v", code, done)
		}
		assertBuiltFor(t, "prod.example.com")
	})
}

// TestAnUnsetDomainVariableStopsTheBuild: nothing is written, and the message
// names the variable to export.
func TestAnUnsetDomainVariableStopsTheBuild(t *testing.T) {
	args := domainProject(t, "${SSG_TEST_UNSET_DOMAIN}")
	args = args[:1] // not --quiet: the error is the point
	out := captureStderr(t, func() {
		if code, done := run(args); !done || code != 1 {
			t.Errorf("run = %d, %v", code, done)
		}
	})
	if !strings.Contains(out, "$SSG_TEST_UNSET_DOMAIN") {
		t.Errorf("stderr does not name the variable: %q", out)
	}
	if _, err := os.Stat("output"); err == nil {
		t.Error("a build with no host must not leave a site behind")
	}
}

// TestAWatchReloadKeepsTheLastGoodDomain: an edit that references an unset
// variable is reported and the watcher keeps building with what it had.
func TestAWatchReloadKeepsTheLastGoodDomain(t *testing.T) {
	domainProject(t, "${SSG_TEST_UNSET_DOMAIN}")
	old := config.DefaultConfig()
	old.Quiet = true
	if _, cfg, ok := reloadWatchConfig([]string{"--quiet"}, "site.yaml", old); ok || cfg != nil {
		t.Fatalf("reload accepted an unresolvable domain: %v", cfg)
	}
	out := captureStderr(t, func() { reloadWatchConfig(nil, "site.yaml", config.DefaultConfig()) })
	if !strings.Contains(out, "$SSG_TEST_UNSET_DOMAIN") {
		t.Errorf("the reload error does not name the variable: %q", out)
	}
	// The flag still wins over the file on a reload.
	_, cfg, ok := reloadWatchConfig([]string{"--quiet", "--domain=staging.example.com"}, "site.yaml", old)
	if !ok || cfg.Domain != "staging.example.com" {
		t.Fatalf("reload = %v, %+v", ok, cfg)
	}
}

// TestOtherEntryPointsRefuseAnUnresolvableDomain: `ssg mcp`, `ssg migrate`
// and `ssg new wrangler` read the same config and stop on the same error,
// before serving, fetching or naming a project after an empty host.
func TestOtherEntryPointsRefuseAnUnresolvableDomain(t *testing.T) {
	setup := func(t *testing.T) {
		t.Helper()
		t.Chdir(t.TempDir())
		yaml := "source: s\ntemplate: simple\ndomain: ${SSG_TEST_UNSET_DOMAIN}\n"
		if err := os.WriteFile(".ssg.yaml", []byte(yaml), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	t.Run("mcp", func(t *testing.T) {
		setup(t)
		out := captureStderr(t, func() {
			if code := runMCP([]string{"--no-watch"}); code != 1 {
				t.Errorf("runMCP = %d, want 1", code)
			}
		})
		if !strings.Contains(out, "$SSG_TEST_UNSET_DOMAIN") {
			t.Errorf("stderr = %q", out)
		}
	})
	t.Run("migrate", func(t *testing.T) {
		setup(t)
		fetched := false
		stubMigrate(t, func(migrate.Options) (*migrate.Report, error) {
			fetched = true
			return &migrate.Report{}, nil
		})
		if code := runMigrate([]string{"wordpress", "https://e.example.com", "--quiet"}); code != 1 || fetched {
			t.Errorf("runMigrate = %d (fetched %v), want 1 before any fetch", code, fetched)
		}
	})
	t.Run("new wrangler", func(t *testing.T) {
		setup(t)
		out := captureStderr(t, func() {
			if code := runNewWrangler(nil); code != 1 {
				t.Errorf("runNewWrangler = %d, want 1", code)
			}
		})
		// Not the "no workers configured" refusal: the domain is checked first.
		if !strings.Contains(out, "$SSG_TEST_UNSET_DOMAIN") {
			t.Errorf("stderr = %q", out)
		}
	})
}
