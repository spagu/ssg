package mcp

import "strings"

// instructions is the guidance handed to the client at initialize time: what each
// section may and may not do, and the expected workflow. This is the model-facing
// contract for the whole server.
func (s *Server) instructions() string {
	var b strings.Builder
	b.WriteString("SSG development server. Two roles, each with its own tools:\n\n")
	if s.opts.Roles["designer"] {
		b.WriteString("DESIGNER (designer_*) — changes how the site LOOKS.\n")
		b.WriteString("  CAN: list/read/create/update templates, partials, CSS and theme assets under " +
			strings.Join(s.designerBases(), ", ") + ".\n")
		if s.opts.ConfigPath != "" {
			b.WriteString("  CAN: read and set presentation settings in " + s.opts.ConfigPath +
				" (theme, highlight style, diagrams, minification) — see designer_config_read.\n")
			b.WriteString("  CANNOT: change any other configuration key — secrets, deployment, server, " +
				"endpoints, hooks and content/URL structure are refused.\n")
		}
		b.WriteString("  CANNOT: touch content, delete files, or write outside those directories.\n\n")
	}
	if s.opts.Roles["content"] {
		b.WriteString("CONTENT MANAGER (content_*) — changes what the site SAYS.\n")
		b.WriteString("  CAN: list/read/create/update/delete Markdown (frontmatter + body) under " +
			strings.Join(s.opts.ContentDirs, ", ") + ".\n")
		b.WriteString("  CANNOT: touch templates or styles, or write non-Markdown files.\n\n")
	}
	if s.opts.Git.Enabled() {
		b.WriteString("GIT (git_*) — safe write-back.\n")
		b.WriteString("  Flow: git_new_branch → edit → git_commit → human reviews → git_open_pr.\n")
		b.WriteString("  Never commit to the base branch; git_open_pr is the explicit, human-approved final step.\n\n")
	}
	b.WriteString("Workflow: to change something, FIND it first — designer_find / content_find return the " +
		"file and line range without reading whole files, and that range is the anchor an edit needs. " +
		"Fall back to listing and reading only when a search finds nothing. Make the smallest change " +
		"that satisfies the request. For a small change use designer_edit / content_edit: give the exact existing " +
		"text as `old` and its replacement as `new`, and the reply shows the change in context " +
		"— no verifying re-read. `old` must match exactly once, or the edit is refused with the count. " +
		"Reserve the whole-file writes (designer_write, content_create, content_update) for new files " +
		"and real rewrites; those take FULL file contents, never a patch.")
	if s.opts.Watch {
		b.WriteString(" The site rebuilds after every change — if a rebuild error comes back, fix it before anything else.")
	}
	b.WriteString(" Call the \"help\" tool anytime for this contract plus the full tool list.")
	return b.String()
}

// helpTool returns the always-present help tool: the section contract plus a
// per-tool reference, so a model can (re)discover what it may and may not do.
func (s *Server) helpTool() tool {
	return tool{
		name: "help",
		description: "Explain this server: what the designer and content-manager roles can and cannot " +
			"do, the git workflow (when configured), and every available tool with its purpose. Call " +
			"this first if you are unsure which tool to use.",
		schema: objectSchema(nil),
		handler: func(map[string]any) toolResult {
			var b strings.Builder
			b.WriteString(s.instructions())
			b.WriteString("\n\nTools:\n")
			for _, t := range s.tools {
				b.WriteString("  " + t.name + " — " + firstSentence(t.description) + "\n")
			}
			return textResult(b.String())
		},
	}
}

// firstSentence trims a description to its first sentence for the compact list.
func firstSentence(s string) string {
	if i := strings.Index(s, ". "); i > 0 {
		return s[:i+1]
	}
	return s
}
