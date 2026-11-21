package main

// The preview answers a missing path with the site's own 404.html (#432).
//
// Static hosts that honour a root 404.html serve its body with status 404 for
// any path they have no file for. The preview answered with Go's plain-text
// "404 page not found", so the custom error page could only be seen at
// /404.html itself — never the way a reader meets it.

import (
	"io"
	"net/http"
	"strconv"
)

// notFoundPageName is the file a host serves for a missing path.
const notFoundPageName = "/404.html"

// keptOnMiss are the headers the middleware above set and http.FileServer
// deletes from an error response; the 404 page is a page, so it keeps them.
// Content-Encoding matters most: under gzip the body is still compressed.
var keptOnMiss = []string{"Cache-Control", "Content-Encoding"}

// notFoundPageHandler serves next, and replaces a 404 it answers with the
// body of 404.html from root — status kept, served as HTML. Without that
// file the plain-text 404 stands. root is the same tree next serves, so a
// site under a base path finds its page under that prefix.
func notFoundPageHandler(next http.Handler, root http.FileSystem) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		kept := map[string][]string{}
		for _, k := range keptOnMiss {
			if v := w.Header().Values(k); len(v) > 0 {
				kept[k] = append([]string(nil), v...)
			}
		}
		nw := &notFoundWriter{ResponseWriter: w}
		next.ServeHTTP(nw, r)
		if !nw.missed {
			return
		}
		for k, v := range kept {
			w.Header()[k] = v
		}
		body, err := readNotFoundPage(root)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Content-Length", strconv.Itoa(len(body)))
		w.WriteHeader(http.StatusNotFound)
		if r.Method != http.MethodHead {
			// #nosec G705 -- the site's own built page, served as the file server serves any page
			_, _ = w.Write(body)
		}
	})
}

// readNotFoundPage reads 404.html, re-read on every miss so a rebuild shows.
// A directory of that name, or no file at all, is an error.
func readNotFoundPage(root http.FileSystem) ([]byte, error) {
	f, err := root.Open(notFoundPageName)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	return io.ReadAll(f)
}

// notFoundWriter holds back a 404 so the handler can answer it itself; every
// other status passes straight through.
type notFoundWriter struct {
	http.ResponseWriter
	missed bool
}

func (w *notFoundWriter) WriteHeader(code int) {
	if code == http.StatusNotFound {
		w.missed = true
		return
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *notFoundWriter) Write(b []byte) (int, error) {
	if w.missed {
		return len(b), nil // the plain-text body being replaced
	}
	return w.ResponseWriter.Write(b)
}
