package php

import (
	"strings"
	"testing"
)

// texts renders tokens as "kind:text" for comparison.
func texts(toks []token) string {
	names := map[tokKind]string{tokIdent: "id", tokVar: "var", tokString: "str", tokNumber: "num",
		tokPunct: "p", tokDoc: "doc", tokAttr: "attr"}
	parts := make([]string, len(toks))
	for i, t := range toks {
		parts[i] = names[t.kind] + ":" + t.text
	}
	return strings.Join(parts, " ")
}

func TestTokenize(t *testing.T) {
	tests := []struct {
		name, src, want string
	}{
		{"html outside php", "<b>{</b><?php $a; ?><i>}</i><?= 1 ?>", "var:$a p:; num:1"},
		{"short open tag", "<? $a ?>", "var:$a"},
		{"no php at all", "<p>plain</p>", ""},
		{"names", `<?php \Foo\Bar namespace\x $$y`, `id:\Foo\Bar id:namespace\x p:$ var:$y`},
		{"numbers", "<?php 1_000 0x1F 3.14", "num:1_000 num:0x1F num:3.14"},
		{"operators", "<?php a::b ?-> => -> ?? ... <<", "id:a p::: id:b p:?-> p:=> p:-> p:?? p:... p:< p:<"},
		{"strings", `<?php 'it\'s }' "a \" {" ` + "`ls }`", `str:'it\'s }' str:"a \" {" str:` + "`ls }`"},
		{"line comments", "<?php # {\n// }\nx // ?> y", "id:x"},
		{"comment closes php", "<?php // c ?> y <?php z", "id:z"},
		{"block comments", "<?php /* { */ /**/ /***/ x", "id:x"},
		{"docblock", "<?php /** Doc. */ x", "doc: Doc.  id:x"},
		{"attribute", `<?php #[A(']'), B([1])] x`, `attr:#[A(']'), B([1])] id:x`},
		{"heredoc", "<?php <<<EOT\n  a { }\n  EOTX\n  EOT;\nx", "str:<<<EOT\n  a { }\n  EOTX\n  EOT p:; id:x"},
		{"quoted heredoc", "<?php <<<\"E\"\n}\nE;", "str:<<<\"E\"\n}\nE p:;"},
		{"nowdoc", "<?php <<<'E'\n{\nE\n", "str:<<<'E'\n{\nE"},
		{"heredoc at end", "<?php <<<E\nE", "str:<<<E\nE"},
		{"shift operator", "<?php $a <<< 2;", "var:$a p:<< p:< num:2 p:;"},
		{"utf8 name", "<?php $zażółć", "var:$zażółć"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			toks, err := tokenize(tt.src)
			if err != nil {
				t.Fatalf("tokenize: %v", err)
			}
			if got := texts(toks); got != tt.want {
				t.Errorf("got  %q\nwant %q", got, tt.want)
			}
		})
	}
}

func TestTokenizeErrors(t *testing.T) {
	tests := []struct {
		name, src, want string
	}{
		{"string", "<?php\n\n'abc", "line 3: unterminated string '"},
		{"double string", `<?php "abc\"`, `line 1: unterminated string "`},
		{"comment", "<?php\n/* abc", "line 2: unterminated comment"},
		{"heredoc", "<?php <<<EOT\nabc\n", "line 1: unterminated heredoc EOT"},
		{"attribute", "<?php #[A(", "line 1: unterminated attribute"},
		{"string in attribute", "<?php #[A('x)]", "line 1: unterminated string in attribute"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := tokenize(tt.src)
			if err == nil || err.Error() != tt.want {
				t.Errorf("err = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestTokenLines(t *testing.T) {
	toks, err := tokenize("<p>\n</p>\n<?php\n/**\n * x\n */\nfunction f() {}")
	if err != nil {
		t.Fatal(err)
	}
	if toks[0].kind != tokDoc || toks[0].line != 4 || toks[1].line != 7 {
		t.Errorf("lines = %d %d", toks[0].line, toks[1].line)
	}
}

func TestCheckBalance(t *testing.T) {
	tests := []struct{ src, want string }{
		{"<?php f(a[1], {b});", ""},
		{"<?php {", "line 1: unbalanced {: never closed"},
		{"<?php\n)", "line 2: unbalanced ): nothing to close"},
		{"<?php [\n}", "line 2: unbalanced }: [ opened on line 1"},
	}
	for _, tt := range tests {
		toks, err := tokenize(tt.src)
		if err != nil {
			t.Fatal(err)
		}
		err = checkBalance(toks)
		if got := ""; err != nil {
			got = err.Error()
			if got != tt.want {
				t.Errorf("%q: %q, want %q", tt.src, got, tt.want)
			}
		} else if tt.want != "" {
			t.Errorf("%q: no error, want %q", tt.src, tt.want)
		}
	}
}
