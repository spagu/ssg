package mcp

// OAuth 2.1 resource server (#362): a fake authorization server signs real
// tokens, and the endpoint must take the right ones and refuse the rest with
// a challenge that tells the client where to sign in.

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	jose "github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
)

const testResource = "https://mcp.example.com/mcp"

// fakeIssuer is an authorization server: RFC 8414 metadata and a key set.
type fakeIssuer struct {
	srv       *httptest.Server
	issuer    string
	key       *rsa.PrivateKey
	jwksCalls atomic.Int32
	metaPath  string // where the metadata is served; "" serves none
}

func newFakeIssuer(t *testing.T, metaPath string) *fakeIssuer {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	f := &fakeIssuer{key: key, metaPath: metaPath}
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		switch {
		case f.metaPath != "" && r.URL.Path == f.metaPath:
			_ = json.NewEncoder(w).Encode(map[string]string{"issuer": f.issuer, "jwks_uri": f.srv.URL + "/keys"})
		case r.URL.Path == "/keys":
			f.jwksCalls.Add(1)
			_ = json.NewEncoder(w).Encode(jose.JSONWebKeySet{Keys: []jose.JSONWebKey{
				{Key: &key.PublicKey, KeyID: "k1", Algorithm: "RS256", Use: "sig"},
				{Key: &key.PublicKey, KeyID: "enc", Use: "enc"},
			}})
		default:
			http.NotFound(w, r)
		}
	})
	f.srv = httptest.NewServer(mux)
	t.Cleanup(f.srv.Close)
	f.issuer = f.srv.URL + "/realm"
	return f
}

func (f *fakeIssuer) options(scopes ...string) OAuthOptions {
	return OAuthOptions{Issuer: f.issuer, Resource: testResource, Scopes: scopes, HTTPClient: f.srv.Client()}
}

// token signs claims with the issuer's key under kid.
func (f *fakeIssuer) token(t *testing.T, kid string, claims jwt.Claims, extra map[string]any) string {
	t.Helper()
	return signToken(t, jose.SigningKey{Algorithm: jose.RS256, Key: f.key}, kid, claims, extra)
}

func signToken(t *testing.T, key jose.SigningKey, kid string, claims jwt.Claims, extra map[string]any) string {
	t.Helper()
	opts := (&jose.SignerOptions{}).WithType("at+jwt")
	if kid != "" {
		opts = opts.WithHeader("kid", kid)
	}
	signer, err := jose.NewSigner(key, opts)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := jwt.Signed(signer).Claims(claims).Claims(extra).Serialize()
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

// goodClaims are what a token for this resource carries.
func (f *fakeIssuer) goodClaims() jwt.Claims {
	now := time.Now()
	return jwt.Claims{
		Issuer: f.issuer, Subject: "editor", Audience: jwt.Audience{testResource},
		Expiry: jwt.NewNumericDate(now.Add(time.Hour)), IssuedAt: jwt.NewNumericDate(now),
	}
}

const listTools = `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`

func bearer(tok string) map[string]string { return map[string]string{"Authorization": "Bearer " + tok} }

// TestOAuthAcceptsTheIssuersTokens, and refuses everything else with the
// challenge RFC 6750 and RFC 9728 describe.
func TestOAuthAcceptsTheIssuersTokens(t *testing.T) {
	f := newFakeIssuer(t, "/.well-known/oauth-authorization-server/realm")
	o := f.options("ssg.mcp")
	h := testEndpoint(t, HTTPOptions{OAuth: &o, Token: "static-secret"})
	scoped := map[string]any{"scope": "openid ssg.mcp"}

	if rec := post(h, listTools, bearer(f.token(t, "k1", f.goodClaims(), scoped))); rec.Code != http.StatusOK {
		t.Fatalf("a good token: %d %s", rec.Code, rec.Body)
	}
	// The static token keeps working beside OAuth.
	if rec := post(h, listTools, bearer("static-secret")); rec.Code != http.StatusOK {
		t.Errorf("the static token: %d", rec.Code)
	}

	rec := post(h, listTools, nil)
	challenge := rec.Header().Get("WWW-Authenticate")
	if rec.Code != http.StatusUnauthorized ||
		challenge != `Bearer resource_metadata="https://mcp.example.com/.well-known/oauth-protected-resource/mcp", scope="ssg.mcp"` {
		t.Errorf("no token: %d %q", rec.Code, challenge)
	}

	wrongAud := f.goodClaims()
	wrongAud.Audience = jwt.Audience{"https://other.example.com"}
	wrongIss := f.goodClaims()
	wrongIss.Issuer = "https://evil.example.com"
	expired := f.goodClaims()
	expired.Expiry = jwt.NewNumericDate(time.Now().Add(-time.Hour))
	noExp := f.goodClaims()
	noExp.Expiry = nil
	for name, tok := range map[string]string{
		"a token for another resource": f.token(t, "k1", wrongAud, scoped),
		"another issuer":               f.token(t, "k1", wrongIss, scoped),
		"an expired token":             f.token(t, "k1", expired, scoped),
		"no expiry":                    f.token(t, "k1", noExp, scoped),
		"an unknown key":               f.token(t, "nope", f.goodClaims(), scoped),
		"the encryption key":           f.token(t, "enc", f.goodClaims(), scoped),
		"not a JWT":                    "garbage",
		"a shared-secret signature": signToken(t, jose.SigningKey{Algorithm: jose.HS256, Key: []byte(strings.Repeat("s", 32))},
			"k1", f.goodClaims(), scoped),
		"a foreign key": func() string {
			other, _ := rsa.GenerateKey(rand.Reader, 2048)
			return signToken(t, jose.SigningKey{Algorithm: jose.RS256, Key: other}, "k1", f.goodClaims(), scoped)
		}(),
	} {
		rec := post(h, listTools, bearer(tok))
		if rec.Code != http.StatusUnauthorized || !strings.Contains(rec.Header().Get("WWW-Authenticate"), `error="invalid_token"`) {
			t.Errorf("%s: %d %q", name, rec.Code, rec.Header().Get("WWW-Authenticate"))
		}
	}

	rec = post(h, listTools, bearer(f.token(t, "k1", f.goodClaims(), map[string]any{"scope": "openid"})))
	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Header().Get("WWW-Authenticate"), `error="insufficient_scope"`) {
		t.Errorf("a missing scope is a 403: %d %q", rec.Code, rec.Header().Get("WWW-Authenticate"))
	}
}

