package externalsource

import (
	"strings"
	"testing"
)

const sampleChangelog = `# Changelog

All notable changes to this project will be documented in this file.

## [Unreleased]

### Added
- Something brewing

## [1.8.16] - 2026-08-02

### Added
- ✨ **Feature one** — does a thing with ` + "`code`" + ` and a
  wrapped continuation line.
- Plain entry without a title

### Fixed
- **Bug fix** — stopped the crash

## [1.8.15] - 2026-08-01

### Changed
- Older change

## 1.8.14 - 2026-07-30

### Added
- Heading without brackets
`

// parseCL is a helper returning the parsed changelog map.
func parseCL(t *testing.T, doc string) map[string]interface{} {
	t.Helper()
	v, err := Parse("changelog", strings.NewReader(doc), CSVOptions{})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	m, ok := v.(map[string]interface{})
	if !ok {
		t.Fatalf("want map, got %T", v)
	}
	return m
}

func versionsOf(t *testing.T, m map[string]interface{}) []map[string]interface{} {
	t.Helper()
	vs, _ := m["versions"].([]map[string]interface{})
	return vs
}

// TestParseChangelogStructure covers #69: versions, dates, released flag, the
// latest/unreleased shortcuts and section keys.
func TestParseChangelogStructure(t *testing.T) {
	m := parseCL(t, sampleChangelog)
	vs := versionsOf(t, m)
	if len(vs) != 4 {
		t.Fatalf("want 4 versions, got %d", len(vs))
	}
	if vs[0]["version"] != "Unreleased" || vs[0]["released"] != false {
		t.Errorf("first version = %v", vs[0])
	}
	if vs[1]["version"] != "1.8.16" || vs[1]["date"] != "2026-08-02" || vs[1]["released"] != true {
		t.Errorf("1.8.16 = %v", vs[1])
	}
	// A heading without brackets still parses.
	if vs[3]["version"] != "1.8.14" || vs[3]["date"] != "2026-07-30" {
		t.Errorf("plain heading = %v", vs[3])
	}

	// latest skips Unreleased; unreleased is exposed separately.
	latest, _ := m["latest"].(map[string]interface{})
	if latest == nil || latest["version"] != "1.8.16" {
		t.Fatalf("latest = %v", m["latest"])
	}
	unrel, _ := m["unreleased"].(map[string]interface{})
	if unrel == nil || unrel["version"] != "Unreleased" {
		t.Errorf("unreleased = %v", m["unreleased"])
	}

	// Sections are lowercased and hold their entries.
	secs, _ := latest["sections"].(map[string]interface{})
	added, _ := secs["added"].([]interface{})
	fixed, _ := secs["fixed"].([]interface{})
	if len(added) != 2 || len(fixed) != 1 {
		t.Fatalf("added=%d fixed=%d", len(added), len(fixed))
	}
	// entries holds every bullet of the version regardless of section.
	all, _ := latest["entries"].([]interface{})
	if len(all) != 3 {
		t.Errorf("entries = %d, want 3", len(all))
	}
}

// TestParseChangelogEntries: title extraction, inline HTML rendering, wrapped
// continuation lines, and entries with no bold title.
func TestParseChangelogEntries(t *testing.T) {
	m := parseCL(t, sampleChangelog)
	latest, _ := m["latest"].(map[string]interface{})
	secs, _ := latest["sections"].(map[string]interface{})
	added, _ := secs["added"].([]interface{})

	first, _ := added[0].(map[string]interface{})
	if first["title"] != "Feature one" {
		t.Errorf("title = %q", first["title"])
	}
	if first["marker"] != "✨" {
		t.Errorf("marker = %q", first["marker"])
	}
	html, _ := first["html"].(string)
	if strings.HasPrefix(html, "<p>") || strings.HasSuffix(html, "</p>") {
		t.Errorf("enclosing <p> should be unwrapped: %q", html)
	}
	// html is the body only — the title must not appear twice.
	if strings.Contains(html, "Feature one") {
		t.Errorf("html should exclude the title: %q", html)
	}
	if !strings.Contains(html, "<code>code</code>") {
		t.Errorf("markdown not rendered: %q", html)
	}
	if !strings.Contains(html, "wrapped continuation line") {
		t.Errorf("wrapped line lost: %q", html)
	}
	// full keeps the whole entry in one piece.
	if f, _ := first["full"].(string); !strings.Contains(f, "<strong>Feature one</strong>") {
		t.Errorf("full should include the title: %q", f)
	}
	if txt, _ := first["text"].(string); !strings.Contains(txt, "**Feature one**") {
		t.Errorf("raw text should keep markdown: %q", txt)
	}

	second, _ := added[1].(map[string]interface{})
	if second["title"] != "" {
		t.Errorf("entry without bold must have empty title, got %q", second["title"])
	}
	if h, _ := second["html"].(string); h != "Plain entry without a title" {
		t.Errorf("plain html = %q", h)
	}

	// A title carrying inline markup is rendered, not left as raw Markdown.
	fx, _ := secs["fixed"].([]interface{})
	fixed0, _ := fx[0].(map[string]interface{})
	if fixed0["title"] != "Bug fix" || fixed0["html"] != "stopped the crash" {
		t.Errorf("fixed entry = %v", fixed0)
	}
}

// TestParseChangelogEdgeCases: empty input, a document with no versions, star
// bullets, and bullets appearing before any version heading.
func TestParseChangelogEdgeCases(t *testing.T) {
	if vs := versionsOf(t, parseCL(t, "")); len(vs) != 0 {
		t.Errorf("empty doc = %d versions", len(vs))
	}
	if m := parseCL(t, "# Changelog\n\nJust prose.\n"); len(versionsOf(t, m)) != 0 {
		t.Error("prose-only doc must yield no versions")
	}
	// Bullets before any heading are dropped, star bullets are accepted.
	m := parseCL(t, "- orphan bullet\n\n## [1.0.0] - 2026-01-01\n\n### Added\n* star entry\n")
	vs := versionsOf(t, m)
	if len(vs) != 1 {
		t.Fatalf("want 1 version, got %d", len(vs))
	}
	secs, _ := vs[0]["sections"].(map[string]interface{})
	added, _ := secs["added"].([]interface{})
	if len(added) != 1 {
		t.Fatalf("star bullet not parsed: %v", secs)
	}
	e, _ := added[0].(map[string]interface{})
	if e["html"] != "star entry" {
		t.Errorf("star entry = %v", e)
	}
	// A version with no ### sections still records its bullets under entries.
	m2 := parseCL(t, "## [2.0.0] - 2026-02-02\n\n- loose entry\n")
	vs2 := versionsOf(t, m2)
	all, _ := vs2[0]["entries"].([]interface{})
	if len(all) != 1 {
		t.Errorf("sectionless entries = %d, want 1", len(all))
	}
}

// TestChangelogFormatSupported: the format passes source validation and is
// rejected for an unknown one.
func TestChangelogFormatSupported(t *testing.T) {
	if !supportedFormats["changelog"] {
		t.Error("changelog must be a supported file format")
	}
	if _, err := Parse("markdown", strings.NewReader("x"), CSVOptions{}); err == nil {
		t.Error("unknown format must error")
	}
}
