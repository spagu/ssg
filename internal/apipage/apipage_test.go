package apipage

import (
	"strings"
	"testing"

	"github.com/spagu/ssg/internal/apimodel"
)

// fixture is a module with a class, a function, a type, an enum, a variable,
// an internal helper and a beta function.
func fixture() *apimodel.Package {
	mod := apimodel.ModuleID("core", "src/lex.js")
	lexer := apimodel.SymbolID(mod, "Lexer")
	dep := "use `scan`"
	return &apimodel.Package{Name: "core", Version: "1.0.0", Readme: "# core\n\nTokenises {@link Lexer}.",
		Modules: []*apimodel.Module{{ID: mod, Path: "src/lex", Doc: &apimodel.Doc{Summary: "The lexer."},
			Symbols: []*apimodel.Symbol{
				{ID: lexer, Name: "Lexer", Kind: apimodel.KindClass, Doc: &apimodel.Doc{Summary: "Splits | text."},
					Extends: []*apimodel.TypeRef{apimodel.Named("Base", "")}, Implements: []*apimodel.TypeRef{apimodel.Named("Iterable", "")},
					Source: &apimodel.Source{File: "src/lex.js", Line: 3},
					Members: []*apimodel.Symbol{
						{ID: apimodel.MemberID(lexer, "constructor"), Name: "constructor", Kind: apimodel.KindConstructor,
							Signatures: []*apimodel.Signature{{Params: []*apimodel.Param{{Name: "src", Type: apimodel.Named("string", ""), Doc: "the\ntext"}}}}},
						{ID: apimodel.MemberID(lexer, "pos"), Name: "pos", Kind: apimodel.KindProperty, Type: apimodel.Named("number", ""),
							Flags: apimodel.Flags{Readonly: true, Optional: true}},
						{ID: apimodel.MemberID(lexer, "next"), Name: "next", Kind: apimodel.KindMethod, Flags: apimodel.Flags{Async: true},
							Signatures: []*apimodel.Signature{{Params: []*apimodel.Param{
								{Name: "n", Type: apimodel.Named("number", ""), Optional: true, Default: "1"},
								{Name: "rest", Type: apimodel.Named("string", ""), Rest: true}},
								Returns: apimodel.Named("Token", "")}},
							Doc: &apimodel.Doc{Summary: "Next token.", Deprecated: &dep, Since: "0.2", Examples: []string{"```js\nl.next()\n```"},
								See: []string{"{@link Lexer}"}}},
						{ID: apimodel.MemberID(lexer, "secret"), Name: "secret", Kind: apimodel.KindMethod, Flags: apimodel.Flags{Internal: true}},
					}},
				{ID: apimodel.SymbolID(mod, "scan"), Name: "scan", Kind: apimodel.KindFunction,
					Signatures: []*apimodel.Signature{{Returns: apimodel.Named("void", "")}}, Flags: apimodel.Flags{Stability: apimodel.Beta}},
				{ID: apimodel.SymbolID(mod, "Token"), Name: "Token", Kind: apimodel.KindType, Type: apimodel.Named("string", "")},
				{ID: apimodel.SymbolID(mod, "VERSION"), Name: "VERSION", Kind: apimodel.KindVariable, Type: apimodel.Named("string", ""), Flags: apimodel.Flags{Readonly: true}},
				{ID: apimodel.SymbolID(mod, "count"), Name: "count", Kind: apimodel.KindVariable, Type: apimodel.Named("number", "")},
				{ID: apimodel.SymbolID(mod, "Mode"), Name: "Mode", Kind: apimodel.KindEnum, Members: []*apimodel.Symbol{
					{ID: apimodel.MemberID(apimodel.SymbolID(mod, "Mode"), "Fast"), Name: "Fast", Kind: apimodel.KindEnumMember, Doc: &apimodel.Doc{Summary: "Quick."}},
					{ID: apimodel.MemberID(apimodel.SymbolID(mod, "Mode"), "Slow"), Name: "Slow", Kind: apimodel.KindEnumMember}}},
				{ID: apimodel.SymbolID(mod, "helper"), Name: "helper", Kind: apimodel.KindFunction, Flags: apimodel.Flags{Internal: true}},
			}}}}
}

