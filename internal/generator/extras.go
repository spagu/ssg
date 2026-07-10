// Package generator — v1.8 pipeline extras: link checking (SEO-005), asset
// bundling (ASSET-002), per-page JSON output (PLAT-003) and a client-side search
// index (PLAT-004). Kept in a separate file to bound generator.go's size.
package generator

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spagu/ssg/internal/models"
	"golang.org/x/net/html"
)

// ─── SEO-005: build-time internal link checker ──────────────────────────────

// brokenLink records a dead internal reference found in generated HTML.
type brokenLink struct {
	from string // HTML file (relative to output)
	href string // referenced URL
}

// checkLinksIfRequested validates internal links when check_links is "warn" or
// "strict"; strict fails the build on any dead internal link (SEO-005).
func (g *Generator) checkLinksIfRequested() error {
	mode := g.config.CheckLinks
	if mode == "" {
		return nil
	}
	g.log("🔗 Checking internal links...")
	broken, err := g.checkLinks()
	if err != nil {
		return err
	}
	for _, b := range broken {
		fmt.Printf("   ⚠️  broken link in %s → %s\n", b.from, b.href)
	}
	if mode == "strict" && len(broken) > 0 {
		return fmt.Errorf("%d broken internal link(s)", len(broken))
	}
	if len(broken) == 0 && !g.config.Quiet {
		fmt.Println("   ✅ no broken internal links")
	}
	return nil
}

// checkLinks parses every generated HTML file and reports internal href/src values
// that do not resolve to a file in the output tree. External links (http/https,
// mailto, tel, data, protocol-relative), fragments and empty refs are ignored so
// the check never touches the network (SEO-005).
func (g *Generator) checkLinks() ([]brokenLink, error) {
	root := g.config.OutputDir
	var broken []brokenLink
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.EqualFold(filepath.Ext(path), ".html") {
			return err
		}
		refs, e := extractRefs(path)
		if e != nil {
			return nil // unreadable file is not a link error
		}
		rel, _ := filepath.Rel(root, path)
		for _, ref := range refs {
			if !isInternalRef(ref) {
				continue
			}
			if !g.refResolves(ref, filepath.Dir(path)) {
				broken = append(broken, brokenLink{from: filepath.ToSlash(rel), href: ref})
			}
		}
		return nil
	})
	sort.Slice(broken, func(i, j int) bool {
		if broken[i].from != broken[j].from {
			return broken[i].from < broken[j].from
		}
		return broken[i].href < broken[j].href
	})
	return broken, err
}

