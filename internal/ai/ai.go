// Package ai runs build-time AI queries for the [ai …] content shortcode.
//
// Two layers are configured: a model is an endpoint (url, key, provider model id,
// generation params), and an agent is a role built on a model — it names the model
// it runs on and layers a persona plus user-defined rules and skills on top. A
// shortcode invokes an agent or a bare model; both reduce to a single effective
// request. Answers are content-addressed cached (keyed by that effective request),
// so a build is deterministic and a query only re-runs when its inputs change —
// the same guarantee the image pipeline gives (#1.8.16).
package ai

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Model is one named AI endpoint (OpenAI-compatible chat completions). It is the
// connection layer: where to reach the provider and the base generation params.
// The persona (rules/skills) lives on an Agent that runs on the model.
type Model struct {
	URL, Key, Model, System string
	MaxTokens               int
	Temperature             float64
}

// Agent is a named role built on a model: it runs on a model (by name, or the
// default/sole model when empty) and layers a persona plus user-defined rules and
// skills on top of the model's own system prompt. A shortcode can invoke an agent
// or a bare model. (#1.8.16)
type Agent struct {
	Model       string   // model it runs on; empty ⇒ default/sole model
	System      string   // persona, layered on top of the model's own system prompt
	Rules       []string // constraints the agent must follow
	Skills      []string // capabilities the agent applies
	MaxTokens   int      // 0 ⇒ inherit the model
	Temperature float64  // 0 ⇒ inherit the model
}

// resolved is an effective request: a model endpoint plus the composed system
// prompt and generation params after any agent layering. All caching and requests
// operate on this, so an agent and a bare model that reduce to the same request
// share a cache entry.
type resolved struct {
	url, key, model, system string
	maxTokens               int
	temperature             float64
}

// bulletList renders items as a "- item" block.
func bulletList(items []string) string {
	var b strings.Builder
	for i, it := range items {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString("- ")
		b.WriteString(it)
	}
	return b.String()
}

// composeSystem layers a model's base system text, an agent persona, and the
// agent's rules/skills into one system prompt.
func composeSystem(base, persona string, rules, skills []string) string {
	parts := make([]string, 0, 4)
	if base != "" {
		parts = append(parts, base)
	}
	if persona != "" {
		parts = append(parts, persona)
	}
	if len(rules) > 0 {
		parts = append(parts, "Rules you must follow:\n"+bulletList(rules))
	}
	if len(skills) > 0 {
		parts = append(parts, "Skills you can use:\n"+bulletList(skills))
	}
	return strings.Join(parts, "\n\n")
}

// Client answers questions via named models and agents, caching every result on
// disk.
type Client struct {
	models   map[string]Model
	agents   map[string]Agent
	def      string // default model name
	defAgent string // default agent name (takes precedence over def)
	cacheDir string
	timeout  time.Duration
	http     *http.Client

	mu  sync.Mutex
	mem map[string]string // in-memory cache mirror, guards concurrent Query
}

// New builds a client. cacheDir defaults to ".ai-cache", timeout to 30s. Model and
// agent keys beginning with "$" are read from the environment.
func New(models map[string]Model, agents map[string]Agent, defaultModel, defaultAgent, cacheDir string, timeout time.Duration) *Client {
	if cacheDir == "" {
		cacheDir = ".ai-cache"
	}
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	return &Client{
		models:   models,
		agents:   agents,
		def:      defaultModel,
		defAgent: defaultAgent,
		cacheDir: cacheDir,
		timeout:  timeout,
		http:     &http.Client{Timeout: timeout},
		mem:      map[string]string{},
	}
}

// Enabled reports whether the feature is usable — at least one model is
// configured. Agents run on models, so agents alone are inert.
func (c *Client) Enabled() bool { return c != nil && len(c.models) > 0 }

// resolveModel returns the named model as an effective request, or the default /
// sole model when name is empty.
func (c *Client) resolveModel(name string) (resolved, error) {
	if name == "" {
		name = c.def
	}
	if name == "" && len(c.models) == 1 {
		for k := range c.models {
			name = k
		}
	}
	m, ok := c.models[name]
	if !ok {
		return resolved{}, fmt.Errorf("unknown ai model %q (configure it under ai.models)", name)
	}
	return resolved{url: m.URL, key: m.Key, model: m.Model, system: m.System, maxTokens: m.MaxTokens, temperature: m.Temperature}, nil
}

