package mcp

// The media tools themselves (#214).

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// mediaTools is the media section: it adds, replaces and removes the pictures a
// site serves. Both roles get it — a photograph is neither purely presentation
// nor purely content, and the owner asking for it does not know the difference.
func (s *Server) mediaTools() []tool {
	bases := strings.Join(s.mediaBases(), ", ")
	payload := map[string]any{
		"path": stringProp("Project-relative path for the file, under " + bases),
		"content_base64": stringProp("The file's bytes, base64-encoded. Use this when you already " +
			"have the image; otherwise give `url` instead."),
		"url": stringProp("An http(s) URL the server downloads. Use this when the owner pasted a " +
			"link or the picture was produced elsewhere."),
	}
	return []tool{
		{
			name: "media_list",
			description: "MEDIA · List every image the site serves (under: " + bases + ") with its " +
				"size and format. Start here to see what exists before adding or replacing anything.",
			schema:  objectSchema(nil),
			handler: s.mediaList,
		},
		{
			name: "media_upload",
			description: "MEDIA · Add a NEW image, from base64 bytes or a URL the server downloads. " +
				"CAN: write images (jpeg, png, gif, webp) under " + bases + ". CANNOT: overwrite an " +
				"existing file (use media_replace), write outside those directories, or write " +
				"anything that is not an image — the format is decided by the file's own bytes, not " +
				"by its name. After it lands, point a page at it with content_edit.",
			schema:  objectSchema(payload, "path"),
			handler: s.mediaUpload,
		},
		{
			name: "media_replace",
			description: "MEDIA · Replace an EXISTING image in place, keeping its path — so every " +
				"page referencing it changes at once, with no edits to any page. Fails if the path " +
				"does not exist (use media_upload for a new file).",
			schema:  objectSchema(payload, "path"),
			handler: s.mediaReplace,
		},
		{
			name: "media_delete",
			description: "MEDIA · Remove an image. Refuses while any page or template still " +
				"references it, naming them — deleting a file three pages point at turns three " +
				"pages into broken images. Destructive: only on an explicit request.",
			schema:  objectSchema(map[string]any{"path": stringProp("Project-relative path of the image to remove")}, "path"),
			handler: s.mediaDelete,
		},
	}
}

func (s *Server) mediaList(map[string]any) toolResult {
	files, err := listFiles(s.opts.Root, s.mediaBases())
	if err != nil {
		return errResult("list failed: " + err.Error())
	}
	var lines []string
	for _, rel := range files {
		info, statErr := os.Stat(joinProject(s.opts.Root, rel))
		if statErr != nil || info.IsDir() {
			continue
		}
		body, readErr := os.ReadFile(joinProject(s.opts.Root, rel)) // #nosec G304 -- listed from the section's own directories
		if readErr != nil {
			continue
		}
		kind, kindErr := mediaKind(body)
		if kindErr != nil {
			continue // not an image: the content and designer tools own it
		}
		lines = append(lines, fmt.Sprintf("%s  (%s, %d bytes)", rel, kind, info.Size()))
	}
	sort.Strings(lines)
	if len(lines) == 0 {
		return textResult("No images yet under: " + strings.Join(s.mediaBases(), ", "))
	}
	return textResult(strings.Join(lines, "\n"))
}

func (s *Server) mediaUpload(args map[string]any) toolResult {
	return s.writeMedia(args, false)
}

func (s *Server) mediaReplace(args map[string]any) toolResult {
	return s.writeMedia(args, true)
}

// writeMedia is the body both share. mustExist splits them for the same reason
// content_create and content_update are split: creating over an existing file
// and replacing a missing one are both mistakes, and separating them turns each
// into an error instead of silent data loss.
func (s *Server) writeMedia(args map[string]any, mustExist bool) toolResult {
	rel, _ := strArg(args, "path")
	abs, _, err := resolveIn(s.opts.Root, s.mediaBases(), rel)
	if err != nil {
		return errResult(err.Error())
	}
	switch existed := fileExists(abs); {
	case mustExist && !existed:
		return errResult(fmt.Sprintf("%q does not exist — use media_upload to add a new file", rel))
	case !mustExist && existed:
		return errResult(fmt.Sprintf("%q already exists — use media_replace to change it", rel))
	}

	data, origin, err := s.mediaBytes(args)
	if err != nil {
		return errResult(err.Error())
	}
	kind, err := mediaKind(data)
	if err != nil {
		return errResult(fmt.Sprintf("%s: %v", rel, err))
	}
	if err := writeBytes(abs, data); err != nil {
		return errResult("write failed: " + err.Error())
	}

	verb := "added"
	if mustExist {
		verb = "replaced"
	}
	summary := fmt.Sprintf("media %s %s (%s, %d bytes, from %s)", verb, rel, kind, len(data), origin)
	if !mustExist {
		summary += "\n\nNothing points at it yet — use content_edit to reference it from a page."
	}
	return s.afterMutate(summary)
}

func (s *Server) mediaDelete(args map[string]any) toolResult {
	rel, _ := strArg(args, "path")
	abs, _, err := resolveIn(s.opts.Root, s.mediaBases(), rel)
	if err != nil {
		return errResult(err.Error())
	}
	if !fileExists(abs) {
		return errResult(fmt.Sprintf("%q does not exist", rel))
	}
	if used := s.mediaReferences(rel); len(used) > 0 {
		return errResult(fmt.Sprintf("%q is still referenced by %d file(s): %s\n"+
			"Point them somewhere else first, or the pages that use it lose their image.",
			rel, len(used), strings.Join(used, ", ")))
	}
	if err := os.Remove(abs); err != nil {
		return errResult("delete failed: " + err.Error())
	}
	return s.afterMutate("media deleted " + rel)
}

// writeBytes creates parent directories and writes a file.
func writeBytes(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil { // #nosec G301 -- project source dir
		return err
	}
	return os.WriteFile(path, data, 0o644) // #nosec G306 -- a file the site serves
}
