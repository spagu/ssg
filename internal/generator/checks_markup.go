package generator

// Build-time detection of source Markdown whose markup is indented four columns
// or more, which CommonMark renders as a literal code block (#127).
//
// Unlike the other checks this one reads the SOURCE, not the output: the fix is
// an edit to the Markdown file, so the report has to name that file. It is also
// the only check that is ON by default — the others weigh a judgement call
// (is this alt text good enough?), while this one reports content that is
// provably not rendering as written. A build that silently ships `</div>` down
// the middle of a page is the failure the whole check exists to prevent.

import (
	"fmt"

	"github.com/spagu/ssg/internal/models"
	"github.com/spagu/ssg/internal/repair"
)

// checkMarkupIfRequested reports pages carrying markup that renders as code.
// Silent when there is nothing to report: it runs on every build, so a clean
// site must not pay a line of output for it.
func (g *Generator) checkMarkupIfRequested() error {
	mode := g.resolveMode(g.config.CheckMarkup)
	if mode == "" || mode == "off" {
		return nil
	}

	findings := g.markupFindings()
	if len(findings) == 0 {
		return nil
	}
	sortFindings(findings)
	for _, f := range findings {
		fmt.Printf("   ⚠️  markup renders as code in %s → %s\n", f.file, f.detail)
	}
	fmt.Println("   💡 The source renders as literal text, not markup — usually a page-builder")
	fmt.Println("      export that indented or fenced its HTML. Fix it with: ssg repair --fix")

	if mode == "strict" {
		return fmt.Errorf("%d block(s) of markup render as literal text", len(findings))
	}
	return nil
}

// markupFindings scans every loaded page and post, naming each block by its
// source file and line.
func (g *Generator) markupFindings() []finding {
	if g.siteData == nil {
		return nil
	}
	var findings []finding
	scan := func(pages []models.Page) {
		for _, p := range pages {
			for _, f := range repair.Scan(p.Content) {
				findings = append(findings, finding{
					file:   markupSourceLabel(p),
					detail: fmt.Sprintf("line %d of the body, %s: %s", f.Line, f.Kind, f.Sample),
				})
			}
		}
	}
	scan(g.siteData.Pages)
	scan(g.siteData.Posts)
	return findings
}

// markupSourceLabel names the file to edit, falling back to the page's URL for
// content that has no file behind it (mddb, external sources).
func markupSourceLabel(p models.Page) string {
	if p.SourceFile != "" {
		return p.SourceFile
	}
	if p.Link != "" {
		return p.Link
	}
	return p.Slug
}
