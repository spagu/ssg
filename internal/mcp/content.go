package mcp

import (
	"fmt"
	"os"
	"strings"
)

// contentExts are the file types the content section manages.
var contentExts = []string{".md", ".markdown"}

// contentTools is the content-manager section: it adds, updates, fixes and removes
// Markdown content. It never edits templates or theme files (that is the designer's
// job).
func (s *Server) contentTools() []tool {
	bases := strings.Join(s.opts.ContentDirs, ", ")
	return []tool{
		{
			name: "content_list",
			description: "CONTENT · List every Markdown content file (under: " + bases + "). " +
				"Start here to see what exists before adding or editing. Returns project-relative paths.",
			schema:  objectSchema(nil),
			handler: s.contentList,
		},
		{
			name: "content_read",
			description: "CONTENT · Read one Markdown file (frontmatter + body) before editing or " +
				"fixing it. `path` is project-relative (e.g. \"content/posts/hello.md\").",
			schema:  objectSchema(map[string]any{"path": stringProp("Project-relative path to the Markdown file")}, "path"),
			handler: s.contentRead,
		},
		{
			name: "content_create",
			description: "CONTENT · Create a NEW Markdown file. Provide the full document including " +
				"YAML frontmatter (title, date, tags, …) and the Markdown body. CAN: add new posts/" +
				"pages under " + bases + ". CANNOT: overwrite an existing file (use content_update), " +
				"edit templates, or write outside content. Fails if the path already exists.",
			schema: objectSchema(map[string]any{
				"path":    stringProp("Project-relative path for the new file, ending .md"),
				"content": stringProp("The complete Markdown document (frontmatter + body)"),
			}, "path", "content"),
			handler: s.contentCreate,
		},
		{
			name: "content_update",
			description: "CONTENT · Replace an EXISTING Markdown file with corrected/updated content. " +
				"Use this to fix typos, rewrite sections, or change frontmatter. Provide the full new " +
				"document. Fails if the file does not exist (use content_create for new files).",
			schema: objectSchema(map[string]any{
				"path":    stringProp("Project-relative path to the existing Markdown file"),
				"content": stringProp("The complete new document (full replacement, not a patch)"),
			}, "path", "content"),
			handler: s.contentUpdate,
		},
		{
			name: "content_delete",
			description: "CONTENT · Remove a Markdown file. Use when a post/page should no longer exist. " +
				"CANNOT delete anything outside the content directories. This is destructive — only do " +
				"it when explicitly asked.",
			schema:  objectSchema(map[string]any{"path": stringProp("Project-relative path to the Markdown file to delete")}, "path"),
			handler: s.contentDelete,
		},
		{
			name: "content_edit",
			description: "CONTENT · Change ONE passage of a Markdown file in place — PREFER THIS over " +
				"content_update for a typo, a sentence, or one frontmatter value. Give the exact " +
				"existing text as `old` and its replacement as `new`; `old` must appear exactly once. " +
				"Returns the change in context, so no verifying re-read is needed \u2014 bounded by " +
				"characters in a very long paragraph, so the reply never grows to the file. Refuses " +
				"(naming the count) when `old` matches zero or several times.",
			schema:  editSchema("Project-relative path to the existing Markdown file"),
			handler: s.contentEdit,
		},
		s.contentFindTool(),
	}
}

func (s *Server) contentEdit(args map[string]any) toolResult {
	return s.runEdit(args, func(rel string) (string, error) {
		abs, _, err := resolveContent(s, rel)
		if err != nil {
			return "", err
		}
		if !fileExists(abs) {
			return "", fmt.Errorf("%q does not exist — use content_create for new files", rel)
		}
		return abs, nil
	}, "content")
}

func (s *Server) contentList(map[string]any) toolResult {
	files, err := listFiles(s.opts.Root, s.opts.ContentDirs, contentExts...)
	if err != nil {
		return errResult("list failed: " + err.Error())
	}
	if len(files) == 0 {
		return textResult("No content files yet under: " + strings.Join(s.opts.ContentDirs, ", "))
	}
	return textResult(strings.Join(files, "\n"))
}

func (s *Server) contentRead(args map[string]any) toolResult {
	rel, _ := strArg(args, "path")
	abs, _, err := resolveIn(s.opts.Root, s.opts.ContentDirs, rel)
	if err != nil {
		return errResult(err.Error())
	}
	b, err := os.ReadFile(abs) // #nosec G304 -- confined to content dirs by resolveIn
	if err != nil {
		return errResult("read failed: " + err.Error())
	}
	return textResult(string(b))
}

func (s *Server) contentCreate(args map[string]any) toolResult {
	rel, _ := strArg(args, "path")
	content, ok := strArg(args, "content")
	if !ok {
		return errResult("`content` is required (the full Markdown document)")
	}
	abs, _, err := resolveContent(s, rel)
	if err != nil {
		return errResult(err.Error())
	}
	if fileExists(abs) {
		return errResult(fmt.Sprintf("%q already exists — use content_update to change it", rel))
	}
	if err := writeFile(abs, content); err != nil {
		return errResult("create failed: " + err.Error())
	}
	return s.afterMutate("content created " + rel)
}

func (s *Server) contentUpdate(args map[string]any) toolResult {
	rel, _ := strArg(args, "path")
	content, ok := strArg(args, "content")
	if !ok {
		return errResult("`content` is required (the full new document)")
	}
	abs, _, err := resolveContent(s, rel)
	if err != nil {
		return errResult(err.Error())
	}
	if !fileExists(abs) {
		return errResult(fmt.Sprintf("%q does not exist — use content_create for new files", rel))
	}
	if err := writeFile(abs, content); err != nil {
		return errResult("update failed: " + err.Error())
	}
	return s.afterMutate("content updated " + rel)
}

func (s *Server) contentDelete(args map[string]any) toolResult {
	rel, _ := strArg(args, "path")
	abs, _, err := resolveContent(s, rel)
	if err != nil {
		return errResult(err.Error())
	}
	if !fileExists(abs) {
		return errResult(fmt.Sprintf("%q does not exist", rel))
	}
	if err := os.Remove(abs); err != nil {
		return errResult("delete failed: " + err.Error())
	}
	return s.afterMutate("content deleted " + rel)
}

// resolveContent resolves a content path and enforces the Markdown extension so
// the content section cannot write arbitrary file types.
func resolveContent(s *Server, rel string) (string, string, error) {
	abs, base, err := resolveIn(s.opts.Root, s.opts.ContentDirs, rel)
	if err != nil {
		return "", "", err
	}
	if !hasExt(abs, contentExts) {
		return "", "", fmt.Errorf("content files must end in .md or .markdown, got %q", rel)
	}
	return abs, base, nil
}
