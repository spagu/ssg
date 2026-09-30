package main

import (
	"net/http"
	"testing"
)

func TestBasePathHandler(t *testing.T) {
	echo := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("served " + r.URL.Path))
	})
	if h := basePathHandler(echo, ""); getPath(h, "/x").Body.String() != "served /x" {
		t.Error("no base path must pass everything through")
	}
	h := basePathHandler(echo, "/site")
	for path, want := range map[string]struct {
		code int
		body string
		loc  string
	}{
		"/":          {http.StatusFound, "", "/site/"},
		"/site":      {http.StatusMovedPermanently, "", "/site/"},
		"/site/":     {http.StatusOK, "served /", ""},
		"/site/a/b/": {http.StatusOK, "served /a/b/", ""},
		"/css/x.css": {http.StatusNotFound, "", ""},
		"/sitemap/":  {http.StatusNotFound, "", ""},
	} {
		rec := getPath(h, path)
		if rec.Code != want.code {
			t.Errorf("%s: status %d, want %d", path, rec.Code, want.code)
		}
		if want.body != "" && rec.Body.String() != want.body {
			t.Errorf("%s: body %q, want %q", path, rec.Body.String(), want.body)
		}
		if want.loc != "" && rec.Header().Get("Location") != want.loc {
			t.Errorf("%s: Location %q, want %q", path, rec.Header().Get("Location"), want.loc)
		}
	}
}