// extractRefs returns the href/src attribute values in an HTML file.
func extractRefs(path string) ([]string, error) {
	f, err := os.Open(path) // #nosec G304 -- CLI reads its own output
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	doc, err := html.Parse(f)
	if err != nil {
		return nil, err
	}
	var refs []string
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			for _, a := range n.Attr {
				if a.Key == "href" || a.Key == "src" {
					refs = append(refs, a.Val)
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	return refs, nil
}

// isInternalRef reports whether a reference points inside the generated site.
func isInternalRef(ref string) bool {
	ref = strings.TrimSpace(ref)
	if ref == "" || strings.HasPrefix(ref, "#") || strings.HasPrefix(ref, "//") {
		return false
	}
	lower := strings.ToLower(ref)
	for _, scheme := range []string{"http:", "https:", "mailto:", "tel:", "data:", "javascript:"} {
		if strings.HasPrefix(lower, scheme) {
			return false
		}
	}
	return true
}

// refResolves reports whether an internal ref maps to an existing output file.
func (g *Generator) refResolves(ref, htmlDir string) bool {
	// Strip query and fragment.
	if i := strings.IndexAny(ref, "?#"); i >= 0 {
		ref = ref[:i]
	}
	if ref == "" {
		return true // pure fragment/query on the same page
	}
	var target string
	if strings.HasPrefix(ref, "/") {
		target = filepath.Join(g.config.OutputDir, filepath.FromSlash(strings.TrimPrefix(ref, "/")))
	} else {
		target = filepath.Join(htmlDir, filepath.FromSlash(ref))
	}
	if info, err := os.Stat(target); err == nil {
		if info.IsDir() {
			_, e := os.Stat(filepath.Join(target, "index.html"))
			return e == nil
		}
		return true
	}
	// Directory-style URL ending in "/" → index.html
	if strings.HasSuffix(ref, "/") {
		_, e := os.Stat(filepath.Join(target, "index.html"))
		return e == nil
	}
	return false
}

// ─── ASSET-002: CSS/JS bundling ─────────────────────────────────────────────

// bundleIfRequested concatenates configured bundles before minify/fingerprint
// (ASSET-002). Each bundle joins its source files (in order) into one artifact in
// the output root; sources are left in place so templates referencing either the
// bundle or the sources keep working.
func (g *Generator) bundleIfRequested() error {
	if len(g.config.Bundles) == 0 {
		return nil
	}
	g.log("📦 Bundling assets...")
	for _, name := range sortedKeys(g.config.Bundles) {
		if err := g.writeBundle(name, g.config.Bundles[name]); err != nil {
			return err
		}
	}
	return nil
}

// writeBundle concatenates sources into the named bundle file under the output root.
func (g *Generator) writeBundle(name string, sources []string) error {
	outPath := filepath.Join(g.config.OutputDir, filepath.FromSlash(models.SanitizeRelPath(name)))
	if err := g.ensureWithinOutput(outPath); err != nil {
		return err
	}
	var buf strings.Builder
	for _, src := range sources {
		srcPath := filepath.Join(g.config.OutputDir, filepath.FromSlash(models.SanitizeRelPath(src)))
		data, err := os.ReadFile(srcPath) // #nosec G304 -- CLI reads its own output
		if err != nil {
			fmt.Printf("   ⚠️  bundle %s: missing source %s\n", name, src)
			continue
		}
		fmt.Fprintf(&buf, "/* %s */\n", filepath.Base(src))
		buf.Write(data)
		buf.WriteString("\n")
	}
	// #nosec G301 -- Web content directories need to be world-traversable
	if err := os.MkdirAll(filepath.Dir(outPath), 0755); err != nil {
		return err
	}
	// #nosec G306 -- Web content files need to be world-readable
	return os.WriteFile(outPath, []byte(buf.String()), 0644)
}

// ─── PLAT-003: per-page JSON output ─────────────────────────────────────────

// wantsOutput reports whether a named output format is enabled (PLAT-003).
func (g *Generator) wantsOutput(format string) bool {
	for _, o := range g.config.Outputs {
		if strings.EqualFold(o, format) {
			return true
		}
	}
	return false
}

// writeJSONOutput writes index.json next to a page's index.html when the json
// output format is enabled (PLAT-003).
func (g *Generator) writeJSONOutput(page models.Page, htmlPath string) {
	if !g.wantsOutput("json") || !strings.HasSuffix(htmlPath, "index.html") {
		return
	}
	rec := g.pageRecord(page)
	data, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		return
	}
	jsonPath := strings.TrimSuffix(htmlPath, "index.html") + "index.json"
	// #nosec G306 -- Web content files need to be world-readable
	_ = os.WriteFile(jsonPath, data, 0644)
}

// pageRecord is the stable JSON representation of a page (PLAT-003 / PLAT-004).
func (g *Generator) pageRecord(page models.Page) map[string]interface{} {
	return map[string]interface{}{
		"schema":      1,
		"title":       page.Title,
		"url":         page.GetURL(),
		"date":        page.Date,
		"type":        page.Type,
		"tags":        page.Tags,
		"categories":  page.Categories,
		"excerpt":     page.Excerpt,
		"wordCount":   page.WordCount,
		"readingTime": page.ReadingTime,
		"content":     tmplStripHTML(g.convertMarkdownToHTML(page.Content)),
	}
}

// ─── PLAT-004: client-side search index ─────────────────────────────────────

// generateSearchIndex writes search-index.json (all posts + pages) for a
// client-side search widget when enabled (PLAT-004).
func (g *Generator) generateSearchIndex() error {
	if !g.config.SearchIndex {
		return nil
	}
	g.log("🔎 Building search index...")
	var docs []map[string]interface{}
	add := func(pages []models.Page) {
		for _, p := range pages {
			docs = append(docs, map[string]interface{}{
				"title":   p.Title,
				"url":     p.GetURL(),
				"tags":    p.Tags,
				"excerpt": p.Excerpt,
				"text":    tmplStripHTML(g.convertMarkdownToHTML(p.Content)),
			})
		}
	}
	add(g.siteData.Posts)
	add(g.siteData.Pages)

	data, err := json.Marshal(docs)
	if err != nil {
		return err
	}
	// #nosec G306 -- Web content files need to be world-readable
	return os.WriteFile(filepath.Join(g.config.OutputDir, "search-index.json"), data, 0644)
}
