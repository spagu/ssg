package python

import (
	"strings"
	"testing"
)

// kinds renders tokens compactly: names and operators as text, strings as
// S(text), layout as N, I, D.
func kinds(toks []token) string {
	var parts []string
	for _, t := range toks {
		switch t.kind {
		case tString:
			parts = append(parts, "S("+t.text+")")
		case tNumber:
			parts = append(parts, "#"+t.text)
		case tNewline:
			parts = append(parts, "N")
		case tIndent:
			parts = append(parts, "I")
		case tDedent:
			parts = append(parts, "D")
		case tEOF:
		default:
			parts = append(parts, t.text)
		}
	}
	return strings.Join(parts, " ")
}

func TestTokenize(t *testing.T) {
	tests := []struct {
		name, src, want string
	}{
		{"comment", "x = 1  # def no(): pass\n", "x = #1 N"},
		{"prefixes", "a = rb'\\d' + F\"x\" + u'y' + br\"z\"\n", `a = S(rb'\d') + S(F"x") + S(u'y') + S(br"z") N`},
		{"triple", "s = '''a\nclass X:\n'''\n", "s = S('''a\nclass X:\n''') N"},
		{"escaped quote", `s = "a\"def x()"` + "\n", `s = S("a\"def x()") N`},
		{"fstring nested", `f"{a["k"]} {{x}} {b:{w}}"` + "\n", `S(f"{a["k"]} {{x}} {b:{w}}") N`},
		{"fstring multi", "f'''{x\n}'''\n", "S(f'''{x\n}''') N"},
		{"continuation", "x = 1 + \\\n    2\n", "x = #1 + #2 N"},
		{"brackets join", "f(a,\n  b)\n", "f ( a , b ) N"},
		{"indent", "if x:\n    y\nz\n", "if x : N I y N D z N"},
		{"tabs", "if x:\n\ty\n        z\n", "if x : N I y N z N D"},
		{"blank lines", "a\n\n   \n  # c\nb\n", "a N b N"},
		{"numbers", "1e-5 0x1E+2 .5 1_000j\n", "#1e-5 #0x1E + #2 #.5 #1_000j"},
		{"operators", "def f(*a, **k) -> None: ...\n", "def f ( * a , ** k ) -> None : ... N"},
		{"crlf", "a\r\nb\rc\n", "a N b N c N"},
		{"bom", "\ufeffx\n", "x N"},
		{"formfeed", "\fx\n", "x N"},
		{"eof dedent", "class A:\n  x", "class A : N I x N D"},
		{"unicode name", "zażółć = 1\n", "zażółć = #1 N"},
		{"empty", "", ""},
		{"string ends file", "'a'", "S('a') N"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			toks, diags := tokenize(tt.src)
			if got := strings.TrimSuffix(kinds(toks), " N"); got != strings.TrimSuffix(tt.want, " N") {
				t.Errorf("tokens = %q, want %q", got, tt.want)
			}
			if len(diags) > 0 {
				t.Errorf("diagnostics: %v", diags)
			}
		})
	}
}

func TestTokenizeDiagnostics(t *testing.T) {
	tests := []struct {
		name, src string
		line      int
		want      string
	}{
		{"string", "a = 1\nb = 'x\nc = 2\n", 2, "unterminated string"},
		{"triple", "a = '''x\n", 1, "unterminated string"},
		{"fstring field", "a = f'{x'\n", 1, "unterminated string"},
		{"fstring nested", "a = f'{\"x}'\n", 1, "unterminated string"},
		{"fstring eof", "a = f'''{x", 1, "unterminated string"},
		{"backslash eof", "a = 'x\\", 1, "unterminated string"},
		{"bracket", "a = (1,\n[2]\n", 1, "unterminated bracket"},
		{"closing", "a = 1)\n", 1, "unmatched closing bracket )"},
		{"dedent", "if x:\n    a\n  b\n", 3, "inconsistent dedent"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, diags := tokenize(tt.src)
			if len(diags) == 0 || diags[0].line != tt.line || !strings.HasPrefix(diags[0].msg, tt.want) {
				t.Errorf("diagnostics = %v, want %q at line %d", diags, tt.want, tt.line)
			}
		})
	}
}

func TestDecodeString(t *testing.T) {
	tests := []struct{ lit, want string }{
		{`"abc"`, "abc"},
		{`'a\'b\n\tc\\d\q'`, "a'b\n\tc\\d\\q"},
		{`r"a\nb"`, `a\nb`},
		{`"""doc"""`, "doc"},
		{`''''''`, ""},
		{`""`, ""},
		{"'a\\\nb'", "ab"},
		{`'`, ""},
		{`noquote`, "noquote"},
		{`"""ab`, `""ab`},
	}
	for _, tt := range tests {
		if got := decodeString(tt.lit); got != tt.want {
			t.Errorf("decodeString(%q) = %q, want %q", tt.lit, got, tt.want)
		}
	}
}
