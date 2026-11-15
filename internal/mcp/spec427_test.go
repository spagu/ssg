package mcp

// #427: three answers brought in line with the 2026-07-28 revision.

import (
	"net/http"
	"slices"
	"testing"
)

// The discover result names its list supportedVersions, as the specification
// does, and still carries the pre-#427 name for one release.
func TestDiscoverNamesSupportedVersions(t *testing.T) {
	resp, _ := testServer(t).handle(modernRequest(1, "server/discover", modernVersion, nil))
	m := resultMap(t, resp)
	for _, key := range []string{"supportedVersions", "protocolVersions"} {
		list, _ := m[key].([]any)
		if len(list) != len(supportedVersions) {
			t.Errorf("%s = %v, want %v", key, m[key], supportedVersions)
		}
	}
}

// The unsupported-version error tells the client what it sent.
func TestUnsupportedVersionEchoesTheRequest(t *testing.T) {
	resp, _ := testServer(t).handle(modernRequest(1, "tools/list", "1999-01-01", nil))
	data, _ := resp.Error.Data.(map[string]any)
	if data["requested"] != "1999-01-01" {
		t.Errorf("data = %+v, want requested 1999-01-01", data)
	}
	if !slices.Equal(data["supported"].([]string), supportedVersions) {
		t.Errorf("supported = %v", data["supported"])
	}
}

// Over HTTP a modern unknown method is 404 and an unsupported version 400;
// other errors and every older-era reply stay 200.
func TestModernErrorsCarryTheirHTTPStatus(t *testing.T) {
	h := testEndpoint(t, HTTPOptions{})
	cases := []struct {
		name, method, version string
		want                  int
	}{
		{"unknown method", "nope/nope", "2026-07-28", http.StatusNotFound},
		{"unsupported version", "tools/list", "1999-01-01", http.StatusBadRequest},
		{"a tool error is a result", "tools/call", "2026-07-28", http.StatusOK},
	}
	for _, c := range cases {
		body := `{"jsonrpc":"2.0","id":1,"method":"` + c.method + `","params":{` +
			`"name":"no-such-tool","_meta":{"io.modelcontextprotocol/protocolVersion":"` + c.version + `"}}}`
		headers := map[string]string{"MCP-Protocol-Version": c.version, "Mcp-Method": c.method}
		if c.method == "tools/call" {
			headers["Mcp-Name"] = "no-such-tool"
		}
		rec := post(h, body, headers)
		if rec.Code != c.want {
			t.Errorf("%s: status = %d, want %d: %s", c.name, rec.Code, c.want, rec.Body)
		}
	}
	legacy := post(h, `{"jsonrpc":"2.0","id":2,"method":"nope/nope"}`, nil)
	if legacy.Code != http.StatusOK {
		t.Errorf("an initialize-era unknown method = %d, want 200 as before", legacy.Code)
	}
}

// modernStatus leaves a successful reply and an error it does not map at 200.
func TestModernStatusDefaults(t *testing.T) {
	req := modernRequest(1, "tools/list", modernVersion, nil)
	if got := modernStatus(req, rpcResponse{}); got != http.StatusOK {
		t.Errorf("no error = %d", got)
	}
	if got := modernStatus(req, rpcResponse{Error: &rpcError{Code: codeInvalidRequest}}); got != http.StatusOK {
		t.Errorf("unmapped error = %d", got)
	}
}

// An initialize carrying modern _meta is answered (a client hedging between
// eras), and a server built without a version calls itself "dev".
func TestModernInitializeAndDevVersion(t *testing.T) {
	s := NewServer(Options{Root: t.TempDir()})
	resp, _ := s.handle(modernRequest(1, "initialize", modernVersion, map[string]any{"protocolVersion": "2025-06-18"}))
	if resp.Error != nil || resultMap(t, resp)["protocolVersion"] != "2025-06-18" {
		t.Errorf("initialize = %+v", resp)
	}
	if v := s.serverInfo()["version"]; v != "dev" {
		t.Errorf("version = %v, want dev", v)
	}
}