func options() Options {
	return Options{Base: "api/core", Readme: true,
		Links: func(md, _ string) string {
			return strings.ReplaceAll(md, "{@link Lexer}", "[Lexer](/api/core/src/lex/Lexer/)")
		},
		SourceURL: func(s *apimodel.Source) string {
			return "https://example.com/blob/main/" + s.File + "#L" + itoa(s.Line)
		}}
}

func itoa(n int) string { return strings.TrimSpace(strings.Repeat(" ", 0) + string(rune('0'+n))) }

func TestURLs(t *testing.T) {
	ix := apimodel.NewIndex(&apimodel.API{Packages: []*apimodel.Package{fixture()}})
	u := NewURLs("api/core", ix)
	for id, want := range map[string]string{
		"core/src/lex#Lexer":      "/api/core/src/lex/Lexer/",
		"core/src/lex#Lexer.next": "/api/core/src/lex/Lexer/#next",
		"core/src/lex#scan":       "/api/core/src/lex/#scan",
		"core/src/lex#Mode.Fast":  "/api/core/src/lex/#Mode.Fast",
	} {
		if got := u.Href(id); got != want {
			t.Errorf("Href(%q) = %q, want %q", id, got, want)
		}
	}
	if u.Index() != "/api/core/" || NewURLs("/", ix).Index() != "/" || topLevel("noanchor") != "noanchor" {
		t.Error("index / topLevel")
	}
}

func TestBuildPages(t *testing.T) {
	pages := Build(fixture(), options())
	var urls []string
	byURL := map[string]Page{}
	for _, p := range pages {
		urls = append(urls, p.URL)
		byURL[p.URL] = p
	}
	if strings.Join(urls, " ") != "/api/core/ /api/core/src/lex/ /api/core/src/lex/Lexer/" {
		t.Fatalf("pages = %v", urls)
	}
	index := byURL["/api/core/"]
	if index.Layout != LayoutIndex || !strings.Contains(index.Markdown, "Tokenises [Lexer](/api/core/src/lex/Lexer/).") ||
		strings.Contains(index.Markdown, "# core\n\nTokenises") || !strings.Contains(index.Markdown, "- [`src/lex`](/api/core/src/lex/) — The lexer.") {
		t.Errorf("index:\n%s", index.Markdown)
	}
	mod := byURL["/api/core/src/lex/"].Markdown
	for _, want := range []string{`<h2 id="kind-classes">Classes</h2>`, "- [`Lexer`](/api/core/src/lex/Lexer/) — Splits \\| text.",
		`<h3 id="scan"><code>scan()</code></h3>`, "`beta`", "```ts\nscan(): void\n```", "```ts\ntype Token = string\n```",
		"```ts\nconst VERSION: string\n```", "```ts\nlet count: number\n```", "- `Fast` — Quick.", "- `Slow`"} {
		if !strings.Contains(mod, want) {
			t.Errorf("module page lacks %q:\n%s", want, mod)
		}
	}
	if strings.Contains(mod, "helper") {
		t.Error("an internal symbol must be hidden by default")
	}
	class := byURL["/api/core/src/lex/Lexer/"]
	if class.Title != "Class Lexer — core" || byURL["/api/core/src/lex/"].Title != "Module src/lex — core" || kindTitle("") != "" {
		t.Errorf("titles: %q", class.Title)
	}
	for _, want := range []string{"Module [`src/lex`](/api/core/src/lex/)", "Extends `Base`", "Implements `Iterable`",
		`<h2 id="kind-constructor">Constructor</h2>`, `<h3 id="constructor"><code>constructor()</code></h3>`, "```ts\nconstructor(src: string)\n```", "| `src` | `string` | the text |", `<h2 id="kind-properties">Properties</h2>`, "`readonly` `optional`",
		"```ts\npos?: number\n```", `<h3 id="next"><code>next()</code></h3>`, "> **Deprecated.** use `scan`", "*Since 0.2.*", "**Example**",
		"next(n?: number, ...rest: string): Token", "| `n = 1` |", "| `...rest` |", "**Returns** `Token`", "`async`",
		"See also: [Lexer](/api/core/src/lex/Lexer/)", "[Source: src/lex.js:3](https://example.com/blob/main/src/lex.js#L3)"} {
		if !strings.Contains(class.Markdown, want) {
			t.Errorf("class page lacks %q:\n%s", want, class.Markdown)
		}
	}
	if strings.Contains(class.Markdown, "secret") || class.Symbol == nil || class.Module == nil || class.Summary != "Splits | text." {
		t.Errorf("class page: internal member shown or context missing")
	}
}

