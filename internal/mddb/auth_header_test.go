package mddb

// Which header the credential goes in (#202).

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// authServer records the credential headers of the request it receives, and
// refuses the request the way MDDB would.
func authServer(t *testing.T) (*httptest.Server, *http.Header) {
	t.Helper()
	var seen http.Header
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = r.Header.Clone()
		// MDDB's middleware: whatever follows "Bearer " is validated as a JWT,
		// and X-API-Key is consulted only when there is no bearer value at all.
		// Reproduced here so a credential in the wrong header fails a test the
		// way it fails a deployment, instead of being quietly accepted by a
		// fake that checks nothing.
		if auth := r.Header.Get("Authorization"); auth != "" {
			if !looksLikeJWT(auth[len("Bearer "):]) {
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte(`{"error":"invalid token"}`))
				return
			}
		} else if r.Header.Get("X-API-Key") == "" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(`{"valid":true,"errors":[],"warnings":[]}`))
	}))
	t.Cleanup(srv.Close)
	return srv, &seen
}

// TestAnAPIKeyGoesInTheHeaderMddbReadsIt is the reported failure. Every request
// from push-theme and from designer_find's MDDB backend came back
// `401 invalid token` with a correct key, and the message blamed the key.
func TestAnAPIKeyGoesInTheHeaderMddbReadsIt(t *testing.T) {
	srv, seen := authServer(t)
	c := NewClient(Config{BaseURL: srv.URL, APIKey: "mddb_live_abc123"})

	if _, _, err := c.Validate("site", nil); err != nil {
		t.Fatalf("a correct API key must be accepted: %v", err)
	}
	if got := seen.Get("X-API-Key"); got != "mddb_live_abc123" {
		t.Errorf("X-API-Key = %q", got)
	}
	if got := seen.Get("Authorization"); got != "" {
		t.Errorf("an API key must not travel as a bearer token, got %q", got)
	}
}

// TestAJWTStillTravelsAsABearerToken: the workaround people reached for while
// this was broken — a long-lived tenant JWT in mddb_api_key — must keep working,
// or the fix breaks the only thing that worked.
func TestAJWTStillTravelsAsABearerToken(t *testing.T) {
	const jwt = "eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiJzc2cifQ.c2lnbmF0dXJlLWhlcmU"
	srv, seen := authServer(t)

	if _, _, err := NewClient(Config{BaseURL: srv.URL, APIKey: jwt}).Validate("site", nil); err != nil {
		t.Fatalf("a JWT must still be accepted: %v", err)
	}
	if got := seen.Get("Authorization"); got != "Bearer "+jwt {
		t.Errorf("Authorization = %q", got)
	}
	if got := seen.Get("X-API-Key"); got != "" {
		t.Errorf("a JWT must not be sent as an API key, got %q", got)
	}
}

// TestJWTDetection: the discriminator itself. Getting this wrong in either
// direction sends a working credential to the path that rejects it.
func TestJWTDetection(t *testing.T) {
	jwts := []string{
		"eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiJhIn0.sig",
		"a.b.c",
		"AAAA-_==.BBBB.CCCC",
	}
	for _, v := range jwts {
		if !looksLikeJWT(v) {
			t.Errorf("looksLikeJWT(%q) = false, want true", v)
		}
	}
	keys := []string{
		"mddb_live_deadbeef",     // the documented key format
		"mddb_test_deadbeef",     //
		"",                       //
		"a.b",                    // two segments
		"a.b.c.d",                // four
		"a..c",                   // an empty segment
		"a.b.c d",                // a space is not base64url
		"a.b.c/d",                // nor is /
		"key.with.dots-but+plus", // + belongs to standard base64, not base64url
	}
	for _, v := range keys {
		if looksLikeJWT(v) {
			t.Errorf("looksLikeJWT(%q) = true, want false", v)
		}
	}
}

