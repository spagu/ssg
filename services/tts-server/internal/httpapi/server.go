// Package httpapi exposes the TTS service over HTTP: routing, auth, rate
// limiting, request validation and the embedded OpenAPI docs.
package httpapi

import (
	"log/slog"
	"net/http"

	"github.com/spagu/ssg/services/tts-server/internal/limit"
	"github.com/spagu/ssg/services/tts-server/internal/synth"
	"github.com/spagu/ssg/services/tts-server/internal/voices"
)

// Deps are the collaborators and limits the handlers need.
type Deps struct {
	Synth          *synth.Service
	Registry       *voices.Registry
	Store          *voices.Store
	Limiter        *limit.RateLimiter
	Logger         *slog.Logger
	Ready          func() error // nil error means ready to serve speech
	PiperReady     func() bool  // whether uploaded Piper voices can be used
	APIKeys        []string
	AdminKeys      []string
	MaxChars       int
	MaxUploadBytes int64
}

// api carries Deps into the handlers.
type api struct{ Deps }

// NewHandler builds the router. Admin keys are also accepted wherever an API
// key is; when no API keys are configured the /v1 read and speech endpoints
// are open, while voice management always needs an admin key.
func NewHandler(d Deps) http.Handler {
	a := &api{d}
	userKeys := newKeySet(append(append([]string{}, d.APIKeys...), d.AdminKeys...))
	adminKeys := newKeySet(d.AdminKeys)
	open := len(d.APIKeys) == 0
	user := func(h http.HandlerFunc) http.Handler {
		return requireKey(userKeys, open, rateLimit(d.Limiter, h))
	}
	admin := func(h http.HandlerFunc) http.Handler {
		return requireAdmin(adminKeys, rateLimit(d.Limiter, h))
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", a.healthz)
	mux.HandleFunc("GET /readyz", a.readyz)
	mux.HandleFunc("GET /openapi.yaml", serveSpec)
	mux.HandleFunc("GET /docs", serveDocs)
	mux.HandleFunc("GET /docs/swagger-init.js", serveDocsInit)
	mux.Handle("GET /{$}", http.RedirectHandler("/docs", http.StatusFound))
	mux.Handle("POST /v1/speech", user(a.speech))
	mux.Handle("GET /v1/voices", user(a.listVoices))
	mux.Handle("POST /v1/voices", admin(a.uploadVoice))
	mux.Handle("DELETE /v1/voices/{id}", admin(a.deleteVoice))
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		writeError(w, http.StatusNotFound, codeNotFound, "no such endpoint")
	})
	return withCommon(d.Logger, mux)
}
