package serverauth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// serve runs one request with a fixed RemoteAddr through the middleware.
func serve(t *testing.T, h http.Handler, remoteAddr string, mutate func(*http.Request)) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = remoteAddr
	if mutate != nil {
		mutate(req)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// okHandler marks that the request reached the protected handler.
var okHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	_, _ = w.Write([]byte("ok"))
})

func TestEnabled(t *testing.T) {
	if (Config{}).Enabled() {
		t.Fatal("empty config must be disabled")
	}
	for _, cfg := range []Config{{Auth: "basic"}, {IPAllowlist: []string{"1.2.3.4"}},
		{IPBlocklist: []string{"1.2.3.4"}}, {RateLimit: 5}} {
		if !cfg.Enabled() {
			t.Fatalf("%+v must be enabled", cfg)
		}
	}
}

func TestIPListsAndValidation(t *testing.T) {
	// Blocklist wins before anything else.
	h, err := Middleware(okHandler, Config{IPBlocklist: []string{"10.0.0.0/8", "192.0.2.7"}})
	if err != nil {
		t.Fatal(err)
	}
	if rec := serve(t, h, "10.1.2.3:1000", nil); rec.Code != http.StatusForbidden {
		t.Fatalf("blocked CIDR = %d", rec.Code)
	}
	if rec := serve(t, h, "192.0.2.7:1000", nil); rec.Code != http.StatusForbidden {
		t.Fatalf("blocked IP = %d", rec.Code)
	}
	if rec := serve(t, h, "198.51.100.9:1000", nil); rec.Code != http.StatusOK {
		t.Fatalf("unblocked = %d", rec.Code)
	}
	// Allowlist: only listed ranges pass; IPv6 entries work.
	h, err = Middleware(okHandler, Config{IPAllowlist: []string{"192.0.2.0/24", "2001:db8::1"}})
	if err != nil {
		t.Fatal(err)
	}
	if rec := serve(t, h, "192.0.2.55:1000", nil); rec.Code != http.StatusOK {
		t.Fatalf("allowlisted = %d", rec.Code)
	}
	if rec := serve(t, h, "[2001:db8::1]:1000", nil); rec.Code != http.StatusOK {
		t.Fatalf("allowlisted v6 = %d", rec.Code)
	}
	if rec := serve(t, h, "203.0.113.5:1000", nil); rec.Code != http.StatusForbidden {
		t.Fatalf("not allowlisted = %d", rec.Code)
	}
	if rec := serve(t, h, "not-an-ip", nil); rec.Code != http.StatusForbidden {
		t.Fatalf("unparseable remote = %d", rec.Code)
	}
	// Invalid entries fail configuration.
	if _, err := Middleware(okHandler, Config{IPAllowlist: []string{"999.1.1.1"}}); err == nil {
		t.Fatal("invalid allowlist entry must error")
	}
	if _, err := Middleware(okHandler, Config{IPBlocklist: []string{"10.0.0.0/99"}}); err == nil {
		t.Fatal("invalid blocklist entry must error")
	}
}

func TestBasicAuth(t *testing.T) {
	t.Setenv("SSG_TEST_PASS", "s3cret")
	h, err := Middleware(okHandler, Config{Auth: "basic", Users: []string{"admin:$SSG_TEST_PASS"}})
	if err != nil {
		t.Fatal(err)
	}
	rec := serve(t, h, "203.0.113.5:1000", nil)
	if rec.Code != http.StatusUnauthorized || rec.Header().Get("WWW-Authenticate") == "" {
		t.Fatalf("no credentials = %d", rec.Code)
	}
	withAuth := func(user, pass string) func(*http.Request) {
		return func(r *http.Request) { r.SetBasicAuth(user, pass) }
	}
	if rec := serve(t, h, "203.0.113.5:1000", withAuth("admin", "s3cret")); rec.Code != http.StatusOK {
		t.Fatalf("valid credentials = %d", rec.Code)
	}
	if rec := serve(t, h, "203.0.113.5:1000", withAuth("admin", "wrong")); rec.Code != http.StatusUnauthorized {
		t.Fatalf("wrong password = %d", rec.Code)
	}
	if rec := serve(t, h, "203.0.113.5:1000", withAuth("ghost", "s3cret")); rec.Code != http.StatusUnauthorized {
		t.Fatalf("unknown user = %d", rec.Code)
	}
	// Config validation matrix.
	for label, cfg := range map[string]Config{
		"no users":       {Auth: "basic"},
		"bad entry":      {Auth: "basic", Users: []string{"nopass"}},
		"literal secret": {Auth: "basic", Users: []string{"admin:plaintext"}},
		"missing env":    {Auth: "basic", Users: []string{"admin:$SSG_UNSET_PASS"}},
		"unknown mode":   {Auth: "ldap"},
	} {
		if _, err := Middleware(okHandler, cfg); err == nil {
			t.Errorf("%s: expected error", label)
		}
	}
}