func TestVisibility(t *testing.T) {
	count := func(p *apimodel.Package) int {
		n := 0
		apimodel.Walk(p.Modules[0].Symbols, func(*apimodel.Symbol, *apimodel.Symbol) { n++ })
		return n
	}
	full := count(fixture())
	if got := count(Visible(fixture(), "all", nil)); got != full {
		t.Errorf("all = %d, want %d", got, full)
	}
	if got := count(Visible(fixture(), "internal", nil)); got != full {
		t.Errorf("internal = %d, want %d", got, full)
	}
	if got := count(Visible(fixture(), "", nil)); got != full-2 {
		t.Errorf("public = %d, want %d", got, full-2)
	}
	if got := count(Visible(fixture(), "", []string{"alpha"})); got != full-3 {
		t.Errorf("stability alpha only = %d, want %d (beta scan dropped)", got, full-3)
	}
	if got := count(Visible(fixture(), "", []string{"beta"})); got != full-2 {
		t.Errorf("stability beta = %d, want %d", got, full-2)
	}
}

func TestNoLinksNoSources(t *testing.T) {
	pkg := fixture()
	pkg.Version, pkg.Modules[0].Doc = "", nil
	pages := Build(pkg, Options{Base: "/api/"})
	if pages[0].Title != "core API" || !strings.Contains(pages[1].Markdown, "{@link") && strings.Contains(pages[1].Markdown, "Source:") {
		t.Errorf("without options: %s", pages[0].Title)
	}
	for _, p := range pages {
		if strings.Contains(p.Markdown, "[Source:") {
			t.Error("no source URL configured, no source links")
		}
	}
	if docSummary(nil, "fb") != "fb" || stripFirstHeading("no heading") != "no heading" || summarySuffix(&writer{opts: &Options{}}, &apimodel.Symbol{}) != "" {
		t.Error("helpers")
	}
	if got := symbolTitle(&apimodel.Symbol{Name: "<x>", Kind: apimodel.KindProperty}); got != "<code>&lt;x&gt;</code>" {
		t.Errorf("symbolTitle = %s", got)
	}
	w := &writer{opts: &Options{SourceURL: func(*apimodel.Source) string { return "" }}}
	w.source(&apimodel.Source{File: "a", Line: 1})
	w.doc(nil)
	if w.b.Len() != 0 {
		t.Error("an empty source URL adds no link")
	}
}

