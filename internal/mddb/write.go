package mddb

// Writing to MDDB (#190).
//
// The client could Get, Search and GetAll — everything a build needs to read
// content out of a collection, and nothing that could put anything in. So
// nothing could keep a collection in step with `templates/` and `static/`,
// which is what an agent needs on the other end of a natural-language search
// over the theme.
//
// Two calls cover it: Add upserts a document under a key, Delete removes one.
// MDDB's /v1/add is already an upsert — same collection, key and lang replaces
// — so a sync is add-what-changed plus delete-what-vanished, with no separate
// update path to keep correct.

import (
	"encoding/json"
	"fmt"
)

// AddRequest is one document to store. Key is the identity within the
// collection: sending the same key again replaces what was there.
type AddRequest struct {
	Collection string              `json:"collection"`
	Key        string              `json:"key"`
	Lang       string              `json:"lang"`
	Meta       map[string][]string `json:"meta,omitempty"`
	ContentMD  string              `json:"contentMd"`
}

// DeleteRequest identifies one document to remove.
type DeleteRequest struct {
	Collection string `json:"collection"`
	Key        string `json:"key"`
	Lang       string `json:"lang"`
}

// Add stores or replaces one document.
func (c *Client) Add(req AddRequest) error {
	if req.Collection == "" || req.Key == "" {
		return fmt.Errorf("add needs a collection and a key")
	}
	return c.writeJSON("/v1/add", req)
}

// Delete removes one document. A key that is not there is not an error: a sync
// that races another writer, or reruns after a partial failure, must converge
// rather than abort.
func (c *Client) Delete(req DeleteRequest) error {
	if req.Collection == "" || req.Key == "" {
		return fmt.Errorf("delete needs a collection and a key")
	}
	return c.writeJSON("/v1/delete", req)
}

// writeJSON posts a body and discards the response, surfacing only the error.
func (c *Client) writeJSON(endpoint string, payload any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshaling request: %w", err)
	}
	resp, err := c.doRequest("POST", endpoint, body)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	return nil
}
