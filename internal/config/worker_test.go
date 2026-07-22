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
