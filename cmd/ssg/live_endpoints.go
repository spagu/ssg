package main

// Endpoints that follow the config file while the server is running (#180).
//
// `--watch` reloads .ssg.yaml and rebuilds the site, but the HTTP server was
// wired once at startup: buildServerHandler captured the endpoint list and
// nothing replaced it. So adding
//
//	endpoints:
//	  - {path: /go/latest, type: redirect, to: /blog/}
//
// to a running preview rebuilt the site and still answered 404, and the only
// ways out were restarting the process or — under `ssg daemon` — restarting a
// project whose entry had not changed, which takes its port down with it.
//
// The reported symptom was the daemon's, but the defect is the server's: a plain
// `ssg --watch --http` had it too. So the routes are rebuilt where they are
// wired, and both are fixed at once.
//
// The swap is a pointer store, not a server restart: the listener, the
// connections and the port are all untouched, which is the whole point of a
// reload that does not interrupt anything.

import (
	"net/http"
	"sync/atomic"

	"github.com/spagu/ssg/internal/config"
)

// liveEndpoints holds the endpoint routes the server is currently answering.
// Read on every request, replaced whole on reload — a request either sees the
// old routing table or the new one, never a half-built one.
var liveEndpoints atomic.Pointer[http.Handler]

// publishEndpoints compiles cfg's endpoints and makes them the live routing
// table. Called at server start and on every watch reload.
//
// next is the handler an unmatched path falls through to — the static file
// server — and it does not change across reloads, so it is captured once by the
// caller rather than re-derived here.
func publishEndpoints(cfg *config.Config, next http.Handler) {
	liveStatic.Store(&next)
	h := endpointHandler(cfg, next)
	liveEndpoints.Store(&h)
}

// liveEndpointHandler is what the server actually mounts: a thin indirection
// that dispatches to whatever publishEndpoints last stored.
//
// Before the first publish it serves next directly, so a server built without a
// reload path behaves exactly as it did.
func liveEndpointHandler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if h := liveEndpoints.Load(); h != nil {
			(*h).ServeHTTP(w, r)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// liveStatic remembers the fall-through handler so a reload can rebuild the
// routing table without re-deriving it. Set once, when the server starts.
var liveStatic atomic.Pointer[http.Handler]

// republishEndpoints rebuilds the routing table from a reloaded config. A no-op
// before a server has started, so a plain build reloading its config does not
// have to care whether one is running.
func republishEndpoints(cfg *config.Config) {
	next := liveStatic.Load()
	if next == nil {
		return
	}
	publishEndpoints(cfg, *next)
}
