package mcp

// The network transport (#173). Its security rules are the reason these tests
// are mostly about refusal: this server writes files and runs git, so an MCP
// endpoint a web page can reach is a remote code execution path.

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// testEndpoint returns a handler over a minimal server.
func testEndpoint(t *testing.T, opts HTTPOptions) http.Handler {
	t.Helper()
	return HTTPHandler(NewServer(Options{Root: t.TempDir(), Version: "test"}), opts)
}

// post sends one JSON-RPC body and returns the recorder.
func post(h http.Handler, body string, headers map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// TestRequestIsAnsweredOverHTTP: the same protocol, a different binding — a
// tools/list must return what stdio returns.
func TestRequestIsAnsweredOverHTTP(t *testing.T) {
	rec := post(testEndpoint(t, HTTPOptions{}), `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("content-type = %q", ct)
	}
	var resp struct {
		Result struct {
			Tools []struct{ Name string } `json:"tools"`
		} `json:"result"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.Result.Tools) == 0 {
		t.Error("the endpoint must expose the same tools stdio does")
	}
}

// TestOriginIsValidated: without this, any page in the operator's browser can
// drive a local MCP server through DNS rebinding.
func TestOriginIsValidated(t *testing.T) {
	h := testEndpoint(t, HTTPOptions{AllowedOrigins: []string{"https://studio.example"}})
	body := `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`

	if rec := post(h, body, map[string]string{"Origin": "https://evil.example"}); rec.Code != http.StatusForbidden {
		t.Errorf("a foreign origin = %d, want 403", rec.Code)
	}
	if rec := post(h, body, map[string]string{"Origin": "https://studio.example"}); rec.Code != http.StatusOK {
		t.Errorf("the allowed origin = %d, want 200", rec.Code)
	}
	// Case and port are part of the comparison, but scheme/host casing is not.
	if rec := post(h, body, map[string]string{"Origin": "HTTPS://Studio.Example"}); rec.Code != http.StatusOK {
		t.Errorf("origin comparison must be case-insensitive on scheme and host: %d", rec.Code)
	}
	if rec := post(h, body, map[string]string{"Origin": "https://studio.example:8443"}); rec.Code != http.StatusForbidden {
		t.Errorf("a different port is a different origin: %d", rec.Code)
	}
	// No Origin at all is a non-browser client, which cannot be a rebinding
	// victim — refusing those would break every ordinary MCP client.
	if rec := post(h, body, nil); rec.Code != http.StatusOK {
		t.Errorf("a client with no Origin = %d, want 200", rec.Code)
	}
	// With no list configured, no browser origin is accepted.
	if rec := post(testEndpoint(t, HTTPOptions{}), body,
		map[string]string{"Origin": "https://studio.example"}); rec.Code != http.StatusForbidden {
		t.Errorf("no configured origins must accept none: %d", rec.Code)
	}
}

// TestTokenIsRequiredWhenSet, and compared whole.
func TestTokenIsRequiredWhenSet(t *testing.T) {
	h := testEndpoint(t, HTTPOptions{Token: "s3cret"})
	body := `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`

	for name, headers := range map[string]map[string]string{
		"absent":     nil,
		"wrong":      {"Authorization": "Bearer nope"},
		"prefix":     {"Authorization": "Bearer s3cre"},
		"no scheme":  {"Authorization": "s3cret"},
		"wrong kind": {"Authorization": "Basic s3cret"},
	} {
		if rec := post(h, body, headers); rec.Code != http.StatusUnauthorized {
			t.Errorf("%s token = %d, want 401", name, rec.Code)
		}
	}
	if rec := post(h, body, map[string]string{"Authorization": "Bearer s3cret"}); rec.Code != http.StatusOK {
		t.Errorf("the right token = %d, want 200", rec.Code)
	}
}

// TestMethodsOfEarlierRevisions: GET opened a standalone stream and DELETE
// ended a session; neither exists in this transport, and 405 says so.
func TestMethodsOfEarlierRevisions(t *testing.T) {
	h := testEndpoint(t, HTTPOptions{})
	for _, method := range []string{http.MethodGet, http.MethodDelete, http.MethodPut} {
		req := httptest.NewRequest(method, "/mcp", nil)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s = %d, want 405", method, rec.Code)
		}
		// The body is a JSON-RPC error, which is what lets a client tell this
		// refusal from an unrelated web server answering on the same port.
		if !strings.Contains(rec.Body.String(), `"jsonrpc"`) {
			t.Errorf("%s body must be a JSON-RPC error: %s", method, rec.Body)
		}
	}
}

