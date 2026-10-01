// Package config reads the server settings from environment variables.
//
// Every setting has a safe default, so an empty environment yields a working
// (if unauthenticated) server. Load never reads os.Getenv directly: it takes a
// lookup function, which keeps it pure and trivially testable.
package config

import (
	"errors"
	"fmt"
	"log/slog"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// Lookup returns the value of an environment variable and whether it was set.
// os.LookupEnv satisfies it.
type Lookup func(key string) (string, bool)

// Config holds every runtime setting of the TTS server.
type Config struct {
	Addr           string        // TTS_ADDR: listen address
	APIKeys        []string      // TTS_API_KEYS: bearer keys for /v1 (empty disables auth)
	AdminKeys      []string      // TTS_ADMIN_KEYS: bearer keys for voice upload/delete
	VoicesDir      string        // TTS_VOICES_DIR: Piper models and voices.yaml
	CacheDir       string        // TTS_CACHE_DIR: on-disk cache; empty keeps the cache in memory
	CacheBytes     int64         // TTS_CACHE_MB: cache budget; 0 disables caching
	MaxChars       int           // TTS_MAX_CHARS: longest accepted text, in characters
	MaxConcurrency int           // TTS_MAX_CONCURRENCY: syntheses running at once
	QueueSize      int           // TTS_QUEUE_SIZE: requests allowed to wait for a slot
	QueueWait      time.Duration // TTS_QUEUE_TIMEOUT: longest wait for a slot
	Timeout        time.Duration // TTS_TIMEOUT: per-request synthesis deadline
	RatePerMinute  float64       // TTS_RATE_LIMIT: requests per minute per key/IP; 0 disables
	RateBurst      int           // TTS_RATE_BURST: token-bucket size
	MaxUploadBytes int64         // TTS_MAX_UPLOAD_MB: largest accepted voice upload
	DefaultLang    string        // TTS_DEFAULT_LANG: language used when a request names none
	EspeakBin      string        // TTS_ESPEAK_BIN
	PiperBin       string        // TTS_PIPER_BIN
	LameBin        string        // TTS_LAME_BIN
	LogLevel       slog.Level    // TTS_LOG_LEVEL: debug, info, warn, error
}

const megabyte = 1 << 20

// Load builds a Config from the environment, reporting every invalid value
// at once rather than stopping at the first.
func Load(lookup Lookup) (Config, error) {
	p := parser{lookup: lookup}
	cpus := runtime.NumCPU()
	cfg := Config{
		Addr:           p.str("TTS_ADDR", ":8080"),
		APIKeys:        p.list("TTS_API_KEYS"),
		AdminKeys:      p.list("TTS_ADMIN_KEYS"),
		VoicesDir:      p.str("TTS_VOICES_DIR", "/voices"),
		CacheDir:       p.str("TTS_CACHE_DIR", ""),
		CacheBytes:     int64(p.integer("TTS_CACHE_MB", 128)) * megabyte,
		MaxChars:       p.integer("TTS_MAX_CHARS", 20000),
		MaxConcurrency: p.integer("TTS_MAX_CONCURRENCY", cpus),
		QueueSize:      p.integer("TTS_QUEUE_SIZE", cpus),
		QueueWait:      p.duration("TTS_QUEUE_TIMEOUT", 10*time.Second),
		Timeout:        p.duration("TTS_TIMEOUT", 60*time.Second),
		RatePerMinute:  p.float("TTS_RATE_LIMIT", 60),
		RateBurst:      p.integer("TTS_RATE_BURST", 10),
		MaxUploadBytes: int64(p.integer("TTS_MAX_UPLOAD_MB", 256)) * megabyte,
		DefaultLang:    p.str("TTS_DEFAULT_LANG", "en"),
		EspeakBin:      p.str("TTS_ESPEAK_BIN", "espeak-ng"),
		PiperBin:       p.str("TTS_PIPER_BIN", "piper"),
		LameBin:        p.str("TTS_LAME_BIN", "lame"),
		LogLevel:       p.level("TTS_LOG_LEVEL"),
	}
	p.check(cfg.CacheBytes >= 0, "TTS_CACHE_MB must be >= 0")
	p.check(cfg.MaxChars > 0, "TTS_MAX_CHARS must be > 0")
	p.check(cfg.MaxConcurrency > 0, "TTS_MAX_CONCURRENCY must be > 0")
	p.check(cfg.QueueSize >= 0, "TTS_QUEUE_SIZE must be >= 0")
	p.check(cfg.Timeout > 0, "TTS_TIMEOUT must be > 0")
	p.check(cfg.RatePerMinute >= 0, "TTS_RATE_LIMIT must be >= 0")
	p.check(cfg.RateBurst > 0, "TTS_RATE_BURST must be > 0")
	p.check(cfg.MaxUploadBytes > 0, "TTS_MAX_UPLOAD_MB must be > 0")
	return cfg, errors.Join(p.errs...)
}

// AuthEnabled reports whether /v1 endpoints require a bearer key.
func (c Config) AuthEnabled() bool { return len(c.APIKeys) > 0 }

// parser collects conversion errors while reading variables.
type parser struct {
	lookup Lookup
	errs   []error
}

func (p *parser) raw(key string) (string, bool) {
	v, ok := p.lookup(key)
	v = strings.TrimSpace(v)
	return v, ok && v != ""
}

func (p *parser) str(key, def string) string {
	if v, ok := p.raw(key); ok {
		return v
	}
	return def
}

func (p *parser) list(key string) []string {
	v, _ := p.raw(key)
	var out []string
	for item := range strings.SplitSeq(v, ",") {
		if item = strings.TrimSpace(item); item != "" {
			out = append(out, item)
		}
	}
	return out
}

func (p *parser) integer(key string, def int) int {
	v, ok := p.raw(key)
	if !ok {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		p.errs = append(p.errs, fmt.Errorf("%s: not an integer: %q", key, v))
		return def
	}
	return n
}

func (p *parser) float(key string, def float64) float64 {
	v, ok := p.raw(key)
	if !ok {
		return def
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		p.errs = append(p.errs, fmt.Errorf("%s: not a number: %q", key, v))
		return def
	}
	return f
}

func (p *parser) duration(key string, def time.Duration) time.Duration {
	v, ok := p.raw(key)
	if !ok {
		return def
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		p.errs = append(p.errs, fmt.Errorf("%s: not a duration (e.g. 60s): %q", key, v))
		return def
	}
	return d
}

func (p *parser) level(key string) slog.Level {
	var lvl slog.Level
	v, ok := p.raw(key)
	if !ok {
		return slog.LevelInfo
	}
	if err := lvl.UnmarshalText([]byte(v)); err != nil {
		p.errs = append(p.errs, fmt.Errorf("%s: unknown level %q", key, v))
		return slog.LevelInfo
	}
	return lvl
}

func (p *parser) check(ok bool, msg string) {
	if !ok {
		p.errs = append(p.errs, errors.New(msg))
	}
}
