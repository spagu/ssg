package main

import (
	"runtime/debug"
	"testing"

	"github.com/spagu/ssg/internal/config"
)

func TestResolveVersion(t *testing.T) {
	built := func(v string) func() (*debug.BuildInfo, bool) {
		return func() (*debug.BuildInfo, bool) { return &debug.BuildInfo{Main: debug.Module{Version: v}}, true }
	}
	cases := []struct {
		name, stamped string
		read          func() (*debug.BuildInfo, bool)
		want          string
	}{
		{"stamped wins", "1.8.73", built("v9.9.9"), "1.8.73"},
		{"go install @version", "dev", built("v1.8.73"), "1.8.73"},
		{"pseudo-version", "dev", built("v1.8.74-0.20261021120000-abcdef123456"), "1.8.74-0.20261021120000-abcdef123456"},
		{"local checkout", "dev", built("(devel)"), "dev"},
		{"no module version", "dev", built(""), "dev"},
		{"no build info", "dev", func() (*debug.BuildInfo, bool) { return nil, false }, "dev"},
	}
	for _, c := range cases {
		if got := resolveVersion(c.stamped, c.read); got != c.want {
			t.Errorf("%s: resolveVersion = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestBuildReportFlag(t *testing.T) {
	for arg, want := range map[string]string{"--build-report": "build.json", "--build-report=meta/report.json": "meta/report.json"} {
		cfg := &config.Config{}
		parseFlags([]string{arg}, cfg)
		if cfg.BuildReport != want {
			t.Errorf("%s → %q, want %q", arg, cfg.BuildReport, want)
		}
	}
}
