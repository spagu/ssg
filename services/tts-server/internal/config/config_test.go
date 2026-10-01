package config

import (
	"log/slog"
	"strings"
	"testing"
	"time"
)

func env(m map[string]string) Lookup {
	return func(k string) (string, bool) { v, ok := m[k]; return v, ok }
}

func TestLoadDefaults(t *testing.T) {
	cfg, err := Load(env(nil))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Addr != ":8080" || cfg.MaxChars != 20000 || cfg.Timeout != 60*time.Second ||
		cfg.VoicesDir != "/voices" || cfg.CacheBytes != 128<<20 || cfg.LogLevel != slog.LevelInfo ||
		cfg.MaxConcurrency < 1 || cfg.AuthEnabled() {
		t.Fatalf("unexpected defaults: %+v", cfg)
	}
}

func TestLoadValues(t *testing.T) {
	cfg, err := Load(env(map[string]string{
		"TTS_API_KEYS": " a , ,b", "TTS_ADMIN_KEYS": "x", "TTS_MAX_CHARS": "10",
		"TTS_TIMEOUT": "5s", "TTS_RATE_LIMIT": "1.5", "TTS_LOG_LEVEL": "debug", "TTS_CACHE_MB": "0",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.APIKeys) != 2 || cfg.APIKeys[1] != "b" || !cfg.AuthEnabled() || cfg.MaxChars != 10 ||
		cfg.Timeout != 5*time.Second || cfg.RatePerMinute != 1.5 || cfg.LogLevel != slog.LevelDebug || cfg.CacheBytes != 0 {
		t.Fatalf("unexpected: %+v", cfg)
	}
}

func TestLoadErrors(t *testing.T) {
	_, err := Load(env(map[string]string{
		"TTS_MAX_CHARS": "x", "TTS_RATE_LIMIT": "y", "TTS_TIMEOUT": "z", "TTS_LOG_LEVEL": "loud",
		"TTS_MAX_CONCURRENCY": "0", "TTS_CACHE_MB": "-1",
	}))
	if err == nil {
		t.Fatal("want error")
	}
	for _, want := range []string{"TTS_MAX_CHARS", "TTS_RATE_LIMIT", "TTS_TIMEOUT", "TTS_LOG_LEVEL", "TTS_MAX_CONCURRENCY", "TTS_CACHE_MB"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error lacks %s: %v", want, err)
		}
	}
}
