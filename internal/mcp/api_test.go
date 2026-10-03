package mcp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spagu/ssg/internal/apimodel"
)

// apiServer writes an api.json and returns a server over it.
func apiServer(t *testing.T) (*Server, string) {
	t.Helper()
	out := t.TempDir()
	lexer := "core/src/lex#Lexer"
	api := &apimodel.API{Packages: []*apimodel.Package{{Name: "core", Modules: []*apimodel.Module{{ID: "core/src/lex", Path: "src/lex",
		Doc: &apimodel.Doc{Summary: "Lexing."}, Symbols: []*apimodel.Symbol{
			{ID: lexer, Name: "Lexer", Kind: apimodel.KindClass, Doc: &apimodel.Doc{Summary: "Splits text."},
				Members: []*apimodel.Symbol{{ID: lexer + ".next", Name: "next", Kind: apimodel.KindMethod, Doc: &apimodel.Doc{Summary: "Next token."},
					Signatures: []*apimodel.Signature{{Returns: apimodel.Named("Token", "")}}}}},
			{ID: "core/src/lex#lex", Name: "lex", Kind: apimodel.KindFunction},
			{ID: "core/src/lex#tidy", Name: "tidy", Kind: apimodel.KindFunction, Doc: &apimodel.Doc{Summary: "Lex helper."}},
		}}}}}}
	data, err := apimodel.Encode(api)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(out, apiFile), data, 0o644); err != nil {
		t.Fatal(err)
	}
	return NewServer(Options{Root: t.TempDir(), OutputDir: out, Roles: map[string]bool{"content": true}}), out
}

func TestAPISearch(t *testing.T) {
	s, _ := apiServer(t)
	m := decode(t, call(t, s, "api_search", map[string]any{"query": "lex"}))
	res := m["results"].([]any)
	var names []string
	for _, r := range res {
		names = append(names, r.(map[string]any)["name"].(string))
	}
	// Name matches first, shortest first; then summary matches.
	if strings.Join(names, ",") != "lex,Lexer,tidy" {
		t.Errorf("results = %v", names)
	}
	m = decode(t, call(t, s, "api_search", map[string]any{"query": "e", "kind": "method", "limit": 1}))
	if m["count"].(float64) != 1 {
		t.Errorf("kind+limit = %v", m)
	}
}

func TestAPISymbolAndModule(t *testing.T) {
	s, _ := apiServer(t)
	m := decode(t, call(t, s, "api_symbol", map[string]any{"id": "core/src/lex#Lexer"}))
	members := m["members"].([]any)
	if len(members) != 1 || members[0].(map[string]any)["id"] != "core/src/lex#Lexer.next" {
		t.Errorf("members = %v", members)
	}
	if sym := m["symbol"].(map[string]any); sym["members"] != nil {
		t.Error("members must come as entries, not in full")
	}
	m = decode(t, call(t, s, "api_symbol", map[string]any{"id": "core/src/lex#Lexer.next"}))
	if m["parent"] != "core/src/lex#Lexer" {
		t.Errorf("parent = %v", m["parent"])
	}
	m = decode(t, call(t, s, "api_module", map[string]any{"id": "core/src/lex"}))
	if len(m["exports"].([]any)) != 3 || m["path"] != "src/lex" {
		t.Errorf("module = %v", m)
	}
	for _, tc := range []struct{ tool, id, want string }{
		{"api_symbol", "core/src/lex#Nope", "api_search"},
		{"api_module", "core/nope", "no documented module"},
	} {
		if r := call(t, s, tc.tool, map[string]any{"id": tc.id}); !r.IsError || !strings.Contains(text(r), tc.want) {
			t.Errorf("%s(%s) = %s", tc.tool, tc.id, text(r))
		}
	}
}

func TestAPIWithoutModel(t *testing.T) {
	s := NewServer(Options{Root: t.TempDir(), OutputDir: t.TempDir()})
	for _, tool := range []string{"api_search", "api_symbol", "api_module"} {
		if r := call(t, s, tool, map[string]any{"query": "x", "id": "x"}); !r.IsError || !strings.Contains(text(r), "api_docs") {
			t.Errorf("%s without api.json = %s", tool, text(r))
		}
	}
	if _, _, err := (&Server{}).loadAPI(); err == nil {
		t.Error("no output directory must be reported")
	}
}

func TestAPICacheReloadsOnChange(t *testing.T) {
	s, out := apiServer(t)
	if _, _, err := s.loadAPI(); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(out, apiFile)
	if err := os.WriteFile(path, []byte(`{"schema": 99}`), 0o644); err != nil {
		t.Fatal(err)
	}
	later := time.Now().Add(time.Minute)
	_ = os.Chtimes(path, later, later)
	if _, _, err := s.loadAPI(); err == nil || !strings.Contains(err.Error(), "schema 99") {
		t.Errorf("a changed file must be re-read, got %v", err)
	}
	if err := os.WriteFile(path, []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}
	_ = os.Chtimes(path, later.Add(time.Minute), later.Add(time.Minute))
	if _, _, err := s.loadAPI(); err == nil {
		t.Error("broken JSON must be reported")
	}
}
