package mcp

// The 2026-07-28 shape beside the initialize-based one (#174).
//
// The tests that matter most are the coexistence ones: this revision changed
// the protocol rather than extending it, and a server that adopts it by
// abandoning the older era strands every client that has not moved.

import (
	"encoding/base64"
	"encoding/json"
	"testing"
)

// modernRequest builds a request in the stateless shape.
func modernRequest(id int, method, version string, params map[string]any) rpcRequest {
	if params == nil {
		params = map[string]any{}
	}
	params["_meta"] = map[string]any{metaProtocolVersion: version}
	raw, _ := json.Marshal(params)
	return rpcRequest{JSONRPC: "2.0", ID: json.RawMessage(itoa(id)), Method: method, Params: raw}
}

func itoa(i int) string { return string(rune('0' + i)) }

// resultMap decodes a response result into a map.
func resultMap(t *testing.T, resp rpcResponse) map[string]any {
	t.Helper()
	raw, err := json.Marshal(resp.Result)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	return m
}

func testServer(t *testing.T) *Server {
	t.Helper()
	return NewServer(Options{Root: t.TempDir(), Version: "test"})
}

// TestServerDiscoverAnswersInBothEras: it is mandatory in the modern shape, and
// on stdio it is how a client probes which era a server implements — so it must
// answer whether or not the request declared a version.
func TestServerDiscoverAnswersInBothEras(t *testing.T) {
	s := testServer(t)

	for _, req := range []rpcRequest{
		{JSONRPC: "2.0", ID: json.RawMessage("1"), Method: "server/discover"},
		modernRequest(2, "server/discover", modernVersion, nil),
	} {
		resp, respond := s.handle(req)
		if !respond || resp.Error != nil {
			t.Fatalf("server/discover = %+v", resp)
		}
		m := resultMap(t, resp)
		if m["resultType"] != "complete" {
			t.Errorf("resultType = %v", m["resultType"])
		}
		versions, _ := m["protocolVersions"].([]any)
		if len(versions) == 0 || versions[0] != modernVersion {
			t.Errorf("protocolVersions = %v, want the newest first", versions)
		}
		info, _ := m["serverInfo"].(map[string]any)
		if info["name"] != "ssg" {
			t.Errorf("serverInfo = %v", info)
		}
	}
}

// TestModernResultsCarryTheEnvelope: every result needs a resultType, and every
// list result needs the freshness hints that let a client cache instead of poll.
func TestModernResultsCarryTheEnvelope(t *testing.T) {
	s := testServer(t)

	resp, _ := s.handle(modernRequest(1, "tools/list", modernVersion, nil))
	m := resultMap(t, resp)
	if m["resultType"] != "complete" {
		t.Errorf("resultType = %v", m["resultType"])
	}
	if m["ttlMs"] == nil || m["cacheScope"] != "private" {
		t.Errorf("a list result needs cache hints: %v", m)
	}
	if tools, _ := m["tools"].([]any); len(tools) == 0 {
		t.Error("the tools must still be there")
	}
	// The server identifies itself in every result's _meta.
	meta, _ := m["_meta"].(map[string]any)
	if meta[metaServerInfo] == nil {
		t.Errorf("_meta = %v, want serverInfo", meta)
	}

	// A tools/call result carries the envelope too.
	resp, _ = s.handle(modernRequest(2, "tools/call", modernVersion,
		map[string]any{"name": "help", "arguments": map[string]any{}}))
	m = resultMap(t, resp)
	if m["resultType"] != "complete" {
		t.Errorf("tools/call resultType = %v", m["resultType"])
	}
	if m["content"] == nil {
		t.Errorf("tools/call must still return content: %v", m)
	}
}

// TestOlderClientsAreUnchanged: the whole compatibility claim. A request with
// no _meta must be answered exactly as it was before this existed.
func TestOlderClientsAreUnchanged(t *testing.T) {
	s := testServer(t)

	resp, _ := s.handle(rpcRequest{
		JSONRPC: "2.0", ID: json.RawMessage("1"), Method: "initialize",
		Params: json.RawMessage(`{"protocolVersion":"2024-11-05"}`),
	})
	m := resultMap(t, resp)
	if m["protocolVersion"] != "2024-11-05" {
		t.Errorf("the client's version must be echoed: %v", m["protocolVersion"])
	}
	if m["resultType"] != nil {
		t.Errorf("an initialize-era result must not grow a resultType: %v", m)
	}

	// And its tools/list is the plain shape, with no cache hints bolted on.
	resp, _ = s.handle(rpcRequest{JSONRPC: "2.0", ID: json.RawMessage("2"), Method: "tools/list"})
	m = resultMap(t, resp)
	if m["ttlMs"] != nil || m["resultType"] != nil {
		t.Errorf("the older shape must stay as it was: %v", m)
	}
	if tools, _ := m["tools"].([]any); len(tools) == 0 {
		t.Error("the tools must be there in either era")
	}
}

// TestUnsupportedVersionListsWhatIsSupported, so a client can retry rather than
// guess.
func TestUnsupportedVersionListsWhatIsSupported(t *testing.T) {
	s := testServer(t)
	resp, _ := s.handle(modernRequest(1, "tools/list", "1999-01-01", nil))

	if resp.Error == nil || resp.Error.Code != codeUnsupportedProtocolVersion {
		t.Fatalf("error = %+v, want -32022", resp.Error)
	}
	data, _ := resp.Error.Data.(map[string]any)
	supported, _ := data["supported"].([]string)
	if len(supported) == 0 {
		t.Errorf("the error must list what is supported: %+v", resp.Error.Data)
	}
	// Both revisions this server speaks are accepted.
	for _, v := range supportedVersions {
		if resp, _ := s.handle(modernRequest(2, "tools/list", v, nil)); resp.Error != nil {
			t.Errorf("%s must be accepted: %+v", v, resp.Error)
		}
	}
}

