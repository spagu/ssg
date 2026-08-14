package migrate

// The "recognised but undeliverable" path (1.8.33). It is dormant right now —
// comments got a real export in #134, so wpUnsupportedKinds is empty — but the
// machinery is what keeps the next such kind from being dropped in silence, so
// it is tested by declaring one for the duration of a test rather than left to
// rot until someone needs it.

import (
	"strings"
	"testing"
)

// withUnsupportedKind declares a kind the engine cannot deliver, and removes it
// again afterwards.
func withUnsupportedKind(t *testing.T, kind, reason string) {
	t.Helper()
	if _, exists := wpUnsupportedKinds[kind]; exists {
		t.Fatalf("%q is already declared — pick a name the provider does not know", kind)
	}
	wpUnsupportedKinds[kind] = reason
	t.Cleanup(func() { delete(wpUnsupportedKinds, kind) })
}

// TestUnsupportedKindIsReportedNotDropped: asking for something the engine
// cannot deliver must come back as a named skip with a reason. Silently
// returning a site without it is the failure this guards.
func TestUnsupportedKindIsReportedNotDropped(t *testing.T) {
	withUnsupportedKind(t, "reactions", "the REST API does not expose reactions")

	// It is a valid kind: listed for the user, never an "unknown kind" error.
	if !strings.Contains(strings.Join(wpKindNames(), ","), "reactions") {
		t.Fatalf("an undeliverable kind must still be listed: %v", wpKindNames())
	}

	args, skipped, err := wpexporterArgs("https://e.com", Options{
		Dest: "content/x", Content: []string{"pages", "reactions"},
	})
	if err != nil {
		t.Fatalf("an undeliverable kind must not fail the run: %v", err)
	}
	if len(skipped) != 1 || skipped[0] != "reactions" {
		t.Fatalf("skipped = %v, want [reactions]", skipped)
	}
	// It has no flag of its own, so nothing about it reaches the engine.
	if strings.Contains(strings.Join(args, " "), "reactions") {
		t.Fatalf("an undeliverable kind must not be passed to the engine: %v", args)
	}
}

// TestUnsupportedKindReachesTheReport: the reason travels to the summary the
// operator reads, not just to an internal slice.
func TestUnsupportedKindReachesTheReport(t *testing.T) {
	withUnsupportedKind(t, "reactions", "the REST API does not expose reactions")

	p, _ := Lookup("wordpress")
	rep, err := p.Fetch("https://e.com", Options{
		Dest:     t.TempDir(),
		Content:  []string{"pages", "reactions"},
		LookPath: func(string) (string, error) { return "/bin/wpexporter", nil },
		Run:      func(string, []string, bool) error { return nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Skipped) != 1 || rep.Skipped[0] != "reactions" {
		t.Fatalf("report skipped = %v", rep.Skipped)
	}
	if len(rep.Warnings) != 1 || !strings.Contains(rep.Warnings[0], "does not expose reactions") {
		t.Fatalf("the reason must reach the operator: %v", rep.Warnings)
	}
}

// TestProvidersSortedByName: `ssg migrate --list` prints them in this order, so
// it must not depend on map iteration.
func TestProvidersSortedByName(t *testing.T) {
	registry["aardvark"] = stubProvider{}
	t.Cleanup(func() { delete(registry, "aardvark") })

	names := make([]string, 0, len(registry))
	for _, p := range Providers() {
		names = append(names, p.Name())
	}
	if len(names) < 2 || names[0] != "aardvark" || names[len(names)-1] != "wordpress" {
		t.Fatalf("Providers() is not sorted: %v", names)
	}
}

type stubProvider struct{}

func (stubProvider) Name() string        { return "aardvark" }
func (stubProvider) Version() string     { return "0.0.0" }
func (stubProvider) Description() string { return "test double" }
func (stubProvider) Fetch(string, Options) (*Report, error) {
	return &Report{Provider: "aardvark@0.0.0"}, nil
}
