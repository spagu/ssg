// Package mcp implements the `ssg mcp` development server: a minimal Model
// Context Protocol server (JSON-RPC 2.0 over stdio) that lets an AI assistant edit
// a site live during development. It exposes two sections of tools — designer
// (templates) and content manager (content) — and, when git is configured, a git
// write-back flow (branch → commit → pull request). See help.go for the guidance
// surfaced to the model (#1.8.16).
package mcp

import "encoding/json"

// protocolVersion is the MCP revision this server implements. The client's
// requested version is echoed back when present; this is the fallback.
const protocolVersion = "2024-11-05"

// rpcRequest is an incoming JSON-RPC 2.0 request or notification. A notification
// has no id and receives no response.
type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

func (r rpcRequest) isNotification() bool { return len(r.ID) == 0 }

// rpcResponse is a JSON-RPC 2.0 response: exactly one of Result or Error.
type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

// JSON-RPC / MCP error codes.
const (
	codeParse          = -32700
	codeInvalidRequest = -32600
	codeMethodNotFound = -32601
	codeInvalidParams  = -32602
	codeInternal       = -32603
)

// toolDef is the wire shape of a tool in a tools/list response.
type toolDef struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	InputSchema any    `json:"inputSchema"`
}

// toolResult is the wire shape of a tools/call response.
type toolResult struct {
	Content []textContent `json:"content"`
	IsError bool          `json:"isError,omitempty"`
}

type textContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// textResult builds a successful single-text tool result.
func textResult(text string) toolResult {
	return toolResult{Content: []textContent{{Type: "text", Text: text}}}
}

// errResult builds a failed single-text tool result. Tool errors are reported in
// the result (isError) rather than as protocol errors, so the model can read and
// recover from them.
func errResult(text string) toolResult {
	return toolResult{Content: []textContent{{Type: "text", Text: text}}, IsError: true}
}

// callParams is the params shape of a tools/call request.
type callParams struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

// initializeResult is returned from the initialize handshake.
type initializeResult struct {
	ProtocolVersion string         `json:"protocolVersion"`
	Capabilities    map[string]any `json:"capabilities"`
	ServerInfo      map[string]any `json:"serverInfo"`
	Instructions    string         `json:"instructions,omitempty"`
}