// TestOAuthWithoutAStaticToken: OAuth alone, scopes as `scp` in both shapes,
// and the unknown-key refetch bounded.
func TestOAuthWithoutAStaticToken(t *testing.T) {
	f := newFakeIssuer(t, "/realm/.well-known/openid-configuration") // OIDC discovery only
	o := f.options("ssg.mcp")
	h := testEndpoint(t, HTTPOptions{OAuth: &o})

	for _, scp := range []any{[]string{"ssg.mcp"}, "read ssg.mcp"} {
		if rec := post(h, listTools, bearer(f.token(t, "k1", f.goodClaims(), map[string]any{"scp": scp}))); rec.Code != http.StatusOK {
			t.Errorf("scp %v: %d %s", scp, rec.Code, rec.Body)
		}
	}
	if rec := post(h, listTools, bearer("anything")); rec.Code != http.StatusUnauthorized {
		t.Errorf("without a static token only OAuth counts: %d", rec.Code)
	}
	before := f.jwksCalls.Load()
	for range 3 {
		post(h, listTools, bearer(f.token(t, "made-up", f.goodClaims(), nil)))
	}
	if got := f.jwksCalls.Load() - before; got != 0 {
		t.Errorf("unknown key IDs refetched the key set %d times within %s", got, jwksMinRefresh)
	}
}

// TestStaticTokenRefusalCarriesAChallenge: RFC 6750 asks for WWW-Authenticate
// on every 401, OAuth or not.
func TestStaticTokenRefusalCarriesAChallenge(t *testing.T) {
	rec := post(testEndpoint(t, HTTPOptions{Token: "s"}), listTools, nil)
	if rec.Code != http.StatusUnauthorized || rec.Header().Get("WWW-Authenticate") != "Bearer" {
		t.Errorf("%d %q", rec.Code, rec.Header().Get("WWW-Authenticate"))
	}
}

// TestProtectedResourceMetadata is the RFC 9728 document and where it lives.
func TestProtectedResourceMetadata(t *testing.T) {
	o := OAuthOptions{Issuer: "https://auth.example.com/realm", Resource: testResource, Scopes: []string{"ssg.mcp"}}
	if got := o.MetadataPaths(); len(got) != 2 || got[0] != "/.well-known/oauth-protected-resource/mcp" || got[1] != protectedResourcePath {
		t.Errorf("paths = %v", got)
	}
	if got := (OAuthOptions{Resource: "https://mcp.example.com"}).MetadataPaths(); len(got) != 1 {
		t.Errorf("a resource at the root has one path: %v", got)
	}
	if got := (OAuthOptions{Resource: "::"}).MetadataPaths(); len(got) != 1 || (OAuthOptions{Resource: "::"}).MetadataURL() != "" {
		t.Errorf("an unparseable resource: %v", got)
	}

	h := ProtectedResourceHandler(o)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, o.MetadataPaths()[0], nil))
	var doc map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &doc); err != nil {
		t.Fatal(err)
	}
	if doc["resource"] != testResource || doc["authorization_servers"].([]any)[0] != o.Issuer ||
		doc["scopes_supported"].([]any)[0] != "ssg.mcp" || rec.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Errorf("metadata = %v", doc)
	}
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, protectedResourcePath, nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST = %d", rec.Code)
	}
}

