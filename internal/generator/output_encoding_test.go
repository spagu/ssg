package generator

import (
	"strings"
	"testing"

	"github.com/spagu/ssg/internal/models"
)

func TestCleanSpecialChars(t *testing.T) {
	g := &Generator{config: Config{CleanSpecialChars: true}}
	in := "“Smart” ‘quotes’ — dash – en… ellipsis nbsp"
	want := "\"Smart\" 'quotes' -- dash - en... ellipsis nbsp"
	if got := g.cleanSpecialChars(in); got != want {
		t.Fatalf("clean:\n got:  %q\n want: %q", got, want)
	}
	// Zero-width characters are removed (built from code points so this test
	// file stays pure ASCII — a literal BOM would be illegal in Go source).
	zw := "a" + string(rune(0x200B)) + "b" + string(rune(0xFEFF)) + "c"
	if got := g.cleanSpecialChars(zw); got != "abc" {
		t.Fatalf("zero-width not stripped: %q", got)
	}
}

func TestCleanSpecialChars_LeavesCJKUntouched(t *testing.T) {
	g := &Generator{config: Config{CleanSpecialChars: true}}
	// Chinese, Japanese, Korean text plus CJK full-width punctuation must pass
	// through byte-for-byte — the filter only targets Western smart punctuation.
	cjk := "你好、世界。テスト（日本）안녕하세요"
	if got := g.cleanSpecialChars(cjk); got != cjk {
		t.Fatalf("CJK altered:\n got:  %q\n want: %q", got, cjk)
	}
}

func TestCleanSpecialChars_Disabled(t *testing.T) {
	g := &Generator{config: Config{CleanSpecialChars: false}}
	in := "“x”"
	if got := g.cleanSpecialChars(in); got != in {
		t.Fatalf("disabled filter must be a no-op, got %q", got)
	}
}

