package generator

// Documentation from code (1.8.69, GO-107/GO-108): each api_docs entry is a
// package read by an extractor into the API model; the model becomes ordinary
// pages of the site — so search, the sitemap, llms.txt, markdown_publish,
// check_links and base_path all apply — and is published whole as api.json.

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spagu/ssg/internal/apidoc"
	"github.com/spagu/ssg/internal/apimodel"
	"github.com/spagu/ssg/internal/apipage"
	"github.com/spagu/ssg/internal/apisource"
	dtsapi "github.com/spagu/ssg/internal/apisource/dts"
	goapi "github.com/spagu/ssg/internal/apisource/golang"
	jsapi "github.com/spagu/ssg/internal/apisource/js"
	phpapi "github.com/spagu/ssg/internal/apisource/php"
	pyapi "github.com/spagu/ssg/internal/apisource/python"
	"github.com/spagu/ssg/internal/depgraph"
	"github.com/spagu/ssg/internal/models"
)

// APIDocsOptions is one package to document (from api_docs).
type APIDocsOptions struct {
	Source     apisource.Config
	URL        string   // page prefix; default /api/<name>/
	Visibility string   // public, internal, all
	Stability  []string // non-stable levels to show; empty = all
	Readme     bool
	SourceURL  string // template with {path}, {line}, {ref}
	SourceRef  string // resolved ref for {ref}
	Playground string // the package as an ES module URL; JavaScript examples run
	OpenAPI    string // an OpenAPI file: a REST API instead of code
	TryIt      bool   // the Try it console on REST operations
}

// apiDocsFile is the published model, at the output root.
const apiDocsFile = "api.json"

// extractAPIPackage reads one package into the model. A variable so tests
// can feed a model directly.
var extractAPIPackage = extractPackage

// extractPackage picks the reader for the package's language (GO-119..121):
// the configured one, or the one its root shows. For JavaScript and
// TypeScript, declarations are read when the package ships them
// (package.json "types", an "exports" types condition, an index.d.ts) or an
// entry names a .d.ts, and its JavaScript otherwise: a TypeScript project
// publishes declarations when it builds, so documenting those reads exactly
// what its users get.
func extractPackage(cfg apisource.Config) (*apimodel.Package, []apisource.Diagnostic, error) {
	switch packageLanguage(cfg) {
	case "go":
		return goapi.Extract(cfg)
	case "php":
		return phpapi.Extract(cfg)
	case "python":
		return pyapi.Extract(cfg)
	}
	if usesDeclarations(cfg) {
		return dtsapi.Extract(cfg)
	}
	return jsapi.Extract(cfg)
}

// languageMarkers are the files that say what a package root holds, in the
// order they are trusted: a package.json wins in a mixed repository, since
// a JavaScript package with a Go or Python helper beside it is common.
var languageMarkers = []struct{ file, language string }{
	{"package.json", "javascript"}, {"go.mod", "go"}, {"composer.json", "php"},
	{"pyproject.toml", "python"}, {"setup.py", "python"}, {"setup.cfg", "python"},
}

// packageLanguage is the configured language, else the first marker file
// found in the root, else whatever code the root holds; JavaScript when
// nothing says.
func packageLanguage(cfg apisource.Config) string {
	switch cfg.Language {
	case "golang":
		return "go"
	case "":
	default:
		return cfg.Language
	}
	for _, m := range languageMarkers {
		if _, err := os.Stat(filepath.Join(cfg.Root, m.file)); err == nil {
			return m.language
		}
	}
	switch {
	case goapi.Detect(cfg.Root):
		return "go"
	case phpapi.Detect(cfg.Root):
		return "php"
	case pyapi.Detect(cfg.Root):
		return "python"
	}
	return "javascript"
}

// usesDeclarations reports whether a package is read from .d.ts files.
func usesDeclarations(cfg apisource.Config) bool {
	for _, e := range cfg.Entries {
		if strings.Contains(e, ".d.") && strings.HasSuffix(e, "ts") {
			return true
		}
	}
	return len(cfg.Entries) == 0 && dtsapi.HasDeclarations(cfg.Root)
}

