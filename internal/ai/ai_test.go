package ai

import (
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func chatServer(t *testing.T, reply string, calls *int) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*calls++
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), `"model":"m1"`) {
			t.Errorf("request missing model id: %s", body)
		}
		if r.Header.Get("Authorization") != "Bearer sekret" {
			t.Errorf("missing bearer key: %q", r.Header.Get("Authorization"))
		}
		_, _ = io.WriteString(w, `{"choices":[{"message":{"content":"`+reply+`"}}]}`)
	}))
}

// noAgents is the empty agent map, for model-only clients.
func noAgents() map[string]Agent { return nil }

// TestQueryCaches: a query hits the endpoint once, then serves from cache (disk +
// memory), and the same (model,question) is deterministic.
func TestQueryCaches(t *testing.T) {
	t.Setenv("AI_KEY", "sekret")
	var calls int
	srv := chatServer(t, "42", &calls)
	defer srv.Close()

	c := New(map[string]Model{"fast": {URL: srv.URL, Key: "$AI_KEY", Model: "m1"}}, noAgents(), "fast", "", t.TempDir(), 0)
	if !c.Enabled() {
		t.Fatal("client should be enabled")
	}

	got, err := c.Query("", "fast", "what is the answer?", 0)
	if err != nil || got != "42" {
		t.Fatalf("query = %q, %v", got, err)
	}
	// Second call: served from memory, endpoint not hit again.
	if _, err := c.Query("", "fast", "what is the answer?", 0); err != nil {
		t.Fatalf("cached query: %v", err)
	}
	if calls != 1 {
		t.Errorf("endpoint called %d times, want 1 (rest cached)", calls)
	}

	// A fresh client with the same cache dir reads the disk cache — still no call.
	c2 := New(map[string]Model{"fast": {URL: srv.URL, Key: "$AI_KEY", Model: "m1"}}, noAgents(), "fast", "", c.cacheDir, 0)
	if got, _ := c2.Query("", "fast", "what is the answer?", 0); got != "42" || calls != 1 {
		t.Errorf("disk cache miss: got %q, calls %d", got, calls)
	}
}

// TestQueryErrors: unknown model, unknown agent and a 5xx endpoint all return
// errors so the shortcode can fall back.
func TestQueryErrors(t *testing.T) {
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer bad.Close()
	c := New(map[string]Model{"x": {URL: bad.URL, Model: "m1"}}, noAgents(), "x", "", t.TempDir(), 2*time.Second)

	if _, err := c.Query("", "nope", "q", 0); err == nil {
		t.Error("unknown model must error")
	}
	if _, err := c.Query("ghost", "", "q", 0); err == nil {
		t.Error("unknown agent must error")
	}
	if _, err := c.Query("", "x", "q", 0); err == nil {
		t.Error("5xx endpoint must error")
	}
}

// TestResolveSingle: with exactly one model and no default, it is used without
// naming; likewise a sole agent resolves without naming.
func TestResolveSingle(t *testing.T) {
	c := New(map[string]Model{"only": {URL: "http://x", Model: "m1"}}, noAgents(), "", "", t.TempDir(), 0)
	if _, err := c.resolve("", ""); err != nil {
		t.Errorf("single-model resolve: %v", err)
	}
	ca := New(
		map[string]Model{"only": {URL: "http://x", Model: "m1", System: "base"}},
		map[string]Agent{"solo": {System: "persona"}}, // empty Model ⇒ sole model
		"", "", t.TempDir(), 0)
	r, err := ca.resolve("", "")
	if err != nil {
		t.Fatalf("single-agent resolve: %v", err)
	}
	if !strings.Contains(r.system, "base") || !strings.Contains(r.system, "persona") {
		t.Errorf("agent should layer persona on the model base: %q", r.system)
	}
}

// TestNewDefaults: empty cacheDir/timeout fall back to sane defaults. The
// default root moved to .ssg-cache/ai in GO-091 (with a read-fallback to the
// legacy .ai-cache, tested separately).
func TestNewDefaults(t *testing.T) {
	c := New(map[string]Model{"m": {URL: "http://x", Model: "m1"}}, noAgents(), "", "", "", 0)
	if c.cacheDir != filepath.Join(".ssg-cache", "ai") || c.timeout != 30*time.Second {
		t.Errorf("defaults = %q / %v", c.cacheDir, c.timeout)
	}
}