// signJWT builds an HS256 token from raw header/claims JSON.
func signJWT(headerJSON, claimsJSON string, secret []byte) string {
	h := base64.RawURLEncoding.EncodeToString([]byte(headerJSON))
	c := base64.RawURLEncoding.EncodeToString([]byte(claimsJSON))
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(h + "." + c))
	return h + "." + c + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func TestJWT(t *testing.T) {
	t.Setenv("SSG_TEST_JWT", "topsecret")
	secret := []byte("topsecret")
	h, err := Middleware(okHandler, Config{Auth: "jwt", JWTSecret: "$SSG_TEST_JWT"})
	if err != nil {
		t.Fatal(err)
	}
	future := time.Now().Add(time.Hour).Unix()
	past := time.Now().Add(-time.Hour).Unix()
	bearer := func(token string) func(*http.Request) {
		return func(r *http.Request) { r.Header.Set("Authorization", "Bearer "+token) }
	}
	valid := signJWT(`{"alg":"HS256","typ":"JWT"}`, fmt.Sprintf(`{"sub":"ed","exp":%d}`, future), secret)
	if rec := serve(t, h, "203.0.113.5:1000", bearer(valid)); rec.Code != http.StatusOK {
		t.Fatalf("valid token = %d", rec.Code)
	}
	noClaims := signJWT(`{"alg":"HS256"}`, `{}`, secret)
	if rec := serve(t, h, "203.0.113.5:1000", bearer(noClaims)); rec.Code != http.StatusOK {
		t.Fatalf("no exp/nbf = %d", rec.Code)
	}
	rejected := map[string]string{
		"expired":     signJWT(`{"alg":"HS256"}`, fmt.Sprintf(`{"exp":%d}`, past), secret),
		"not yet nbf": signJWT(`{"alg":"HS256"}`, fmt.Sprintf(`{"nbf":%d}`, future), secret),
		"alg none":    signJWT(`{"alg":"none"}`, `{}`, secret),
		"wrong key":   signJWT(`{"alg":"HS256"}`, `{}`, []byte("other")),
		"two parts":   "a.b",
		"bad header":  "!!!." + base64.RawURLEncoding.EncodeToString([]byte(`{}`)) + ".sig",
		"bad sig b64": signJWT(`{"alg":"HS256"}`, `{}`, secret)[:20] + ".x.!!!",
		"bad claims":  mangleClaims(signJWT(`{"alg":"HS256"}`, `{}`, secret), secret),
	}
	for label, token := range rejected {
		if rec := serve(t, h, "203.0.113.5:1000", bearer(token)); rec.Code != http.StatusUnauthorized {
			t.Errorf("%s: code = %d", label, rec.Code)
		}
	}
	if rec := serve(t, h, "203.0.113.5:1000", nil); rec.Code != http.StatusUnauthorized {
		t.Fatalf("no header = %d", rec.Code)
	}
	// Secret rules: literal and missing env.
	if _, err := Middleware(okHandler, Config{Auth: "jwt", JWTSecret: "hardcoded"}); err == nil {
		t.Fatal("literal jwt secret must error")
	}
	if _, err := Middleware(okHandler, Config{Auth: "jwt"}); err == nil {
		t.Fatal("empty jwt secret must error")
	}
}

// mangleClaims re-signs a token whose claims segment is not valid JSON.
func mangleClaims(_ string, secret []byte) string {
	h := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256"}`))
	c := base64.RawURLEncoding.EncodeToString([]byte(`{broken`))
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(h + "." + c))
	return h + "." + c + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func TestRateLimiter(t *testing.T) {
	h, err := Middleware(okHandler, Config{RateLimit: 1, RateBurst: 2})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if rec := serve(t, h, "203.0.113.5:1000", nil); rec.Code != http.StatusOK {
			t.Fatalf("burst request %d = %d", i, rec.Code)
		}
	}
	rec := serve(t, h, "203.0.113.5:1000", nil)
	if rec.Code != http.StatusTooManyRequests || rec.Header().Get("Retry-After") == "" {
		t.Fatalf("over burst = %d", rec.Code)
	}
	// Another IP has its own bucket.
	if rec := serve(t, h, "198.51.100.1:1000", nil); rec.Code != http.StatusOK {
		t.Fatalf("other ip = %d", rec.Code)
	}
	// Refill over time with an injected clock.
	l := newLimiter(10, 1)
	base := time.Now()
	l.now = func() time.Time { return base }
	if !l.allow("a") || l.allow("a") {
		t.Fatal("burst of 1")
	}
	l.now = func() time.Time { return base.Add(200 * time.Millisecond) }
	if !l.allow("a") {
		t.Fatal("refill after 200ms at 10/s")
	}
	// Default burst = 2×rate; minimum 1.
	if newLimiter(5, 0).burst != 10 || newLimiter(0.2, 0).burst != 1 {
		t.Fatal("default burst")
	}
	// Purge drops idle buckets when the map is full.
	lp := newLimiter(1, 1)
	tick := base
	lp.now = func() time.Time { return tick }
	for i := 0; i < maxBuckets; i++ {
		lp.allow(fmt.Sprintf("ip-%d", i))
	}
	tick = base.Add(time.Hour)
	lp.allow("fresh")
	if len(lp.buckets) >= maxBuckets {
		t.Fatalf("purge did not shrink the map: %d", len(lp.buckets))
	}
}

func TestMiddlewareChainOrder(t *testing.T) {
	// A blocklisted IP is refused even with valid credentials.
	t.Setenv("SSG_TEST_PASS", "s3cret")
	h, err := Middleware(okHandler, Config{Auth: "basic", Users: []string{"admin:$SSG_TEST_PASS"},
		IPBlocklist: []string{"203.0.113.0/24"}, RateLimit: 100})
	if err != nil {
		t.Fatal(err)
	}
	rec := serve(t, h, "203.0.113.5:1000", func(r *http.Request) { r.SetBasicAuth("admin", "s3cret") })
	if rec.Code != http.StatusForbidden {
		t.Fatalf("blocklist must precede auth: %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "forbidden") {
		t.Fatalf("body = %q", rec.Body.String())
	}
}
