package parser

// HTML entities in plain-text frontmatter fields (1.8.30). A WordPress export
// carries them legitimately — the REST API serves title.rendered as HTML — but
// ssg puts those fields in <title>, meta description, og:title, feeds and
// JSON-LD, where a raw entity is visible to the reader.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func parseFrontmatterFields(t *testing.T, body string) (title, excerpt, description, content string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "page.md")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	page, err := ParseMarkdownFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return page.Title, page.Excerpt, page.Description, page.Content
}

func TestFrontmatterEntitiesDecoded(t *testing.T) {
	title, excerpt, description, content := parseFrontmatterFields(t, `---
title: "Domowe Kino &#8211; Warszawa"
description: "Akustyka &amp; adaptacja"
excerpt: "Ceny od 100&nbsp;z&#x142;"
---

Body keeps its own markup: &amp;copy; and &#8211; stay put.
`)
	if title != "Domowe Kino – Warszawa" {
		t.Errorf("title = %q", title)
	}
	if description != "Akustyka & adaptacja" {
		t.Errorf("description = %q", description)
	}
	if !strings.Contains(excerpt, "zł") { // numeric hex + nbsp both decoded
		t.Errorf("excerpt = %q", excerpt)
	}
	// The body is markup, not text: entities there are the author's and must
	// survive verbatim, or escaped HTML in a code sample would silently change.
	if !strings.Contains(content, "&amp;copy;") || !strings.Contains(content, "&#8211;") {
		t.Errorf("body entities must be untouched: %q", content)
	}
}

func TestFrontmatterWithoutEntitiesUnchanged(t *testing.T) {
	title, _, _, _ := parseFrontmatterFields(t, "---\ntitle: \"Plain title\"\n---\n\nBody.\n")
	if title != "Plain title" {
		t.Errorf("title = %q", title)
	}
	// A bare ampersand is not an entity and must stay a bare ampersand.
	title, _, _, _ = parseFrontmatterFields(t, "---\ntitle: \"Rock & Roll\"\n---\n\nBody.\n")
	if title != "Rock & Roll" {
		t.Errorf("bare ampersand mangled: %q", title)
	}
}
