package mcp

// OAuth 2.1 for the Streamable HTTP transport (#362).
//
// The MCP authorization specification builds on OAuth 2.1, and a client that
// only offers OAuth sign-in for a remote server has no box to type a static
// token into. ssg is the resource server in that picture and nothing more:
// tokens come from an authorization server the operator already runs
// (Keycloak, Auth0, Cloudflare Access, …). What the resource server owes the
// client is:
//
//   - Protected Resource Metadata (RFC 9728), naming the authorization server;
//   - a 401 whose WWW-Authenticate header points at that metadata;
//   - access tokens checked for signature, issuer, lifetime, an audience that
//     names this server (RFC 8707 — a token minted for another service must not
//     work here) and the scopes it requires.

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// protectedResourcePath is the well-known path RFC 9728 defines.
const protectedResourcePath = "/.well-known/oauth-protected-resource"

// OAuthOptions makes the endpoint an OAuth 2.1 resource server.
type OAuthOptions struct {
	// Issuer is the authorization server. A token's `iss` must equal it.
	Issuer string
	// Resource is this server's canonical URI, the one clients reach it by,
	// e.g. https://mcp.example.com/mcp. A token's `aud` must name it.
	Resource string
	// Scopes a token must carry, also advertised in the metadata. Empty means
	// any token from the issuer for this resource is enough.
	Scopes []string
	// JWKSURL is where the issuer's signing keys are. Empty discovers it from
	// the issuer's own metadata (RFC 8414, then OpenID Connect discovery).
	JWKSURL string
	// HTTPClient fetches metadata and keys. Nil uses a client with a timeout.
	HTTPClient *http.Client
}

// Validate rejects options that cannot protect anything: both URLs absolute,
// and https unless they point at this machine.
func (o OAuthOptions) Validate() error {
	for name, raw := range map[string]string{"issuer": o.Issuer, "resource": o.Resource} {
		u, err := url.Parse(strings.TrimSpace(raw))
		if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") {
			return fmt.Errorf("mcp.oauth.%s must be an absolute URL, got %q", name, raw)
		}
		if u.Scheme == "http" && !IsLoopback(u.Host) {
			return fmt.Errorf("mcp.oauth.%s must use https, got %q", name, raw)
		}
		if u.Fragment != "" {
			return fmt.Errorf("mcp.oauth.%s must not have a fragment, got %q", name, raw)
		}
	}
	return nil
}

// MetadataPaths are the paths the metadata is served at: the one RFC 9728
// derives from the resource's path, and the bare well-known path a client
// tries when it only knows the host.
func (o OAuthOptions) MetadataPaths() []string {
	u, err := url.Parse(o.Resource)
	if err != nil {
		return []string{protectedResourcePath}
	}
	if p := strings.TrimSuffix(u.Path, "/"); p != "" {
		return []string{protectedResourcePath + p, protectedResourcePath}
	}
	return []string{protectedResourcePath}
}

// MetadataURL is the absolute address of the metadata, as the
// WWW-Authenticate challenge names it.
func (o OAuthOptions) MetadataURL() string {
	u, err := url.Parse(o.Resource)
	if err != nil {
		return ""
	}
	return u.Scheme + "://" + u.Host + o.MetadataPaths()[0]
}

// challenge is the WWW-Authenticate value for a refused request. errCode is
// "" when no token was sent, which RFC 6750 answers without an error code.
func (o OAuthOptions) challenge(errCode string) string {
	parts := []string{`resource_metadata="` + o.MetadataURL() + `"`}
	if len(o.Scopes) > 0 {
		parts = append(parts, `scope="`+strings.Join(o.Scopes, " ")+`"`)
	}
	if errCode != "" {
		parts = append(parts, `error="`+errCode+`"`)
	}
	return "Bearer " + strings.Join(parts, ", ")
}

// ProtectedResourceHandler serves the RFC 9728 metadata document.
func ProtectedResourceHandler(o OAuthOptions) http.Handler {
	doc := map[string]any{
		"resource":                 o.Resource,
		"authorization_servers":    []string{o.Issuer},
		"bearer_methods_supported": []string{"header"},
		"resource_name":            "ssg mcp",
	}
	if len(o.Scopes) > 0 {
		doc["scopes_supported"] = o.Scopes
	}
	body, _ := json.Marshal(doc)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		// Public by definition: a browser-based client reads it cross-origin.
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	})
}
