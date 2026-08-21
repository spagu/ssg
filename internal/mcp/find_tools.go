package mcp

// The find tools themselves: the same search over each section's own
// directories, plus the optional MDDB backend (#190).

import (
	"fmt"
	"strings"
)

// findSchema is the argument shape both find tools take.
func findSchema(where string) map[string]any {
	return objectSchema(map[string]any{
		"query": stringProp("What to look for in " + where + ". Matched case-insensitively as a " +
			"regular expression when it is valid syntax, otherwise literally. Identifiers, colours, " +
			"class names and selectors all work; so does a phrase when an MDDB backend is configured."),
		"limit": map[string]any{
			"type":        "integer",
			"description": fmt.Sprintf("Maximum matching regions to return (default %d).", findDefaultLimit),
		},
	}, "query")
}

// designerFindTool and contentFindTool are appended to their sections.
func (s *Server) designerFindTool() tool {
	return tool{
		name: "designer_find",
		description: "DESIGNER · Find WHERE something lives in the theme without reading whole files — " +
			"START HERE for any \"change the X\" request instead of reading files until you spot it. " +
			"Returns matching line ranges with a little context, which is exactly the anchor " +
			"designer_edit needs. Searches: " + strings.Join(s.designerBases(), ", ") + ".",
		schema:  findSchema("templates and theme assets"),
		handler: s.designerFind,
	}
}

func (s *Server) contentFindTool() tool {
	return tool{
		name: "content_find",
		description: "CONTENT · Find WHICH Markdown files mention something, and where, without " +
			"reading them. Use before editing to locate the passage; the returned line ranges are " +
			"the anchor content_edit needs. Searches: " + strings.Join(s.opts.ContentDirs, ", ") + ".",
		schema:  findSchema("Markdown content"),
		handler: s.contentFind,
	}
}

func (s *Server) designerFind(args map[string]any) toolResult {
	return s.find(args, s.designerBases(), nil)
}

func (s *Server) contentFind(args map[string]any) toolResult {
	return s.find(args, s.opts.ContentDirs, contentExts)
}

// find runs the query over one section's directories, preferring the MDDB
// backend when one is configured and falling back to the local walk when it
// cannot answer. The fallback is not a nicety: a search backend that is down
// must not take the ability to edit the site down with it.
func (s *Server) find(args map[string]any, bases []string, exts []string) toolResult {
	raw, _ := strArg(args, "query")
	q, err := compileQuery(raw)
	if err != nil {
		return errResult(err.Error())
	}
	limit := intArg(args, "limit", findDefaultLimit)

	if s.opts.Search != nil {
		hits, err := s.opts.Search(raw, limit)
		switch {
		case err != nil:
			s.opts.Logf("   ⚠️  search backend: %v — falling back to a local scan", err)
		case len(hits) > 0:
			return textResult(renderHits(hits, raw, q.literal) + "\n\n(via the configured search backend)")
		}
	}

	files, err := listFiles(s.opts.Root, bases, exts...)
	if err != nil {
		return errResult("search failed: " + err.Error())
	}
	return textResult(renderHits(searchFiles(s.opts.Root, sortedUnique(files), q, limit), raw, q.literal))
}
