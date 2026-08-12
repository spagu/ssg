package models

import (
	"encoding/json"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestParsePrettyURLMode(t *testing.T) {
	cases := map[string]PrettyURLMode{
		"": PrettyOff, "false": PrettyOff, "off": PrettyOff, "no": PrettyOff,
		"true": PrettyStripSlash, "yes": PrettyStripSlash, "strip-slash": PrettyStripSlash,
		"strip": PrettyStrip, " STRIP ": PrettyStrip, "TRUE": PrettyStripSlash,
	}
	for in, want := range cases {
		got, err := ParsePrettyURLMode(in)
		if err != nil || got != want {
			t.Errorf("ParsePrettyURLMode(%q) = %v, %v; want %v", in, got, err, want)
		}
	}
	if _, err := ParsePrettyURLMode("bogus"); err == nil {
		t.Fatal("unknown value must error")
	}
}

func TestPrettyURLModeEnabled(t *testing.T) {
	if PrettyOff.Enabled() || !PrettyStrip.Enabled() || !PrettyStripSlash.Enabled() {
		t.Fatal("Enabled truth table wrong")
	}
}

// TestServedURL covers the host-behaviour matrix that canonical/og:url/sitemap
// rely on (#103): Cloudflare-style strip vs strip-slash, index collapsing,
// non-HTML passthrough and query/fragment carriage.
func TestServedURL(t *testing.T) {
	cases := []struct {
		mode PrettyURLMode
		in   string
		want string
	}{
		// off: everything passes through.
		{PrettyOff, "/docs/intro.html", "/docs/intro.html"},

		// strip (Cloudflare Pages): drop .html, NO trailing slash.
		{PrettyStrip, "/docs/intro.html", "/docs/intro"},
		{PrettyStrip, "/docs/index.html", "/docs"},
		{PrettyStrip, "/index.html", "/"},
		{PrettyStrip, "/styles.css", "/styles.css"}, // non-HTML untouched
		{PrettyStrip, "/docs/", "/docs/"},           // directory URL untouched
		{PrettyStrip, "/docs/intro.html?x=1", "/docs/intro?x=1"},
		{PrettyStrip, "/docs/intro.html#top", "/docs/intro#top"},
		{PrettyStrip, "/INTRO.HTML", "/INTRO"}, // case-insensitive extension

		// strip-slash (historical true): drop .html, ADD trailing slash.
		{PrettyStripSlash, "/docs/intro.html", "/docs/intro/"},
		{PrettyStripSlash, "/docs/index.html", "/docs/"},
		{PrettyStripSlash, "/index.html", "/"},
		{PrettyStripSlash, "/docs/intro.html#s", "/docs/intro/#s"},
		{PrettyStripSlash, "/download.zip", "/download.zip"},
	}
	for _, c := range cases {
		if got := c.mode.ServedURL(c.in); got != c.want {
			t.Errorf("%s.ServedURL(%q) = %q, want %q", c.mode, c.in, got, c.want)
		}
	}
	// Bare fragment/query: clean part empty → untouched.
	if got := PrettyStrip.ServedURL("#frag"); got != "#frag" {
		t.Errorf("bare fragment = %q", got)
	}
}

func TestPrettyURLModeUnmarshalYAML(t *testing.T) {
	var m PrettyURLMode
	if err := yaml.Unmarshal([]byte("true"), &m); err != nil || m != PrettyStripSlash {
		t.Fatalf("yaml true = %v, %v", m, err)
	}
	if err := yaml.Unmarshal([]byte("strip"), &m); err != nil || m != PrettyStrip {
		t.Fatalf("yaml strip = %v, %v", m, err)
	}
	if err := yaml.Unmarshal([]byte(`"off"`), &m); err != nil || m != PrettyOff {
		t.Fatalf("yaml off = %v, %v", m, err)
	}
	if err := yaml.Unmarshal([]byte("bogus"), &m); err == nil {
		t.Fatal("yaml bogus must error")
	}
	if err := yaml.Unmarshal([]byte("[1,2]"), &m); err == nil {
		t.Fatal("yaml non-scalar must error")
	}
}

func TestPrettyURLModeUnmarshalJSON(t *testing.T) {
	var m PrettyURLMode
	if err := json.Unmarshal([]byte("true"), &m); err != nil || m != PrettyStripSlash {
		t.Fatalf("json true = %v, %v", m, err)
	}
	if err := json.Unmarshal([]byte("false"), &m); err != nil || m != PrettyOff {
		t.Fatalf("json false = %v, %v", m, err)
	}
	if err := json.Unmarshal([]byte(`"strip"`), &m); err != nil || m != PrettyStrip {
		t.Fatalf("json strip = %v, %v", m, err)
	}
	if err := json.Unmarshal([]byte(`"bogus"`), &m); err == nil {
		t.Fatal("json bogus must error")
	}
}

func TestPrettyURLModeUnmarshalText(t *testing.T) {
	var m PrettyURLMode
	if err := m.UnmarshalText([]byte("strip-slash")); err != nil || m != PrettyStripSlash {
		t.Fatalf("text strip-slash = %v, %v", m, err)
	}
	if err := m.UnmarshalText([]byte("nope")); err == nil {
		t.Fatal("text nope must error")
	}
}

// TestResolveTags: numeric WordPress tag ids resolve to names via metadata
// `tags` (recording the canonical slug); hand-written names and unknown ids
// pass through untouched (issue #27).
func TestResolveTags(t *testing.T) {
	tags := map[int]Category{
		7: {ID: 7, Name: "Release", Slug: "release-notes"},
		9: {ID: 9, Name: ""}, // empty name: id passes through
	}
	slugs := map[string]string{}
	p := &Page{Tags: []string{"7", " 9 ", "42", "hand-written"}}
	resolveTags(p, tags, slugs)
	if p.Tags[0] != "Release" {
		t.Fatalf("id 7 not resolved: %v", p.Tags)
	}
	if p.Tags[1] != " 9 " || p.Tags[2] != "42" || p.Tags[3] != "hand-written" {
		t.Fatalf("passthroughs broken: %v", p.Tags)
	}
	if slugs["release"] != "release-notes" {
		t.Fatalf("canonical slug not recorded: %v", slugs)
	}
	// Empty maps → no-op.
	resolveTags(&Page{Tags: []string{"7"}}, nil, slugs)
	resolveTags(&Page{}, tags, slugs)
}

func TestSplitURLHelpers(t *testing.T) {
	if base, tail := splitURLBase("/a/b/c.html"); base != "/a/b/" || tail != "c.html" {
		t.Fatalf("splitURLBase = %q %q", base, tail)
	}
	if base, tail := splitURLBase("nofslash"); base != "" || tail != "nofslash" {
		t.Fatalf("splitURLBase bare = %q %q", base, tail)
	}
	if clean, suffix := splitURLSuffix("/x?a=1#b"); clean != "/x" || suffix != "?a=1#b" {
		t.Fatalf("splitURLSuffix = %q %q", clean, suffix)
	}
	if clean, suffix := splitURLSuffix("/plain"); clean != "/plain" || suffix != "" {
		t.Fatalf("splitURLSuffix plain = %q %q", clean, suffix)
	}
}
