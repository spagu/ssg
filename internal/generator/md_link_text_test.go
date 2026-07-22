package generator

import (
	"strings"
	"testing"
)

func TestStripMdLinkText(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "bare filename link text loses .md",
			in:   `<a href="/configuration/">CONFIGURATION.md</a>`,
			want: `<a href="/configuration/">CONFIGURATION</a>`,
		},
		{
			name: "href anchor preserved, text stripped",
			in:   `<a href="/configuration/#mddb-content">CONFIGURATION.md</a>`,
			want: `<a href="/configuration/#mddb-content">CONFIGURATION</a>`,
		},
		{
			name: "path-style filename",
			in:   `<a href="/x/">docs/TEMPLATES.md</a>`,
			want: `<a href="/x/">docs/TEMPLATES</a>`,
		},
		{
			name: "inline code is not a link — untouched",
			in:   `<p>see <code>CONFIGURATION.md</code> for keys</p>`,
			want: `<p>see <code>CONFIGURATION.md</code> for keys</p>`,
		},
		{
			name: "prose mention outside an anchor — untouched",
			in:   `<p>edit README.md then rebuild</p>`,
			want: `<p>edit README.md then rebuild</p>`,
		},
		{
			name: "anchor text with surrounding words — untouched",
			in:   `<a href="/x/">see CONFIGURATION.md here</a>`,
			want: `<a href="/x/">see CONFIGURATION.md here</a>`,
		},
		{
			name: "anchor wrapping inline code — untouched (text does not start with filename)",
			in:   `<a href="/x/"><code>CONFIGURATION.md</code></a>`,
			want: `<a href="/x/"><code>CONFIGURATION.md</code></a>`,
		},
		{
			name: "non-md link text — untouched",
			in:   `<a href="/x/">Configuration</a>`,
			want: `<a href="/x/">Configuration</a>`,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := stripMdLinkText(c.in); got != c.want {
				t.Fatalf("stripMdLinkText:\n got:  %s\n want: %s", got, c.want)
			}
		})
	}
}

func TestStripMdLinkText_MultipleInOnePage(t *testing.T) {
	in := `<p>See <a href="/configuration/">CONFIGURATION.md</a> and ` +
		`<a href="/templates/">TEMPLATES.md</a>.</p>`
	out := stripMdLinkText(in)
	if strings.Contains(out, ".md</a>") {
		t.Fatalf("some link text kept .md:\n%s", out)
	}
	if !strings.Contains(out, ">CONFIGURATION</a>") || !strings.Contains(out, ">TEMPLATES</a>") {
		t.Fatalf("expected both texts stripped:\n%s", out)
	}
}
