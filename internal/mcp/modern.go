package mcp

// The 2026-07-28 shape, beside the initialize-based one (#174).
//
// That revision did not extend the protocol, it changed it: there is no
// `initialize` handshake, every request carries its own protocol version and
// client capabilities in `_meta`, `server/discover` is mandatory, and every
// result carries a `resultType`. A server can speak both eras at once, and must
// — clients on the older shape are still the majority, and the specification
// defines the era-detection fallback precisely so neither side is stranded.
//
// So nothing here replaces the existing path. A request carrying
// `_meta.io.modelcontextprotocol/protocolVersion` is answered in the modern
// shape; a request that does not is answered exactly as before. The tools are
// the same tools either way — this is a lifecycle and envelope change, not a
// change to what the server does.

import (
	"encoding/json"
	"strings"
)

// Protocol revisions this server speaks, newest first. Both are advertised by
// server/discover, and a client asking for either gets it.
var supportedVersions = []string{"2026-07-28", "2025-06-18"}

// modernVersion is the revision whose stateless shape this file implements.
const modernVersion = "2026-07-28"

// _meta keys the modern shape carries on every request.
const (
	metaProtocolVersion    = "io.modelcontextprotocol/protocolVersion"
	metaClientInfo         = "io.modelcontextprotocol/clientInfo"
	metaClientCapabilities = "io.modelcontextprotocol/clientCapabilities"
	metaServerInfo         = "io.modelcontextprotocol/serverInfo"
)

// MCP-allocated error codes. The specification reserves -32020..-32099 for
// itself; -32000..-32019 stays implementation-defined.
const (
	codeHeaderMismatch             = -32020
	codeMissingRequiredCapability  = -32021
	codeUnsupportedProtocolVersion = -32022
)

// requestMeta is the per-request envelope of the stateless shape.
type requestMeta struct {
	ProtocolVersion string          `json:"io.modelcontextprotocol/protocolVersion"`
	ClientInfo      json.RawMessage `json:"io.modelcontextprotocol/clientInfo"`
	Capabilities    json.RawMessage `json:"io.modelcontextprotocol/clientCapabilities"`
}

// metaOf reads the _meta block from a request's params. An absent or
// unparseable block is not an error: it simply means the request came from the
// initialize era, which is answered in that era's shape.
func metaOf(params json.RawMessage) requestMeta {
	var p struct {
		Meta requestMeta `json:"_meta"`
	}
	if len(params) == 0 {
		return requestMeta{}
	}
	_ = json.Unmarshal(params, &p)
	return p.Meta
}

// isModern reports whether a request declared a protocol version in _meta,
// which is what distinguishes the two eras on the wire.
func (m requestMeta) isModern() bool {
	return strings.TrimSpace(m.ProtocolVersion) != ""
}

// supports reports whether this server implements the named revision.
func supports(version string) bool {
	for _, v := range supportedVersions {
		if v == version {
			return true
		}
	}
	return false
}

// unsupportedVersionError is what a client gets for a revision this server does
// not implement: the specification requires the supported list, so the client
// can retry with one rather than guess.
func unsupportedVersionError(requested string) *rpcError {
	return &rpcError{
		Code:    codeUnsupportedProtocolVersion,
		Message: "unsupported protocol version: " + requested,
		Data:    map[string]any{"supported": supportedVersions},
	}
}

// cacheableResult carries the freshness hints the modern shape requires on
// every list result, so a client can cache instead of polling.
type cacheableResult struct {
	// TTLMs is a freshness hint. A development server's tool list changes only
	// when the binary does, so this is generous rather than cautious — the
	// point of the field is fewer round trips.
	TTLMs int `json:"ttlMs"`
	// CacheScope is "public" or "private". This server's tool list depends on
	// the roles it was started with, so it is private to the client that asked.
	CacheScope string `json:"cacheScope"`
}

// listCacheHints is what every list result carries.
func listCacheHints() cacheableResult {
	return cacheableResult{TTLMs: 60_000, CacheScope: "private"}
}

// discoverResult answers server/discover: what this server is, which revisions
// it speaks, and what it can do. A client MAY call it before anything else, and
// on stdio it doubles as the era probe.
type discoverResult struct {
	ResultType string   `json:"resultType"`
	Versions   []string `json:"protocolVersions"`
	ServerInfo any      `json:"serverInfo"`
	Caps       any      `json:"capabilities"`
	Meta       any      `json:"_meta,omitempty"`
}

// discover builds the server/discover result.
func (s *Server) discover() discoverResult {
	return discoverResult{
		ResultType: "complete",
		Versions:   supportedVersions,
		ServerInfo: s.serverInfo(),
		Caps:       map[string]any{"tools": map[string]any{"listChanged": false}},
		Meta:       map[string]any{metaServerInfo: s.serverInfo()},
	}
}

// serverInfo identifies this server. The modern shape asks for it in every
// result's _meta, so it is built once here.
func (s *Server) serverInfo() map[string]any {
	version := s.opts.Version
	if version == "" {
		version = "dev"
	}
	return map[string]any{"name": "ssg", "version": version}
}

// modernResult wraps a result in the envelope the modern shape requires: a
// resultType on everything, and the server's identity in _meta.
func (s *Server) modernResult(inner map[string]any) map[string]any {
	out := map[string]any{"resultType": "complete"}
	for k, v := range inner {
		out[k] = v
	}
	out["_meta"] = map[string]any{metaServerInfo: s.serverInfo()}
	return out
}

// handleModern answers a request that declared a protocol version, in the
// stateless shape. The tools are the same tools; only the envelope differs.
func (s *Server) handleModern(req rpcRequest, meta requestMeta) (rpcResponse, bool) {
	base := rpcResponse{JSONRPC: "2.0", ID: req.ID}
	if !supports(meta.ProtocolVersion) {
		base.Error = unsupportedVersionError(meta.ProtocolVersion)
		return base, !req.isNotification()
	}
	if req.isNotification() {
		return rpcResponse{}, false
	}

	switch req.Method {
	case "server/discover":
		base.Result = s.discover()
	case "tools/list":
		hints := listCacheHints()
		base.Result = s.modernResult(map[string]any{
			"tools":      s.toolDefs(),
			"ttlMs":      hints.TTLMs,
			"cacheScope": hints.CacheScope,
		})
	case "tools/call":
		result := s.callTool(req.Params)
		base.Result = s.modernResult(map[string]any{
			"content": result.Content,
			"isError": result.IsError,
		})
	case "initialize":
		// An initialize carrying modern _meta is a client hedging between eras.
		// Answering it keeps that client working rather than making it guess.
		base.Result = s.initialize(req.Params)
	default:
		base.Error = &rpcError{Code: codeMethodNotFound, Message: "unknown method: " + req.Method}
	}
	return base, true
}
