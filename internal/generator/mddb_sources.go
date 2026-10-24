package generator

// Several MDDB collections per build, each with a metadata filter (#361).
//
// A site rendered from events, news, kb and articles, where only documents
// with visibility=public may be published, had one collection and no filter.
// Pulling everything and filtering in templates leaks private documents into
// the build directory and the search index, so the filter goes to the server.

import (
	"fmt"
	"strings"

	"github.com/spagu/ssg/internal/mddb"
	"github.com/spagu/ssg/internal/models"
)

// MddbSource is one collection an MDDB build reads.
type MddbSource struct {
	Collection string
	// Filter is sent as filterMeta: each key lists the values a document may
	// have. The server applies it, so nothing it leaves out is downloaded.
	Filter map[string][]string
	// Type "page" or "post" files every document as that; empty keeps each
	// document's own `type` meta, which is how a single collection always read.
	Type string
	// Section is the content section the documents belong to, the key
	// schema_defaults, language_sections and declared feeds match.
	Section string
}

// mddbDocTypes are the `type` meta values a source without a Type reads.
var mddbDocTypes = []string{"page", "post"}

// mddbSources is what a build reads: the configured sources, or the one
// collection a config without sources has always named.
func (g *Generator) mddbSources() []MddbSource {
	if len(g.config.Mddb.Sources) > 0 {
		return g.config.Mddb.Sources
	}
	return []MddbSource{{Collection: g.config.Mddb.Collection}}
}

// loadMddbSources reads every source into the site's pages and posts.
func (g *Generator) loadMddbSources(client mddb.MddbClient) error {
	var pages, posts []models.Page
	for _, src := range g.mddbSources() {
		if err := validateMddbSource(src); err != nil {
			return err
		}
		srcPages, srcPosts, err := g.loadMddbSource(client, src)
		if err != nil {
			return err
		}
		if len(g.config.Mddb.Sources) > 0 {
			g.log(fmt.Sprintf("   📚 %s → %d pages, %d posts", src.Collection, len(srcPages), len(srcPosts)))
		}
		pages = append(pages, srcPages...)
		posts = append(posts, srcPosts...)
	}
	g.siteData.Pages = pages
	// The same one sort a file-backed site gets (#155).
	g.siteData.Posts = sortPostsByDate(posts)
	return nil
}

// validateMddbSource rejects a source the build cannot read as written.
func validateMddbSource(src MddbSource) error {
	if strings.TrimSpace(src.Collection) == "" {
		return fmt.Errorf("mddb.sources: collection is required")
	}
	if !contentSourceTypes[src.Type] {
		return fmt.Errorf("mddb.sources: %q has unsupported type %q (supported: page, post)", src.Collection, src.Type)
	}
	return nil
}

// loadMddbSource fetches one source's documents and converts them.
func (g *Generator) loadMddbSource(client mddb.MddbClient, src MddbSource) (pages, posts []models.Page, err error) {
	types := mddbDocTypes
	if src.Type != "" {
		types = []string{src.Type}
	}
	for _, docType := range types {
		filter := mddbFilter(src.Filter)
		if src.Type == "" {
			// The document's own type decides, as it always did.
			filter["type"] = []any{docType}
		}
		docs, err := client.GetMatching(src.Collection, g.config.Mddb.Lang, filter)
		if err != nil {
			return nil, nil, fmt.Errorf("loading %ss from mddb (%s): %w", docType, src.Collection, err)
		}
		g.reportStructuredMeta(docs)
		converted, err := mddb.ToPages(docs)
		if err != nil {
			return nil, nil, fmt.Errorf("converting %ss: %w", docType, err)
		}
		for i := range converted {
			converted[i].PageFormat = g.config.PageFormat
			converted[i].Section = src.Section
			if src.Type != "" {
				converted[i].Type = src.Type
			}
			if docType == "post" {
				converted[i].URLFormat = g.config.PostURLFormat
			}
		}
		if docType == "post" {
			posts = append(posts, converted...)
		} else {
			pages = append(pages, converted...)
		}
	}
	return pages, posts, nil
}

// mddbFilter turns a configured filter into the filterMeta the server takes,
// as a fresh map the caller may add to.
func mddbFilter(filter map[string][]string) map[string][]any {
	out := make(map[string][]any, len(filter)+1)
	for key, values := range filter {
		vs := make([]any, len(values))
		for i, v := range values {
			vs[i] = v
		}
		out[key] = vs
	}
	return out
}
