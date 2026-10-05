package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/spagu/ssg/internal/generator"
)

func TestSnapPrivatePaths(t *testing.T) {
	cfg := generator.Config{OutputDir: "/tmp/site/out", ContentDir: "/var/tmp/content", TemplatesDir: "/home/u/templates"}
	if got := snapPrivatePaths(cfg, false); got != nil {
		t.Errorf("outside a snap: %v", got)
	}
	got := snapPrivatePaths(cfg, true)
	if strings.Join(got, ",") != "output /tmp/site/out,content /var/tmp/content" {
		t.Errorf("in a snap: %v", got)
	}
	if snapPrivatePaths(generator.Config{OutputDir: "/tmpfoo/out"}, true) != nil {
		t.Error("/tmpfoo is not /tmp")
	}
	var b bytes.Buffer
	warnSnapPrivatePaths(&b, generator.Config{OutputDir: "/tmp"}, true)
	if !strings.Contains(b.String(), "the output directory /tmp is inside it") {
		t.Errorf("warning: %q", b.String())
	}
}

func TestRunnerMissingMessage(t *testing.T) {
	docker := runnerMissingMessage("npx", "npx wrangler pages dev .", true)
	if !strings.Contains(docker, "has no Node") || !strings.Contains(docker, "watch_runner: none") {
		t.Errorf("docker: %s", docker)
	}
	host := runnerMissingMessage("npx", "wrangler", false)
	if !strings.Contains(host, "Install Node.js") || strings.Contains(host, "Docker") {
		t.Errorf("host: %s", host)
	}
}
