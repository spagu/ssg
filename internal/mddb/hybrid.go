package mddb

// Hybrid search: BM25 fused with vectors (#207).
//
// A collection carrying embeddings was searched lexically anyway, because the
// client only ever asked /v1/fts. So "how the navigation looks on phones" found
// nothing unless those words appeared in the code — which is exactly the class
// of question the MDDB backend gets configured for, the identifiers and colours
// being already answered by the local scan. The vectors were computed and never
// consulted: the worst of both costs.
//
// What hybrid does not do is say *where*. Its results carry no highlights and
// no line ranges — storage.Doc has no positional information at all — so this
// is only half of an answer, and the caller pairs it with /v1/fts for the
// other half. Reporting a document without a locus is what #203 was about.

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// HybridRequest is a fused keyword-and-vector query.
type HybridRequest struct {
	Collection string `json:"collection"`
	Query      string `json:"query"`
	TopK       int    `json:"topK,omitempty"`
	Fuzzy      int    `json:"fuzzy,omitempty"`
	Lang       string `json:"lang,omitempty"`
	// IncludeContent is asked for so a hit with no lexical match can still be
	// labelled with the document's opening line rather than nothing.
	IncludeContent bool `json:"includeContent,omitempty"`
}

// HybridHit is one fused result. The scores are kept apart because they answer
// different questions: a high vector score with no keyword score is precisely
// the hit the lexical path could never have found.
type HybridHit struct {
	Document      Document
	CombinedScore float64
	FTSScore      float64
	VectorScore   float64
	Rank          int
}

// hybridResponse mirrors the endpoint's envelope.
type hybridResponse struct {
	Results []struct {
		Document      mddbDocument `json:"document"`
		CombinedScore float64      `json:"combinedScore"`
		FTSScore      float64      `json:"ftsScore"`
		VectorScore   float64      `json:"vectorScore"`
		Rank          int          `json:"rank"`
	} `json:"results"`
	Total int `json:"total"`
}

// HybridSearch runs a fused query.
//
// unsupported is true when the server has no such endpoint, or refuses the
// request because the collection carries no vectors. Neither is an error: a
// caller is expected to fall back to the lexical path, and a collection without
// embeddings is an ordinary collection rather than a misconfiguration.
func (c *Client) HybridSearch(req HybridRequest) (hits []HybridHit, unsupported bool, err error) {
	if req.Collection == "" || req.Query == "" {
		return nil, false, fmt.Errorf("hybrid search needs a collection and a query")
	}
	// Cannot fail: a struct of strings, ints and a bool has no type
	// json.Marshal refuses. Guarding it would be a branch no caller can reach.
	body, _ := json.Marshal(req)
	resp, err := c.doRequest("POST", "/v1/hybrid-search", body)
	if err != nil {
		// doRequest turns any status past 400 into an error, so a server that
		// does not know the route, or a collection with no vectors, arrives
		// here rather than as a status to inspect. Both mean the same thing to
		// a caller: ask the other endpoint.
		if isUnsupported(err) {
			return nil, true, nil
		}
		return nil, false, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusNotFound {
		return nil, true, nil
	}

	var out hybridResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, false, fmt.Errorf("decoding response: %w", err)
	}
	hits = make([]HybridHit, len(out.Results))
	for i, r := range out.Results {
		doc := r.Document
		hits[i] = HybridHit{
			Document:      doc.toDocument(req.Collection),
			CombinedScore: r.CombinedScore,
			FTSScore:      r.FTSScore,
			VectorScore:   r.VectorScore,
			Rank:          r.Rank,
		}
	}
	return hits, false, nil
}

// isUnsupported reports whether an error from doRequest means "this server
// cannot answer that", as opposed to "something went wrong".
//
// doRequest folds every status past 400 into one error string, so the status
// has to be read back out of it. Narrow on purpose: 404 is a route the server
// does not have, and 400 is a request it will not accept — a collection with no
// vectors. Anything else, including 401 and 5xx, is a real failure and must not
// be quietly downgraded to "fall back and say nothing".
func isUnsupported(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "status 404") || strings.Contains(msg, "status 400")
}
