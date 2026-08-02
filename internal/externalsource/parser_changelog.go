package externalsource

// Keep-a-Changelog parser (#69). A CHANGELOG.md is already the canonical record
// of what shipped; `format: changelog` turns it into structured data so a
// template can render a "What's New" panel straight from it, instead of a
// pre-build script or hand-maintained HTML that goes stale.

import (
	"bytes"
	"io"
	"regexp"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
)

// Changelog headings: "## [1.8.16] - 2026-08-02", "## [Unreleased]", "## 1.8.15".
var (
	clVersionRe = regexp.MustCompile(`^##\s+(.+?)\s*$`)
	clSectionRe = regexp.MustCompile(`^###\s+(.+?)\s*$`)
	clBracketRe = regexp.MustCompile(`^\[(.+?)\]\s*(?:-\s*(.*))?$`)
	clPlainRe   = regexp.MustCompile(`^(\S+?)\s*(?:-\s*(.*))?$`)
	// A leading bold run is the entry's title, optionally preceded by an emoji
	// marker (changelogs commonly open an entry with one) and followed by a dash
	// separating it from the body.
	clTitleRe = regexp.MustCompile(`^([^\w*]*?)\s*\*\*(.+?)\*\*\s*[—–-]?\s*`)
)

// parseChangelog turns a Keep-a-Changelog document into:
//
//	{versions: [{version, date, released, sections: {added: [{title, html, text}], …}, entries: [...]}],
//	 latest: <first released version>, unreleased: <the Unreleased version, if any>}
//
// Section keys are lowercased ("### Added" ⇒ "added"), so a template reads
// `.sections.added`. Entries keep both the rendered `html` and the raw `text`;
// `title` is the leading bold run when the entry has one.
func parseChangelog(r io.Reader) (interface{}, error) {
	raw, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}

	var versions []map[string]interface{}
	var cur map[string]interface{}
	var sections map[string]interface{}
	var entries []interface{}
	curSection := ""
	var item []string

	// flushItem closes the entry being accumulated into the current section.
	flushItem := func() {
		if len(item) == 0 {
			return
		}
		entry := changelogEntry(strings.Join(item, "\n"))
		item = nil
		if cur == nil {
			return
		}
		entries = append(entries, entry)
		if curSection == "" {
			return
		}
		list, _ := sections[curSection].([]interface{})
		sections[curSection] = append(list, entry)
	}
	// flushVersion closes the version block being accumulated.
	flushVersion := func() {
		flushItem()
		if cur == nil {
			return
		}
		cur["sections"] = sections
		cur["entries"] = entries
		versions = append(versions, cur)
		cur, sections, entries, curSection = nil, nil, nil, ""
	}

	for _, line := range strings.Split(string(raw), "\n") {
		switch {
		case clSectionRe.MatchString(line) && cur != nil:
			flushItem()
			curSection = strings.ToLower(strings.TrimSpace(clSectionRe.FindStringSubmatch(line)[1]))
			if _, ok := sections[curSection]; !ok {
				sections[curSection] = []interface{}{}
			}
		case clVersionRe.MatchString(line) && !strings.HasPrefix(line, "###"):
			flushVersion()
			version, date := parseChangelogHeading(clVersionRe.FindStringSubmatch(line)[1])
			if version == "" {
				continue
			}
			cur = map[string]interface{}{
				"version":  version,
				"date":     date,
				"released": !strings.EqualFold(version, "unreleased"),
			}
			sections, entries, curSection = map[string]interface{}{}, nil, ""
		case strings.HasPrefix(strings.TrimSpace(line), "- "), strings.HasPrefix(strings.TrimSpace(line), "* "):
			flushItem()
			item = []string{strings.TrimSpace(line)[2:]}
		case len(item) > 0 && strings.TrimSpace(line) != "":
			item = append(item, strings.TrimSpace(line)) // continuation of a wrapped entry
		default:
			flushItem()
		}
	}
	flushVersion()

	out := map[string]interface{}{"versions": versions}
	for _, v := range versions {
		if released, _ := v["released"].(bool); released {
			if _, ok := out["latest"]; !ok {
				out["latest"] = v
			}
		} else if _, ok := out["unreleased"]; !ok {
			out["unreleased"] = v
		}
	}
	return out, nil
}

// parseChangelogHeading splits a version heading into its version and date,
// accepting both "[1.8.16] - 2026-08-02" and "1.8.16 - 2026-08-02".
func parseChangelogHeading(h string) (version, date string) {
	h = strings.TrimSpace(h)
	if m := clBracketRe.FindStringSubmatch(h); m != nil {
		return m[1], strings.TrimSpace(m[2])
	}
	if m := clPlainRe.FindStringSubmatch(h); m != nil {
		return m[1], strings.TrimSpace(m[2])
	}
	return "", ""
}

// changelogEntry splits one bullet into its parts:
//
//	marker — the leading emoji, when the entry opens with one
//	title  — the leading bold run, rendered inline ("" when the entry has none)
//	html   — the rest of the entry, rendered inline
//	text   — the whole entry as raw Markdown
//	full   — the whole entry rendered inline, for templates that want it in one piece
//
// Splitting title from body is what lets a template emit its own markup around
// each ("<strong>{{.title}}</strong> — {{.html}}") without the title appearing
// twice.
func changelogEntry(md string) map[string]interface{} {
	md = strings.TrimSpace(md)
	marker, title, body := "", "", md
	if m := clTitleRe.FindStringSubmatch(md); m != nil {
		marker = strings.TrimSpace(m[1])
		title = renderChangelogInline(strings.TrimSpace(m[2]))
		body = strings.TrimSpace(md[len(m[0]):])
	}
	return map[string]interface{}{
		"marker": marker,
		"title":  title,
		"text":   md,
		"html":   renderChangelogInline(body),
		"full":   renderChangelogInline(md),
	}
}

// clMD renders changelog entries. GFM only — no raw HTML, no unsafe rendering:
// the output is inserted by templates via safeHTML.
var clMD = goldmark.New(goldmark.WithExtensions(extension.GFM))

// renderChangelogInline renders an entry to inline HTML, unwrapping the single
// enclosing <p> so a template can drop it straight into a list item.
func renderChangelogInline(md string) string {
	var buf bytes.Buffer
	if err := clMD.Convert([]byte(md), &buf); err != nil {
		return md
	}
	out := strings.TrimSpace(buf.String())
	if strings.HasPrefix(out, "<p>") && strings.HasSuffix(out, "</p>") &&
		!strings.Contains(out[3:len(out)-4], "<p>") {
		out = out[3 : len(out)-4]
	}
	return strings.TrimSpace(out)
}
