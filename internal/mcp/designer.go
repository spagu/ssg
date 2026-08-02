package mcp

import (
	"fmt"
	"os"
	"strings"
)

// designerBases is the set of directories the designer section may touch:
// templates and any static/theme asset directories.
func (s *Server) designerBases() []string {
	return append(append([]string{}, s.opts.TemplateDirs...), s.opts.StaticDirs...)
}

// designerTools is the designer section: it changes how the site looks by editing
// templates and theme assets. It never touches content and never deletes files.
func (s *Server) designerTools() []tool {
	bases := strings.Join(s.designerBases(), ", ")
	return []tool{
		{
			name: "designer_list",
			description: "DESIGNER · List every template and theme-asset file you may edit " +
				"(under: " + bases + "). Start here to see the layout before changing anything. " +
				"Returns project-relative paths.",
			schema:  objectSchema(nil),
			handler: s.designerList,
		},
		{
			name: "designer_read",
			description: "DESIGNER · Read one template or theme-asset file so you can see the " +
				"current markup/styles before editing. `path` is project-relative (e.g. " +
				"\"templates/post.html\").",
			schema:  objectSchema(map[string]any{"path": stringProp("Project-relative path to the template/asset file")}, "path"),
			handler: s.designerRead,
		},
		{
			name: "designer_write",
			description: "DESIGNER · Create or replace a template or theme-asset file with the full " +
				"new content. Use this to change layout, markup, CSS or partials. CAN: edit/create " +
				"files under " + bases + ". CANNOT: edit content (use the content_* tools), delete " +
				"files, or write outside those directories. In watch mode the site rebuilds " +
				"immediately and any template error is returned to you — fix it before moving on.",
			schema: objectSchema(map[string]any{
				"path":    stringProp("Project-relative path under a template/asset directory"),
				"content": stringProp("The complete new file content (full replacement, not a patch)"),
			}, "path", "content"),
			handler: s.designerWrite,
		},
	}
}

func (s *Server) designerList(map[string]any) toolResult {
	files, err := listFiles(s.opts.Root, s.designerBases())
	if err != nil {
		return errResult("list failed: " + err.Error())
	}
	if len(files) == 0 {
		return textResult("No template/asset files yet under: " + strings.Join(s.designerBases(), ", "))
	}
	return textResult(strings.Join(files, "\n"))
}

func (s *Server) designerRead(args map[string]any) toolResult {
	rel, _ := strArg(args, "path")
	abs, _, err := resolveIn(s.opts.Root, s.designerBases(), rel)
	if err != nil {
		return errResult(err.Error())
	}
	b, err := os.ReadFile(abs) // #nosec G304 -- confined to template/asset dirs by resolveIn
	if err != nil {
		return errResult("read failed: " + err.Error())
	}
	return textResult(string(b))
}

func (s *Server) designerWrite(args map[string]any) toolResult {
	rel, _ := strArg(args, "path")
	content, ok := strArg(args, "content")
	if !ok {
		return errResult("`content` is required (the full new file content)")
	}
	abs, _, err := resolveIn(s.opts.Root, s.designerBases(), rel)
	if err != nil {
		return errResult(err.Error())
	}
	existed := fileExists(abs)
	if err := writeFile(abs, content); err != nil {
		return errResult("write failed: " + err.Error())
	}
	verb := "created"
	if existed {
		verb = "updated"
	}
	return s.afterMutate(fmt.Sprintf("designer %s %s", verb, rel))
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