// resolveAgent returns the named agent as an effective request: its model with the
// persona, rules and skills layered onto the system prompt and any param overrides
// applied.
func (c *Client) resolveAgent(name string) (resolved, error) {
	a, ok := c.agents[name]
	if !ok {
		return resolved{}, fmt.Errorf("unknown ai agent %q (configure it under ai.agents)", name)
	}
	r, err := c.resolveModel(a.Model) // empty ⇒ default/sole model
	if err != nil {
		return resolved{}, fmt.Errorf("ai agent %q: %w", name, err)
	}
	r.system = composeSystem(r.system, a.System, a.Rules, a.Skills)
	if a.MaxTokens > 0 {
		r.maxTokens = a.MaxTokens
	}
	if a.Temperature > 0 {
		r.temperature = a.Temperature
	}
	return r, nil
}

// resolve selects the effective request for a shortcode. Precedence: an explicit
// agent, then an explicit model, then the default agent, then the default model,
// then a sole agent, then a sole model.
func (c *Client) resolve(agentName, modelName string) (resolved, error) {
	switch {
	case agentName != "":
		return c.resolveAgent(agentName)
	case modelName != "":
		return c.resolveModel(modelName)
	case c.defAgent != "":
		return c.resolveAgent(c.defAgent)
	case c.def != "":
		return c.resolveModel(c.def)
	case len(c.agents) == 1:
		for k := range c.agents {
			return c.resolveAgent(k)
		}
	case len(c.models) == 1:
		return c.resolveModel("")
	}
	return resolved{}, fmt.Errorf("no ai agent or model specified (set agent= or model= on the shortcode, or ai.default_agent / ai.default_model)")
}

// cacheKey derives the deterministic cache key for an effective request.
func cacheKey(r resolved, question string) string {
	h := sha256.Sum256([]byte(r.url + "\x00" + r.model + "\x00" + r.system + "\x00" +
		fmt.Sprintf("%d\x00%g\x00", r.maxTokens, r.temperature) + question))
	return hex.EncodeToString(h[:])
}

// Query answers question via the named agent (preferred) or model, returning the
// cached answer when present. Either name may be empty; see resolve for the
// precedence. timeout <= 0 uses the client default. On any transport/parse failure
// it returns the error so the caller can fall back.
func (c *Client) Query(agentName, modelName, question string, timeout time.Duration) (string, error) {
	r, err := c.resolve(agentName, modelName)
	if err != nil {
		return "", err
	}
	key := cacheKey(r, question)

	c.mu.Lock()
	if v, ok := c.mem[key]; ok {
		c.mu.Unlock()
		return v, nil
	}
	c.mu.Unlock()
	if v, ok := c.readCache(key); ok {
		c.mu.Lock()
		c.mem[key] = v
		c.mu.Unlock()
		return v, nil
	}

	answer, err := c.ask(r, question, timeout)
	if err != nil {
		return "", err
	}
	c.writeCache(key, answer)
	c.mu.Lock()
	c.mem[key] = answer
	c.mu.Unlock()
	return answer, nil
}

// ask performs the live chat-completions request for an effective request.
func (c *Client) ask(r resolved, question string, timeout time.Duration) (string, error) {
	if timeout <= 0 {
		timeout = c.timeout
	}
	msgs := []map[string]string{}
	if r.system != "" {
		msgs = append(msgs, map[string]string{"role": "system", "content": r.system})
	}
	msgs = append(msgs, map[string]string{"role": "user", "content": question})
	payload := map[string]interface{}{"model": r.model, "messages": msgs}
	if r.maxTokens > 0 {
		payload["max_tokens"] = r.maxTokens
	}
	if r.temperature > 0 {
		payload["temperature"] = r.temperature
	}
	body, _ := json.Marshal(payload)

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	// #nosec G704 -- r.url is author config (not request-controlled); the AI
	// endpoint and key are trusted build-time settings.
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.url, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	if key := expandEnv(r.key); key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("ai endpoint returned %d", resp.StatusCode)
	}
	var out struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", fmt.Errorf("ai response parse: %w", err)
	}
	if len(out.Choices) == 0 {
		return "", fmt.Errorf("ai response had no choices")
	}
	return strings.TrimSpace(out.Choices[0].Message.Content), nil
}

func (c *Client) cachePath(key string) string {
	return filepath.Join(c.cacheDir, key+".txt")
}

func (c *Client) readCache(key string) (string, bool) {
	b, err := os.ReadFile(c.cachePath(key)) // #nosec G304 -- key is a sha256 hex string
	if err != nil {
		return "", false
	}
	return string(b), true
}

func (c *Client) writeCache(key, answer string) {
	// #nosec G301 -- cache dir is a build artifact directory
	if err := os.MkdirAll(c.cacheDir, 0o755); err != nil {
		return
	}
	// #nosec G306 -- cache entries are non-sensitive build artifacts
	_ = os.WriteFile(c.cachePath(key), []byte(answer), 0o644)
}

// expandEnv returns the value of $VAR, or the literal string otherwise.
func expandEnv(v string) string {
	if strings.HasPrefix(v, "$") {
		return os.Getenv(strings.TrimPrefix(v, "$"))
	}
	return v
}
