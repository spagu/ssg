package config

// mcp.oauth (#362).

import "testing"

// TestMCPOAuthEnabled: either URL set means OAuth was meant, so a half-done
// config fails validation at start instead of silently leaving OAuth off.
func TestMCPOAuthEnabled(t *testing.T) {
	if (MCPOAuth{}).Enabled() {
		t.Error("empty is off")
	}
	if !(MCPOAuth{Resource: "https://mcp.example.com/mcp"}).Enabled() {
		t.Error("a resource alone is meant as OAuth")
	}
}
