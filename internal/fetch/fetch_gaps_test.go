package fetch

// The failure paths a config include takes when the network misbehaves
// (third coverage raise): a stalled server, a redirect loop, a truncated
// response, a misconfigured auth block and an out-of-range retry count.

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// An unset Timeout/RetryDelay must fall back to the documented policy: a
// request cannot hang forever, and a retry must not turn into a hot loop
// hammering the server with zero delay.
func TestUnsetTimeoutAndDelayFallBackToTheDefaultPolicy(t *testing.T) {
	if got := (Options{}).timeout(); got != defaultTimeout {
		t.Errorf("zero Timeout = %v, want the %v default", got, defaultTimeout)
	}
	if got := (Options{}).retryDelay(); got != defaultRetryDelay {
		t.Errorf("zero RetryDelay = %v, want the %v default", got, defaultRetryDelay)
	}
	if got := (Options{Timeout: time.Second}).timeout(); got != time.Second {
		t.Errorf("configured Timeout = %v, want 1s", got)
	}
	if got := (Options{RetryDelay: 2 * time.Second}).retryDelay(); got != 2*time.Second {
		t.Errorf("configured RetryDelay = %v, want 2s", got)
	}
}

// A server that accepts the connection and then never answers must not stall
// the build: the per-attempt timeout has to cut it off, and the resulting
// transport error must name the URL that failed.
func TestBytesGivesUpOnAServerThatNeverResponds(t *testing.T) {
	stall := make(chan struct{})
	defer close(stall)
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		select {
		case <-stall:
		case <-r.Context().Done():
		}
	}))
	defer srv.Close()

	start := time.Now()
	_, err := Bytes(srv.URL+"/slow.yaml", Auth{}, 0, Options{Timeout: 200 * time.Millisecond})
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("a stalled server must produce an error")
	}
	if elapsed > 20*time.Second {
		t.Fatalf("the timeout was not honoured: waited %v", elapsed)
	}
	if !strings.Contains(err.Error(), "/slow.yaml") {
		t.Errorf("error should name the URL that timed out, got %v", err)
	}
}

// A server bouncing a request around forever must be stopped by the redirect
// cap rather than looping until the client runs out of memory or patience.
func TestBytesStopsAfterTheRedirectCap(t *testing.T) {
	var hits atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		http.Redirect(w, r, fmt.Sprintf("/hop-%d", hits.Load()), http.StatusFound)
	}))
	defer srv.Close()

	_, err := Bytes(srv.URL+"/start", Auth{}, 0, Options{})
	if err == nil || !strings.Contains(err.Error(), "stopped after 5 redirects") {
		t.Fatalf("redirect loop not capped, got %v", err)
	}
	if got := hits.Load(); got != maxRedirects {
		t.Errorf("server saw %d requests, want %d (the cap)", got, maxRedirects)
	}
}

// A response that promises more bytes than it delivers is a mid-stream read
// error: it must be reported as retriable, because the next attempt may well
// get the whole body.
func TestFetchOnceTreatsATruncatedBodyAsRetriable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		conn, buf, err := w.(http.Hijacker).Hijack()
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()
		// Promise 4096 bytes, deliver 8, then hang up mid-body.
		_, _ = buf.WriteString("HTTP/1.1 200 OK\r\nContent-Length: 4096\r\n\r\ntruncate")
		_ = buf.Flush()
	}))
	defer srv.Close()

	body, retriable, err := fetchOnce(srv.URL+"/half.yaml", Auth{}, DefaultMaxBytes, 5*time.Second)
	if err == nil {
		t.Fatalf("truncated body accepted as complete: %q", body)
	}
	if !retriable {
		t.Errorf("a truncated body must be retriable, got err=%v", err)
	}
	if !strings.Contains(err.Error(), "reading") {
		t.Errorf("error should say the read failed, got %v", err)
	}
}

