package mcp

import "strings"

// tool is one MCP tool: its wire metadata plus the handler that runs it.
type tool struct {
	name        string
	description string
	schema      any
	handler     func(args map[string]any) toolResult
}

// buildTools assembles the registry from the enabled roles and git config. The
// help tool is always present; designer and content tools depend on Roles; git
// tools appear only when git write-back is configured.
func (s *Server) buildTools() []tool {
	tools := []tool{s.helpTool()}
	if s.opts.Roles["designer"] {
		tools = append(tools, s.designerTools()...)
	}
	if s.opts.Roles["content"] {
		tools = append(tools, s.contentTools()...)
	}
	if s.opts.Git.Enabled() {
		tools = append(tools, s.gitTools()...)
	}
	return tools
}

// objectSchema builds a JSON Schema object with the given properties and required
// keys.
func objectSchema(props map[string]any, required ...string) map[string]any {
	if props == nil {
		props = map[string]any{}
	}
	if required == nil {
		required = []string{}
	}
	return map[string]any{"type": "object", "properties": props, "required": required}
}

// stringProp is a string property with a description.
func stringProp(desc string) map[string]any {
	return map[string]any{"type": "string", "description": desc}
}

// afterMutate reports a mutation, rebuilding the site first when watch is on so a
// running dev server live-reloads and any template/content error is surfaced back
// to the model.
func (s *Server) afterMutate(summary string) toolResult {
	if !s.opts.Watch || s.opts.Rebuild == nil {
		return textResult(summary)
	}
	out, err := s.opts.Rebuild()
	if err != nil {
		return errResult(summary + "\n\n⚠️ the site did NOT rebuild — fix this before continuing:\n" + tail(out, 40) + "\n" + err.Error())
	}
	note := "\n\n✅ rebuilt cleanly"
	if t := tail(out, 8); t != "" {
		note += ":\n" + t
	}
	return textResult(summary + note)
}

// tail returns the last n non-empty lines of s, for compact build/command output.
func tail(s string, n int) string {
	lines := make([]string, 0, 16)
	for _, l := range strings.Split(s, "\n") {
		if strings.TrimSpace(l) != "" {
			lines = append(lines, l)
		}
	}
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}
