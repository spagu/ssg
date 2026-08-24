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
		BaseURL:   sc.MddbURL,
		APIKey:    expandEnvValue(sc.MddbAPIKey),
		AllowHTTP: sc.MddbAllowHTTP,
	})
	logf("   🔎 search: MDDB %s collection %q (local scan is the fallback)", sc.MddbURL, sc.MddbCollection)

	return func(query string, limit int) ([]mcp.FindHit, error) {
		hits, err := client.FTS(mddb.FTSRequest{
			Collection: sc.MddbCollection,
			Query:      query,
			Limit:      limit,
			Fuzzy:      sc.MddbFuzzy,
			Lang:       sc.MddbLang,
			// Without highlights a hit names a file and nothing else, and the
			// answer carries a real path beside a made-up location (#203).
			Highlight:     true,
			MaxHighlights: mcpSearchMaxHighlights,
		})
		if err != nil {
			return nil, err
		}
		return docsToFindHits(hits, limit), nil
	}
}

// mcpSearchMaxHighlights caps the regions asked for per document. A find is
// meant to point somewhere; a document that matches in twenty places is a
// document to read, not a locus.
const mcpSearchMaxHighlights = 3

// docsToFindHits turns scored documents into the find-tool shape.
//
// Each highlight is its own locus, because that is what the caller does next:
// an anchored edit needs the neighbourhood of the match, and a document that
// matched in two places has two of them. The key is the project-relative path
// the theme sync stored the document under, so a hit names the file to open.
func docsToFindHits(hits []mddb.FTSHit, limit int) []mcp.FindHit {
	out := make([]mcp.FindHit, 0, len(hits))
	for _, h := range hits {
		for _, loc := range locate(h) {
			if len(out) >= limit {
				return out
			}
			out = append(out, loc)
		}
	}
	return out
}

// locate turns one scored document into the regions worth reporting.
func locate(h mddb.FTSHit) []mcp.FindHit {
	path, note := documentPath(h.Document), fmt.Sprintf("score %.2f", h.Score)

	var out []mcp.FindHit
	for _, hl := range h.Highlights {
		// A server older than 2.12.0 sends fragments without line numbers.
		// Reporting 0, or inventing a range, would hand the agent a location
		// that is not one — worse than the local scan, which at least reports
		// the lines it matched.
		if hl.StartLine < 1 || hl.EndLine < hl.StartLine {
			continue
		}
		out = append(out, mcp.FindHit{
			Path:     path,
			From:     hl.StartLine,
			To:       hl.EndLine,
			Fragment: hl.Fragment,
			Note:     note,
		})
	}
	if len(out) > 0 {
		return out
	}
	// No usable highlight: name the document and say so, rather than print its
	// first lines as though the match were there.
	return []mcp.FindHit{{
		Path:     path,
		From:     1,
		To:       1,
		Fragment: firstLine(h.Document.Content),
		Note:     note + ", line unknown — this MDDB predates highlight line ranges",
	}}
}

// firstLine returns the document's opening line, as a label rather than as a
// claim about where the match is.
func firstLine(content string) string {
	if i := strings.IndexByte(content, '\n'); i >= 0 {
		return content[:i]
	}
	return content
}

// documentPath prefers the stored path metadata and falls back to the key,
// which is what the sync uses as the key in the first place.
func documentPath(d mddb.Document) string {
	if p, ok := d.Metadata["path"].(string); ok && p != "" {
		return p
	}
	return d.Key
}
