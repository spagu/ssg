package main

// Endpoints that follow the config file while the server runs (#180).

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/spagu/ssg/internal/config"
)

// staticSaying returns a stand-in for the file server.
func staticSaying(body string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(body))
	})
}

// resetLiveEndpoints clears the package state between tests.
func resetLiveEndpoints(t *testing.T) {
	t.Helper()
	liveEndpoints.Store(nil)
	liveStatic.Store(nil)
	t.Cleanup(func() {
		liveEndpoints.Store(nil)
		liveStatic.Store(nil)
	})
}

// getPath runs one request through a handler.
func getPath(h http.Handler, path string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec
}

// redirectConfig is the shape from the report.
func redirectConfig(from, to string) *config.Config {
	cfg := config.DefaultConfig()
	cfg.Quiet = true
	cfg.Endpoints = []config.Endpoint{{Path: from, Type: "redirect", To: to}}
	return cfg
}

// TestAnEndpointAddedToTheConfigGoesLive: the reported case. A route added to a
// running preview must answer without restarting the process — restarting takes
// the port down, and under `ssg daemon` it takes a project down with it.
func TestAnEndpointAddedToTheConfigGoesLive(t *testing.T) {
	resetLiveEndpoints(t)
	static := staticSaying("static")

	// A server started with no endpoints falls through to the file server.
	empty := config.DefaultConfig()
	empty.Quiet = true
	publishEndpoints(empty, static)
	h := liveEndpointHandler(static)

	if rec := getPath(h, "/go/latest"); rec.Body.String() != "static" {
		t.Fatalf("before: body = %q, want the static handler", rec.Body)
	}

	// The config gains the endpoint and is reloaded.
	republishEndpoints(redirectConfig("/go/latest", "/blog/"))

	rec := getPath(h, "/go/latest")
	if rec.Code != http.StatusFound {
		t.Fatalf("after: status = %d, want 302", rec.Code)
	}
	if got := rec.Header().Get("Location"); got != "/blog/" {
		t.Errorf("Location = %q, want /blog/", got)
	}
	// Everything else still reaches the file server.
	if rec := getPath(h, "/other"); rec.Body.String() != "static" {
		t.Errorf("an unmatched path must still fall through: %q", rec.Body)
	}
}

// TestAnEndpointRemovedFromTheConfigStopsAnswering: the reverse, which matters
// as much — a route deleted in a panel must stop working.
func TestAnEndpointRemovedFromTheConfigStopsAnswering(t *testing.T) {
	resetLiveEndpoints(t)
	static := staticSaying("static")
	publishEndpoints(redirectConfig("/go/latest", "/blog/"), static)
	h := liveEndpointHandler(static)

	if rec := getPath(h, "/go/latest"); rec.Code != http.StatusFound {
		t.Fatalf("the endpoint must start alive: %d", rec.Code)
	}

	empty := config.DefaultConfig()
	empty.Quiet = true
	republishEndpoints(empty)

	if rec := getPath(h, "/go/latest"); rec.Body.String() != "static" {
		t.Errorf("a removed endpoint must stop answering: %q", rec.Body)
	}
}

// TestATargetChangeIsPickedUp, which is the ordinary edit in a settings page.
func TestATargetChangeIsPickedUp(t *testing.T) {
	resetLiveEndpoints(t)
	static := staticSaying("static")
	publishEndpoints(redirectConfig("/go/latest", "/blog/"), static)
	h := liveEndpointHandler(static)

	republishEndpoints(redirectConfig("/go/latest", "/news/"))
	if got := getPath(h, "/go/latest").Header().Get("Location"); got != "/news/" {
		t.Errorf("Location = %q, want the new target", got)
	}
}

// TestRepublishBeforeAServerStartsIsANoOp: a plain build reloading its config
// must not have to know whether a server is running.
func TestRepublishBeforeAServerStartsIsANoOp(t *testing.T) {
	resetLiveEndpoints(t)
	republishEndpoints(redirectConfig("/a", "/b")) // must not panic
	if liveEndpoints.Load() != nil {
		t.Error("nothing should be published without a server")
	}
}

// TestHandlerBeforeFirstPublishServesStatic: the indirection must be safe from
// the moment it is built, not from the moment it is filled.
func TestHandlerBeforeFirstPublishServesStatic(t *testing.T) {
	resetLiveEndpoints(t)
	h := liveEndpointHandler(staticSaying("static"))
	if rec := getPath(h, "/anything"); rec.Body.String() != "static" {
		t.Errorf("body = %q", rec.Body)
	}
}

// TestSwapIsAtomic: a request sees the old table or the new one, never a
// half-built one. Run under -race, this is the assertion that matters.
func TestSwapIsAtomic(t *testing.T) {
	resetLiveEndpoints(t)
	static := staticSaying("static")
	publishEndpoints(redirectConfig("/go", "/a/"), static)
	h := liveEndpointHandler(static)

	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 200; i++ {
			target := "/a/"
			if i%2 == 0 {
				target = "/b/"
			}
			republishEndpoints(redirectConfig("/go", target))
		}
	}()
	for i := 0; i < 200; i++ {
		rec := getPath(h, "/go")
		// Whatever it saw, it must be a complete answer: a redirect to one of
		// the two targets, never an empty or partial one.
		if rec.Code != http.StatusFound {
			t.Fatalf("status = %d during a swap", rec.Code)
		}
		if loc := rec.Header().Get("Location"); loc != "/a/" && loc != "/b/" {
			t.Fatalf("Location = %q during a swap", loc)
		}
	}
	<-done
}
