package httpapi

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestProbesAreNotLogged: a successful /healthz or /readyz stays out of the
// access log, a failing one and ordinary requests do not.
func TestProbesAreNotLogged(t *testing.T) {
	var buf bytes.Buffer
	log := slog.New(slog.NewJSONHandler(&buf, nil))
	status := http.StatusOK
	h := withCommon(log, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(status) }))
	get := func(path string) {
		h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, path, nil))
	}
	get("/healthz")
	get("/readyz")
	if buf.Len() != 0 {
		t.Errorf("successful probes were logged: %s", buf.String())
	}
	status = http.StatusServiceUnavailable
	get("/readyz")
	if !strings.Contains(buf.String(), `"path":"/readyz"`) {
		t.Errorf("a failing probe must be logged: %s", buf.String())
	}
	buf.Reset()
	status = http.StatusOK
	get("/v1/voices")
	if !strings.Contains(buf.String(), `"path":"/v1/voices"`) {
		t.Errorf("ordinary requests must be logged: %s", buf.String())
	}
}
