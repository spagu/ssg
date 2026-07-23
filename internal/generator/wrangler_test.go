package generator

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnsureWranglerConfigCreatesFromWorkers(t *testing.T) {
	proj := t.TempDir()
	worker := filepath.Join(proj, "workers", "cc")
	if err := os.MkdirAll(worker, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(worker, "wrangler.snippet.toml"),
		[]byte("# [[kv_namespaces]]\n# binding = \"CONSENT_LOG\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	path, created, err := EnsureWranglerConfig(proj, "ssg.tradik.com", "output", []string{worker}, "")
	if err != nil || !created {
		t.Fatalf("EnsureWranglerConfig = %q, %v, %v", path, created, err)
	}
	data, _ := os.ReadFile(path)
	got := string(data)
	for _, want := range []string{
		`name = "ssg-tradik-com"`,
		`pages_build_output_dir = "./output"`,
		"compatibility_date",
		"CONSENT_LOG", // the worker's snippet was appended
	} {
		if !strings.Contains(got, want) {
			t.Errorf("generated config missing %q:\n%s", want, got)
		}
	}
}

func TestEnsureWranglerConfigNoWorkersNoOp(t *testing.T) {
	proj := t.TempDir()
	_, created, err := EnsureWranglerConfig(proj, "x", "output", nil, "")
	if err != nil || created {
		t.Fatalf("no workers should create nothing: %v %v", created, err)
	}
	if _, err := os.Stat(filepath.Join(proj, "wrangler.toml")); err == nil {
		t.Error("wrangler.toml written with no workers")
	}
}

func TestEnsureWranglerConfigNeverOverwrites(t *testing.T) {
	proj := t.TempDir()
	worker := filepath.Join(proj, "w")
	_ = os.MkdirAll(worker, 0o750)
	existing := filepath.Join(proj, "wrangler.toml")
	if err := os.WriteFile(existing, []byte("name = \"mine\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	path, created, err := EnsureWranglerConfig(proj, "x", "output", []string{worker}, "")
	if err != nil || created {
		t.Fatalf("existing config should be left alone: created=%v err=%v", created, err)
	}
	if path != existing {
		t.Errorf("path = %q, want the existing one", path)
	}
	data, _ := os.ReadFile(existing)
	if string(data) != "name = \"mine\"\n" {
		t.Error("existing wrangler.toml was overwritten")
	}
}

func TestEnsureWranglerConfigExplicitWins(t *testing.T) {
	proj := t.TempDir()
	w := filepath.Join(proj, "w")
	_ = os.MkdirAll(w, 0o750)
	path, created, err := EnsureWranglerConfig(proj, "x", "output", []string{w}, "deploy/wrangler.toml")
	if err != nil || created || path != "deploy/wrangler.toml" {
		t.Fatalf("explicit config should win: %q %v %v", path, created, err)
	}
}

func TestWranglerName(t *testing.T) {
	cases := map[string]string{
		"ssg.tradik.com":      "ssg-tradik-com",
		"HTTPS://Example.COM": "example-com",
		"my_site":             "my-site",
		"a..b":                "a-b",
		"":                    "ssg-site",
		"---":                 "ssg-site",
	}
	for in, want := range cases {
		if got := wranglerName(in); got != want {
			t.Errorf("wranglerName(%q) = %q, want %q", in, got, want)
		}
	}
}
