package mddb

// Asking MDDB about a document before storing it (#192).
//
// The render side already recognises a Go-stringified structure and names the
// document and the field rather than letting a theme fail with
// `can't evaluate field question in type interface {}`. That is the consumer
// half. The producer half was silent: a document went in, the stringified value
// stored successfully, and the failure surfaced at the *next render* — possibly
// on another machine, possibly weeks later, by which time the warning names a
// document whose author has moved on.
//
// The distance between "the bad value was written" and "someone sees the
// warning" is the whole problem. At write time the producer is right there,
// with the source in hand.
//
// MDDB 2.12.0 added the same lint to /v1/validate, on every surface. It returns
// warnings beside errors and — deliberately — never fails validation on one,
// because `map[answer:… question:…]` is a valid string and MDDB has no business
// deciding a string is not what its author meant. ssg follows that: a warning
// is reported, never fatal.

import (
	"encoding/json"
	"fmt"
	"net/http"
)

// ValidationResult is what /v1/validate answers.
type ValidationResult struct {
	Valid    bool     `json:"valid"`
	Errors   []string `json:"errors"`
	Warnings []string `json:"warnings"`
}

// Validate asks the server to check a document's metadata before it is stored.
//
// unsupported is true when the server does not offer the endpoint at all, which
// is every MDDB before 2.12.0. That is not an error: a site pointed at an older
// server must keep working exactly as it did, silently, rather than fail every
// write or print a warning about the server on each document.
func (c *Client) Validate(collection string, meta map[string][]string) (result ValidationResult, unsupported bool, err error) {
	if collection == "" {
		return result, false, fmt.Errorf("validate needs a collection")
	}
	// Cannot fail: a map of a string and a map[string][]string has no type
	// json.Marshal refuses. Guarding it would be a branch no caller can reach,
	// which is the same thing as untested code that claims to be tested.
	body, _ := json.Marshal(map[string]any{"collection": collection, "meta": meta})
	resp, err := c.doRequest("POST", "/v1/validate", body)
	if err != nil {
		return result, false, err
	}
	defer func() { _ = resp.Body.Close() }()

	// doRequest passes 404 through rather than treating it as an error, so an
	// older server answering "no such route" is recognisable here.
	if resp.StatusCode == http.StatusNotFound {
		return result, true, nil
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return result, false, fmt.Errorf("decoding the validation result: %w", err)
	}
	return result, false, nil
}
