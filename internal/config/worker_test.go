package config

import "testing"

func TestApplyWorkerWatchDefaults_SetsWranglerRunner(t *testing.T) {
	cfg := &Config{Worker: WorkerConfig{Dir: "workers/api", WranglerConfig: "deploy/wrangler.toml"}}
	ApplyWorkerWatchDefaults(cfg)
	if cfg.WatchRunner != "wrangler" {
		t.Fatalf("expected wrangler runner, got %q", cfg.WatchRunner)
	}
	if cfg.WatchRunnerDir != "workers/api" {
		t.Fatalf("expected runner dir from worker, got %q", cfg.WatchRunnerDir)
	}
	if cfg.WatchRunnerConfig != "deploy/wrangler.toml" {
		t.Fatalf("expected runner config from worker, got %q", cfg.WatchRunnerConfig)
	}
}

func TestApplyWorkerWatchDefaults_ExplicitRunnerWins(t *testing.T) {
	cfg := &Config{
		Worker:      WorkerConfig{Dir: "workers/api"},
		WatchRunner: "workerd",
	}
	ApplyWorkerWatchDefaults(cfg)
	if cfg.WatchRunner != "workerd" {
		t.Fatalf("explicit runner should win, got %q", cfg.WatchRunner)
	}
}

func TestApplyWorkerWatchDefaults_NoWorkerNoop(t *testing.T) {
	cfg := &Config{}
	ApplyWorkerWatchDefaults(cfg)
	if cfg.WatchRunner != "" {
		t.Fatalf("no worker should leave runner empty, got %q", cfg.WatchRunner)
	}
}

// GO-076: the plural workers: list, per-worker config passthrough, and the
// back-compat resolution to the singular worker:.
func TestResolvedWorkers(t *testing.T) {
	// plural wins
	c := &Config{
		Worker:  WorkerConfig{Dir: "workers/legacy"},
		Workers: []WorkerConfig{{Name: "a", Dir: "workers/a"}, {Name: "b", Source: "https://x/z.zip"}},
	}
	got := c.ResolvedWorkers()
	if len(got) != 2 || got[0].Name != "a" || got[1].Source == "" {
		t.Fatalf("plural not returned: %+v", got)
	}
	// singular fallback
	c = &Config{Worker: WorkerConfig{Dir: "workers/one"}}
	if r := c.ResolvedWorkers(); len(r) != 1 || r[0].Dir != "workers/one" {
		t.Fatalf("singular fallback failed: %+v", r)
	}
	// neither
	if r := (&Config{}).ResolvedWorkers(); r != nil {
		t.Fatalf("empty config should resolve to nil, got %+v", r)
	}
}

func TestWorkersConfigParsesWithPassthrough(t *testing.T) {
	dir := t.TempDir()
	p := writeYAML(t, dir, ".ssg.yaml",
		"template: t\ndomain: x\nworkers:\n"+
			"  - name: comments\n    dir: workers/comments\n    routes_include: [\"/api/comments\"]\n"+
			"    config:\n      d1_binding: COMMENTS\n      retention_days: 90\n")
	cfg, err := Load(p)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(cfg.Workers) != 1 {
		t.Fatalf("workers = %+v", cfg.Workers)
	}
	w := cfg.Workers[0]
	if w.Name != "comments" || w.Config["d1_binding"] != "COMMENTS" {
		t.Errorf("per-worker config not parsed: %+v", w)
	}
}