// TestAskPayloadAndParse: system/max_tokens/temperature reach the request, and a
// malformed or empty response is an error.
func TestAskPayloadAndParse(t *testing.T) {
	var gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		_, _ = io.WriteString(w, `{"choices":[{"message":{"content":"ok"}}]}`)
	}))
	defer srv.Close()
	c := New(map[string]Model{"m": {URL: srv.URL, Model: "m1", System: "be brief", MaxTokens: 50, Temperature: 0.2}}, noAgents(), "m", "", t.TempDir(), 0)
	if _, err := c.Query("", "m", "q", time.Second); err != nil {
		t.Fatalf("query: %v", err)
	}
	for _, want := range []string{`"role":"system"`, `"be brief"`, `"max_tokens":50`, `"temperature":0.2`} {
		if !strings.Contains(gotBody, want) {
			t.Errorf("payload missing %q in %s", want, gotBody)
		}
	}

	// Empty choices → error.
	empty := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"choices":[]}`)
	}))
	defer empty.Close()
	if _, err := New(map[string]Model{"m": {URL: empty.URL, Model: "m1"}}, noAgents(), "m", "", t.TempDir(), 0).Query("", "m", "q", 0); err == nil {
		t.Error("empty choices must error")
	}
	// Malformed JSON → error.
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `not json`)
	}))
	defer bad.Close()
	if _, err := New(map[string]Model{"m": {URL: bad.URL, Model: "m1"}}, noAgents(), "m", "", t.TempDir(), 0).Query("", "m", "q", 0); err == nil {
		t.Error("malformed response must error")
	}
}

// TestAgentComposition covers #1.8.16: an agent layers its persona, rules and
// skills on the model's base system prompt (and they reach the request), agent
// params override the model, and editing a rule changes the cache key so the
// answer is re-queried.
func TestAgentComposition(t *testing.T) {
	var gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		_, _ = io.WriteString(w, `{"choices":[{"message":{"content":"ok"}}]}`)
	}))
	defer srv.Close()

	models := map[string]Model{"base": {URL: srv.URL, Model: "m1", System: "House style.", MaxTokens: 10}}
	agents := map[string]Agent{"writer": {
		Model:     "base",
		System:    "You write summaries.",
		Rules:     []string{"Answer in Polish", "No links"},
		Skills:    []string{"summarise"},
		MaxTokens: 99, // overrides the model's 10
	}}
	c := New(models, agents, "", "", t.TempDir(), 0)

	if _, err := c.Query("writer", "", "q", 0); err != nil {
		t.Fatalf("query: %v", err)
	}
	for _, want := range []string{
		"House style.", "You write summaries.",
		"Rules you must follow:", "- Answer in Polish", "- No links",
		"Skills you can use:", "- summarise",
		`"max_tokens":99`, // agent override reached the request
	} {
		if !strings.Contains(gotBody, want) {
			t.Errorf("composed request missing %q in:\n%s", want, gotBody)
		}
	}

	// resolve() the two agents and confirm a rule change moves the cache key.
	base, _ := c.resolveAgent("writer")
	agents["writer2"] = Agent{Model: "base", Rules: []string{"Answer in English"}}
	c2 := New(models, agents, "", "", t.TempDir(), 0)
	other, _ := c2.resolveAgent("writer2")
	if cacheKey(base, "q") == cacheKey(other, "q") {
		t.Error("changing a rule must change the cache key")
	}
}

// TestResolvePrecedence: explicit agent beats explicit model beats default agent
// beats default model.
func TestResolvePrecedence(t *testing.T) {
	models := map[string]Model{
		"ma": {URL: "http://a", Model: "m1", System: "A"},
		"mb": {URL: "http://b", Model: "m2", System: "B"},
	}
	agents := map[string]Agent{
		"aa": {Model: "ma", System: "agentA"},
		"ab": {Model: "mb", System: "agentB"},
	}
	c := New(models, agents, "ma", "aa", t.TempDir(), 0)

	// No selector ⇒ default agent (aa on ma).
	if r, _ := c.resolve("", ""); !strings.Contains(r.system, "agentA") {
		t.Errorf("default should be the default agent: %q", r.system)
	}
	// Explicit model beats the default agent.
	if r, _ := c.resolve("", "mb"); r.system != "B" || strings.Contains(r.system, "agent") {
		t.Errorf("explicit model must win over default agent: %q", r.system)
	}
	// Explicit agent beats explicit model would (agent arg takes precedence).
	if r, _ := c.resolve("ab", "ma"); !strings.Contains(r.system, "agentB") {
		t.Errorf("explicit agent must win: %q", r.system)
	}
}
