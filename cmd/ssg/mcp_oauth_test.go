package main

// mcp.oauth through the CLI (#362).

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/spagu/ssg/internal/config"
	"github.com/spagu/ssg/internal/mcp"
)

func TestMCPOAuthOf(t *testing.T) {
	if got := mcpOAuthOf(&config.Config{}); got != nil {
		t.Errorf("no oauth = %+v", got)
	}
	cfg := &config.Config{MCP: config.MCPConfig{OAuth: config.MCPOAuth{
		Issuer: "https://auth.example.com", Resource: "https://mcp.example.com/mcp",
		Scopes: []string{"ssg.mcp"}, JWKSURL: "https://auth.example.com/keys",
	}}}
	got := mcpOAuthOf(cfg)
	if got == nil || got.Issuer != "https://auth.example.com" || got.Resource != "https://mcp.example.com/mcp" ||
		got.Scopes[0] != "ssg.mcp" || got.JWKSURL != "https://auth.example.com/keys" {
		t.Errorf("got %+v", got)
	}
}

// TestOAuthEndpointMintsNoToken: with OAuth a static token exists only when
// the operator gave one.
func TestOAuthEndpointMintsNoToken(t *testing.T) {
	oauth := &mcp.OAuthOptions{Issuer: "https://auth.example.com", Resource: "http://127.0.0.1:7823/mcp"}
	t.Setenv(mcpTokenEnv, "")
	if tok, minted, err := endpointToken(mcpNetFlags{oauth: oauth}); tok != "" || minted || err != nil {
		t.Errorf("nothing given: %q %v %v", tok, minted, err)
	}
	if tok, _, _ := endpointToken(mcpNetFlags{oauth: oauth, token: " flag "}); tok != "flag" {
		t.Errorf("flag = %q", tok)
	}
	t.Setenv(mcpTokenEnv, "from-env")
	if tok, _, _ := endpointToken(mcpNetFlags{oauth: oauth}); tok != "from-env" {
		t.Errorf("env = %q", tok)
	}
	if tok, minted, _ := endpointToken(mcpNetFlags{token: "x"}); tok != "x" || minted {
		t.Errorf("without oauth the old rules apply: %q %v", tok, minted)
	}
}

// TestOAuthEndpointServesItsMetadata, and refuses to start on options that
// cannot protect anything.
func TestOAuthEndpointServesItsMetadata(t *testing.T) {
	t.Setenv(mcpTokenEnv, "")
	bad := &mcp.OAuthOptions{Issuer: "https://auth.example.com", Resource: "http://mcp.example.com/mcp"}
	if code := serveMCPEndpoint(testMCPServer(), mcpNetFlags{listen: "127.0.0.1:0", oauth: bad}, func(string, ...any) {}); code != 2 {
		t.Errorf("an http resource off this machine: exit %d, want 2", code)
	}

	oauth := &mcp.OAuthOptions{Issuer: "https://auth.example.com", Resource: "http://localhost:7823/mcp"}
	var lines []string
	var addr string
	code := serveMCPEndpoint(testMCPServer(), mcpNetFlags{listen: "127.0.0.1:0", oauth: oauth}, func(f string, a ...any) {
		line := sprintfLine(f, a...)
		lines = append(lines, line)
		if i := strings.Index(line, "http://"); i >= 0 && strings.Contains(line, "MCP endpoint") {
			addr = strings.TrimSuffix(strings.Fields(line[i+len("http://"):])[0], "/mcp")
		}
	})
	if code != -1 || addr == "" {
		t.Fatalf("endpoint did not start: %d %v", code, lines)
	}
	out := strings.Join(lines, "\n")
	if !strings.Contains(out, "OAuth 2.1") || strings.Contains(out, "Authorization: Bearer") {
		t.Errorf("announce OAuth, and no static token nobody asked for:\n%s", out)
	}
	resp, err := http.Get("http://" + addr + "/.well-known/oauth-protected-resource/mcp")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(body), `"authorization_servers":["https://auth.example.com"]`) {
		t.Errorf("metadata: %d %s", resp.StatusCode, body)
	}
}
