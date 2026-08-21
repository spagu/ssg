package main

// The optional MDDB search backend for designer_find / content_find (#190).
//
// A local scan matches text; an MDDB collection's full-text search answers a
// question. The two are complementary, so the backend is consulted first and
// the scan still runs when it has nothing — which also means a search server
// that is down degrades the answer rather than the ability to edit the site.

import (
	"fmt"
	"strings"

	"github.com/spagu/ssg/internal/config"
	"github.com/spagu/ssg/internal/mcp"
	"github.com/spagu/ssg/internal/mddb"
)

// buildMCPSearch returns the search hook for the MCP server, or nil when no
// backend is configured — nil is the documented "scan locally" case, not an
// error, so a project without MDDB needs no configuration at all.
func buildMCPSearch(cfg *config.Config, logf func(string, ...any)) func(string, int) ([]mcp.FindHit, error) {
	sc := cfg.MCP.Search
	if !sc.Enabled() {
		return nil
	}
	client := mddb.NewClient(mddb.Config{
		BaseURL: sc.MddbURL,
		APIKey:  expandEnvValue(sc.MddbAPIKey),
	})
	logf("   🔎 search: MDDB %s collection %q (local scan is the fallback)", sc.MddbURL, sc.MddbCollection)

	return func(query string, limit int) ([]mcp.FindHit, error) {
		hits, err := client.FTS(mddb.FTSRequest{
			Collection: sc.MddbCollection,
			Query:      query,
			Limit:      limit,
			Fuzzy:      sc.MddbFuzzy,
			Lang:       sc.MddbLang,
		})
		if err != nil {
			return nil, err
		}
		return docsToFindHits(hits, limit), nil
	}
}

// docsToFindHits turns scored documents into the find-tool shape. The key is
// the project-relative path the theme sync stored them under, so a hit is
// directly actionable: read it, or anchor an edit in it.
func docsToFindHits(hits []mddb.FTSHit, limit int) []mcp.FindHit {
	out := make([]mcp.FindHit, 0, len(hits))
	for _, h := range hits {
		if len(out) >= limit {
			break
		}
		lines := strings.Split(h.Document.Content, "\n")
		to := min(len(lines), mcp.FindFragmentLines)
		out = append(out, mcp.FindHit{
			Path:     documentPath(h.Document),
			From:     1,
			To:       max(to, 1),
			Fragment: strings.Join(lines[:to], "\n"),
			Note:     fmt.Sprintf("score %.2f", h.Score),
		})
	}
	return out
}

// documentPath prefers the stored path metadata and falls back to the key,
// which is what the sync uses as the key in the first place.
func documentPath(d mddb.Document) string {
	if p, ok := d.Metadata["path"].(string); ok && p != "" {
		return p
	}
	return d.Key
}