// rewriteDocLinks turns inline links in documentation Markdown into Markdown
// links, reporting the targets it could not resolve (GO-106).
var rewriteDocLinks = apidoc.RewriteLinks

// loadAPIDocs extracts every configured package, adds its pages to the site
// and keeps the model for api.json and check_api.
func (g *Generator) loadAPIDocs() error {
	if len(g.config.APIDocs) == 0 {
		return nil
	}
	g.log("📚 Reading code for API docs...")
	var code []APIDocsOptions
	for _, opt := range g.config.APIDocs {
		if opt.OpenAPI != "" {
			if err := g.addRESTPages(opt); err != nil {
				return err
			}
			continue
		}
		code = append(code, opt)
	}
	if len(code) == 0 {
		g.log(fmt.Sprintf("   📚 API: %d page(s)", g.apiPageCount))
		return nil
	}
	api := &apimodel.API{Schema: apimodel.Schema}
	for _, opt := range code {
		pkg, diags, err := extractAPIPackage(opt.Source)
		if err != nil {
			return fmt.Errorf("api_docs %s: %w", opt.Source.Root, err)
		}
		g.apiDiagnostics = append(g.apiDiagnostics, diags...)
		api.Packages = append(api.Packages, pkg)
	}
	for _, p := range apimodel.Validate(api) {
		g.apiDiagnostics = append(g.apiDiagnostics, apisource.Diagnostic{Severity: apisource.Warning, File: p.ID, Message: p.Message})
	}
	resolver := apipage.NewResolver(api)
	hrefs := g.apiHrefs(api, code)
	g.apiHref = hrefs
	for i, opt := range code {
		g.addAPIPages(api.Packages[i], opt, resolver, hrefs)
	}
	g.apiModel = api
	g.log(fmt.Sprintf("   📚 API: %d package(s), %d page(s)", len(api.Packages), g.apiPageCount))
	return nil
}

// apiHrefs maps every symbol ID to its URL, across packages, so a link from
// one package to another lands on the right page.
func (g *Generator) apiHrefs(api *apimodel.API, opts []APIDocsOptions) func(id string) string {
	ix := apimodel.NewIndex(api)
	byPkg := map[string]*apipage.URLs{}
	for i, p := range api.Packages {
		byPkg[p.Name] = apipage.NewURLs(apiBase(opts[i], p.Name), ix)
	}
	return func(id string) string {
		pkg, _, _ := strings.Cut(id, "/")
		if u, ok := byPkg[pkg]; ok {
			return u.Href(id)
		}
		return ""
	}
}

// apiBase is a package's URL prefix: the configured one, or /api/<name>/.
func apiBase(opt APIDocsOptions, name string) string {
	if opt.URL != "" {
		return opt.URL
	}
	return "/api/" + name + "/"
}

// addAPIPages turns one package into site pages.
func (g *Generator) addAPIPages(pkg *apimodel.Package, opt APIDocsOptions, r *apipage.Resolver, href func(string) string) {
	pages := apipage.Build(pkg, apipage.Options{
		Base: apiBase(opt, pkg.Name), Visibility: opt.Visibility, Stability: opt.Stability, Readme: opt.Readme,
		Links: func(md, module string) string {
			out, missing := rewriteDocLinks(md, func(target string) (string, bool) {
				if id, ok := r.Resolve(module, target); ok {
					return href(id), true
				}
				return "", false
			})
			for _, m := range missing {
				g.apiDiagnostics = append(g.apiDiagnostics, apisource.Diagnostic{Severity: apisource.Warning,
					File: module, Message: "unresolved link to " + m})
			}
			return out
		},
		SourceURL:  sourceLinker(opt),
		Playground: opt.Playground,
	})
	visible := apipage.Visible(pkg, opt.Visibility, opt.Stability)
	urls := apipage.NewURLs(apiBase(opt, pkg.Name), apimodel.NewIndex(&apimodel.API{Packages: []*apimodel.Package{visible}}))
	nav := apipage.Nav(visible, urls)
	for _, p := range pages {
		page := apiSitePage(p, visible, g.config.PageFormat)
		api := page.Extra["api"].(map[string]interface{})
		api["nav"], api["crumbs"] = nav, apipage.Crumbs(visible, p, urls)
		g.siteData.Pages = append(g.siteData.Pages, page)
		g.apiPageCount++
	}
}