func TestNormalizeEncoding(t *testing.T) {
	cases := map[string]string{
		"": "utf-8", "utf-8": "utf-8", "UTF8": "utf-8",
		"utf-16le": "utf-16le", "utf16": "utf-16le", "utf-16": "utf-16le",
		"utf-16be": "utf-16be", "bogus": "utf-8",
	}
	for in, want := range cases {
		if got := normalizeEncoding(in); got != want {
			t.Errorf("normalizeEncoding(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestEncodeText_UTF16BOM(t *testing.T) {
	// UTF-8 passthrough.
	if got := encodeText("hi", "utf-8"); string(got) != "hi" {
		t.Fatalf("utf-8 must be passthrough, got %q", got)
	}
	// UTF-16LE: BOM 0xFF 0xFE, then 'A' as 0x41 0x00.
	le := encodeText("A", encodingUTF16LE)
	if len(le) < 4 || le[0] != 0xFF || le[1] != 0xFE || le[2] != 0x41 || le[3] != 0x00 {
		t.Fatalf("utf-16le bytes wrong: % x", le)
	}
	// UTF-16BE: BOM 0xFE 0xFF, then 'A' as 0x00 0x41.
	be := encodeText("A", encodingUTF16BE)
	if len(be) < 4 || be[0] != 0xFE || be[1] != 0xFF || be[2] != 0x00 || be[3] != 0x41 {
		t.Fatalf("utf-16be bytes wrong: % x", be)
	}
	// CJK round-trips through UTF-16 (no data loss).
	cjk := encodeText("你", encodingUTF16LE)
	if len(cjk) != 4 { // BOM + one BMP code unit
		t.Fatalf("CJK utf-16le length wrong: % x", cjk)
	}
}

func TestSetHTMLCharset(t *testing.T) {
	html := `<head><meta charset="utf-8"><title>x</title></head>`
	if got := setHTMLCharset(html, "utf-8"); got != html {
		t.Fatalf("utf-8 must not rewrite charset")
	}
	got := setHTMLCharset(html, encodingUTF16LE)
	if !strings.Contains(got, `<meta charset="utf-16">`) || strings.Contains(got, "utf-8") {
		t.Fatalf("charset not rewritten for utf-16: %s", got)
	}
}

func TestEncodingFor_SectionOverride(t *testing.T) {
	g := &Generator{config: Config{
		ContentDir:     "content",
		Source:         "site",
		OutputEncoding: "utf-8",
		OutputEncodingSections: map[string]string{
			"legacy":         "utf-16le",
			"legacy/archive": "utf-16be",
		},
	}}
	// A page under content/site/legacy/archive → longest prefix wins.
	p := models.Page{SourceDir: "content/site/legacy/archive", Slug: "x"}
	p.Link = "/legacy/archive/x/"
	if got := g.encodingFor(&p); got != "utf-16be" {
		t.Fatalf("longest-prefix section = %q, want utf-16be", got)
	}
	// A page under content/site/legacy → the shorter prefix.
	p2 := models.Page{SourceDir: "content/site/legacy"}
	if got := g.encodingFor(&p2); got != "utf-16le" {
		t.Fatalf("section = %q, want utf-16le", got)
	}
	// Unlisted section → global default.
	p3 := models.Page{SourceDir: "content/site/guides"}
	if got := g.encodingFor(&p3); got != "utf-8" {
		t.Fatalf("fallback = %q, want utf-8", got)
	}
	// nil page → global default.
	if got := g.encodingFor(nil); got != "utf-8" {
		t.Fatalf("nil page = %q, want utf-8", got)
	}
}

func TestCleanSpecialChars_GuillemetsPrimesMinus(t *testing.T) {
	g := &Generator{config: Config{CleanSpecialChars: true}}
	in := "«quote» 5′ 10″ 3−1"
	want := "\"quote\" 5' 10\" 3-1"
	if got := g.cleanSpecialChars(in); got != want {
		t.Fatalf("clean:\n got:  %q\n want: %q", got, want)
	}
}

func TestSetHTMLCharset_NoMetaIsNoop(t *testing.T) {
	html := `<html><head><title>x</title></head></html>`
	if got := setHTMLCharset(html, encodingUTF16LE); got != html {
		t.Fatalf("no <meta charset> present → must be a no-op, got: %s", got)
	}
}

func TestEncodingFor_HomeOverride(t *testing.T) {
	g := &Generator{config: Config{
		OutputEncoding:         "utf-8",
		OutputEncodingSections: map[string]string{"home": "utf-16be"},
	}}
	home := models.Page{Link: "/"}
	if got := g.encodingFor(&home); got != "utf-16be" {
		t.Fatalf("home override = %q, want utf-16be", got)
	}
}

func TestRenderRobots_EmptyUserAgentDefaultsToStar(t *testing.T) {
	got := renderRobots([]RobotsRule{{Allow: []string{"/"}}}, "example.com")
	if !strings.Contains(got, "User-agent: *\nAllow: /") {
		t.Fatalf("empty user_agent should default to *:\n%s", got)
	}
}

func TestRenderRobots(t *testing.T) {
	// Default (no rules) reproduces the historical allow-all.
	def := renderRobots(nil, "example.com")
	if !strings.Contains(def, "User-agent: *\nAllow: /\n") || !strings.Contains(def, "Sitemap: https://example.com/sitemap.xml") {
		t.Fatalf("default robots wrong:\n%s", def)
	}
	// Custom AI-crawler rules.
	rules := []RobotsRule{
		{UserAgent: "GPTBot", Allow: []string{"/"}},
		{UserAgent: "OAI-SearchBot", Allow: []string{"/"}},
		{UserAgent: "*", Disallow: []string{"/private/"}, CrawlDelay: 5},
	}
	got := renderRobots(rules, "example.com/")
	for _, want := range []string{
		"User-agent: GPTBot\nAllow: /",
		"User-agent: OAI-SearchBot\nAllow: /",
		"User-agent: *\nDisallow: /private/\nCrawl-delay: 5",
		"Sitemap: https://example.com/sitemap.xml",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in:\n%s", want, got)
		}
	}
}
