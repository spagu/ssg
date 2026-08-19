package mcp

// Validating the mirrored request headers (#174).
//
// The Streamable HTTP transport mirrors selected body fields into HTTP headers
// so load balancers and gateways can route without parsing the body. That is
// useful and it is also a hazard: if an intermediary routes on the header while
// the server executes on the body, the two can disagree, and the disagreement is
// exactly where a request ends up somewhere it was not authorised for.
//
// So the server validates them against the body and rejects a mismatch with
// HeaderMismatch (-32020). A value that cannot be written as a plain ASCII
// header travels base64-wrapped in a sentinel, which is decoded before the
// comparison — otherwise every tool with a non-ASCII name would look like an
// attack.

import (
	"encoding/base64"
	"encoding/json"
	"strings"
)

// Sentinel markers for a base64-wrapped header value. Case-sensitive, exactly
// as the specification writes them.
const (
	b64Prefix = "=?base64?"
	b64Suffix = "?="
)

// decodeHeaderValue unwraps a base64-sentinel header value, returning the value
// and whether it was well-formed. A plain value passes through.
func decodeHeaderValue(v string) (string, bool) {
	if !strings.HasPrefix(v, b64Prefix) || !strings.HasSuffix(v, b64Suffix) {
		return v, true
	}
	body := v[len(b64Prefix) : len(v)-len(b64Suffix)]
	raw, err := base64.StdEncoding.DecodeString(body)
	if err != nil {
		return "", false
	}
	return string(raw), true
}

// headerSubject returns the body value the Mcp-Name header must mirror: the
// tool name, the resource URI or the prompt name, depending on the method.
// An empty string means the method carries no name to mirror.
func headerSubject(method string, params json.RawMessage) string {
	var p struct {
		Name string `json:"name"`
		URI  string `json:"uri"`
	}
	if len(params) > 0 {
		_ = json.Unmarshal(params, &p)
	}
	switch method {
	case "tools/call", "prompts/get":
		return p.Name
	case "resources/read":
		return p.URI
	}
	return ""
}

// headerMismatch describes a rejected request, or nil when the headers agree
// with the body.
type headerMismatch struct{ reason string }

// validateHeaders compares the mirrored headers against the request body.
//
// It is applied only to a request already identified as modern: the headers are
// required from 2026-07-28 onward, and demanding them from an older client
// would reject every request it has ever sent.
func validateHeaders(req rpcRequest, method, name string) *headerMismatch {
	if got := strings.TrimSpace(method); got == "" {
		return &headerMismatch{"the Mcp-Method header is required"}
	} else if got != req.Method {
		return &headerMismatch{"Mcp-Method header value " + quote(got) +
			" does not match body value " + quote(req.Method)}
	}

	want := headerSubject(req.Method, req.Params)
	if want == "" {
		return nil // this method mirrors no name
	}
	decoded, ok := decodeHeaderValue(strings.TrimSpace(name))
	if !ok {
		return &headerMismatch{"the Mcp-Name header is not valid base64"}
	}
	if decoded == "" {
		return &headerMismatch{"the Mcp-Name header is required for " + req.Method}
	}
	if decoded != want {
		return &headerMismatch{"Mcp-Name header value " + quote(decoded) +
			" does not match body value " + quote(want)}
	}
	return nil
}

// quote wraps a value for an error message.
func quote(s string) string { return "'" + s + "'" }
