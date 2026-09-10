package editui

// Editing a document's frontmatter (GO-102, phase 1).
//
// A Markdown file is a YAML document and a body separated by a fence, and the
// YAML half is edited by the same code that edits a config file (GO-101): the
// change is spliced into the text at the position the parser found, so the
// author's comments, key order and blank lines survive a save made from a
// browser exactly as they survive one made from a terminal.
//
// What this file adds is only the splitting and the rejoining — and the rule
// that the body is never touched by a frontmatter edit.

import (
	"fmt"
	"strings"

	"github.com/spagu/ssg/internal/config"
)

// fence is the frontmatter delimiter.
const fence = "---"

// document is one Markdown file, split.
type document struct {
	// lead is whatever precedes the opening fence: blank lines, and nothing
	// else in a well-formed file. Kept so a save restores it.
	lead        string
	frontmatter string
	body        string
	// bare marks a file with no frontmatter at all, which cannot take a
	// frontmatter edit without one being invented.
	bare bool
}

// splitDocument separates a Markdown file into its frontmatter and its body.
func splitDocument(src string) (document, error) {
	normalized := strings.ReplaceAll(src, "\r\n", "\n")
	rest := normalized
	lead := ""
	for {
		line, remainder, found := strings.Cut(rest, "\n")
		if strings.TrimSpace(line) == "" && found {
			lead += line + "\n"
			rest = remainder
			continue
		}
		break
	}
	if !strings.HasPrefix(strings.TrimSpace(rest), fence) {
		return document{lead: lead, body: rest, bare: true}, nil
	}
	_, afterOpen, _ := strings.Cut(rest, "\n")
	fm, body, closed := cutAtFence(afterOpen)
	if !closed {
		return document{}, fmt.Errorf("unclosed frontmatter (missing closing %q)", fence)
	}
	return document{lead: lead, frontmatter: fm, body: body}, nil
}

// cutAtFence splits at the first line that is exactly a fence.
func cutAtFence(s string) (before, after string, found bool) {
	var kept []string
	rest := s
	for {
		line, remainder, more := strings.Cut(rest, "\n")
		if strings.TrimSpace(line) == fence {
			return strings.Join(kept, "\n"), remainder, true
		}
		kept = append(kept, line)
		if !more {
			return strings.Join(kept, "\n"), "", false
		}
		rest = remainder
	}
}

// join puts a document back together.
func (d document) join() string {
	if d.bare {
		return d.lead + d.body
	}
	fm := strings.TrimRight(d.frontmatter, "\n")
	return d.lead + fence + "\n" + fm + "\n" + fence + "\n" + d.body
}

// setFrontmatter writes one frontmatter value, leaving the body alone.
func setFrontmatter(src, key string, value interface{}) (string, error) {
	doc, err := splitDocument(src)
	if err != nil {
		return "", err
	}
	if doc.bare {
		return "", fmt.Errorf("this file has no frontmatter to edit")
	}
	updated, err := config.SetYAMLPath([]byte(doc.frontmatter+"\n"), key, value)
	if err != nil {
		return "", err
	}
	doc.frontmatter = string(updated)
	return doc.join(), nil
}

// unsetFrontmatter removes one frontmatter key.
func unsetFrontmatter(src, key string) (string, error) {
	doc, err := splitDocument(src)
	if err != nil {
		return "", err
	}
	if doc.bare {
		return "", fmt.Errorf("this file has no frontmatter to edit")
	}
	updated, err := config.UnsetYAMLPath([]byte(doc.frontmatter+"\n"), key)
	if err != nil {
		return "", err
	}
	doc.frontmatter = string(updated)
	return doc.join(), nil
}

// readFrontmatter returns the frontmatter as a map, for the form to fill in.
func readFrontmatter(src string) (map[string]interface{}, error) {
	doc, err := splitDocument(src)
	if err != nil {
		return nil, err
	}
	if doc.bare {
		return map[string]interface{}{}, nil
	}
	return config.ParseYAMLMap([]byte(doc.frontmatter))
}
