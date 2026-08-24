package mddb

// Full-text search against a collection (#190).
//
// Search here is a metadata filter — it answers "which documents have this
// tag", not "where is the page background set". MDDB's /v1/fts endpoint is the
// one that answers a question phrased as a sentence, and that is what makes an
// MDDB-backed find worth having over a local regular expression.

import (
	"encoding/json"
	"fmt"
)

// FTSRequest is a full-text query. Everything but Collection and Query is
// optional; the server's defaults are deliberately not restated here, so a
// change on its side does not need one here too.
type FTSRequest struct {
	Collection string `json:"collection"`
	Query      string `json:"query"`
	Limit      int    `json:"limit,omitempty"`
	Fuzzy      int    `json:"fuzzy,omitempty"`
	Lang       string `json:"lang,omitempty"`
	Mode       string `json:"mode,omitempty"`
	// Highlight asks for the matching fragments and, from MDDB 2.12.0, the
	// line range of each. Without it a hit names a document and nothing more,
	// and the caller is left printing the head of the file as if that were
	// where the match was (#203).
	Highlight bool `json:"highlight,omitempty"`
	// MaxHighlights caps the fragments per document; 0 leaves it to the server.
	MaxHighlights int `json:"maxHighlights,omitempty"`
}

// Highlight is one matching region of a document.
//
// StartLine and EndLine are 1-based and inclusive. They are zero against a
// server older than 2.12.0, which is the case a caller has to handle rather
// than paper over: a made-up line range is worse than none, because the next
// step is an anchored edit that has nothing to anchor to.
type Highlight struct {
	Fragment  string `json:"fragment"`
	StartLine int    `json:"startLine"`
	EndLine   int    `json:"endLine"`
}

// FTSHit is one scored document from a full-text query, with the regions that
// matched when highlights were asked for.
type FTSHit struct {
	Document   Document
	Score      float64
	Highlights []Highlight
}

// ftsResponse mirrors the endpoint's envelope.
type ftsResponse struct {
	Results []struct {
		Document   mddbDocument `json:"document"`
		Score      float64      `json:"score"`
		Highlights []Highlight  `json:"highlights"`
	} `json:"results"`
	Total int `json:"total"`
}

// FTS runs a full-text query and returns the hits in score order.
func (c *Client) FTS(req FTSRequest) ([]FTSHit, error) {
	if req.Collection == "" || req.Query == "" {
		return nil, fmt.Errorf("full-text search needs a collection and a query")
	}
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshaling request: %w", err)
	}
	resp, err := c.doRequest("POST", "/v1/fts", body)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	var out ftsResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}
	hits := make([]FTSHit, len(out.Results))
	for i, r := range out.Results {
		doc := r.Document
		hits[i] = FTSHit{
			Document:   doc.toDocument(req.Collection),
			Score:      r.Score,
			Highlights: r.Highlights,
		}
	}
	return hits, nil
}
