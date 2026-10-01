// Package app wires configuration, engines, voices, cache and limits into
// the HTTP handler and server. It is the only place that knows every part.
package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/spagu/ssg/services/tts-server/internal/audio"
	"github.com/spagu/ssg/services/tts-server/internal/cache"
	"github.com/spagu/ssg/services/tts-server/internal/config"
	"github.com/spagu/ssg/services/tts-server/internal/engine"
	"github.com/spagu/ssg/services/tts-server/internal/execx"
	"github.com/spagu/ssg/services/tts-server/internal/httpapi"
	"github.com/spagu/ssg/services/tts-server/internal/limit"
	"github.com/spagu/ssg/services/tts-server/internal/synth"
	"github.com/spagu/ssg/services/tts-server/internal/voices"
)

// App is the assembled service.
type App struct {
	Handler http.Handler
	closers []func() error
}

// Close releases the directory handles held by the app.
func (a *App) Close() error {
	var errs []error
	for _, c := range a.closers {
		errs = append(errs, c())
	}
	return errors.Join(errs...)
}

// Build assembles the service. runner executes the engine binaries; tests
// pass a fake.
func Build(ctx context.Context, cfg config.Config, log *slog.Logger, runner execx.Runner) (*App, error) {
	a := &App{}
	espeak := engine.Espeak{Bin: cfg.EspeakBin, Runner: runner}
	piper := engine.Piper{Bin: cfg.PiperBin, Runner: runner}
	enc := audio.Encoder{LameBin: cfg.LameBin, Runner: runner}
	if !cfg.AuthEnabled() {
		log.Warn("TTS_API_KEYS is empty: /v1 endpoints accept requests without a key")
	}
	voiceRoot, err := openRoot(cfg.VoicesDir)
	if err != nil {
		return nil, fmt.Errorf("voices dir: %w", err)
	}
	a.closers = append(a.closers, voiceRoot.Close)
	src := voices.Source{
		Root: voiceRoot, Dir: cfg.VoicesDir, Builtins: espeakBuiltins(ctx, espeak, log),
		Piper: piper.Available(), Logger: log,
	}
	reg, err := voices.NewRegistry(src.Load)
	if err != nil {
		return nil, errors.Join(err, a.Close())
	}
	audioCache, err := a.buildCache(cfg)
	if err != nil {
		return nil, errors.Join(err, a.Close())
	}
	svc := &synth.Service{
		Registry: reg, Encoder: enc, Cache: audioCache, Timeout: cfg.Timeout, DefaultLang: cfg.DefaultLang,
		Engines: map[string]engine.Engine{voices.EngineEspeak: espeak, voices.EnginePiper: piper},
		Gate:    limit.NewGate(cfg.MaxConcurrency, cfg.QueueSize, cfg.QueueWait),
	}
	log.Info("voices loaded", "count", reg.Catalog().Len(), "piper", src.Piper, "espeak", espeak.Available())
	a.Handler = httpapi.NewHandler(httpapi.Deps{
		Synth: svc, Registry: reg, Store: &voices.Store{Root: voiceRoot},
		Limiter: limit.NewRateLimiter(cfg.RatePerMinute, cfg.RateBurst), Logger: log,
		Ready:      readiness(enc, espeak, reg),
		PiperReady: piper.Available,
		APIKeys:    cfg.APIKeys, AdminKeys: cfg.AdminKeys,
		MaxChars: cfg.MaxChars, MaxUploadBytes: cfg.MaxUploadBytes,
	})
	return a, nil
}

// NewServer applies the timeouts: the write deadline covers queueing plus
// the synthesis deadline, uploads extend their own deadlines.
func NewServer(cfg config.Config, h http.Handler, log *slog.Logger) *http.Server {
	return &http.Server{
		Addr:              cfg.Addr,
		Handler:           h,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       60 * time.Second,
		WriteTimeout:      cfg.QueueWait + cfg.Timeout + 30*time.Second,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    64 << 10,
		ErrorLog:          slog.NewLogLogger(log.Handler(), slog.LevelWarn),
	}
}

func openRoot(dir string) (*os.Root, error) {
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, err
	}
	return os.OpenRoot(dir)
}

func (a *App) buildCache(cfg config.Config) (*cache.Cache, error) {
	if cfg.CacheDir == "" {
		return cache.NewMemory(cfg.CacheBytes), nil
	}
	root, err := openRoot(cfg.CacheDir)
	if err != nil {
		return nil, fmt.Errorf("cache dir: %w", err)
	}
	a.closers = append(a.closers, root.Close)
	return cache.NewDisk(root, cfg.CacheBytes)
}

// espeakBuiltins lists espeak-ng languages, or none when espeak is missing.
func espeakBuiltins(ctx context.Context, e engine.Espeak, log *slog.Logger) []voices.Voice {
	if !e.Available() {
		log.Warn("espeak-ng not found; built-in voices disabled", "bin", e.Bin)
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	list, err := e.ListVoices(ctx)
	if err != nil {
		log.Warn("cannot list espeak-ng voices", "err", err)
	}
	return list
}

// readiness reports why the service cannot synthesize yet, if it cannot.
func readiness(enc audio.Encoder, e engine.Espeak, reg *voices.Registry) func() error {
	return func() error {
		switch {
		case !enc.Available():
			return fmt.Errorf("mp3 encoder %q not found", enc.LameBin)
		case !e.Available():
			return fmt.Errorf("espeak-ng %q not found", e.Bin)
		case reg.Catalog().Len() == 0:
			return errors.New("no voices loaded")
		}
		return nil
	}
}
