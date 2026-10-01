package app

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spagu/ssg/services/tts-server/internal/audio"
	"github.com/spagu/ssg/services/tts-server/internal/config"
)

// fakeRunner answers `--voices` with two languages and anything else with WAV.
type fakeRunner struct{ listErr error }

func (f fakeRunner) Run(_ context.Context, _ string, args []string, _ []byte) ([]byte, error) {
	if len(args) == 1 && args[0] == "--voices" {
		return []byte("Pty Language Age/Gender VoiceName File Other\n 5 en-us --/M English gmw/en-US (en 3)\n 5 pl --/M Polish zlw/pl\n"), f.listErr
	}
	return audio.PCM{SampleRate: 22050, Channels: 1, Data: []byte{1, 2}}.WAV()
}

func testConfig(t *testing.T) config.Config {
	cfg, err := config.Load(func(string) (string, bool) { return "", false })
	if err != nil {
		t.Fatal(err)
	}
	cfg.VoicesDir = filepath.Join(t.TempDir(), "voices")
	cfg.EspeakBin, cfg.LameBin, cfg.PiperBin = "sh", "sh", "no-such-piper"
	return cfg
}

var quiet = slog.New(slog.NewTextHandler(io.Discard, nil))

func TestBuildServesSpeech(t *testing.T) {
	cfg := testConfig(t)
	cfg.CacheDir = filepath.Join(t.TempDir(), "cache")
	a, err := Build(context.Background(), cfg, quiet, fakeRunner{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = a.Close() }()
	rec := httptest.NewRecorder()
	a.Handler.ServeHTTP(rec, httptest.NewRequest("POST", "/v1/speech", strings.NewReader(`{"text":"Cześć","lang":"pl","format":"wav"}`)))
	if rec.Code != 200 || rec.Header().Get("X-TTS-Voice") != "espeak:pl" {
		t.Fatal(rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	a.Handler.ServeHTTP(rec, httptest.NewRequest("GET", "/readyz", nil))
	if rec.Code != 200 {
		t.Fatal(rec.Body.String())
	}
	if srv := NewServer(cfg, a.Handler, quiet); srv.WriteTimeout <= cfg.Timeout || srv.Addr != cfg.Addr {
		t.Fatal("server timeouts")
	}
}

func TestReadiness(t *testing.T) {
	cfg := testConfig(t)
	cfg.EspeakBin = "no-such-espeak"
	a, err := Build(context.Background(), cfg, quiet, fakeRunner{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = a.Close() }()
	check := func(want string) {
		t.Helper()
		rec := httptest.NewRecorder()
		a.Handler.ServeHTTP(rec, httptest.NewRequest("GET", "/readyz", nil))
		if rec.Code != 503 || !strings.Contains(rec.Body.String(), want) {
			t.Fatal(rec.Body.String())
		}
	}
	check("espeak-ng")
	cfg.LameBin = "no-such-lame"
	a2, _ := Build(context.Background(), cfg, quiet, fakeRunner{})
	defer func() { _ = a2.Close() }()
	a = a2
	check("mp3 encoder")
	cfg = testConfig(t)
	a3, _ := Build(context.Background(), cfg, quiet, fakeRunner{listErr: errors.New("x")})
	defer func() { _ = a3.Close() }()
	a = a3
	check("no voices")
}

func TestBuildErrors(t *testing.T) {
	cfg := testConfig(t)
	file := filepath.Join(t.TempDir(), "file")
	_ = os.WriteFile(file, nil, 0o600)
	cfg.VoicesDir = filepath.Join(file, "voices")
	if _, err := Build(context.Background(), cfg, quiet, fakeRunner{}); err == nil {
		t.Fatal("voices dir under a file")
	}
	cfg = testConfig(t)
	cfg.CacheDir = filepath.Join(file, "cache")
	if _, err := Build(context.Background(), cfg, quiet, fakeRunner{}); err == nil {
		t.Fatal("cache dir under a file")
	}
	cfg = testConfig(t)
	_ = os.MkdirAll(cfg.VoicesDir, 0o750)
	_ = os.WriteFile(filepath.Join(cfg.VoicesDir, "voices.yaml"), []byte("bogus: 1"), 0o600)
	if _, err := Build(context.Background(), cfg, quiet, fakeRunner{}); err == nil {
		t.Fatal("bad voices.yaml")
	}
}

func TestBuildWithAuth(t *testing.T) {
	cfg := testConfig(t)
	cfg.APIKeys = []string{"k"}
	a, err := Build(context.Background(), cfg, quiet, fakeRunner{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = a.Close() }()
	rec := httptest.NewRecorder()
	a.Handler.ServeHTTP(rec, httptest.NewRequest("GET", "/v1/voices", nil))
	if rec.Code != 401 {
		t.Fatal(rec.Code)
	}
}