func TestResolver(t *testing.T) {
	other := &apimodel.Package{Name: "util", Modules: []*apimodel.Module{{ID: "util/fmt", Path: "fmt", Symbols: []*apimodel.Symbol{
		{ID: "util/fmt#Token", Name: "Token", Kind: apimodel.KindType}, {ID: "util/fmt#format", Name: "format", Kind: apimodel.KindFunction}}}}}
	r := NewResolver(&apimodel.API{Packages: []*apimodel.Package{fixture(), other}})
	from := "core/src/lex"
	for _, tc := range []struct{ from, target, want string }{
		{from, "core/src/lex#Lexer", "core/src/lex#Lexer"},
		{from, "Lexer", "core/src/lex#Lexer"},
		{from, " Lexer.next() ", "core/src/lex#Lexer.next"},
		{from, "Token", "core/src/lex#Token"},   // the module's own Token wins
		{"util/fmt", "Token", "util/fmt#Token"}, // and the other module's own there
		{from, "format", "util/fmt#format"},     // unique across the API
		{from, "src/lex#scan", "core/src/lex#scan"},
		{"", "Lexer", "core/src/lex#Lexer"},
	} {
		if got, ok := r.Resolve(tc.from, tc.target); !ok || got != tc.want {
			t.Errorf("Resolve(%q, %q) = %q, %v; want %q", tc.from, tc.target, got, ok, tc.want)
		}
	}
	for _, target := range []string{"", "Nope", "src/lex#Nope"} {
		if _, ok := r.Resolve(from, target); ok {
			t.Errorf("Resolve(%q) must fail", target)
		}
	}
	// Token exists in two modules: from a third place it is ambiguous.
	if _, ok := r.Resolve("x/y", "Token"); ok {
		t.Error("an ambiguous name must not resolve")
	}
}

func TestTypeHTML(t *testing.T) {
	href := func(id string) string { return map[string]string{"m#Token": "/api/m/#Token"}[id] }
	tok := apimodel.Named("Token", "m#Token")
	for _, tc := range []struct {
		t    *apimodel.TypeRef
		want string
	}{
		{nil, "unknown"},
		{apimodel.Named("Promise", "", &apimodel.TypeRef{Kind: apimodel.TypeArray, Args: []*apimodel.TypeRef{tok}}),
			`Promise&lt;<a href="/api/m/#Token">Token</a>[]&gt;`},
		{&apimodel.TypeRef{Kind: apimodel.TypeUnion, Args: []*apimodel.TypeRef{tok, apimodel.Named("null", "")}}, `<a href="/api/m/#Token">Token</a> | null`},
		{&apimodel.TypeRef{Kind: apimodel.TypeIntersection, Args: []*apimodel.TypeRef{apimodel.Named("A", ""), apimodel.Named("B", "")}}, "A &amp; B"},
		{&apimodel.TypeRef{Kind: apimodel.TypeTuple, Args: []*apimodel.TypeRef{apimodel.Named("A", ""), apimodel.Named("B", "")}}, "[A, B]"},
		{&apimodel.TypeRef{Kind: apimodel.TypeArray, Args: []*apimodel.TypeRef{{Kind: apimodel.TypeUnion, Args: []*apimodel.TypeRef{apimodel.Named("A", ""), apimodel.Named("B", "")}}}}, "(A | B)[]"},
		{&apimodel.TypeRef{Kind: apimodel.TypeArray}, "unknown[]"},
		{&apimodel.TypeRef{Kind: apimodel.TypeLiteral, Name: `"<x>"`}, `&#34;&lt;x&gt;&#34;`},
		{apimodel.Named("Gone", "m#Gone"), "Gone"},
	} {
		if got := TypeHTML(tc.t, href); got != tc.want {
			t.Errorf("TypeHTML = %q, want %q", got, tc.want)
		}
	}
	if TypeHTML(tok, nil) != "Token" {
		t.Error("no resolver, no link")
	}
}