// An auth block that cannot produce a header (bearer with no token) must fail
// before anything goes on the wire, so a private URL is never requested
// unauthenticated.
func TestBytesRejectsIncompleteAuthBeforeSendingTheRequest(t *testing.T) {
	var hits atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		_, _ = w.Write([]byte("workers: []\n"))
	}))
	defer srv.Close()

	_, err := Bytes(srv.URL+"/private.yaml", Auth{Type: "bearer"}, 0, Options{})
	if err == nil || !strings.Contains(err.Error(), "needs a token") {
		t.Fatalf("incomplete bearer auth accepted: %v", err)
	}
	if hits.Load() != 0 {
		t.Errorf("the request was sent despite unusable auth (%d hits)", hits.Load())
	}
}

// A negative Retries (a hand-edited config) must still make one attempt, not
// zero: the include has to be fetched, and the error must be the plain one
// rather than an "after N attempts" wrapper.
func TestBytesMakesOneAttemptWhenRetriesIsNegative(t *testing.T) {
	var hits atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	_, err := Bytes(srv.URL+"/x.yaml", Auth{}, 0, Options{Retries: -3, RetryDelay: time.Millisecond})
	if err == nil || !strings.Contains(err.Error(), "HTTP 500") {
		t.Fatalf("expected the 500 to be reported, got %v", err)
	}
	if strings.Contains(err.Error(), "attempts") {
		t.Errorf("a single attempt must not be reported as a retry run: %v", err)
	}
	if got := hits.Load(); got != 1 {
		t.Errorf("server saw %d requests, want exactly 1", got)
	}
}

// ExpandAuth checks every secret field, not just the token: a basic password or
// a header value pointing at an unset variable must be named in the error, and
// the resolved Auth must not be returned half-built.
func TestExpandAuthNamesAnUnsetPasswordOrHeaderValue(t *testing.T) {
	t.Setenv("SSG_TEST_TOKEN", "tok")

	got, err := ExpandAuth(Auth{Type: "basic", Username: "user", Password: "$SSG_TEST_MISSING_PW"})
	if err == nil || !strings.Contains(err.Error(), "SSG_TEST_MISSING_PW") ||
		!strings.Contains(err.Error(), "auth.password") {
		t.Fatalf("unset password not reported: %v", err)
	}
	if got != (Auth{}) {
		t.Errorf("a failed expansion must return the zero Auth, got %+v", got)
	}

	got, err = ExpandAuth(Auth{Type: "header", Token: "$SSG_TEST_TOKEN", Header: "X-Api-Key", Value: "${SSG_TEST_MISSING_KEY}"})
	if err == nil || !strings.Contains(err.Error(), "SSG_TEST_MISSING_KEY") ||
		!strings.Contains(err.Error(), "auth.value") {
		t.Fatalf("unset header value not reported: %v", err)
	}
	if got.Token != "" {
		t.Errorf("the already-resolved token must not leak out of a failed expansion: %+v", got)
	}
}

// TestATokenInTheURLNeverReachesATransportError.
//
// safeURL keeps a credential out of the message this package writes, but the
// *url.Error the HTTP client returns builds its own message from the raw URL
// and redacts only the userinfo password. So `?token=…` — how half the world's
// private feeds are addressed — used to survive into the second half of the
// same line, and from there into a log or a CI transcript.
func TestATokenInTheURLNeverReachesATransportError(t *testing.T) {
	// A port nothing is listening on: the client fails in the transport, which
	// is the only path that wraps a *url.Error.
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}

	for _, target := range []string{
		"http://" + addr + "/feed.yaml?token=leakme",
		"http://leakme@" + addr + "/feed.yaml",
	} {
		_, err := Bytes(target, Auth{}, 0, Options{Retries: 0, Timeout: 2 * time.Second})
		if err == nil {
			t.Fatalf("%s should have failed to connect", target)
		}
		if strings.Contains(err.Error(), "leakme") {
			t.Errorf("the credential reached the error message: %v", err)
		}
	}
}

// TestATransportErrorKeepsItsCause: unwrapping the *url.Error must not cost a
// caller the ability to tell a timeout from a refused connection.
func TestATransportErrorKeepsItsCause(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer srv.Close()
	_, err := Bytes(srv.URL+"/slow", Auth{}, 0, Options{Retries: 0, Timeout: 150 * time.Millisecond})
	if err == nil {
		t.Fatal("a server that never answers should time out")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("the cause was lost: %v", err)
	}
}
