package apidoc

import "strings"

// FindLinks returns the inline links in Markdown, in order:
// {@link Target}, {@link Target | text}, {@link Target text}, {@linkcode ...},
// {@linkplain ...}. Links inside fenced code blocks and inline code spans are
// ignored.
func FindLinks(md string) []Link {
	var links []Link
	for _, r := range proseRanges(md) {
		links = appendLinks(links, md, r)
	}
	return links
}

// RewriteLinks replaces every inline link with a Markdown link: resolve maps a
// target to an href; a resolved {@link T} becomes [T](href) (or [text](href));
// {@linkcode} becomes [`T`](href). An unresolved link becomes its text (or the
// target) — as inline code for linkcode — and its target is returned in
// unresolved, in order, without duplicates. Code blocks/spans are untouched.
// A nil resolve resolves nothing.
func RewriteLinks(md string, resolve func(target string) (href string, ok bool)) (out string, unresolved []string) {
	links := FindLinks(md)
	if len(links) == 0 {
		return md, nil
	}
	var b strings.Builder
	seen := map[string]bool{}
	last := 0
	for _, l := range links {
		b.WriteString(md[last:l.Start])
		last = l.End
		if resolve != nil {
			if href, ok := resolve(l.Target); ok {
				b.WriteString("[" + l.label() + "](" + href + ")")
				continue
			}
		}
		b.WriteString(l.label())
		if !seen[l.Target] {
			seen[l.Target] = true
			unresolved = append(unresolved, l.Target)
		}
	}
	b.WriteString(md[last:])
	return b.String(), unresolved
}

// label is the visible text of a link: its text or its target, wrapped in
// backticks for {@linkcode}.
func (l Link) label() string {
	s := firstNonEmpty(l.Text, l.Target)
	if l.Code {
		return "`" + s + "`"
	}
	return s
}

// appendLinks appends the links found in md[r.start:r.end] to links.
func appendLinks(links []Link, md string, r span) []Link {
	for i := r.start; i < r.end; {
		j := strings.Index(md[i:r.end], "{@")
		if j < 0 {
			break
		}
		pos := i + j
		l, ok := parseLink(md[pos:r.end])
		if !ok {
			i = pos + 2
			continue
		}
		l.Start, l.End = pos, pos+l.End
		links = append(links, l)
		i = l.End
	}
	return links
}

// parseLink parses the inline link at the start of s ("{@link ...}"). The
// returned Link's End is relative to s; Start is 0.
func parseLink(s string) (Link, bool) {
	n := 2
	for n < len(s) && s[n] >= 'a' && s[n] <= 'z' {
		n++
	}
	name := s[2:n]
	if name != "link" && name != "linkcode" && name != "linkplain" {
		return Link{}, false
	}
	end := strings.IndexByte(s, '}')
	if end < 0 || (n < end && !strings.ContainsRune(" \t\n", rune(s[n]))) {
		return Link{}, false
	}
	target, text := splitLinkBody(strings.TrimSpace(s[n:end]))
	if target == "" {
		return Link{}, false
	}
	return Link{Target: target, Text: text, Code: name == "linkcode", End: end + 1}, true
}

// splitLinkBody splits "Target | text" or "Target text" into its parts.
func splitLinkBody(body string) (target, text string) {
	if t, x, ok := strings.Cut(body, "|"); ok {
		return strings.TrimSpace(t), strings.TrimSpace(x)
	}
	return firstWord(body)
}
