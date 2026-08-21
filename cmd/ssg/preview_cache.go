package main

// No caching while a preview server is running (#185).
//
// The ordinary cache policy is right for a published site and wrong for a
// preview: the middleware caches non-fingerprinted assets for an hour, and the
// generated `_headers` the preview now honours (#181) caches `/css/*` and
// `/js/*` for a year with `immutable` — a value browsers will not revalidate at
// all. The result is a live-reload that refreshes the page, shows the new HTML,
// and keeps the previous stylesheet. Watch mode is the one place a cache has
// negative value, so every response is stamped `no-cache` there.
//
// `no-cache` means "revalidate", not "do not store": conditional requests still
// answer 304 when nothing changed, so the preview stays cheap.

import (
	"net/http"
	"strings"

	"github.com/spagu/ssg/internal/config"
)

// previewMode reports whether this server is a development preview rather than
// a server for published output. Both entry points count: `--watch --http`
// (with or without --auto-reload — a manual refresh goes just as stale), and
// `ssg mcp --http`, which rebuilds on every agent edit and installs a reload
// hub without setting Watch.
func previewMode(cfg *config.Config) bool {
	return cfg.Watch || currentReloadHub() != nil
}

// previewNoCacheMiddleware overrides the cache headers of everything below it.
// It has to stamp the response rather than set a header up front: the file
// server, the endpoint table and the `_headers` rules all write Cache-Control
// after an outer middleware has run, so the last word is the only word that
// counts.
func previewNoCacheMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// A cached validator is as stale as a cached body: without this the
		// browser re-offers If-None-Match for the asset it already has and the
		// file server is free to answer 304 from an ETag that no longer
		// describes the file it just rebuilt.
		r.Header.Del("If-None-Match")
		r.Header.Del("If-Modified-Since")
		next.ServeHTTP(&noCacheWriter{ResponseWriter: w}, r)
	})
}

// noCacheWriter rewrites Cache-Control as the response headers are committed.
type noCacheWriter struct {
	http.ResponseWriter
	stamped bool
}

// stamp replaces Cache-Control unless something below asked for something
// stricter. Endpoints set `no-store` deliberately (their bodies may hold
// per-request data); relaxing that to `no-cache` would permit a copy on disk.
func (w *noCacheWriter) stamp() {
	if w.stamped {
		return
	}
	w.stamped = true
	h := w.Header()
	if strings.Contains(h.Get(cacheControlHeader), "no-store") {
		return
	}
	h.Set(cacheControlHeader, "no-cache")
	// Expires and Pragma outrank Cache-Control in older caches and in every
	// intermediary that still speaks HTTP/1.0, so a stale Expires from the
	// `_headers` file would survive the rewrite above.
	h.Del("Expires")
	h.Del("Pragma")
}

func (w *noCacheWriter) WriteHeader(code int) {
	w.stamp()
	w.ResponseWriter.WriteHeader(code)
}

func (w *noCacheWriter) Write(b []byte) (int, error) {
	w.stamp()
	return w.ResponseWriter.Write(b)
}

// Flush keeps streaming handlers below this wrapper working; without it the
// http.Flusher assertion they make fails and they refuse the request.
func (w *noCacheWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}
