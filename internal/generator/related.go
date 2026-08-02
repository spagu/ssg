package generator

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spagu/ssg/internal/mddb"
	"github.com/spagu/ssg/internal/models"
)

// relatedFuncs exposes the related-posts template helpers (#1.8.16):
//
//	{{ range related . 5 }}…{{ end }}           in-memory keyword overlap
//	{{ range relatedFromMddb . 5 }}…{{ end }}   live query to the mddb corpus
func (g *Generator) relatedFuncs() map[string]interface{} {
	return map[string]interface{}{
		"related":         g.tmplRelated,
		"relatedFromMddb": g.tmplRelatedFromMddb,
	}
}

// tmplRelated returns up to n posts most related to page, ranked by the number of
// shared keywords and tags (then recency, then slug for a total, deterministic
// order). It reads the already-loaded posts — whatever the content source — so it
// needs no network and is reproducible. Posts with no overlap are excluded.
func (g *Generator) tmplRelated(page models.Page, n int) []models.Page {
	want := keywordSet(page)
	if len(want) == 0 || n <= 0 {
		return nil
	}
	type scored struct {
		p     models.Page
		score int
	}
	var ranked []scored
	for _, cand := range g.siteData.Posts {
		if cand.Slug == page.Slug {
			continue
		}
		if s := overlap(want, keywordSet(cand)); s > 0 {
			ranked = append(ranked, scored{cand, s})
		}
	}
	sort.SliceStable(ranked, func(i, j int) bool {
		if ranked[i].score != ranked[j].score {
			return ranked[i].score > ranked[j].score
		}
		if !ranked[i].p.Date.Equal(ranked[j].p.Date) {
			return ranked[i].p.Date.After(ranked[j].p.Date)
		}
		return ranked[i].p.Slug < ranked[j].p.Slug
	})
	if len(ranked) > n {
		ranked = ranked[:n]
	}
	out := make([]models.Page, len(ranked))
	for i := range ranked {
		out[i] = ranked[i].p
	}
	return out
}

// tmplRelatedFromMddb queries the mddb server for up to n posts sharing this
// page's keywords/tags — reaching the whole corpus, not just the pages built into
// this site. It is a live query (not cached); returns nil when mddb is not
// configured, the page has no keywords, or the query fails (logged, never fatal).
func (g *Generator) tmplRelatedFromMddb(page models.Page, n int) []models.Page {
	if !g.config.Mddb.Enabled || n <= 0 {
		return nil
	}
	filter := relatedFilter(page)
	if len(filter) == 0 {
		return nil
	}
	client := g.relatedMddbClient()
	if client == nil {
		return nil
	}
	docs, _, err := client.Search(mddb.SearchRequest{
		Collection: g.config.Mddb.Collection,
		Lang:       g.config.Mddb.Lang,
		FilterMeta: filter,
		Limit:      n + 1, // +1 to absorb the page itself when the corpus contains it
	})
	if err != nil {
		fmt.Printf("   ⚠️  relatedFromMddb: %v\n", err)
		return nil
	}
	pages, err := mddb.ToPages(docs)
	if err != nil {
		fmt.Printf("   ⚠️  relatedFromMddb: %v\n", err)
		return nil
	}
	out := make([]models.Page, 0, n)
	for _, p := range pages {
		if p.Slug == page.Slug {
			continue
		}
		out = append(out, p)
		if len(out) == n {
			break
		}
	}
	return out
}

// relatedMddbClient lazily builds the shared mddb client the related helper
// queries. Safe under the parallel render; closed in Generate's teardown.
func (g *Generator) relatedMddbClient() mddb.MddbClient {
	g.relatedMddbOnce.Do(func() {
		client, err := mddb.NewMddbClient(mddb.ClientConfig{
			URL:       g.config.Mddb.URL,
			Protocol:  g.config.Mddb.Protocol,
			APIKey:    g.config.Mddb.APIKey,
			Timeout:   g.config.Mddb.Timeout,
			BatchSize: g.config.Mddb.BatchSize,
		})
		if err != nil {
			fmt.Printf("   ⚠️  relatedFromMddb: %v\n", err)
			return
		}
		g.relatedMddb = client
	})
	return g.relatedMddb
}

// closeRelatedMddb releases the related-helper client after a build.
func (g *Generator) closeRelatedMddb() {
	if g.relatedMddb != nil {
		_ = g.relatedMddb.Close()
		g.relatedMddb = nil
	}
}

// relatedFilter builds the mddb FilterMeta from a page's tags (preferred) or
// keywords, so the server returns documents sharing them.
func relatedFilter(page models.Page) map[string][]any {
	if len(page.Tags) > 0 {
		vals := make([]any, len(page.Tags))
		for i, t := range page.Tags {
			vals[i] = t
		}
		return map[string][]any{"tags": vals}
	}
	kw := splitKeywords(page.Keywords)
	if len(kw) == 0 {
		return nil
	}
	vals := make([]any, len(kw))
	for i, k := range kw {
		vals[i] = k
	}
	return map[string][]any{"keywords": vals}
}

// keywordSet is a page's normalised tag + keyword set for overlap scoring.
func keywordSet(p models.Page) map[string]bool {
	set := map[string]bool{}
	for _, t := range p.Tags {
		if t = normalizeKeyword(t); t != "" {
			set[t] = true
		}
	}
	for _, k := range splitKeywords(p.Keywords) {
		set[normalizeKeyword(k)] = true
	}
	return set
}

func overlap(a, b map[string]bool) int {
	n := 0
	for k := range a {
		if b[k] {
			n++
		}
	}
	return n
}

func splitKeywords(s string) []string {
	parts := strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == ';' || r == '\n' })
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func normalizeKeyword(s string) string { return strings.ToLower(strings.TrimSpace(s)) }
