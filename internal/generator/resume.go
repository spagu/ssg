package generator

// Schema Resume (https://schema-resume.org): a CV kept as one JSON document
// becomes a page of the site. The page is ordinary — its body is Markdown
// made from the document, so every theme renders it through page.html — and
// a theme that wants more provides a resume.html layout and reads the
// document from .Page.Extra.resume. The structured data is a ProfilePage
// whose Person carries the CV, and the document itself is published beside
// the page as resume.json. See docs/RESUME.md.

import (
	"fmt"
	stdhtml "html"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spagu/ssg/internal/models"
	"github.com/spagu/ssg/internal/resume"
)

// ResumeOptions is the resume: section of the configuration.
type ResumeOptions struct {
	File  string // the Schema Resume JSON document; empty = no résumé page
	URL   string // the page's URL; default /cv/
	Title string // the page title; default basics.name
	// Contact publishes the e-mail, phone and street address — in the page,
	// the structured data and resume.json. Off by default.
	Contact bool
}

// Résumé page constants.
const (
	defaultResumeURL = "/cv/"
	resumeJSONName   = "resume.json"
	resumeLayout     = "resume"
)

// resumeSite is what the build keeps of the résumé between loading and
// writing: the page's URL and the document's bytes as published.
type resumeSite struct {
	url  string
	json []byte
}

// resumeURL is the configured URL in directory form: "/cv" → "/cv/".
func resumeURL(configured string) string {
	u := strings.Trim(strings.TrimSpace(configured), "/")
	if u == "" {
		if strings.TrimSpace(configured) == "/" {
			return "/"
		}
		return defaultResumeURL
	}
	return "/" + u + "/"
}

// loadResume reads the document and adds its page. A document without a
// name fails the build; any other problem is a warning.
func (g *Generator) loadResume() error {
	opt := g.config.Resume
	if strings.TrimSpace(opt.File) == "" {
		return nil
	}
	doc, diags, err := resume.Load(opt.File)
	if err != nil {
		return fmt.Errorf("resume %s: %w", opt.File, err)
	}
	for _, d := range diags {
		if d.Severity == resume.Warning {
			fmt.Printf("   ⚠️  resume %s: %s: %s\n", opt.File, d.Path, d.Message)
		}
	}
	if resume.HasErrors(diags) {
		return fmt.Errorf("resume %s: %v", opt.File, diags)
	}
	if !opt.Contact {
		if doc, err = doc.WithoutContact(); err != nil {
			return fmt.Errorf("resume %s: %w", opt.File, err)
		}
	}
	page := resumePage(doc, opt, g.config.PageFormat)
	g.siteData.Pages = append(g.siteData.Pages, page)
	g.resume = &resumeSite{url: page.Link, json: doc.Source}
	g.log("   📄 resume: " + page.Link)
	return nil
}

// resumePage is the document as a site page.
func resumePage(doc *resume.Resume, opt ResumeOptions, pageFormat string) models.Page {
	url := resumeURL(opt.URL)
	b := doc.Basics
	title := strings.TrimSpace(opt.Title)
	if title == "" {
		title = strings.TrimSpace(b.Name)
	}
	description := strings.TrimSpace(b.Summary)
	if description == "" {
		description = strings.TrimSpace(b.Label)
	}
	return models.Page{
		// Slug carries the path too: themes build a page's canonical from it.
		Title: title, Link: url, Slug: strings.Trim(url, "/"), Status: "publish", Type: "page",
		Layout: resumeLayout, Content: resume.Markdown(doc, opt.Contact), Excerpt: description,
		Description: description, FeaturedImage: strings.TrimSpace(b.Image), PageFormat: pageFormat,
		Modified: resumeModified(doc.Meta),
		Schema:   resumeLD(doc, opt.Contact),
		Extra: map[string]interface{}{
			"resume":      doc,
			"resume_json": url + resumeJSONName,
		},
	}
}

// resumeModified is the document's own modification time, when it states
// one a machine can read.
func resumeModified(m resume.Meta) time.Time {
	for _, s := range []string{m.DateModified, m.LastModified} {
		if t, err := time.Parse(time.RFC3339, s); err == nil {
			return t
		}
	}
	return time.Time{}
}

// resumeAlternate announces resume.json in the résumé page's head.
func (g *Generator) resumeAlternate(page models.Page, existing string) string {
	if g.resume == nil || page.GetURL() != g.resume.url {
		return ""
	}
	href := g.resume.url + resumeJSONName
	if strings.Contains(existing, `href="`+href+`"`) {
		return ""
	}
	return fmt.Sprintf(`<link rel="alternate" type="application/json" href="%s">`+"\n", stdhtml.EscapeString(href))
}

// writeResumeJSON publishes the document beside its page.
func (g *Generator) writeResumeJSON() error {
	if g.resume == nil {
		return nil
	}
	dir := filepath.Join(g.config.OutputDir, filepath.FromSlash(strings.Trim(g.resume.url, "/")))
	path := filepath.Join(dir, resumeJSONName)
	if err := g.ensureWithinOutput(path); err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil { // #nosec G301 -- a public web directory
		return err
	}
	// #nosec G306,G703 -- a public web file, inside the output directory
	return os.WriteFile(path, g.resume.json, 0o644)
}
