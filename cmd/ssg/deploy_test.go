package main

import (
	"testing"

	"github.com/spagu/ssg/internal/config"
)

func TestRunDeployDisabled(t *testing.T) {
	if err := runDeploy(&config.Config{}); err != nil {
		t.Errorf("runDeploy with no provider should be a no-op, got %v", err)
	}
}

func TestRunDeployUnknownProvider(t *testing.T) {
	cfg := &config.Config{Deploy: "dropbox", OutputDir: t.TempDir(), Quiet: true}
	if err := runDeploy(cfg); err == nil {
		t.Error("expected error for an unknown deploy provider")
	}
}

func TestDeployFlagParsing(t *testing.T) {
	cfg := &config.Config{}
	for _, arg := range []string{
		"--deploy=cloudflare",
		"--deploy-project=my-site",
		"--deploy-branch=gh-pages",
		"--deploy-target=ftp://host/path",
	} {
		parseEqualFlags(arg, cfg)
	}
	if cfg.Deploy != "cloudflare" || cfg.DeployProject != "my-site" ||
		cfg.DeployBranch != "gh-pages" || cfg.DeployTarget != "ftp://host/path" {
		t.Errorf("deploy flags not parsed: %+v", cfg)
	}
}
