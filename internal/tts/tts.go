// Package tts turns article text into MP3 through an external text-to-speech
// API at build time (1.8.65). It owns the parts every provider shares: splitting
// text to the provider's size limit, retrying what is worth retrying, giving up
// for the rest of a build once the API is plainly down, and joining the parts —
// and an optional jingle — into one file.
//
// It knows nothing about pages, caches or output paths; the generator decides
// what to read aloud and where the audio lands.
package tts

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Config describes one TTS endpoint and how hard to try it.
type Config struct {
	Provider string  // generic (default), openai, elevenlabs, google
	APIURL   string  // endpoint; the hosted providers have a default
	APIKey   string  // already resolved from $ENV by the caller
	Voice    string  // provider voice id or name
	Lang     string  // default language when a call gives none
	Model    string  // provider model id (openai, elevenlabs)
	Speed    float64 // 1.0 = normal; 0 means the provider default
	// Instructions steer delivery on providers that take them (openai
	// gpt-4o-mini-tts): "calm, unhurried".
	Instructions string
	Timeout      time.Duration // per HTTP request; default 60s
	Retries      int           // extra attempts after the first; default 2
	MaxChars     int           // per request; default is the provider's limit
	// Breaker is how many failed syntheses in a row open the circuit: after
	// that, every call fails fast with ErrCircuitOpen until the build ends.
	// Default 3; a negative value never opens it.
	Breaker int
}

// ErrCircuitOpen is returned once the breaker has opened: the API failed
// Breaker times in a row, and calling it again for every remaining page would
// only multiply the timeout by the number of pages.
var ErrCircuitOpen = errors.New("tts: API unavailable, skipped for the rest of this build")

// Client synthesizes speech. It is safe for concurrent use.
type Client struct {
	cfg      Config
	provider provider
	http     *http.Client
	sleep    func(context.Context, time.Duration) error

	mu       sync.Mutex
	failures int
}

// New validates cfg and returns a client.
func New(cfg Config) (*Client, error) {
	p, err := providerFor(cfg)
	if err != nil {
		return nil, err
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 60 * time.Second
	}
	if cfg.Retries == 0 {
		cfg.Retries = 2
	}
	if cfg.Retries < 0 {
		cfg.Retries = 0
	}
	if cfg.MaxChars <= 0 {
		cfg.MaxChars = p.maxChars()
	}
	if cfg.Breaker == 0 {
		cfg.Breaker = 3
	}
	return &Client{cfg: cfg, provider: p, http: &http.Client{Timeout: cfg.Timeout}, sleep: sleepCtx}, nil
}

// Describe is the provider and voice, for cache keys and log lines.
func (c *Client) Describe() string {
	return strings.Join([]string{c.cfg.Provider, c.cfg.APIURL, c.cfg.Model, c.cfg.Voice,
		fmt.Sprintf("%.2f", c.cfg.Speed), c.cfg.Instructions}, "|")
}

// Synthesize reads text aloud in lang ("" = the configured default) and
// returns one MP3. Long text is split at sentence boundaries and the parts
// joined. A failure counts toward the breaker; a success resets it.
func (c *Client) Synthesize(ctx context.Context, text, lang string) ([]byte, error) {
	if c.circuitOpen() {
		return nil, ErrCircuitOpen
	}
	if lang == "" {
		lang = c.cfg.Lang
	}
	var parts [][]byte
	for _, chunk := range Chunk(text, c.cfg.MaxChars) {
		audio, err := c.call(ctx, chunk, lang)
		if err != nil {
			c.record(false)
			return nil, err
		}
		parts = append(parts, audio)
	}
	if len(parts) == 0 {
		return nil, errors.New("tts: nothing to read")
	}
	c.record(true)
	return Join(parts...), nil
}

// circuitOpen reports whether the breaker has tripped.
func (c *Client) circuitOpen() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.cfg.Breaker > 0 && c.failures >= c.cfg.Breaker
}

// record counts a failure or resets the count on success.
func (c *Client) record(ok bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if ok {
		c.failures = 0
		return
	}
	c.failures++
}

// sleepCtx waits d or until ctx ends.
func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
