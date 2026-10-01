package tts

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// noSleep records waits instead of waiting.
func noSleep(waits *[]time.Duration) func(context.Context, time.Duration) error {
	return func(_ context.Context, d time.Duration) error {
		*waits = append(*waits, d)
		return nil
	}
}

func TestNewValidates(t *testing.T) {
	for _, cfg := range []Config{
		{Provider: "generic"},
		{Provider: "elevenlabs"},
		{Provider: "nope", APIURL: "x"},
	} {
		if _, err := New(cfg); err == nil {
			t.Errorf("New(%+v) must fail", cfg)
		}
	}
	c, err := New(Config{Provider: "openai", Retries: -1, Breaker: -1})
	if err != nil {
		t.Fatal(err)
	}
	if c.cfg.Retries != 0 || c.cfg.MaxChars != 4000 || c.cfg.Timeout != 60*time.Second {
		t.Errorf("defaults: %+v", c.cfg)
	}
	if !strings.HasPrefix(c.Describe(), "openai|") {
		t.Errorf("Describe = %q", c.Describe())
	}
}

// TestGenericContract: the request ssg sends to services/tts-server.
func TestGenericContract(t *testing.T) {
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer k" {
			t.Errorf("auth = %q", r.Header.Get("Authorization"))
		}
		_ = json.NewDecoder(r.Body).Decode(&got)
		w.Header().Set("Content-Type", "audio/mpeg")
		_, _ = w.Write([]byte("MP3"))
	}))
	defer srv.Close()
	c, err := New(Config{APIURL: srv.URL, APIKey: "k", Voice: "v", Lang: "en", Speed: 1.2})
	if err != nil {
		t.Fatal(err)
	}
	audio, err := c.Synthesize(context.Background(), "Hello there.", "")
	if err != nil || string(audio) != "MP3" {
		t.Fatalf("Synthesize = %q, %v", audio, err)
	}
	if got["text"] != "Hello there." || got["voice"] != "v" || got["lang"] != "en" || got["format"] != "mp3" || got["speed"] != 1.2 {
		t.Errorf("body = %v", got)
	}
}

// TestProvidersShapeTheirRequests checks each hosted provider's endpoint,
// auth header and body, against a stand-in server.
func TestProvidersShapeTheirRequests(t *testing.T) {
	for _, tc := range []struct {
		provider, voice, wantPath, wantHeader, wantField string
		respond                                          func(w http.ResponseWriter)
	}{
		{"openai", "", "/", "Authorization", "input", func(w http.ResponseWriter) { _, _ = w.Write([]byte("MP3")) }},
		{"elevenlabs", "voice 1", "/voice%201", "Xi-Api-Key", "text", func(w http.ResponseWriter) { _, _ = w.Write([]byte("MP3")) }},
		{"google", "pl-PL-Wavenet-A", "/", "X-Goog-Api-Key", "input", func(w http.ResponseWriter) {
			_ = json.NewEncoder(w).Encode(map[string]string{"audioContent": base64.StdEncoding.EncodeToString([]byte("MP3"))})
		}},
	} {
		t.Run(tc.provider, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.EscapedPath() != tc.wantPath {
					t.Errorf("path = %q, want %q", r.URL.EscapedPath(), tc.wantPath)
				}
				if r.Header.Get(tc.wantHeader) == "" {
					t.Errorf("missing %s", tc.wantHeader)
				}
				var body map[string]any
				_ = json.NewDecoder(r.Body).Decode(&body)
				if _, ok := body[tc.wantField]; !ok {
					t.Errorf("body lacks %q: %v", tc.wantField, body)
				}
				tc.respond(w)
			}))
			defer srv.Close()
			c, err := New(Config{Provider: tc.provider, APIURL: srv.URL, APIKey: "k", Voice: tc.voice,
				Speed: 1.1, Instructions: "calm", Model: "m"})
			if err != nil {
				t.Fatal(err)
			}
			audio, err := c.Synthesize(context.Background(), "Hi.", "pl-PL")
			if err != nil || string(audio) != "MP3" {
				t.Fatalf("Synthesize = %q, %v", audio, err)
			}
		})
	}
}

func TestGoogleDecodeErrors(t *testing.T) {
	for _, body := range []string{"{", `{"audioContent":""}`, `{"audioContent":"!!"}`} {
		if _, err := (google{}).decode([]byte(body)); err == nil {
			t.Errorf("decode(%q) must fail", body)
		}
	}
}

