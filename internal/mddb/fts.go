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
}

// FTSHit is one scored document from a full-text query.
type FTSHit struct {
	Document Document
	Score    float64
}

// ftsResponse mirrors the endpoint's envelope.
type ftsResponse struct {
	Results []struct {
		Document mddbDocument `json:"document"`
		Score    float64      `json:"score"`
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
		hits[i] = FTSHit{Document: doc.toDocument(req.Collection), Score: r.Score}
	}
	return hits, nil
}
