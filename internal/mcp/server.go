package mcp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// Options configures a Server. Root is the project root; the designer and content
// tools are confined to TemplateDirs / ContentDirs beneath it. Rebuild (when set
// and Watch is true) is called after every successful mutation so a running dev
// server live-reloads. Logf writes human progress to stderr — stdout is the
// JSON-RPC channel and must never carry log noise.
type Options struct {
	Root         string
	TemplateDirs []string
	StaticDirs   []string
	ContentDirs  []string
	Roles        map[string]bool // "designer" and/or "content"; empty ⇒ both
	Git          GitOptions
	Watch        bool
	Version      string
	Rebuild      func() (string, error)
	Logf         func(string, ...any)

	// ConfigPath is the site config file the designer may edit presentation keys
	// in; empty disables the config tools. ValidateConfig re-loads it after an
	// edit so an invalid change can be rolled back.
	ConfigPath     string
	ValidateConfig func(path string) error
}

// Server is a running MCP stdio server.
type Server struct {
	opts   Options
	tools  []tool
	byName map[string]tool
}

// NewServer builds a server and its tool registry from opts.
func NewServer(opts Options) *Server {
	if opts.Logf == nil {
		opts.Logf = func(string, ...any) {}
	}
	if len(opts.Roles) == 0 {
		opts.Roles = map[string]bool{"designer": true, "content": true}
	}
	s := &Server{opts: opts}
	s.tools = s.buildTools()
	s.byName = make(map[string]tool, len(s.tools))
	for _, t := range s.tools {
		s.byName[t.name] = t
	}
	return s
}

// Serve reads newline-delimited JSON-RPC messages from in and writes responses to
// out until in reaches EOF. Notifications receive no response.
func (s *Server) Serve(in io.Reader, out io.Writer) error {
	sc := bufio.NewScanner(in)
	sc.Buffer(make([]byte, 0, 64*1024), 16*1024*1024) // allow large tool payloads
	enc := json.NewEncoder(out)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var req rpcRequest
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			_ = enc.Encode(rpcResponse{JSONRPC: "2.0", Error: &rpcError{Code: codeParse, Message: "parse error"}})
			continue
		}
		resp, respond := s.handle(req)
		if respond {
			if err := enc.Encode(resp); err != nil {
				return err
			}
		}
	}
	return sc.Err()
}

// handle dispatches one request. The second return is false for notifications
// (no response is written).
func (s *Server) handle(req rpcRequest) (rpcResponse, bool) {
	if req.isNotification() {
		return rpcResponse{}, false
	}
	base := rpcResponse{JSONRPC: "2.0", ID: req.ID}
	switch req.Method {
	case "initialize":
		base.Result = s.initialize(req.Params)
	case "ping":
		base.Result = map[string]any{}
	case "tools/list":
		base.Result = map[string]any{"tools": s.toolDefs()}
	case "tools/call":
		base.Result = s.callTool(req.Params)
	default:
		base.Error = &rpcError{Code: codeMethodNotFound, Message: "unknown method: " + req.Method}
	}
	return base, true
}

// initialize answers the handshake, echoing the client's protocol version and
// surfacing the section guidance as instructions.
func (s *Server) initialize(params json.RawMessage) initializeResult {
	ver := protocolVersion
	var p struct {
		ProtocolVersion string `json:"protocolVersion"`
	}
	if len(params) > 0 && json.Unmarshal(params, &p) == nil && p.ProtocolVersion != "" {
		ver = p.ProtocolVersion
	}
	version := s.opts.Version
	if version == "" {
		version = "dev"
	}
	return initializeResult{
		ProtocolVersion: ver,
		Capabilities:    map[string]any{"tools": map[string]any{"listChanged": false}},
		ServerInfo:      map[string]any{"name": "ssg", "version": version},
		Instructions:    s.instructions(),
	}
}

// toolDefs renders the registry as wire tool definitions.
func (s *Server) toolDefs() []toolDef {
	defs := make([]toolDef, 0, len(s.tools))
	for _, t := range s.tools {
		defs = append(defs, toolDef{Name: t.name, Description: t.description, InputSchema: t.schema})
	}
	return defs
}

// callTool routes a tools/call to its handler, reporting unknown tools and bad
// arguments as tool errors the model can read.
func (s *Server) callTool(params json.RawMessage) toolResult {
	var cp callParams
	if err := json.Unmarshal(params, &cp); err != nil {
		return errResult("invalid tools/call params: " + err.Error())
	}
	t, ok := s.byName[cp.Name]
	if !ok {
		return errResult(fmt.Sprintf("unknown tool %q — call \"help\" to list the available tools", cp.Name))
	}
	args := map[string]any{}
	if len(cp.Arguments) > 0 {
		if err := json.Unmarshal(cp.Arguments, &args); err != nil {
			return errResult("invalid arguments: " + err.Error())
		}
	}
	return t.handler(args)
}
