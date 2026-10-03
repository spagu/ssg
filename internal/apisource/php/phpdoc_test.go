package php

import (
	"strings"
	"testing"
)

func TestNormalizeDoc(t *testing.T) {
	tests := []struct {
		name, raw, want string
	}{
		{"param", "* @param int $n The count.", "* @param {int} n The count."},
		{"param without type", " * @param $n The count.", " * @param n The count."},
		{"variadic", " * @param string ...$parts", " * @param {string} ...parts"},
		{"by reference", " * @param array &$out Filled in.", " * @param {array} out Filled in."},
		{"generic type", " * @param array<string, int> $map", " * @param {array<string, int>} map"},
		{"shape type", " * @param array{a: int, b: string} $row", " * @param {array{a: int, b: string}} row"},
		{"callable type", " * @param callable(int): bool $f", " * @param {callable(int): bool} f"},
		{"already jsdoc", " * @param {int} n", " * @param {int} n"},
		{"return", " * @return Doc|null The doc.", " * @returns {Doc|null} The doc."},
		{"returns this", " * @return $this", " * @returns {$this}"},
		{"return no text", "@return", "@returns"},
		{"throws", " * @throws \\RuntimeException When broken.", " * @throws {\\RuntimeException} When broken."},
		{"see inline", " See {@see Doc} and {@link Kind}.", " See {@link Doc} and {@link Kind}."},
		{"other tag", " * @since 2.0", " * @since 2.0"},
		{"var removed", " * Text.\n * @var int", " * Text."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got, _, _ := normalizeDoc(tt.raw); got != tt.want {
				t.Errorf("got  %q\nwant %q", got, tt.want)
			}
		})
	}
}

func TestParseDocVar(t *testing.T) {
	tests := []struct {
		raw, typ, summary string
	}{
		{" @var int The count. ", "int", "The count."},
		{" @var int $n The count. ", "int", "The count."},
		{" @var string[] ", "string[]", ""},
		{" Own summary.\n * @var bool Ignored. ", "bool", "Own summary."},
	}
	for _, tt := range tests {
		info := parseDoc(tt.raw)
		if info.varType != tt.typ || info.Doc.Summary != tt.summary {
			t.Errorf("%q: type %q summary %q", tt.raw, info.varType, info.Doc.Summary)
		}
	}
}

func TestParseDocTags(t *testing.T) {
	info := parseDoc(`
	 * Parses.
	 *
	 * @param string $src The text.
	 * @param int ...$rest More.
	 * @return Doc The doc.
	 * @throws \InvalidArgumentException When empty.
	 * @throws LogicException
	 * @example
	 * parse('x');
	 * @deprecated Use other().
	 * @internal
	 `)
	if text, typ := info.param("src"); text != "The text." || typ != "string" {
		t.Errorf("param src = %q %q", text, typ)
	}
	if text, typ := info.param("rest"); text != "More." || typ != "int" {
		t.Errorf("param rest = %q %q", text, typ)
	}
	if text, typ := info.param("none"); text != "" || typ != "" {
		t.Errorf("param none = %q %q", text, typ)
	}
	if info.returnType() != "Doc" || info.Doc.Returns != "The doc." {
		t.Errorf("returns = %q %q", info.returnType(), info.Doc.Returns)
	}
	if got := strings.Join(info.Doc.Throws, "; "); got != `\InvalidArgumentException When empty.; LogicException` {
		t.Errorf("throws = %q", got)
	}
	if len(info.Doc.Examples) != 1 || !strings.HasPrefix(info.Doc.Examples[0], "```php\n") {
		t.Errorf("examples = %q", info.Doc.Examples)
	}
	if info.Doc.Deprecated == nil || *info.Doc.Deprecated != "Use other()." || !info.Mods.Internal {
		t.Errorf("deprecated/internal = %v %v", info.Doc.Deprecated, info.Mods.Internal)
	}
	if empty := parseDoc("  "); empty.returnType() != "" || empty.Doc == nil {
		t.Error("an empty docblock should parse to an empty comment")
	}
}

func TestDeprecatedAttr(t *testing.T) {
	tests := []struct {
		attrs []string
		text  string
		ok    bool
	}{
		{nil, "", false},
		{[]string{"#[Override]"}, "", false},
		{[]string{`#[\Deprecated]`}, "", true},
		{[]string{`#[Pure, deprecated("Use x.")]`}, "Use x.", true},
		{[]string{`#[\Deprecated(since: "2")]`}, "2", true},
		{[]string{`#[Deprecated(message: 'broken`}, "", true},
	}
	for _, tt := range tests {
		text, ok := deprecatedAttr(tt.attrs)
		if text != tt.text || ok != tt.ok {
			t.Errorf("%v: %q %v", tt.attrs, text, ok)
		}
	}
}