// TestRetryThenSucceed: 503 and 429 are retried, with backoff and Retry-After.
func TestRetryThenSucceed(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		switch calls.Add(1) {
		case 1:
			w.WriteHeader(http.StatusServiceUnavailable)
		case 2:
			w.Header().Set("Retry-After", "5")
			w.WriteHeader(http.StatusTooManyRequests)
		default:
			_, _ = w.Write([]byte("MP3"))
		}
	}))
	defer srv.Close()
	c, _ := New(Config{APIURL: srv.URL, Retries: 3})
	var waits []time.Duration
	c.sleep = noSleep(&waits)
	if _, err := c.Synthesize(context.Background(), "x", ""); err != nil {
		t.Fatal(err)
	}
	if len(waits) != 2 || waits[0] != time.Second || waits[1] != 5*time.Second {
		t.Errorf("waits = %v, want [1s 5s]", waits)
	}
}

// TestClientErrorsAreNotRetried: a bad key is a 401 every time.
func TestClientErrorsAreNotRetried(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		http.Error(w, `{"error":"bad key"}`, http.StatusUnauthorized)
	}))
	defer srv.Close()
	c, _ := New(Config{APIURL: srv.URL})
	_, err := c.Synthesize(context.Background(), "x", "")
	if err == nil || !strings.Contains(err.Error(), "401") || !strings.Contains(err.Error(), "bad key") || calls.Load() != 1 {
		t.Errorf("err = %v after %d calls", err, calls.Load())
	}
}

// TestBreakerOpens: after Breaker failed syntheses the API is not called again.
func TestBreakerOpens(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()
	c, _ := New(Config{APIURL: srv.URL, Retries: -1, Breaker: 2})
	var waits []time.Duration
	c.sleep = noSleep(&waits)
	for i := 0; i < 2; i++ {
		if _, err := c.Synthesize(context.Background(), "x", ""); err == nil || errors.Is(err, ErrCircuitOpen) {
			t.Fatalf("call %d: %v", i, err)
		}
	}
	if _, err := c.Synthesize(context.Background(), "x", ""); !errors.Is(err, ErrCircuitOpen) {
		t.Fatalf("third call = %v, want ErrCircuitOpen", err)
	}
	if calls.Load() != 2 {
		t.Errorf("API called %d times, want 2", calls.Load())
	}
}

func TestNetworkErrorAndCancel(t *testing.T) {
	c, _ := New(Config{APIURL: "http://127.0.0.1:1", Retries: 1})
	var waits []time.Duration
	c.sleep = noSleep(&waits)
	if _, err := c.Synthesize(context.Background(), "x", ""); err == nil || len(waits) != 1 {
		t.Errorf("network error: %v, waits %v", err, waits)
	}
	c.sleep = func(ctx context.Context, _ time.Duration) error { return context.Canceled }
	if _, err := c.Synthesize(context.Background(), "x", ""); !errors.Is(err, context.Canceled) {
		t.Errorf("cancelled sleep = %v", err)
	}
	if retryable(context.Canceled) {
		t.Error("a cancelled context is not retryable")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := sleepCtx(ctx, time.Hour); !errors.Is(err, context.Canceled) {
		t.Errorf("sleepCtx = %v", err)
	}
	if err := sleepCtx(context.Background(), time.Millisecond); err != nil {
		t.Errorf("sleepCtx = %v", err)
	}
}

func TestEmptyTextAndOversizeResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.CopyN(w, zeroReader{}, maxAudioBytes+1)
	}))
	defer srv.Close()
	c, _ := New(Config{APIURL: srv.URL, Retries: -1})
	if _, err := c.Synthesize(context.Background(), "   ", ""); err == nil {
		t.Error("empty text must fail")
	}
	if _, err := c.Synthesize(context.Background(), "x", ""); err == nil || !strings.Contains(err.Error(), "larger") {
		t.Errorf("oversize = %v", err)
	}
	bad, _ := New(Config{APIURL: "://bad", Retries: -1})
	if _, err := bad.Synthesize(context.Background(), "x", ""); err == nil {
		t.Error("a bad URL must fail")
	}
}

type zeroReader struct{}

func (zeroReader) Read(p []byte) (int, error) {
	clear(p)
	return len(p), nil
}

func TestHelpers(t *testing.T) {
	if retryAfter("7") != 7*time.Second || retryAfter("999") != maxRetryAfter || retryAfter("soon") != 0 || retryAfter("-1") != 0 {
		t.Error("retryAfter")
	}
	if s := snippet([]byte(strings.Repeat("a ", 300))); len(s) > 210 || !strings.HasSuffix(s, "…") {
		t.Errorf("snippet = %q", s)
	}
	if bearer("") != "" || orDefault("", "d") != "d" || orDefault("v", "d") != "v" {
		t.Error("bearer/orDefault")
	}
	if _, err := jsonRequest(context.Background(), "http://x", func() {}, nil); err == nil {
		t.Error("an unmarshalable body must fail")
	}
}
