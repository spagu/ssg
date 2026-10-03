package python

import (
	"testing"

	"github.com/spagu/ssg/internal/apimodel"
)

func TestParseTypeText(t *testing.T) {
	refs := map[string]string{"Token": "p/m#Token", "Lexer": "p/m#Lexer"}
	resolve := func(name string) string { return refs[name] }
	tests := []struct {
		src, want string
		kind      apimodel.TypeKind
	}{
		{"int", "int", apimodel.TypeName},
		{"None", "None", apimodel.TypeName},
		{"int | str | None", "int | str | None", apimodel.TypeUnion},
		{"Optional[int]", "int | None", apimodel.TypeUnion},
		{"typing.Optional[int | str]", "int | str | None", apimodel.TypeUnion},
		{"Union[int, Union[str, bytes]]", "int | str | bytes", apimodel.TypeUnion},
		{"list[Token]", "list<Token>", apimodel.TypeName},
		{"dict[str, list[int]]", "dict<str, list<int>>", apimodel.TypeName},
		{"Callable[[int, str], bool]", "Callable<[int, str], bool>", apimodel.TypeName},
		{"Callable[..., Any]", "Callable<..., Any>", apimodel.TypeName},
		{"Literal['a', 1]", "Literal<'a', 1>", apimodel.TypeName},
		{`"Lexer"`, "Lexer", apimodel.TypeName},
		{`"list['Token']"`, "list<Token>", apimodel.TypeName},
		{"collections.abc.Iterator[Token]", "collections.abc.Iterator<Token>", apimodel.TypeName},
		{"tuple[()]", "tuple<()>", apimodel.TypeName},
		{"int, optional", "int, optional", apimodel.TypeVerbatim},
		{"x[1](2)", "x[1](2)", apimodel.TypeVerbatim},
		{"a[b]]", "a[b]]", apimodel.TypeVerbatim},
		{`""`, `""`, apimodel.TypeVerbatim},
		{`"'unterminated"`, `"'unterminated"`, apimodel.TypeVerbatim},
		{"-1", "-1", apimodel.TypeVerbatim},
	}
	for _, tt := range tests {
		t.Run(tt.src, func(t *testing.T) {
			got := parseTypeText(tt.src, resolve)
			if got.Kind != tt.kind || got.String() != tt.want {
				t.Errorf("parseTypeText(%q) = %s %q, want %s %q", tt.src, got.Kind, got.String(), tt.kind, tt.want)
			}
		})
	}
	if got := parseTypeText("list[Token]", resolve); got.Args[0].Ref != "p/m#Token" {
		t.Errorf("Token ref = %q", got.Args[0].Ref)
	}
	if got := parseTypeText("", resolve); got != nil {
		t.Errorf("empty type = %v, want nil", got)
	}
}

func TestParseTypeDepth(t *testing.T) {
	nested := `"'\"int\"'"`
	got := parseTypeDepth(exprTokens(mustTokens(t, nested)), func(string) string { return "" }, maxTypeDepth)
	if got.Kind != apimodel.TypeVerbatim {
		t.Errorf("too deep = %v, want verbatim", got)
	}
}

// mustTokens tokenizes src, failing on diagnostics.
func mustTokens(t *testing.T, src string) []token {
	t.Helper()
	toks, diags := tokenize(src)
	if len(diags) > 0 {
		t.Fatalf("tokenize(%q): %v", src, diags)
	}
	return toks
}
