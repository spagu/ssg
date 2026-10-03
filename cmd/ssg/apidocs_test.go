package main

import (
	"testing"

	"github.com/spagu/ssg/internal/config"
)

func TestBuildAPIDocsOptions(t *testing.T) {
	off := false
	got := buildAPIDocsOptions([]config.APIDocsConfig{
		{Name: "core", Root: "packages/core", Language: " Go ", Entry: []string{"src/index.js"}, URL: "/ref/", Visibility: " Internal ",
			Readme: &off, SourceURL: "https://example.com/{ref}/{path}#L{line}", SourceRef: "v1.0.0",
			Playground: " https://cdn.example.com/core.js ", TryIt: &off},
		{OpenAPI: " api/openapi.yaml "},
	})
	if len(got) != 2 {
		t.Fatalf("len = %d", len(got))
	}
	a, b := got[0], got[1]
	if a.Source.Root != "packages/core" || a.Source.Name != "core" || a.URL != "/ref/" || a.Visibility != "internal" || a.Source.Language != "go" ||
		a.Readme || a.SourceRef != "v1.0.0" || len(a.Source.Entries) != 1 || a.Playground != "https://cdn.example.com/core.js" || a.TryIt {
		t.Errorf("first = %+v", a)
	}
	if b.Source.Root != "." || !b.Readme || b.SourceRef == "" || b.OpenAPI != "api/openapi.yaml" || !b.TryIt {
		t.Errorf("defaults = %+v", b)
	}
}

func TestSourceRef(t *testing.T) {
	if sourceRef("main", ".") != "main" {
		t.Error("a configured ref is used as is")
	}
	if ref := sourceRef("auto", "."); len(ref) != 40 && ref != "HEAD" {
		t.Errorf("auto = %q, want a commit or HEAD", ref)
	}
	if sourceRef("", t.TempDir()) != "HEAD" {
		t.Error("outside git the ref is HEAD")
	}
}