// TestOAuthOptionsValidate: what cannot protect anything is refused at start.
func TestOAuthOptionsValidate(t *testing.T) {
	good := OAuthOptions{Issuer: "https://auth.example.com", Resource: testResource}
	if err := good.Validate(); err != nil {
		t.Error(err)
	}
	local := OAuthOptions{Issuer: "http://127.0.0.1:8080/realm", Resource: "http://localhost:7823/mcp"}
	if err := local.Validate(); err != nil {
		t.Errorf("http on this machine is fine: %v", err)
	}
	for name, o := range map[string]OAuthOptions{
		"must be an absolute URL":  {Issuer: "/realm", Resource: testResource},
		"must use https":           {Issuer: "https://auth.example.com", Resource: "http://mcp.example.com/mcp"},
		"must not have a fragment": {Issuer: "https://auth.example.com", Resource: testResource + "#x"},
	} {
		if err := o.Validate(); err == nil || !strings.Contains(err.Error(), name) {
			t.Errorf("%s: %v", name, err)
		}
	}
}

// TestJWKSDiscoveryFailures: metadata naming someone else, nothing to find,
// an issuer that is down, and a key set that is not JSON.
func TestJWKSDiscoveryFailures(t *testing.T) {
	f := newFakeIssuer(t, "/.well-known/oauth-authorization-server/realm")
	mismatch := f.options()
	mismatch.Issuer = f.srv.URL + "/other"
	if _, err := newJWKSCache(mismatch).key("k1"); err == nil || !strings.Contains(err.Error(), "discovering") {
		t.Errorf("no metadata for that issuer: %v", err)
	}

	f.metaPath = ""
	if _, err := newJWKSCache(f.options()).key("k1"); err == nil {
		t.Error("an issuer without metadata cannot be discovered")
	}

	explicit := f.options()
	explicit.JWKSURL = f.srv.URL + "/missing"
	if _, err := newJWKSCache(explicit).key("k1"); err == nil || !strings.Contains(err.Error(), "HTTP 404") {
		t.Errorf("a missing key set: %v", err)
	}

	notJSON := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("<html>")) }))
	defer notJSON.Close()
	explicit.JWKSURL = notJSON.URL
	if _, err := newJWKSCache(explicit).key("k1"); err == nil || !strings.Contains(err.Error(), "not JSON") {
		t.Errorf("a key set that is not JSON: %v", err)
	}

	down := OAuthOptions{Issuer: "https://auth.example.com", JWKSURL: "http://127.0.0.1:1/keys"}
	if _, err := newJWKSCache(down).key("k1"); err == nil {
		t.Error("an issuer that is down")
	}
	if got := issuerMetadataURLs("::"); got != nil {
		t.Errorf("an unparseable issuer: %v", got)
	}
}

// TestJWKSCacheAgesOut: a key set is refetched after jwksMaxAge, and a token
// without a kid works only when one signing key could be meant.
func TestJWKSCacheAgesOut(t *testing.T) {
	f := newFakeIssuer(t, "/.well-known/oauth-authorization-server/realm")
	c := newJWKSCache(f.options())
	now := time.Now()
	c.now = func() time.Time { return now }
	if _, err := c.key("k1"); err != nil {
		t.Fatal(err)
	}
	if _, err := c.key(""); err != nil {
		t.Errorf("one signing key, no kid: %v", err)
	}
	now = now.Add(jwksMaxAge + time.Second)
	if _, err := c.key("k1"); err != nil || f.jwksCalls.Load() != 2 {
		t.Errorf("an old key set is refetched: calls %d, %v", f.jwksCalls.Load(), err)
	}

	ec, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	two := jose.JSONWebKeySet{Keys: []jose.JSONWebKey{{Key: &ec.PublicKey, KeyID: "a"}, {Key: &ec.PublicKey, KeyID: "b"}}}
	if signingKey(two, "") != nil {
		t.Error("two signing keys and no kid: ambiguous, so none")
	}
}

// TestOAuthRefusesAMultiSignatureToken: one signature, from one key.
func TestOAuthRefusesAMultiSignatureToken(t *testing.T) {
	f := newFakeIssuer(t, "/.well-known/oauth-authorization-server/realm")
	signer, err := jose.NewMultiSigner([]jose.SigningKey{
		{Algorithm: jose.RS256, Key: f.key}, {Algorithm: jose.RS256, Key: f.key},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	payload, _ := json.Marshal(f.goodClaims())
	obj, err := signer.Sign(payload)
	if err != nil {
		t.Fatal(err)
	}
	raw := obj.FullSerialize()
	if err := newTokenVerifier(f.options()).verify(raw); err == nil {
		t.Error("a multi-signature token must be refused")
	}
}

// TestJWKSDiscoveryRefusesAnotherIssuersMetadata: a metadata document at the
// right address that names a different issuer describes someone else.
func TestJWKSDiscoveryRefusesAnotherIssuersMetadata(t *testing.T) {
	f := newFakeIssuer(t, "/.well-known/oauth-authorization-server/realm")
	o := f.options()
	f.issuer = "https://impostor.example.com"
	if _, err := newJWKSCache(o).key("k1"); err == nil || !strings.Contains(err.Error(), "impostor") {
		t.Errorf("err = %v", err)
	}
}
