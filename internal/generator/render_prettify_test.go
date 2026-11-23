package generator

import (
	"strings"
	"testing"
)

// TestPrettifyKeepsWhitespaceSensitiveElements guards #438: pretty_html must
// leave the content of <pre> and <textarea> (and the raw text of <script> and
// <style>) byte for byte, while still tidying the markup around them.
func TestPrettifyKeepsWhitespaceSensitiveElements(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "blank line inside fenced code block",
			input:    "<div>\n\n<pre><code>func a() {}\n\nfunc b() {}\n</code></pre>\n\n</div>",
			expected: "<div>\n<pre><code>func a() {}\n\nfunc b() {}\n</code></pre>\n</div>\n",
		},
		{
			name:     "several consecutive blank lines",
			input:    "<pre>a\n\n\n\nb</pre>",
			expected: "<pre>a\n\n\n\nb</pre>\n",
		},
		{
			name:     "leading and trailing blank lines inside the element",
			input:    "<pre>\n\na\n\n</pre>",
			expected: "<pre>\n\na\n\n</pre>\n",
		},
		{
			name:     "whitespace-only lines and trailing spaces",
			input:    "<pre>a  \n   \n\t\nb\t</pre>",
			expected: "<pre>a  \n   \n\t\nb\t</pre>\n",
		},
		{
			name:     "CRLF inside kept, outside normalised",
			input:    "<p>x</p>\r\n\r\n<pre>a\r\n\r\nb</pre>\r\n",
			expected: "<p>x</p>\n<pre>a\r\n\r\nb</pre>\n",
		},
		{
			name:     "several blocks on one page",
			input:    "<pre>1\n\n2</pre>\n\n<p>mid</p>\n\n<pre>3\n\n4</pre>",
			expected: "<pre>1\n\n2</pre>\n<p>mid</p>\n<pre>3\n\n4</pre>\n",
		},
		{
			name:     "attributes on the tag",
			input:    "<pre class=\"x\" data-lang=\"go\">a\n\nb</pre>",
			expected: "<pre class=\"x\" data-lang=\"go\">a\n\nb</pre>\n",
		},
		{
			name:     "uppercase tags",
			input:    "<PRE>a\n\nb</PRE>\n\n<Pre>c\n\nd</pRe>",
			expected: "<PRE>a\n\nb</PRE>\n<Pre>c\n\nd</pRe>\n",
		},
		{
			name:     "indented block keeps its prefix on the same line",
			input:    "<div>\n    <pre>a\n\nb</pre>   \n</div>",
			expected: "<div>\n    <pre>a\n\nb</pre>\n</div>\n",
		},
		{
			name:     "textarea",
			input:    "<form>\n\n<textarea name=\"t\">line\n\n  \nend</textarea>\n\n</form>",
			expected: "<form>\n<textarea name=\"t\">line\n\n  \nend</textarea>\n</form>\n",
		},
		{
			name:     "script template literal",
			input:    "<script>\nconst s = `a\n\nb`;\n</script>\n\n<p>x</p>",
			expected: "<script>\nconst s = `a\n\nb`;\n</script>\n<p>x</p>\n",
		},
		{
			name:     "style",
			input:    "<style>\na { }\n\nb { }\n</style>",
			expected: "<style>\na { }\n\nb { }\n</style>\n",
		},
		{
			name:     "pre written inside a script is part of the script",
			input:    "<script>var s = \"<pre>\";\n\n</script>\n\n<p>x</p>",
			expected: "<script>var s = \"<pre>\";\n\n</script>\n<p>x</p>\n",
		},
		{
			name:     "unclosed pre does not swallow the page",
			input:    "<p>a</p>\n\n<pre>x\n\ny\n\n<p>b</p>\n",
			expected: "<p>a</p>\n<pre>x\ny\n<p>b</p>\n",
		},
		{
			name:     "placeholder-like text in content is restored literally",
			input:    "<pre>\x00prettify0\x00\n\n</pre>\n\n<pre>b\n\n</pre>",
			expected: "<pre>\x00prettify0\x00\n\n</pre>\n<pre>b\n\n</pre>\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := prettifyHTMLString(tt.input); got != tt.expected {
				t.Errorf("prettifyHTMLString(%q)\n got  %q\n want %q", tt.input, got, tt.expected)
			}
		})
	}
}

// TestPrettifyPreContentIsByteIdentical checks the promise directly: every
// whitespace-sensitive element in the output equals the one in the input.
func TestPrettifyPreContentIsByteIdentical(t *testing.T) {
	input := "<main>\n\n  <pre class=\"chroma\"><code>\n\nfunc a() {\r\n\r\n}\n   \n\n</code></pre>\n\n" +
		"<textarea>\n\n x \n\n</textarea>\n\n</main>\n\n"
	want := minPreserveTagRe.FindAllString(input, -1)
	got := minPreserveTagRe.FindAllString(prettifyHTMLString(input), -1)
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("preserved elements changed:\n got  %q\n want %q", got, want)
	}
}