func TestNavAndCrumbs(t *testing.T) {
	pkg := Visible(fixture(), "", nil)
	urls := NewURLs("/api/core/", apimodel.NewIndex(&apimodel.API{Packages: []*apimodel.Package{pkg}}))
	nav := Nav(pkg, urls)
	if len(nav) != 2 || nav[0].URL != "/api/core/" || nav[1].Title != "src/lex" || len(nav[1].Children) != 1 ||
		nav[1].Children[0].URL != "/api/core/src/lex/Lexer/" || nav[1].Children[0].Kind != "class" {
		t.Errorf("nav = %+v", nav)
	}
	pages := Build(fixture(), Options{Base: "/api/core/"})
	var trail []string
	for _, c := range Crumbs(pkg, pages[2], urls) {
		trail = append(trail, c.Title+"="+c.URL)
	}
	if got := strings.Join(trail, " "); got != "core=/api/core/ src/lex=/api/core/src/lex/ Lexer=/api/core/src/lex/Lexer/" {
		t.Errorf("crumbs = %s", got)
	}
	if len(Crumbs(pkg, pages[0], urls)) != 1 {
		t.Error("the index has one crumb")
	}
}

func TestPlaygroundExamples(t *testing.T) {
	js := "```js\nconsole.log(1)\n```"
	pkg := &apimodel.Package{Name: "kit", Modules: []*apimodel.Module{{ID: "kit/index", Path: "index.js", Symbols: []*apimodel.Symbol{
		{ID: "kit/index#f", Name: "f", Kind: apimodel.KindFunction, Doc: &apimodel.Doc{Summary: "F.",
			Examples: []string{js, "```ts\nf()\n```", "plain f()", "```js\na\n```\n\n```js\nb\n```"}}},
	}}}}
	md := func(playground string) string {
		var all strings.Builder
		for _, p := range Build(pkg, Options{Base: "/api/kit/", Playground: playground}) {
			all.WriteString(p.Markdown)
		}
		return all.String()
	}
	if strings.Contains(md(""), "data-ssg-playground") {
		t.Error("no playground configured, no marker")
	}
	got := md("https://cdn.example.com/kit.js?a=1&b=2")
	if strings.Count(got, "data-ssg-playground") != 1 {
		t.Errorf("only the single JavaScript block is runnable:\n%s", got)
	}
	want := `<div class="ssg-playground" data-ssg-playground data-package="kit" data-module="https://cdn.example.com/kit.js?a=1&amp;b=2">` +
		"\n\n" + js + "\n\n</div>"
	if !strings.Contains(got, want) {
		t.Errorf("marker:\n%s", got)
	}
}

func TestRunnable(t *testing.T) {
	for ex, want := range map[string]bool{
		"```js\nx\n```": true, "```javascript\nx\n```": true, "```mjs\nx\n```": true,
		"```\nx\n```": false, "```ts\nx\n```": false, "x": false, "```js\nx": false,
	} {
		if runnable(ex) != want {
			t.Errorf("runnable(%q) = %v", ex, !want)
		}
	}
}

func TestLanguageCode(t *testing.T) {
	pkg := &apimodel.Package{Name: "kit", Language: "go", Modules: []*apimodel.Module{{ID: "kit/kit", Path: "kit", Symbols: []*apimodel.Symbol{
		{ID: "kit/kit#Parse", Name: "Parse", Kind: apimodel.KindFunction, Doc: &apimodel.Doc{Summary: "Parses."},
			Signatures: []*apimodel.Signature{{Code: "func Parse(src string) (*Doc, error)"}}},
		{ID: "kit/kit#Mode", Name: "Mode", Kind: apimodel.KindType, Code: "type Mode int", Doc: &apimodel.Doc{Summary: "M."}},
	}}}}
	var md strings.Builder
	for _, p := range Build(pkg, Options{Base: "/api/kit/"}) {
		md.WriteString(p.Markdown)
	}
	for _, want := range []string{"```go\nfunc Parse(src string) (*Doc, error)\n```", "```go\ntype Mode int\n```"} {
		if !strings.Contains(md.String(), want) {
			t.Errorf("lacks %q:\n%s", want, md.String())
		}
	}
	for lang, fence := range map[string]string{"": "ts", "javascript": "ts", "php": "php", "python": "python"} {
		w := &writer{opts: &Options{language: lang}}
		if w.fence() != fence {
			t.Errorf("fence(%q) = %q", lang, w.fence())
		}
	}
}