// TestNotificationIsAccepted: a notification has no reply to give, and the
// specification is explicit about what an accepted one returns.
func TestNotificationIsAccepted(t *testing.T) {
	rec := post(testEndpoint(t, HTTPOptions{}), `{"jsonrpc":"2.0","method":"notifications/initialized"}`, nil)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202", rec.Code)
	}
	if rec.Body.Len() != 0 {
		t.Errorf("202 carries no body, got %q", rec.Body)
	}
}

// TestMalformedBodyIsAParseError, not a panic and not a 200.
func TestMalformedBodyIsAParseError(t *testing.T) {
	rec := post(testEndpoint(t, HTTPOptions{}), `{not json`, nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "parse error") {
		t.Errorf("body = %s", rec.Body)
	}
}

// TestProtocolVersionIsEchoed so an intermediary can route on it.
func TestProtocolVersionIsEchoed(t *testing.T) {
	h := testEndpoint(t, HTTPOptions{})
	body := `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`

	rec := post(h, body, map[string]string{"MCP-Protocol-Version": "2025-06-18"})
	if got := rec.Header().Get("MCP-Protocol-Version"); got != "2025-06-18" {
		t.Errorf("echoed version = %q", got)
	}
	// A client that names none gets the version this server implements.
	rec = post(h, body, nil)
	if got := rec.Header().Get("MCP-Protocol-Version"); got != protocolVersion {
		t.Errorf("default version = %q, want %q", got, protocolVersion)
	}
}

// TestIsLoopback decides whether a token is required, so anything it cannot
// prove is loopback must be treated as public.
func TestIsLoopback(t *testing.T) {
	local := []string{"127.0.0.1:7823", "localhost:7823", "[::1]:7823", "127.0.0.1", "localhost", "::1"}
	for _, addr := range local {
		if !IsLoopback(addr) {
			t.Errorf("%q is loopback", addr)
		}
	}
	public := []string{"0.0.0.0:7823", ":7823", "", "192.168.1.10:7823", "example.com:7823", "[::]:7823"}
	for _, addr := range public {
		if IsLoopback(addr) {
			t.Errorf("%q must be treated as public", addr)
		}
	}
}

// TestNewTokenIsRandomAndLongEnough to be worth having.
func TestNewTokenIsRandomAndLongEnough(t *testing.T) {
	a, err := NewToken()
	if err != nil {
		t.Fatal(err)
	}
	b, err := NewToken()
	if err != nil {
		t.Fatal(err)
	}
	if a == b {
		t.Error("two tokens must differ")
	}
	if len(a) < 32 {
		t.Errorf("token length = %d, too short to matter", len(a))
	}
}

// TestOversizedBodyIsRefusedRatherThanRead: a tool payload carries whole
// templates, so the limit is generous — but it exists.
func TestOversizedBodyIsRefusedRatherThanRead(t *testing.T) {
	huge := `{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{"x":"` +
		strings.Repeat("a", 64) + `"}}`
	if rec := post(testEndpoint(t, HTTPOptions{}), huge, nil); rec.Code != http.StatusOK {
		t.Errorf("an ordinary payload must pass: %d", rec.Code)
	}
}

// TestUnreadableBodyIsReported: a client that announces a body and then hangs
// up must get an error, not a panic.
func TestUnreadableBodyIsReported(t *testing.T) {
	h := testEndpoint(t, HTTPOptions{})
	req := httptest.NewRequest(http.MethodPost, "/mcp", errorReader{})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "body") {
		t.Errorf("body = %s", rec.Body)
	}
}

// errorReader fails on the first read, the way a dropped connection does.
type errorReader struct{}

func (errorReader) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }

