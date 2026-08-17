package daemon

// Several projects watched by one process.
//
// `ssg --watch` serves one project. Anyone running four sites keeps four
// terminals, four scrollbacks and four things to remember to restart. This is
// that, declared once:
//
//	# .ssg_projects
//	projects:
//	  - name: blog
//	    dir: /srv/blog
//	    http: true
//	    port: 8801
//	  - name: shop
//	    dir: /srv/shop
//	    port: 8802
//
// Each project is an ordinary ssg build with its own config, watched
// independently. Nothing here changes how a project is built: the daemon starts
// `ssg --watch` in the project's directory and supervises it, so a project's
// crash cannot take the others down and no single-project code path had to grow
// a notion of "which site am I".

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// DefaultConfigFile is the projects file a bare `ssg daemon` reads.
const DefaultConfigFile = ".ssg_projects"

// Project is one site the daemon keeps building.
type Project struct {
	// Name identifies the project in the log and in reload messages. Defaults
	// to the directory's base name.
	Name string `yaml:"name" json:"name"`
	// Dir is the project root — the directory holding its config, and the
	// working directory its build runs in. Relative paths resolve against the
	// projects file, so a checkout can be moved without editing it.
	Dir string `yaml:"dir" json:"dir"`
	// Config is the project's own config file, relative to Dir. Empty lets ssg
	// find it the way a plain build does.
	Config string `yaml:"config" json:"config"`
	// HTTP serves the project while watching it, and Port is where. A port of
	// zero with http: true lets ssg pick its own, which is only useful for one
	// project — four sites on one machine want four numbers.
	HTTP bool `yaml:"http" json:"http"`
	Port int  `yaml:"port" json:"port"`
	// Host binds the project's server. Empty is ssg's own default.
	Host string `yaml:"host" json:"host"`
	// Args are extra flags handed to the build verbatim (--minify-all,
	// --drafts), so a project is not limited to what this file models.
	Args []string `yaml:"args" json:"args"`
	// Disabled keeps a project in the file without running it — the reason to
	// comment one out, without commenting it out.
	Disabled bool `yaml:"disabled" json:"disabled"`
}

// Config is the whole projects file.
type Config struct {
	Projects []Project `yaml:"projects" json:"projects"`
}

// Load reads and validates a projects file, resolving every path against it.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- the operator's own projects file
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	base, err := filepath.Abs(filepath.Dir(path))
	if err != nil {
		return nil, err
	}
	if err := cfg.normalize(base); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return &cfg, nil
}

// normalize fills defaults, resolves directories and rejects a file that could
// not run — a daemon that starts half a fleet and says nothing about the rest
// is worse than one that refuses to start.
func (c *Config) normalize(base string) error {
	seenName := map[string]bool{}
	seenPort := map[int]string{}

	for i := range c.Projects {
		p := &c.Projects[i]
		if strings.TrimSpace(p.Dir) == "" {
			return fmt.Errorf("project %d: dir is required", i+1)
		}
		if !filepath.IsAbs(p.Dir) {
			p.Dir = filepath.Join(base, p.Dir)
		}
		p.Dir = filepath.Clean(p.Dir)
		if p.Name = strings.TrimSpace(p.Name); p.Name == "" {
			p.Name = filepath.Base(p.Dir)
		}
		if seenName[p.Name] {
			return fmt.Errorf("project %q is named twice — names identify a project in the log and on reload", p.Name)
		}
		seenName[p.Name] = true

		if p.Port != 0 {
			if p.Port < 1 || p.Port > 65535 {
				return fmt.Errorf("project %q: port %d is not a port", p.Name, p.Port)
			}
			if other, clash := seenPort[p.Port]; clash {
				return fmt.Errorf("projects %q and %q both ask for port %d", other, p.Name, p.Port)
			}
			seenPort[p.Port] = p.Name
			// A port is only meaningful with a server, and asking for one is
			// the clearest statement that a server was meant.
			p.HTTP = true
		}
	}
	// A stable order so two reloads of the same file compare equal and the log
	// reads the same way twice.
	sort.SliceStable(c.Projects, func(a, b int) bool { return c.Projects[a].Name < c.Projects[b].Name })
	return nil
}

// Active returns the projects that should be running.
func (c *Config) Active() []Project {
	out := make([]Project, 0, len(c.Projects))
	for _, p := range c.Projects {
		if !p.Disabled {
			out = append(out, p)
		}
	}
	return out
}

// Command returns the argument list that builds and watches this project. It is
// a plain `ssg --watch`, which is the point: the daemon adds supervision, not a
// second way to build a site.
func (p Project) Command() []string {
	args := []string{"--watch"}
	if p.Config != "" {
		args = append(args, "--config="+p.Config)
	}
	if p.HTTP {
		args = append(args, "--http")
		if p.Port != 0 {
			args = append(args, fmt.Sprintf("--port=%d", p.Port))
		}
		if p.Host != "" {
			args = append(args, "--host="+p.Host)
		}
	}
	return append(args, p.Args...)
}

// Fingerprint is everything that decides how a project runs. Two projects with
// equal fingerprints are the same running thing, which is what lets a reload
// leave untouched projects alone instead of restarting the fleet.
func (p Project) Fingerprint() string {
	return strings.Join(append([]string{p.Dir}, p.Command()...), "\x00")
}
