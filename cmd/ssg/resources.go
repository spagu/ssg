package main

// The CPU and memory ceiling every command runs under (#384). The build, the
// watch loop, the server and `ssg mcp` read it from the config and their
// flags; `ssg daemon` and `ssg migrate` have no site config of their own and
// take the flags alone (a daemon's projects are separate ssg processes, each
// reading its own config). All of them fit a cgroup memory limit when none is
// set.

import (
	"strconv"
	"strings"

	"github.com/spagu/ssg/internal/config"
	"github.com/spagu/ssg/internal/resources"
)

// resourceRequest is what a site config asks for: the `resources:` block,
// with `mem_limit` standing in for a memory the block leaves unset.
func resourceRequest(cfg *config.Config) resources.Request {
	r := resources.Request{CPU: cfg.Resources.CPU, Memory: cfg.Resources.Memory,
		MemoryField: "resources.memory", GCPercent: cfg.Resources.GCPercent}
	if strings.TrimSpace(r.Memory) == "" && strings.TrimSpace(cfg.MemLimit) != "" {
		r.Memory, r.MemoryField = cfg.MemLimit, "mem_limit"
	}
	return r
}

// applyResources resolves and applies the limits, naming them once through
// logf unless quiet. It reports false, having said why, for a value that is
// not a limit.
func applyResources(r resources.Request, quiet bool, logf func(string, ...any)) bool {
	limits, err := resources.Resolve(r)
	if err != nil {
		errf("❌ %v\n", err)
		return false
	}
	limits.Apply()
	if !quiet && limits.Set() {
		logf("   %s\n", limits)
	}
	return true
}

// buildWorkers is the worker pool size: build_workers (or one per usable CPU)
// capped at resources.cpu, so a CPU ceiling also bounds the external encoders
// started one per worker.
func buildWorkers(cfg *config.Config) int {
	return resources.Limits{CPU: cfg.Resources.CPU}.Workers(resolveBuildWorkers(cfg.BuildWorkers))
}

// parseResourceFlag reads --cpu-limit=, --mem-limit= and --gc-percent= into a
// request, for the commands with a parser of their own. handled is false for
// any other argument; ok is false for a value that is not a number.
func parseResourceFlag(arg string, r *resources.Request) (handled, ok bool) {
	name, value, _ := strings.Cut(arg, "=")
	switch name {
	case "--mem-limit":
		r.Memory, r.MemoryField = value, "--mem-limit"
		return true, true
	case "--cpu-limit":
		n, err := strconv.Atoi(value)
		r.CPU = n
		return true, err == nil
	case "--gc-percent":
		n, err := strconv.Atoi(value)
		r.GCPercent = &n
		return true, err == nil
	}
	return false, false
}

// isResourceFlag is parseResourceFlag for a switch: true when arg was a
// resource flag with a usable value. A malformed value falls through to the
// caller's unknown-flag report, which names the argument.
func isResourceFlag(arg string, r *resources.Request) bool {
	handled, ok := parseResourceFlag(arg, r)
	return handled && ok
}