// TestNoCredentialSendsNeitherHeader.
func TestNoCredentialSendsNeitherHeader(t *testing.T) {
	h := http.Header{}
	setCredential(h, "mddb_live_x")
	if h.Get("X-API-Key") == "" || h.Get("Authorization") != "" {
		t.Errorf("headers = %v", h)
	}
}

// TestPlaintextIsRefusedUnlessTheOperatorSaysOtherwise (#201).
//
// Right on the internet, blind on a private network: a container network that
// never leaves the host is the same trust boundary as loopback, spelled with a
// service name, and there was no way to say so. The escape used to be a private
// CA mounted into every container that runs ssg.
func TestPlaintextIsRefusedUnlessTheOperatorSaysOtherwise(t *testing.T) {
	cases := []struct {
		url       string
		allowHTTP bool
		wantErr   bool
	}{
		{"http://mddb:11023", false, true},         // the reported case
		{"http://mddb:11023", true, false},         // …with the opt-in
		{"http://10.0.0.5:11023", false, true},     //
		{"http://10.0.0.5:11023", true, false},     //
		{"https://mddb.example.com", false, false}, // TLS needs no opt-in
		{"http://localhost:11023", false, false},   // loopback never did
		{"http://127.0.0.1:11023", false, false},   //
	}
	for _, c := range cases {
		err := ensureSecureForAPIKey(c.url, c.allowHTTP)
		if (err != nil) != c.wantErr {
			t.Errorf("ensureSecureForAPIKey(%q, allowHTTP=%v) err=%v, wantErr=%v",
				c.url, c.allowHTTP, err, c.wantErr)
		}
	}
}

// TestTheRefusalNamesTheSettingThatChangesIt: a refusal whose only remedy is
// reading the source teaches people to route around the tool.
func TestTheRefusalNamesTheSettingThatChangesIt(t *testing.T) {
	err := ensureSecureForAPIKey("http://mddb:11023", false)
	if err == nil {
		t.Fatal("plaintext to a named host must be refused by default")
	}
	for _, want := range []string{"mddb.allow_http", "mcp.search.mddb_allow_http", "https://"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the message does not mention %q: %v", want, err)
		}
	}
}

// TestTheOptInReachesTheClient, not just the guard — a setting that parses and
// then goes nowhere is worse than none.
//
// Asserted on which error comes back rather than on a successful round trip:
// the guard runs before the dial, so with the opt-in off the request never
// leaves, and with it on the only thing left to fail is the connection. Using a
// hostname that resolves to 127.0.0.1 would have been the direct test, and the
// first version of this did that — but nothing guarantees such a name exists on
// the machine running the suite, and it did not on mine.
func TestTheOptInReachesTheClient(t *testing.T) {
	const plain = "http://mddb.invalid:11023"

	_, _, err := NewClient(Config{BaseURL: plain, APIKey: "mddb_live_x"}).Validate("site", nil)
	if err == nil || !strings.Contains(err.Error(), "refusing to send API key") {
		t.Errorf("without the opt-in the guard must refuse: %v", err)
	}

	_, _, err = NewClient(Config{BaseURL: plain, APIKey: "mddb_live_x", AllowHTTP: true}).Validate("site", nil)
	if err == nil {
		t.Fatal("the host does not exist, so the request must still fail")
	}
	if strings.Contains(err.Error(), "refusing to send API key") {
		t.Errorf("with the opt-in the guard must stand aside: %v", err)
	}
}

// TestAnUnparseableURLIsAnErrorNotAPass: the guard must fail closed. A URL it
// cannot read is not a URL it can vouch for.
func TestAnUnparseableURLIsAnErrorNotAPass(t *testing.T) {
	if err := ensureSecureForAPIKey("http://[::1", false); err == nil {
		t.Error("an unparseable URL must be refused")
	}
	// …and the opt-in does not excuse it: allow_http says "this network is
	// private", not "skip the check".
	if err := ensureSecureForAPIKey("http://[::1", true); err == nil {
		t.Error("an unparseable URL must be refused even with the opt-in")
	}
}
