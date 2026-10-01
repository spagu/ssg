package httpapi

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/spagu/ssg/services/tts-server/internal/limit"
)

func TestAuth(t *testing.T) {
	hs := newHarness(t, func(d *Deps) { d.APIKeys = []string{"user"} })
	if rec := hs.do("GET", "/v1/voices", ""); rec.Code != 401 || rec.Header().Get("WWW-Authenticate") == "" {
		t.Fatal(rec.Code)
	}
	if rec := hs.do("GET", "/v1/voices", "", "Authorization", "Basic user"); rec.Code != 401 {
		t.Fatal("basic scheme must fail")
	}
	if rec := hs.do("GET", "/v1/voices", "", "Authorization", "Bearer wrong"); rec.Code != 401 {
		t.Fatal(rec.Code)
	}
	for _, key := range []string{"user", "admin"} {
		if rec := hs.do("GET", "/v1/voices", "", "Authorization", "bearer "+key); rec.Code != 200 {
			t.Fatal(key, rec.Code)
		}
	}
	if rec := hs.do("DELETE", "/v1/voices/gosia", "", "Authorization", "Bearer user"); rec.Code != 401 {
		t.Fatal("user key must not manage voices", rec.Code)
	}
}

func TestAdminDisabled(t *testing.T) {
	hs := newHarness(t, func(d *Deps) { d.AdminKeys = nil })
	if rec := hs.do("DELETE", "/v1/voices/gosia", ""); rec.Code != 403 || errCode(t, rec.Body.String()) != codeForbidden {
		t.Fatal(rec.Code)
	}
}

func TestRateLimit(t *testing.T) {
	hs := newHarness(t, func(d *Deps) { d.Limiter = limit.NewRateLimiter(1, 1); d.APIKeys = []string{"k"} })
	auth := []string{"Authorization", "Bearer k"}
	if rec := hs.do("GET", "/v1/voices", "", auth...); rec.Code != 200 {
		t.Fatal(rec.Code)
	}
	rec := hs.do("GET", "/v1/voices", "", auth...)
	if rec.Code != 429 || rec.Header().Get("Retry-After") == "" {
		t.Fatal(rec.Code)
	}
	if retryAfter(0) != "1" || retryAfter(1500*1e6) != "2" {
		t.Fatal("retryAfter")
	}
}

func TestClientID(t *testing.T) {
	r, _ := http.NewRequest("GET", "/", nil)
	r.RemoteAddr = "nonsense"
	if clientID(r) != "ip:nonsense" {
		t.Fatal(clientID(r))
	}
}

func TestListVoices(t *testing.T) {
	hs := newHarness(t)
	var out voiceList
	rec := hs.do("GET", "/v1/voices", "")
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil || out.Count != 2 || out.Voices[0].ID != "gosia" || !out.Voices[0].Default {
		t.Fatal(err, rec.Body.String())
	}
	for q, want := range map[string]int{"?lang=pl": 1, "?lang=pl-PL": 1, "?lang=p": 0, "?engine=espeak": 1, "?lang=de": 0} {
		_ = json.Unmarshal(hs.do("GET", "/v1/voices"+q, "").Body.Bytes(), &out)
		if out.Count != want {
			t.Errorf("%s: %d", q, out.Count)
		}
	}
}

func TestHealthDocsAndRouting(t *testing.T) {
	hs := newHarness(t)
	if rec := hs.do("GET", "/healthz", ""); rec.Code != 200 {
		t.Fatal(rec.Code)
	}
	if rec := hs.do("GET", "/readyz", ""); rec.Code != 200 || !strings.Contains(rec.Body.String(), `"voices":2`) {
		t.Fatal(rec.Body.String())
	}
	hs.readyErr = errBoom
	if rec := hs.do("GET", "/readyz", ""); rec.Code != 503 {
		t.Fatal(rec.Code)
	}
	if rec := hs.do("GET", "/openapi.yaml", ""); rec.Code != 200 || !strings.HasPrefix(rec.Body.String(), "openapi: 3.1") {
		t.Fatal(rec.Code)
	}
	rec := hs.do("GET", "/docs", "")
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "swagger-ui-dist@") || rec.Header().Get("Content-Security-Policy") == "" {
		t.Fatal(rec.Code)
	}
	if rec := hs.do("GET", "/docs/swagger-init.js", ""); !strings.Contains(rec.Body.String(), "/openapi.yaml") {
		t.Fatal("init script")
	}
	if rec := hs.do("GET", "/", ""); rec.Code != 302 || rec.Header().Get("Location") != "/docs" {
		t.Fatal(rec.Code)
	}
	if rec := hs.do("GET", "/nope", ""); rec.Code != 404 || errCode(t, rec.Body.String()) != codeNotFound {
		t.Fatal(rec.Code)
	}
	if rec := hs.do("GET", "/healthz", ""); rec.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatal("security header")
	}
}

func TestPanicRecovery(t *testing.T) {
	hs := newHarness(t)
	hs.deps.Ready = nil // readyz will panic on nil func
	hs.h = NewHandler(hs.deps)
	if rec := hs.do("GET", "/readyz", ""); rec.Code != 500 || errCode(t, rec.Body.String()) != codeInternal {
		t.Fatal(rec.Code)
	}
}

func TestSpecMatchesErrorCodes(t *testing.T) {
	spec, err := os.ReadFile("openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	for _, code := range []string{codeBadRequest, codeTextTooLong, codeUnknownVoice, codeUnknownLang, codeUnauthorized,
		codeForbidden, codeNotFound, codeConflict, codeTooLarge, codeRateLimited, codeBusy, codeEngineFailure,
		codeTimeout, codeNotReady, codeInternal} {
		if !strings.Contains(string(spec), code) {
			t.Errorf("openapi.yaml does not document %s", code)
		}
	}
}