// TestModernNotificationGetsNoResponse, the same rule as the older era.
func TestModernNotificationGetsNoResponse(t *testing.T) {
	req := modernRequest(0, "notifications/whatever", modernVersion, nil)
	req.ID = nil
	if _, respond := testServer(t).handle(req); respond {
		t.Error("a notification receives no response")
	}
}

// TestUnknownModernMethodIsMethodNotFound.
func TestUnknownModernMethodIsMethodNotFound(t *testing.T) {
	resp, _ := testServer(t).handle(modernRequest(1, "nope/nope", modernVersion, nil))
	if resp.Error == nil || resp.Error.Code != codeMethodNotFound {
		t.Fatalf("error = %+v", resp.Error)
	}
}

// TestMetaDetection: what puts a request in one era or the other.
func TestMetaDetection(t *testing.T) {
	if metaOf(nil).isModern() {
		t.Error("no params is not modern")
	}
	if metaOf(json.RawMessage(`{}`)).isModern() {
		t.Error("no _meta is not modern")
	}
	if metaOf(json.RawMessage(`{"_meta":{}}`)).isModern() {
		t.Error("an empty _meta declares no version")
	}
	if metaOf(json.RawMessage(`not json`)).isModern() {
		t.Error("unparseable params must not be mistaken for modern")
	}
	m := metaOf(json.RawMessage(`{"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28"}}`))
	if !m.isModern() || m.ProtocolVersion != modernVersion {
		t.Errorf("meta = %+v", m)
	}
}

// TestHeaderValidation: an intermediary routing on the header while the server
// executes on the body is how a request ends up somewhere it was not
// authorised for, so the two must agree.
func TestHeaderValidation(t *testing.T) {
	call := rpcRequest{
		Method: "tools/call",
		Params: json.RawMessage(`{"name":"help"}`),
	}

	if bad := validateHeaders(call, "tools/call", "help"); bad != nil {
		t.Errorf("matching headers must pass: %s", bad.reason)
	}
	if bad := validateHeaders(call, "", "help"); bad == nil {
		t.Error("a missing Mcp-Method must be rejected")
	}
	if bad := validateHeaders(call, "tools/list", "help"); bad == nil {
		t.Error("a method that disagrees with the body must be rejected")
	}
	if bad := validateHeaders(call, "tools/call", "other"); bad == nil {
		t.Error("a name that disagrees with the body must be rejected")
	}
	if bad := validateHeaders(call, "tools/call", ""); bad == nil {
		t.Error("tools/call requires a name header")
	}

	// A method that mirrors no name needs none.
	list := rpcRequest{Method: "tools/list"}
	if bad := validateHeaders(list, "tools/list", ""); bad != nil {
		t.Errorf("tools/list mirrors no name: %s", bad.reason)
	}

	// resources/read mirrors the URI instead.
	read := rpcRequest{Method: "resources/read", Params: json.RawMessage(`{"uri":"file:///a"}`)}
	if bad := validateHeaders(read, "resources/read", "file:///a"); bad != nil {
		t.Errorf("a URI must be accepted: %s", bad.reason)
	}
	if bad := validateHeaders(read, "resources/read", "file:///b"); bad == nil {
		t.Error("a URI that disagrees must be rejected")
	}
}

// TestBase64SentinelIsDecodedBeforeComparing: without this, every tool with a
// non-ASCII name would look like an attack.
func TestBase64SentinelIsDecodedBeforeComparing(t *testing.T) {
	name := "Zażółć gęślą"
	encoded := b64Prefix + base64.StdEncoding.EncodeToString([]byte(name)) + b64Suffix

	got, ok := decodeHeaderValue(encoded)
	if !ok || got != name {
		t.Fatalf("decodeHeaderValue = %q, %v", got, ok)
	}

	params, err := json.Marshal(map[string]string{"name": name})
	if err != nil {
		t.Fatal(err)
	}
	call := rpcRequest{Method: "tools/call", Params: params}
	if bad := validateHeaders(call, "tools/call", encoded); bad != nil {
		t.Errorf("an encoded name must be decoded before comparing: %s", bad.reason)
	}

	// A plain value passes through untouched.
	if got, ok := decodeHeaderValue("plain"); !ok || got != "plain" {
		t.Errorf("plain value = %q, %v", got, ok)
	}
	// A malformed sentinel is rejected rather than compared as text: silently
	// treating it as literal would compare garbage against the body and reject
	// a request for the wrong reason.
	if _, ok := decodeHeaderValue(b64Prefix + "!!!not base64!!!" + b64Suffix); ok {
		t.Error("invalid base64 in the sentinel must be rejected")
	}
}

// TestHeaderSubject names what each method mirrors.
func TestHeaderSubject(t *testing.T) {
	cases := []struct{ method, params, want string }{
		{"tools/call", `{"name":"a"}`, "a"},
		{"prompts/get", `{"name":"p"}`, "p"},
		{"resources/read", `{"uri":"u"}`, "u"},
		{"tools/list", `{}`, ""},
		{"server/discover", ``, ""},
		{"tools/call", `broken`, ""},
	}
	for _, c := range cases {
		if got := headerSubject(c.method, json.RawMessage(c.params)); got != c.want {
			t.Errorf("headerSubject(%s, %s) = %q, want %q", c.method, c.params, got, c.want)
		}
	}
}
