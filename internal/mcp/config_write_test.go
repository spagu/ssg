package mcp

import (
	"os"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// setKey calls designer_config_set and fails the test on a refusal.
func setKey(t *testing.T, s *Server, key, value string) {
	t.Helper()
	if r := call(t, s, "designer_config_set", map[string]any{"key": key, "value": value}); r.IsError {
		t.Fatalf("%s = %s: %s", key, value, text(r))
	}
}

// TestTheFeaturesAddedSinceAreSettable — the allow-list had not grown since
// #212, so an assistant could restyle code highlighting but not turn on story
// mode or publish the site graph (#410).
func TestTheFeaturesAddedSinceAreSettable(t *testing.T) {
	s, path := configServer(t, baseConfig, func(string) error { return nil })
	for _, key := range []string{"markdown_publish", "site_graph", "webmcp", "search_index",
		"clean_special_chars", "autolink_list_items", "strip_md_link_text"} {
		setKey(t, s, key, "true")
		if got := readFileString(t, path); !strings.Contains(got, key+": true") {
			t.Errorf("%s not written:\n%s", key, got)
		}
	}
	listing := text(call(t, s, "designer_config_read", nil))
	for _, key := range []string{"story", "listen", "og_cards", "image_formats", "marketing.og_image", "site_graph"} {
		if !strings.Contains(listing, key) {
			t.Errorf("designer_config_read does not offer %q", key)
		}
	}
}

// TestASwitchKeepsTheFeaturesSettings: turning story mode off must not throw
// away the slideshow the author configured.
func TestASwitchKeepsTheFeaturesSettings(t *testing.T) {
	s, path := configServer(t, baseConfig+"story:\n  sections: [posts]   # recipes too\n", nil)
	setKey(t, s, "story", "false")
	got := readFileString(t, path)
	var doc struct {
		Story map[string]any `yaml:"story"`
	}
	if err := yaml.Unmarshal([]byte(got), &doc); err != nil {
		t.Fatal(err)
	}
	if doc.Story["enabled"] != false || doc.Story["sections"] == nil {
		t.Errorf("story mapping lost or not switched:\n%s", got)
	}
	if !strings.Contains(got, "# recipes too") {
		t.Errorf("comment lost:\n%s", got)
	}
	if out := text(call(t, s, "designer_config_read", nil)); !strings.Contains(out, "enabled: false") {
		t.Errorf("read should show the switch:\n%s", out)
	}
}

// TestASwitchOnAnAbsentOrScalarKey: an absent feature gets a mapping (`listen`
// has no `listen: true` shorthand), a scalar one stays a scalar.
func TestASwitchOnAnAbsentOrScalarKey(t *testing.T) {
	s, path := configServer(t, baseConfig+"heading_anchors: false\n", nil)
	setKey(t, s, "listen", "true")
	setKey(t, s, "heading_anchors", "true")
	got := readFileString(t, path)
	if !strings.Contains(got, "listen:\n  enabled: true") {
		t.Errorf("absent switch not written as a mapping:\n%s", got)
	}
	if !strings.Contains(got, "heading_anchors: true") {
		t.Errorf("scalar switch not kept scalar:\n%s", got)
	}
	if r := call(t, s, "designer_config_set", map[string]any{"key": "print", "value": "maybe"}); !r.IsError {
		t.Error("a switch takes true or false only")
	}
}

// TestListsAndDottedKeys: `image_formats` replaces `webp:`'s job and takes a
// list; `marketing.*` is nested.
func TestListsAndDottedKeys(t *testing.T) {
	s, path := configServer(t, baseConfig+"marketing:\n  logo: /media/logo.png\n", nil)
	setKey(t, s, "image_formats", " webp, avif,, ")
	setKey(t, s, "marketing.og_image", "/media/share.png")
	setKey(t, s, "marketing.twitter_site", "@example")
	got := readFileString(t, path)
	var doc struct {
		ImageFormats []string          `yaml:"image_formats"`
		Marketing    map[string]string `yaml:"marketing"`
	}
	if err := yaml.Unmarshal([]byte(got), &doc); err != nil {
		t.Fatal(err)
	}
	if strings.Join(doc.ImageFormats, ",") != "webp,avif" {
		t.Errorf("image_formats = %v:\n%s", doc.ImageFormats, got)
	}
	if doc.Marketing["og_image"] != "/media/share.png" || doc.Marketing["logo"] != "/media/logo.png" ||
		doc.Marketing["twitter_site"] != "@example" {
		t.Errorf("marketing = %v:\n%s", doc.Marketing, got)
	}
	out := text(call(t, s, "designer_config_read", nil))
	if !strings.Contains(out, "[webp avif]") || !strings.Contains(out, "/media/share.png") {
		t.Errorf("read should show list and nested values:\n%s", out)
	}
}

// TestAMappingIsNotReplacedByAList: `outputs:` per content type would be lost
// to a flat list, so the write is refused and the file kept.
func TestAMappingIsNotReplacedByAList(t *testing.T) {
	s, path := configServer(t, baseConfig+"outputs:\n  post: [html, markdown]\n", nil)
	before, _ := os.ReadFile(path)
	r := call(t, s, "designer_config_set", map[string]any{"key": "outputs", "value": "html,json"})
	if !r.IsError || !strings.Contains(text(r), "mapping") {
		t.Errorf("replacing a mapping must be refused: %s", text(r))
	}
	if after, _ := os.ReadFile(path); string(after) != string(before) {
		t.Error("a refused write must leave the file alone")
	}
}

// TestSecretsUnderAllowedParentsStayRefused: allowing marketing.og_image does
// not open marketing.verification (search-console tokens).
func TestSecretsUnderAllowedParentsStayRefused(t *testing.T) {
	s, _ := configServer(t, baseConfig, nil)
	for _, key := range []string{"marketing", "marketing.verification", "marketing.verification.google", "story.slideshow"} {
		if r := call(t, s, "designer_config_set", map[string]any{"key": key, "value": "x"}); !r.IsError {
			t.Errorf("%q must be refused", key)
		}
	}
}

// TestDisplayAndLookupEdges covers the read helpers' fallbacks.
func TestDisplayAndLookupEdges(t *testing.T) {
	if got := displayConfigValue(map[string]any{"sections": []any{"posts"}}); got != "(settings in the file)" {
		t.Errorf("mapping without enabled = %q", got)
	}
	if _, ok := lookupConfigValue(map[string]any{"marketing": "flat"}, "marketing.logo"); ok {
		t.Error("a scalar parent has no children")
	}
	if configNodeKind([]byte("- a\n"), "story") != 0 || configNodeKind([]byte(":\t:"), "story") != 0 {
		t.Error("a file without a mapping has no keys")
	}
	if configNodeKind([]byte("marketing: flat\n"), "marketing.logo") != 0 {
		t.Error("a scalar parent has no children")
	}
}
