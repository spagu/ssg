package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/spagu/ssg/services/tts-server/internal/limit"
)

func errCode(t *testing.T, body string) string {
	t.Helper()
	var e apiError
	if err := json.Unmarshal([]byte(body), &e); err != nil {
		t.Fatalf("not an error body: %q", body)
	}
	return e.Code
}

func TestSpeechSuccessAndETag(t *testing.T) {
	hs := newHarness(t)
	rec := hs.do("POST", "/v1/speech", `{"text":"Dzień dobry","lang":"pl"}`)
	if rec.Code != 200 || rec.Header().Get("Content-Type") != "audio/mpeg" || !strings.HasPrefix(rec.Body.String(), "ID3") ||
		rec.Header().Get("X-TTS-Voice") != "gosia" || rec.Header().Get("X-TTS-Cache") != "miss" {
		t.Fatalf("%d %v %q", rec.Code, rec.Header(), rec.Body.String())
	}
	etag := rec.Header().Get("ETag")
	rec = hs.do("POST", "/v1/speech", `{"text":"Dzień dobry","lang":"pl"}`)
	if rec.Header().Get("X-TTS-Cache") != "hit" || rec.Header().Get("ETag") != etag {
		t.Fatal("cache hit expected")
	}
	rec = hs.do("POST", "/v1/speech", `{"text":"Dzień dobry","lang":"pl"}`, "If-None-Match", `"x", W/`+etag)
	if rec.Code != http.StatusNotModified || rec.Body.Len() != 0 {
		t.Fatal(rec.Code)
	}
	rec = hs.do("POST", "/v1/speech", `{"text":"Hi","voice":"espeak:en","format":"wav","speed":1.5}`)
	if rec.Code != 200 || rec.Header().Get("Content-Type") != "audio/wav" || !strings.HasPrefix(rec.Body.String(), "RIFF") {
		t.Fatal(rec.Code, rec.Body.String())
	}
	if rec := hs.do("POST", "/v1/speech", `{"text":"Hi"}`, "If-None-Match", "*"); rec.Code != 304 {
		t.Fatal("wildcard If-None-Match")
	}
}

func TestSpeechValidation(t *testing.T) {
	hs := newHarness(t)
	cases := map[string]struct {
		body   string
		status int
		code   string
	}{
		"bad json":    {`{`, 400, codeBadRequest},
		"unknown key": {`{"text":"a","pitch":3}`, 400, codeBadRequest},
		"two objects": {`{"text":"a"}{}`, 400, codeBadRequest},
		"empty text":  {`{"text":"  "}`, 400, codeBadRequest},
		"too long":    {`{"text":"` + strings.Repeat("a", 51) + `"}`, 400, codeTextTooLong},
		"bad lang":    {`{"text":"a","lang":"!!"}`, 400, codeUnknownLang},
		"no lang":     {`{"text":"a","lang":"fr"}`, 400, codeUnknownLang},
		"no voice":    {`{"text":"a","voice":"nope"}`, 400, codeUnknownVoice},
		"speed":       {`{"text":"a","speed":0}`, 400, codeBadRequest},
		"format":      {`{"text":"a","format":"ogg"}`, 400, codeBadRequest},
		"body size":   {`{"text":"` + strings.Repeat("a", 5000) + `"}`, 413, codeTooLarge},
	}
	for name, c := range cases {
		rec := hs.do("POST", "/v1/speech", c.body)
		if rec.Code != c.status || errCode(t, rec.Body.String()) != c.code {
			t.Errorf("%s: %d %s", name, rec.Code, rec.Body.String())
		}
	}
}

func TestSpeechEngineErrors(t *testing.T) {
	hs := newHarness(t)
	hs.engine.err = errBoom
	if rec := hs.do("POST", "/v1/speech", `{"text":"a"}`); rec.Code != 500 || errCode(t, rec.Body.String()) != codeEngineFailure {
		t.Fatal(rec.Code)
	}
	hs.engine.err = nil
	hs.engine.block = true
	hs.deps.Synth.Timeout = 1
	if rec := hs.do("POST", "/v1/speech", `{"text":"b"}`); rec.Code != 504 {
		t.Fatal(rec.Code)
	}
	delete(hs.deps.Synth.Engines, "espeak")
	if rec := hs.do("POST", "/v1/speech", `{"text":"c","voice":"espeak:en"}`); rec.Code != 503 || errCode(t, rec.Body.String()) != codeNotReady {
		t.Fatal(rec.Code)
	}
}

func TestSpeechBusyAndCancel(t *testing.T) {
	hs := newHarness(t)
	release, err := hs.deps.Synth.Gate.Acquire(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	rec := hs.do("POST", "/v1/speech", `{"text":"a"}`)
	if rec.Code != 503 || rec.Header().Get("Retry-After") == "" || errCode(t, rec.Body.String()) != codeBusy {
		t.Fatal(rec.Code)
	}
	release()
	hs.engine.block = true
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	req := httptest.NewRequestWithContext(ctx, "POST", "/v1/speech", strings.NewReader(`{"text":"z"}`))
	_ = hs.send(req) // client gone: nothing useful to assert beyond not panicking
	_ = limit.ErrBusy
}

func TestETagMatches(t *testing.T) {
	if (&reqError{msg: "m"}).Error() != "m" {
		t.Fatal("reqError")
	}
	if etagMatches("", `"a"`) || !etagMatches(`"b", "a"`, `"a"`) {
		t.Fatal("etagMatches")
	}
}
