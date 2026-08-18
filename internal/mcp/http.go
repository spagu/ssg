package mcp

// The Streamable HTTP transport, so an MCP server can be reached over a network
// rather than only by a client that spawns it (#173).
//
// stdio is a local contract: the client launches the process and owns its
// standard streams. That is the right default and stays the default — but it
// cannot serve a CMS to an assistant running anywhere else, which is the whole
// reason this exists.
//
// The transport is the one the specification defines for that: a single MCP
// endpoint accepting POST, answering each request with a JSON object. Protocol
// semantics are identical on both bindings — `handle` does not know which one it
// is on — because a transport is a binding, not a dialect.
//
// The security rules here are the specification's MUST/SHOULD, and they are not
// decoration. A local MCP server reachable from a web page is a remote code
// execution path: this server writes files and runs git.
//
//   - The Origin header is validated, because without it any page in the
//     operator's browser can drive this server through DNS rebinding.
//   - Binding is to localhost unless told otherwise.
//   - A bearer token is required whenever the listener is not on localhost, and
//     one is generated rather than left to a default.

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
)

// maxRequestBytes bounds one JSON-RPC message. Tool payloads carry whole
// templates, so it matches the stdio scanner's limit rather than being smaller.
const maxRequestBytes = 16 << 20

// HTTPOptions configures the MCP endpoint.
type HTTPOptions struct {
	// Token, when set, must arrive as `Authorization: Bearer <token>`.
	Token string
	// AllowedOrigins lists the exact Origin values accepted. A request with no
	// Origin header is allowed — that is a non-browser client, which cannot be a
	// DNS-rebinding victim. Empty means no browser origin is accepted.
	AllowedOrigins []string
	// Logf receives one line per rejected request, so a client that cannot
	// connect can be diagnosed from the server's side.
	Logf func(string, ...any)
}

// HTTPHandler returns the MCP endpoint for s.
//
// Every JSON-RPC request is its own POST and gets its own response. GET and
// DELETE are answered 405: they belonged to the session and standalone-stream
// mechanics of earlier revisions, which this transport does not implement.
func HTTPHandler(s *Server, opts HTTPOptions) http.Handler {
	if opts.Logf == nil {
		opts.Logf = func(string, ...any) {}
	}
	return &httpTransport{server: s, opts: opts}
}

type httpTransport struct {
	server *Server
	opts   HTTPOptions
}

func (h *httpTransport) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if code, msg := h.reject(r); code != 0 {
		h.opts.Logf("   ⚠️  MCP %s %s → %d %s", r.Method, r.URL.Path, code, msg)
		writeRPCError(w, code, nil, codeInvalidRequest, msg)
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, maxRequestBytes))
	if err != nil {
		writeRPCError(w, http.StatusBadRequest, nil, codeParse, "cannot read request body")
		return
	}
	var req rpcRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeRPCError(w, http.StatusBadRequest, nil, codeParse, "parse error")
		return
	}

	// A notification has no reply to give, and the specification is explicit
	// about what an accepted one returns.
	if req.isNotification() {
		w.WriteHeader(http.StatusAccepted)
		return
	}

	resp, _ := h.server.handle(req)
	w.Header().Set("Content-Type", "application/json")
	// The version this exchange spoke, so an intermediary can route on it.
	w.Header().Set("MCP-Protocol-Version", negotiatedVersion(r))
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}

// reject applies the transport's rules, returning the HTTP status and message
// for a request that must not reach the server, or 0 to proceed.
func (h *httpTransport) reject(r *http.Request) (int, string) {
	// Earlier revisions used GET for a standalone stream and DELETE to end a
	// session; neither exists here, and 405 is what says so.
	if r.Method != http.MethodPost {
		return http.StatusMethodNotAllowed, "the MCP endpoint accepts POST"
	}
	if origin := r.Header.Get("Origin"); origin != "" && !h.originAllowed(origin) {
		// DNS rebinding is the attack this closes: a page the operator visits
		// resolving a name to 127.0.0.1 and then driving this server.
		return http.StatusForbidden, "origin not allowed"
	}
	if !h.authorized(r) {
		return http.StatusUnauthorized, "a bearer token is required"
	}
	return 0, ""
}

// originAllowed reports whether a browser origin is on the list. Comparison is
// exact after normalising the scheme and host: an origin is a security boundary,
// so no prefix or suffix matching is done on it.
func (h *httpTransport) originAllowed(origin string) bool {
	got, err := url.Parse(origin)
	if err != nil {
		return false
	}
	for _, allowed := range h.opts.AllowedOrigins {
		want, err := url.Parse(strings.TrimSpace(allowed))
		if err != nil {
			continue
		}
		if strings.EqualFold(got.Scheme, want.Scheme) && strings.EqualFold(got.Host, want.Host) {
			return true
		}
	}
	return false
}

// authorized checks the bearer token in constant time. No token configured
// means the endpoint is open, which the CLI permits only on localhost.
func (h *httpTransport) authorized(r *http.Request) bool {
	if h.opts.Token == "" {
		return true
	}
	got, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	if !ok {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(strings.TrimSpace(got)), []byte(h.opts.Token)) == 1
}

// negotiatedVersion echoes the client's requested protocol version when it sent
// one. The body remains the source of truth; this mirrors it for intermediaries.
func negotiatedVersion(r *http.Request) string {
	if v := strings.TrimSpace(r.Header.Get("MCP-Protocol-Version")); v != "" {
		return v
	}
	return protocolVersion
}

// writeRPCError answers with an HTTP status and a JSON-RPC error body. The body
// is what lets a client tell a modern server's refusal from a plain 404 by an
// unrelated web server on the same port.
func writeRPCError(w http.ResponseWriter, status int, id json.RawMessage, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(rpcResponse{
		JSONRPC: "2.0", ID: id, Error: &rpcError{Code: code, Message: msg},
	})
}

// NewToken returns a random bearer token. A server reachable off localhost
// without one is an open remote-code-execution endpoint, so the CLI mints this
// rather than defaulting to nothing.
func NewToken() (string, error) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generating a token: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// IsLoopback reports whether a listen address binds only the loopback
// interface. It decides whether a token is required, so an unparseable or
// wildcard address is treated as public.
func IsLoopback(addr string) bool {
	host, _, err := net.SplitHostPort(strings.TrimSpace(addr))
	if err != nil {
		host = strings.TrimSpace(addr)
	}
	if host == "" {
		return false // ":7823" binds every interface
	}
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(strings.Trim(host, "[]"))
	return ip != nil && ip.IsLoopback()
}
