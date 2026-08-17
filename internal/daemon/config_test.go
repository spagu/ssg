package daemon

// The projects file (#169).

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeProjects puts a projects file in a temp dir and returns its path.
func writeProjects(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, DefaultConfigFile)
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// TestLoadResolvesAndDefaults: a relative dir resolves against the projects
// file, so a checkout can be moved without editing it, and a project with no
// name takes its directory's.
func TestLoadResolvesAndDefaults(t *testing.T) {
	path := writeProjects(t, `
projects:
  - dir: ./blog
    port: 8801
  - name: the shop
    dir: ./shop
`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Projects) != 2 {
		t.Fatalf("projects = %+v", cfg.Projects)
	}
	// Sorted by name, so two reloads of one file compare equal.
	if cfg.Projects[0].Name != "blog" || cfg.Projects[1].Name != "the shop" {
		t.Fatalf("names = %q, %q", cfg.Projects[0].Name, cfg.Projects[1].Name)
	}
	base := filepath.Dir(path)
	if cfg.Projects[0].Dir != filepath.Join(base, "blog") {
		t.Errorf("dir = %q, want it resolved against the projects file", cfg.Projects[0].Dir)
	}
	// A port is the clearest statement that a server was meant.
	if !cfg.Projects[0].HTTP {
		t.Error("a port must imply http")
	}
	if cfg.Projects[1].HTTP {
		t.Error("a project with no port and no http: stays a plain build")
	}
}

// TestLoadRefusesAFileThatCouldNotRun: a daemon that starts half a fleet and
// says nothing about the rest is worse than one that refuses to start.
func TestLoadRefusesAFileThatCouldNotRun(t *testing.T) {
	cases := map[string]string{
		"dir is required":   "projects:\n  - name: blog\n",
		"named twice":       "projects:\n  - {name: a, dir: ./x}\n  - {name: a, dir: ./y}\n",
		"both ask for port": "projects:\n  - {name: a, dir: ./x, port: 8801}\n  - {name: b, dir: ./y, port: 8801}\n",
		"is not a port":     "projects:\n  - {name: a, dir: ./x, port: 99999}\n",
	}
	for want, body := range cases {
		_, err := Load(writeProjects(t, body))
		if err == nil {
			t.Errorf("%q: must be refused", want)
			continue
		}
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error must say %q, got: %v", want, err)
		}
	}
	// A file that is not there, and one that is not YAML.
	if _, err := Load(filepath.Join(t.TempDir(), "nope")); err == nil {
		t.Error("a missing projects file is an error")
	}
	if _, err := Load(writeProjects(t, "projects: [ unterminated")); err == nil {
		t.Error("unparseable YAML is an error")
	}
}

// TestActiveSkipsDisabled: the reason to keep a project in the file without
// running it.
func TestActiveSkipsDisabled(t *testing.T) {
	cfg, err := Load(writeProjects(t, `
projects:
  - {name: a, dir: ./x}
  - {name: b, dir: ./y, disabled: true}
`))
	if err != nil {
		t.Fatal(err)
	}
	active := cfg.Active()
	if len(active) != 1 || active[0].Name != "a" {
		t.Fatalf("Active() = %+v", active)
	}
	if len(cfg.Projects) != 2 {
		t.Error("a disabled project stays in the file")
	}
}

// TestCommandIsAPlainWatch: the daemon adds supervision, not a second way to
// build a site — so what it runs must be readable as an ordinary invocation.
func TestCommandIsAPlainWatch(t *testing.T) {
	plain := Project{Name: "a", Dir: "/srv/a"}
	if got := strings.Join(plain.Command(), " "); got != "--watch" {
		t.Errorf("plain project = %q", got)
	}

	full := Project{
		Name: "b", Dir: "/srv/b", Config: ".ssg.prod.yaml",
		HTTP: true, Port: 8802, Host: "0.0.0.0", Args: []string{"--minify-all"},
	}
	got := strings.Join(full.Command(), " ")
	for _, want := range []string{"--watch", "--config=.ssg.prod.yaml", "--http", "--port=8802", "--host=0.0.0.0", "--minify-all"} {
		if !strings.Contains(got, want) {
			t.Errorf("command %q is missing %q", got, want)
		}
	}
	// http without a port lets ssg choose, which the port flag must not force.
	if got := strings.Join(Project{HTTP: true}.Command(), " "); strings.Contains(got, "--port") {
		t.Errorf("no port configured, none passed: %q", got)
	}
}

// TestFingerprintIsWhatDecidesARestart: the reload rule depends entirely on
// this — anything that changes how a project runs must change it, and anything
// that does not, must not.
func TestFingerprintIsWhatDecidesARestart(t *testing.T) {
	base := Project{Name: "a", Dir: "/srv/a", Port: 8801, HTTP: true}

	same := base
	same.Name = "renamed" // the name is a label, not a way of running
	if same.Fingerprint() != base.Fingerprint() {
		t.Error("renaming a project must not restart it")
	}

	for _, changed := range []Project{
		func() Project { p := base; p.Port = 8802; return p }(),
		func() Project { p := base; p.Dir = "/srv/b"; return p }(),
		func() Project { p := base; p.Config = "other.yaml"; return p }(),
		func() Project { p := base; p.Host = "0.0.0.0"; return p }(),
		func() Project { p := base; p.Args = []string{"--drafts"}; return p }(),
		func() Project { p := base; p.HTTP = false; p.Port = 0; return p }(),
	} {
		if changed.Fingerprint() == base.Fingerprint() {
			t.Errorf("%+v must be seen as changed", changed)
		}
	}
}