// apiSitePage is an API page as an ordinary site page. The model travels in
// Extra["api"] for themes with api-* layouts: package, module, symbol, and
// the precomputed nav and crumbs.
func apiSitePage(p apipage.Page, pkg *apimodel.Package, pageFormat string) models.Page {
	return referencePage(p, pageFormat, map[string]interface{}{"package": pkg, "module": p.Module, "symbol": p.Symbol})
}

// referencePage is a generated reference page (code or REST) as a site page.
func referencePage(p apipage.Page, pageFormat string, api map[string]interface{}) models.Page {
	return models.Page{
		// Slug carries the path too: themes build a page's canonical from it.
		Title: p.Title, Link: p.URL, Slug: strings.Trim(p.URL, "/"), Status: "publish", Type: "page", Layout: p.Layout,
		Content: p.Markdown, Excerpt: p.Summary, Description: p.Summary, PageFormat: pageFormat,
		Extra: map[string]interface{}{
			"api": api,
			// A guides list or a card grid shows the package's front page, not
			// every module and class: those are reached through it.
			"hide_from_lists": p.Layout != apipage.LayoutIndex,
		},
	}
}

// sourceLinker fills the source_url template, or returns nil without one.
func sourceLinker(opt APIDocsOptions) func(*apimodel.Source) string {
	if opt.SourceURL == "" {
		return nil
	}
	return func(s *apimodel.Source) string {
		return strings.NewReplacer("{path}", s.File, "{line}", fmt.Sprint(s.Line), "{ref}", opt.SourceRef).Replace(opt.SourceURL)
	}
}

// writeAPIModel publishes api.json when any package was documented.
func (g *Generator) writeAPIModel() error {
	if g.apiModel == nil {
		return nil
	}
	data, err := apimodel.Encode(g.apiModel)
	if err != nil {
		return err
	}
	// #nosec G306 -- a public web file
	return os.WriteFile(filepath.Join(g.config.OutputDir, apiDocsFile), data, 0644)
}

// codeExtensions are the files an api_docs package is read from.
var codeExtensions = map[string]bool{".js": true, ".mjs": true, ".cjs": true, ".jsx": true,
	".ts": true, ".mts": true, ".cts": true, ".tsx": true, ".json": true, ".md": true,
	".go": true, ".mod": true, ".php": true, ".py": true, ".toml": true, ".cfg": true}

// skippedCodeDirs hold other people's code or build output, never the
// package's own sources.
var skippedCodeDirs = map[string]bool{"node_modules": true, "vendor": true, "__pycache__": true}

// recordCodeInputs registers every source file of every api_docs package as a
// config-kind input of the dependency graph (GO-112): a change to the code a
// reference is built from makes the next --watch build full, rather than
// leaving its pages as the previous code described them. node_modules and
// hidden directories are not the package's own code and are skipped.
func (g *Generator) recordCodeInputs() {
	for _, opt := range g.config.APIDocs {
		if opt.OpenAPI != "" {
			g.recordInput(opt.OpenAPI, depgraph.KindConfig)
			continue
		}
		_ = filepath.WalkDir(opt.Source.Root, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return nil //nolint:nilerr // an unreadable entry is not a build failure
			}
			if d.IsDir() {
				if path != opt.Source.Root && (skippedCodeDirs[d.Name()] || strings.HasPrefix(d.Name(), ".")) {
					return filepath.SkipDir
				}
				return nil
			}
			if codeExtensions[strings.ToLower(filepath.Ext(path))] {
				g.recordInput(path, depgraph.KindConfig)
			}
			return nil
		})
	}
}
