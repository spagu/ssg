package httpapi

import (
	"log/slog"
	"net/http"
	"time"
)

// statusWriter remembers the status code for the access log.
type statusWriter struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (w *statusWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusWriter) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	n, err := w.ResponseWriter.Write(b)
	w.bytes += n
	return n, err
}

// Unwrap lets http.ResponseController reach the underlying writer.
func (w *statusWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

// withCommon adds security headers, panic recovery and a structured access
// log. Request bodies, headers and query strings are never logged.
func withCommon(log *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w}
		h := sw.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "no-referrer")
		defer func() {
			if p := recover(); p != nil {
				log.Error("panic", "path", r.URL.Path, "panic", p)
				writeError(sw, http.StatusInternalServerError, codeInternal, "internal error")
			}
			// Successful probes are not logged: the container healthcheck asks
			// every 30 s and would bury real traffic. A failing probe still is.
			if isProbe(r.URL.Path) && sw.status < 400 {
				return
			}
			log.Info("request", "method", r.Method, "path", r.URL.Path, "status", sw.status,
				"bytes", sw.bytes, "durationMs", time.Since(start).Milliseconds(), "client", remoteIP(r))
		}()
		next.ServeHTTP(sw, r)
	})
}

// isProbe reports a liveness or readiness path.
func isProbe(path string) bool { return path == "/healthz" || path == "/readyz" }
