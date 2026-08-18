package migrate

// Reaching the engine's own flags (#171). A site behind bot protection returns
// an empty export and a green tick; the engine diagnoses it exactly and names
// --user-agent and --rate-limit, and until now that advice was printed inside a
// run `ssg migrate` had no way to act on.

import (
	"strings"
	"testing"
)

// argvFor builds the engine command line for one set of options, as a slice —
// the existing argsFor joins it, and flag/value adjacency is the thing under
// test here.
func argvFor(t *testing.T, opts Options) []string {
	t.Helper()
	args, _, err := wpexporterArgs("https://example.com", opts)
	if err != nil {
		t.Fatal(err)
	}
	return args
}

// argPair reports whether args carries flag immediately followed by value,
// which is how the engine reads them.
func argPair(args []string, flag, value string) bool {
	for i := 0; i+1 < len(args); i++ {
		if args[i] == flag && args[i+1] == value {
			return true
		}
	}
	return false
}

// TestUserAgentAndRateLimitReachTheEngine: the two flags bot protection needs.
func TestUserAgentAndRateLimitReachTheEngine(t *testing.T) {
	args := argvFor(t, Options{UserAgent: "Mozilla/5.0 (X11)", RateLimit: 250})
	if !argPair(args, "--user-agent", "Mozilla/5.0 (X11)") {
		t.Errorf("--user-agent did not reach the engine: %v", args)
	}
	if !argPair(args, "--rate-limit", "250") {
		t.Errorf("--rate-limit did not reach the engine: %v", args)
	}
}

// TestEngineTuningIsOptional: a migration that asks for neither must produce
// exactly the command line it always did, or every existing invocation changes.
func TestEngineTuningIsOptional(t *testing.T) {
	args := strings.Join(argvFor(t, Options{}), " ")
	for _, unwanted := range []string{"--user-agent", "--rate-limit"} {
		if strings.Contains(args, unwanted) {
			t.Errorf("%s must not appear unasked: %s", unwanted, args)
		}
	}
	// A blank agent and a zero limit are "not set", the same as absent — zero is
	// the engine's own "no limit", so passing it would be a lie about intent.
	quiet := strings.Join(argvFor(t, Options{UserAgent: "   ", RateLimit: 0}), " ")
	if strings.Contains(quiet, "--user-agent") || strings.Contains(quiet, "--rate-limit") {
		t.Errorf("blank values must not be passed: %s", quiet)
	}
}

// TestEngineArgsArriveVerbatimAndLast: the pass-through exists so the next
// engine flag does not need a release of ssg, and it comes last so it can
// override what ssg derived.
func TestEngineArgsArriveVerbatimAndLast(t *testing.T) {
	extra := []string{"--some-future-flag", "value", "--another"}
	args := argvFor(t, Options{EngineArgs: extra})
	if len(args) < len(extra) {
		t.Fatalf("args = %v", args)
	}
	tail := args[len(args)-len(extra):]
	for i := range extra {
		if tail[i] != extra[i] {
			t.Fatalf("engine args must arrive verbatim and last: tail = %v, want %v", tail, extra)
		}
	}

	// And with a --content selection, which takes the other return path.
	withContent := argvFor(t, Options{Content: []string{"pages", "posts"}, EngineArgs: extra})
	tail = withContent[len(withContent)-len(extra):]
	if strings.Join(tail, " ") != strings.Join(extra, " ") {
		t.Fatalf("with --content, tail = %v, want %v", tail, extra)
	}
	// The derived flags are still there, before them.
	if !strings.Contains(strings.Join(withContent, " "), "--no-media") {
		t.Errorf("a content selection must still disable what it left out: %v", withContent)
	}
}

// TestEngineTuningArgs covers the builder on its own, where the boundaries are
// easiest to read.
func TestEngineTuningArgs(t *testing.T) {
	if got := engineTuningArgs(Options{}); len(got) != 0 {
		t.Errorf("nothing asked for, nothing passed: %v", got)
	}
	if got := engineTuningArgs(Options{RateLimit: -1}); len(got) != 0 {
		t.Errorf("a negative delay is not a delay: %v", got)
	}
	got := engineTuningArgs(Options{UserAgent: " Agent ", RateLimit: 1})
	if !argPair(got, "--user-agent", "Agent") {
		t.Errorf("the agent must be trimmed: %v", got)
	}
	if !argPair(got, "--rate-limit", "1") {
		t.Errorf("one millisecond is a delay: %v", got)
	}
}
