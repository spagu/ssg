package generator

import (
	"strings"
	"testing"
)

func TestHeadingIDsFromVisibleText(t *testing.T) {
	g := newTestGen(t, "")
	md := "## <span style=\"color: #ffff00\"><strong>Why Baby Swimming</strong></span>\n\nBody.\n\n## Plain heading\n\nMore.\n"
	html := g.convertMarkdownToHTML(md)
	if !strings.Contains(html, `id="why-baby-swimming"`) {
		t.Fatalf("anchor not from visible text:\n%s", html)
	}
	if strings.Contains(html, "span-style") {
		t.Fatalf("markup leaked into the id:\n%s", html)
	}
	if !strings.Contains(html, `id="plain-heading"`) {
		t.Fatalf("a plain heading must keep its id:\n%s", html)
	}
}
