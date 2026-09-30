package main

// The preview serves a site under its base path (#306), the way GitHub Pages
// serves a project page: the build prefixes every link with the path, so a
// preview at the root would 404 on the first click.

import (
	"net/http"
	"strings"
)

// basePathHandler serves next under base: /base/x reaches next as /x, the
// root redirects to /base/, and any other path is a 404 — which is what the
// host will answer for a link that forgot the prefix, so the preview shows it
// rather than hiding it. With no base path it is next itself.
func basePathHandler(next http.Handler, base string) http.Handler {
	if base == "" {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/":
			http.Redirect(w, r, base+"/", http.StatusFound)
		case r.URL.Path == base:
			http.Redirect(w, r, base+"/", http.StatusMovedPermanently)
		case strings.HasPrefix(r.URL.Path, base+"/"):
			http.StripPrefix(base, next).ServeHTTP(w, r)
		default:
			http.NotFound(w, r)
		}
	})
}