func TestResolveEnumMember(t *testing.T) {
	enum := func(mod, typ, member string) *apimodel.Symbol {
		id := apimodel.SymbolID(mod, typ)
		return &apimodel.Symbol{ID: id, Name: typ, Kind: apimodel.KindEnum,
			Members: []*apimodel.Symbol{{ID: apimodel.MemberID(id, member), Name: member, Kind: apimodel.KindEnumMember}}}
	}
	r := NewResolver(&apimodel.API{Packages: []*apimodel.Package{{Name: "p", Modules: []*apimodel.Module{
		{ID: "p/a", Symbols: []*apimodel.Symbol{enum("p/a", "Style", "Title"), enum("p/a", "Level", "Debug")}},
		{ID: "p/b", Symbols: []*apimodel.Symbol{enum("p/b", "Mode", "Title")}},
	}}}})
	for _, tt := range []struct{ from, target, want string }{
		{"p/a", "Title", "p/a#Style.Title"},
		{"p/b", "Title", "p/b#Mode.Title"},
		{"p/b", "Debug", "p/a#Level.Debug"},
		{"p/c", "Title", ""},
	} {
		if got, _ := r.Resolve(tt.from, tt.target); got != tt.want {
			t.Errorf("Resolve(%s, %s) = %q, want %q", tt.from, tt.target, got, tt.want)
		}
	}
}

func TestResolveWithinPackage(t *testing.T) {
	lexer := func(pkg string) *apimodel.Module {
		mod := pkg + "/lex"
		return &apimodel.Module{ID: mod, Symbols: []*apimodel.Symbol{{ID: apimodel.SymbolID(mod, "Lexer"), Name: "Lexer", Kind: apimodel.KindClass}}}
	}
	r := NewResolver(&apimodel.API{Packages: []*apimodel.Package{
		{Name: "go", Modules: []*apimodel.Module{lexer("go"), {ID: "go/root"}}},
		{Name: "py", Modules: []*apimodel.Module{lexer("py")}},
	}})
	if id, ok := r.Resolve("go/root", "Lexer"); !ok || id != "go/lex#Lexer" {
		t.Errorf("own package: %q", id)
	}
	if _, ok := r.Resolve("", "Lexer"); ok {
		t.Error("ambiguous without a package")
	}
}

func TestPythonTypeText(t *testing.T) {
	w := &writer{opts: &Options{language: "python"}}
	tok := apimodel.Named("Token", "")
	for want, typ := range map[string]*apimodel.TypeRef{
		"list[Token]":            apimodel.Named("list", "", tok),
		"Token | None":           {Kind: apimodel.TypeUnion, Args: []*apimodel.TypeRef{tok, apimodel.Named("None", "")}},
		"tuple[Token, int]":      {Kind: apimodel.TypeTuple, Args: []*apimodel.TypeRef{tok, apimodel.Named("int", "")}},
		"list[Token]#array":      {Kind: apimodel.TypeArray, Args: []*apimodel.TypeRef{tok}},
		"\"on\"":                 {Kind: apimodel.TypeLiteral, Name: `"on"`},
		"dict[str, list[Token]]": apimodel.Named("dict", "", apimodel.Named("str", ""), apimodel.Named("list", "", tok)),
	} {
		want, _, _ = strings.Cut(want, "#")
		if got := w.typeText(typ); got != want {
			t.Errorf("typeText = %q, want %q", got, want)
		}
	}
	if (&writer{opts: &Options{}}).typeText(apimodel.Named("Array", "", tok)) != "Array<Token>" {
		t.Error("TypeScript keeps angle brackets")
	}
	if paramLabel(&apimodel.Param{Name: "x", Optional: true, Default: "1"}) != "x = 1" || paramLabel(&apimodel.Param{Name: "x", Optional: true}) != "x?" {
		t.Error("paramLabel")
	}
}
