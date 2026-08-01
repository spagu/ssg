// Package ai runs build-time AI queries for the [ai …] content shortcode. Answers
// are content-addressed cached (keyed by model + question), so a build is
// deterministic and a model is only re-queried when its question or config
// changes — the same guarantee the image pipeline gives (#1.8.16).
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

// Model is one named AI endpoint (OpenAI-compatible chat completions).
type Model struct {
	URL, Key, Model, System string
	MaxTokens               int
	Temperature             float64
}

// Client answers questions via named models, caching every result on disk.
type Client struct {
	models   map[string]Model
	def      string
	cacheDir string
	timeout  time.Duration
	http     *http.Client

	mu  sync.Mutex
	mem map[string]string // in-memory cache mirror, guards concurrent Query
}

// New builds a client. cacheDir defaults to ".ai-cache", timeout to 30s. Model
// keys beginning with "$" are read from the environment.
func New(models map[string]Model, defaultModel, cacheDir string, timeout time.Duration) *Client {
	if cacheDir == "" {
		cacheDir = ".ai-cache"
	}
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	return &Client{
		models:   models,
		def:      defaultModel,
		cacheDir: cacheDir,
		timeout:  timeout,
		http:     &http.Client{Timeout: timeout},
		mem:      map[string]string{},
	}
}

// Enabled reports whether any model is configured.
func (c *Client) Enabled() bool { return c != nil && len(c.models) > 0 }

// resolveModel returns the named model, or the default when name is empty.
func (c *Client) resolveModel(name string) (Model, string, error) {
	if name == "" {
		name = c.def
	}
	if name == "" {
		// Exactly one model configured ⇒ use it without naming.
		if len(c.models) == 1 {
			for k := range c.models {
				name = k
			}
		}
	}
	m, ok := c.models[name]
	if !ok {
		return Model{}, name, fmt.Errorf("unknown ai model %q (configure it under ai.models)", name)
	}
	return m, name, nil
}

// cacheKey derives the deterministic cache key for a query.
func cacheKey(m Model, question string) string {
	h := sha256.Sum256([]byte(m.URL + "\x00" + m.Model + "\x00" + m.System + "\x00" +
		fmt.Sprintf("%d\x00%g\x00", m.MaxTokens, m.Temperature) + question))
	return hex.EncodeToString(h[:])
}

// Query answers question via the named model (default when empty), returning the
// cached answer when present. timeout <= 0 uses the client default. On any
// transport/parse failure it returns the error so the caller can fall back.
func (c *Client) Query(modelName, question string, timeout time.Duration) (string, error) {
	m, _, err := c.resolveModel(modelName)
	if err != nil {
		return "", err
	}
	key := cacheKey(m, question)

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

	answer, err := c.ask(m, question, timeout)
	if err != nil {
		return "", err
	}
	c.writeCache(key, answer)
	c.mu.Lock()
	c.mem[key] = answer
	c.mu.Unlock()
	return answer, nil
}

// ask performs the live chat-completions request.
func (c *Client) ask(m Model, question string, timeout time.Duration) (string, error) {
	if timeout <= 0 {
		timeout = c.timeout
	}
	msgs := []map[string]string{}
	if m.System != "" {
		msgs = append(msgs, map[string]string{"role": "system", "content": m.System})
	}
	msgs = append(msgs, map[string]string{"role": "user", "content": question})
	payload := map[string]interface{}{"model": m.Model, "messages": msgs}
	if m.MaxTokens > 0 {
		payload["max_tokens"] = m.MaxTokens
	}
	if m.Temperature > 0 {
		payload["temperature"] = m.Temperature
	}
	body, _ := json.Marshal(payload)

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	// #nosec G704 -- m.URL is author config (not request-controlled); the AI
	// endpoint and key are trusted build-time settings.
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, m.URL, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	if key := expandEnv(m.Key); key != "" {
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
