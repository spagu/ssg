package ai

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// TestResolveAgentModelAndTemperature: an agent naming a missing model fails
// with both names, and a positive temperature overrides the model's default.
func TestResolveAgentModelAndTemperature(t *testing.T) {
	models := map[string]Model{"m": {URL: "http://x", Model: "m1"}}
	agents := map[string]Agent{
		"broken": {Model: "ghost"},
		"warm":   {Model: "m", Temperature: 0.9},
	}
	c := New(models, agents, "", "", t.TempDir(), 0)
	if _, err := c.resolveAgent("broken"); err == nil ||
		!strings.Contains(err.Error(), "broken") || !strings.Contains(err.Error(), "ghost") {
		t.Errorf("agent with a missing model must name both, got: %v", err)
	}
	r, err := c.resolveAgent("warm")
	if err != nil || r.temperature != 0.9 {
		t.Errorf("temperature override = %v, %v (want 0.9)", r.temperature, err)
	}
}

// TestResolveDefaultModelAndUnconfigured: with several models the configured
// default wins, and with no default at all resolve refuses with guidance.
func TestResolveDefaultModelAndUnconfigured(t *testing.T) {
	models := map[string]Model{
		"ma": {URL: "http://a", Model: "m1"},
		"mb": {URL: "http://b", Model: "m2"},
	}
	c := New(models, noAgents(), "ma", "", t.TempDir(), 0)
	r, err := c.resolve("", "")
	if err != nil || r.model != "m1" {
		t.Errorf("default model resolve = %+v, %v (want m1)", r, err)
	}
	bare := New(models, noAgents(), "", "", t.TempDir(), 0)
	if _, err := bare.resolve("", ""); err == nil ||
		!strings.Contains(err.Error(), "no ai agent or model") {
		t.Errorf("ambiguous client must refuse, got: %v", err)
	}
}

// TestAskTransportErrors: an unparsable endpoint URL fails at request build,
// and a dead endpoint fails at dispatch.
func TestAskTransportErrors(t *testing.T) {
	bad := New(map[string]Model{"m": {URL: "http://127.0.0.1\n", Model: "m1"}},
		noAgents(), "m", "", t.TempDir(), 0)
	if _, err := bad.Query("", "m", "q", 0); err == nil {
		t.Error("control character in the URL must fail the request build")
	}
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	url := srv.URL
	srv.Close() // the port is now refused
	dead := New(map[string]Model{"m": {URL: url, Model: "m1"}},
		noAgents(), "m", "", t.TempDir(), time.Second)
	if _, err := dead.Query("", "m", "q", 0); err == nil {
		t.Error("a closed endpoint must fail the dispatch")
	}
}

// TestCompareOperatorEdges: unknown operators are false, and ordered compare
// covers the numeric and lexicographic <= paths plus its own unknown-op guard.
func TestCompareOperatorEdges(t *testing.T) {
	if compare("~~", "a", "b") {
		t.Error("unknown operator must compare false")
	}
	cases := []struct {
		op, got, want string
		expect        bool
	}{
		{"<=", "2", "3", true},  // numeric
		{"<=", "3", "2", false}, // numeric
		{"<=", "a", "b", true},  // lexicographic
	}
	for _, c := range cases {
		if got := compareOrdered(c.op, c.got, c.want); got != c.expect {
			t.Errorf("compareOrdered(%q, %q, %q) = %v, want %v", c.op, c.got, c.want, got, c.expect)
		}
	}
	if compareOrdered("~", "a", "b") {
		t.Error("unknown ordered operator must compare false")
	}
}