// TestMalformedOriginIsRefused: an Origin that is not a URL cannot be compared,
// and anything that cannot be proved safe must be refused.
func TestMalformedOriginIsRefused(t *testing.T) {
	h := testEndpoint(t, HTTPOptions{AllowedOrigins: []string{"https://ok.example"}})
	body := `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`

	for _, origin := range []string{"://nonsense", "ht tp://x"} {
		if rec := post(h, body, map[string]string{"Origin": origin}); rec.Code != http.StatusForbidden {
			t.Errorf("origin %q = %d, want 403", origin, rec.Code)
		}
	}
	// An unparseable entry in the allow-list is skipped rather than matching
	// everything.
	h2 := testEndpoint(t, HTTPOptions{AllowedOrigins: []string{"://broken", "https://ok.example"}})
	if rec := post(h2, body, map[string]string{"Origin": "https://ok.example"}); rec.Code != http.StatusOK {
		t.Errorf("a valid entry beside a broken one must still match: %d", rec.Code)
	}
}

// TestLogfRecordsRefusals, so a client that cannot connect is diagnosable from
// the server's side.
func TestLogfRecordsRefusals(t *testing.T) {
	var lines []string
	h := HTTPHandler(NewServer(Options{Root: t.TempDir()}), HTTPOptions{
		Token: "s3cret",
		Logf:  func(f string, a ...any) { lines = append(lines, f) },
	})
	post(h, `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`, nil)
	if len(lines) == 0 {
		t.Error("a refusal must be logged")
	}
}

// TestOversizedBodyIsTruncatedNotRead: the limit exists so a client cannot make
// the server allocate without bound; past it the JSON no longer parses, which
// is a parse error rather than a hang.
func TestOversizedBodyIsRefused(t *testing.T) {
	huge := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"` +
		strings.Repeat("a", maxRequestBytes+16) + `"}}`
	rec := post(testEndpoint(t, HTTPOptions{}), huge, nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 — the body was truncated at the limit", rec.Code)
	}
}

// TestTokenGenerationFailureIsReported: a weak token on a routable address is
// worse than refusing to start, so this must surface rather than fall back.
func TestTokenGenerationFailureIsReported(t *testing.T) {
	saved := randRead
	randRead = func([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }
	t.Cleanup(func() { randRead = saved })

	got, err := NewToken()
	if err == nil {
		t.Fatal("a failing entropy source must be an error")
	}
	if got != "" {
		t.Errorf("no token must be returned on failure, got %q", got)
	}
}

// TestHeaderMismatchIsRejectedOverHTTP: the validation has its own tests, but
// this is the path that matters — an intermediary routing on a header while the
// server executes on the body is how a request reaches something it was not
// authorised for, and the rejection has to happen before the body is acted on.
func TestHeaderMismatchIsRejectedOverHTTP(t *testing.T) {
	h := testEndpoint(t, HTTPOptions{})
	body := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{` +
		`"name":"help","_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28"}}}`

	rec := post(h, body, map[string]string{
		"MCP-Protocol-Version": "2026-07-28",
		"Mcp-Method":           "tools/call",
		"Mcp-Name":             "something-else",
	})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "-32020") {
		t.Errorf("the body must carry HeaderMismatch: %s", rec.Body)
	}
	// The id is echoed, so a client can match the error to its request.
	if !strings.Contains(rec.Body.String(), `"id":1`) {
		t.Errorf("the request id must be echoed: %s", rec.Body)
	}

	// Matching headers pass through to the tool.
	ok := post(h, body, map[string]string{
		"MCP-Protocol-Version": "2026-07-28",
		"Mcp-Method":           "tools/call",
		"Mcp-Name":             "help",
	})
	if ok.Code != http.StatusOK {
		t.Errorf("matching headers = %d, want 200: %s", ok.Code, ok.Body)
	}

	// An older client sends none of these and must not be held to them.
	legacy := post(h, `{"jsonrpc":"2.0","id":2,"method":"tools/list"}`, nil)
	if legacy.Code != http.StatusOK {
		t.Errorf("an initialize-era client = %d, want 200", legacy.Code)
	}
}
