package generator

// Cloudflare rejects a _routes.json whose rules overlap, and the arrangement
// docs/WORKERS.md recommends produced exactly that (#252).

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestCollapseRouteOverlaps: the shapes a worker list actually produces.
func TestCollapseRouteOverlaps(t *testing.T) {
	cases := []struct {
		name string
		in   []string
		want []string
	}{
		{
			// The reported case: two route-owning workers plus a middleware one.
			name: "a splat absorbs the specific rules",
			in:   []string{"/api/contact", "/api/consent/*", "/api/*"},
			want: []string{"/api/*"},
		},
		{
			name: "a narrower splat folds into a broader one",
			in:   []string{"/api/*", "/api/consent/*"},
			want: []string{"/api/*"},
		},
		{
			name: "the broadest splat wins outright",
			in:   []string{"/api/*", "/consent/*", "/*"},
			want: []string{"/*"},
		},
		{
			name: "rules that do not overlap are all kept",
			in:   []string{"/api/*", "/consent/*", "/health"},
			want: []string{"/api/*", "/consent/*", "/health"},
		},
		{
			name: "exact rules alone are left alone",
			in:   []string{"/api/contact", "/api/consent"},
			want: []string{"/api/contact", "/api/consent"},
		},
		{
			name: "a sibling prefix is not a parent",
			in:   []string{"/api/*", "/apiary"},
			want: []string{"/api/*", "/apiary"},
		},
		{name: "empty", in: nil, want: []string{}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := collapseRouteOverlaps(c.in)
			if strings.Join(got, ",") != strings.Join(c.want, ",") {
				t.Errorf("collapse(%v) = %v, want %v", c.in, got, c.want)
			}
			// Idempotent: renderRoutesJSON collapses again as a last defence, and
			// a second pass must not eat anything more.
			if again := collapseRouteOverlaps(got); strings.Join(again, ",") != strings.Join(got, ",") {
				t.Errorf("not idempotent: %v then %v", got, again)
			}
		})
	}
}

// TestRenderRoutesJSONNeverWritesAnOverlap: the generator writes this file
// itself, so it can simply not write an invalid one.
func TestRenderRoutesJSONNeverWritesAnOverlap(t *testing.T) {
	raw, err := renderRoutesJSON(
		[]string{"/api/contact", "/api/consent/*", "/api/*"},
		[]string{"/assets/*", "/assets/img/logo.png"},
	)
	if err != nil {
		t.Fatalf("renderRoutesJSON: %v", err)
	}
	var doc routesJSON
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if strings.Join(doc.Include, ",") != "/api/*" {
		t.Errorf("include = %v, want [/api/*]", doc.Include)
	}
	if strings.Join(doc.Exclude, ",") != "/assets/*" {
		t.Errorf("exclude = %v, want [/assets/*]", doc.Exclude)
	}
	// Include and exclude are collapsed separately: Cloudflare applies the rule
	// to each list on its own, and a splat in one says nothing about the other.
	raw, err = renderRoutesJSON([]string{"/api/contact"}, []string{"/api/*"})
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	if len(doc.Include) != 1 || doc.Include[0] != "/api/contact" {
		t.Errorf("an exclude splat must not eat an include rule: %v", doc.Include)
	}
}

// TestWorkerRoutesCollapseIsReported: the published file no longer matches what
// the config asked for, and silence would leave that to be found by reading the
// output.
func TestWorkerRoutesCollapseIsReported(t *testing.T) {
	g := newTestGen(t, "")
	out := captureBuildOutput(t, func() {
		g.reportCollapsedRoutes("include",
			[]string{"/api/contact", "/api/consent/*", "/api/*"},
			[]string{"/api/*"})
	})

	for _, want := range []string{"/api/contact", "/api/consent/*", "/api/*", "Cloudflare"} {
		if !strings.Contains(out, want) {
			t.Errorf("the report does not mention %s:\n%s", want, out)
		}
	}
	// Nothing collapsed, nothing said.
	quiet := captureBuildOutput(t, func() {
		g.reportCollapsedRoutes("include", []string{"/api/*"}, []string{"/api/*"})
	})
	if quiet != "" {
		t.Errorf("an unchanged list printed %q", quiet)
	}
}

// TestCollapsedRouteReportReadsForOneRule: the sentence has to be grammatical
// whether one rule was absorbed or several.
func TestCollapsedRouteReportReadsForOneRule(t *testing.T) {
	g := newTestGen(t, "")
	out := captureBuildOutput(t, func() {
		g.reportCollapsedRoutes("include", []string{"/api/contact", "/api/*"}, []string{"/api/*"})
	})
	if !strings.Contains(out, "/api/contact is already covered") {
		t.Errorf("singular reads wrong:\n%s", out)
	}
	// --quiet keeps the build quiet, like every other report.
	g.config.Quiet = true
	if q := captureBuildOutput(t, func() {
		g.reportCollapsedRoutes("include", []string{"/a", "/*"}, []string{"/*"})
	}); q != "" {
		t.Errorf("--quiet printed %q", q)
	}
}
