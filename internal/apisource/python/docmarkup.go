package python

import (
	"regexp"
	"strings"
)

// prose turns the free text of a docstring into Summary and Body:
// doctest blocks move to Examples, literal blocks become fenced code and
// Sphinx roles become {@link} references. Admonitions close the body.
func (info *docInfo) prose(lines []string) {
	md, doctests := convert(lines, false)
	info.doc.Examples = append(doctests, info.doc.Examples...)
	for len(md) > 0 && strings.TrimSpace(md[0]) == "" {
		md = md[1:]
	}
	end := 0
	for end < len(md) && md[end] != "" && !strings.HasPrefix(md[end], "```") {
		end++
	}
	parts := make([]string, end)
	for i, l := range md[:end] {
		parts[i] = strings.TrimSpace(l)
	}
	info.doc.Summary = strings.Join(parts, " ")
	body := strings.TrimSpace(strings.Join(md[end:], "\n"))
	info.doc.Body = joinNonEmpty("\n\n", append([]string{body}, info.notes...)...)
}

// exampleMarkdown renders an Examples section: doctests and literal
// blocks fenced in place; a section with no code at all is code.
func exampleMarkdown(lines []string) string {
	md, _ := convert(lines, true)
	out := strings.TrimSpace(strings.Join(md, "\n"))
	if !strings.Contains(out, "```") {
		return strings.Join(fence("python", lines), "\n")
	}
	return out
}

// codeDirective matches ".. code-block:: lang" and its aliases.
var codeDirective = regexp.MustCompile(`^\.\.\s+(?:code-block|code|sourcecode)::\s*(\S*)\s*$`)

// convert turns docstring lines into Markdown lines. Doctest runs (">>>"
// up to a blank line) are fenced in place when inline, else returned
// apart. Existing ``` fences pass through untouched.
func convert(lines []string, inline bool) (md, doctests []string) {
	inFence := false
	for i := 0; i < len(lines); {
		l := lines[i]
		trimmed := strings.TrimSpace(l)
		switch {
		case strings.HasPrefix(trimmed, "```") || inFence:
			if strings.HasPrefix(trimmed, "```") {
				inFence = !inFence
			}
			md = append(md, l)
			i++
		case strings.HasPrefix(trimmed, ">>>"):
			j := i
			for j < len(lines) && strings.TrimSpace(lines[j]) != "" {
				j++
			}
			code := strings.Join(fence("python", dedent(lines[i:j])), "\n")
			if inline {
				md = append(md, code)
			} else {
				doctests = append(doctests, code)
			}
			i = j
		case codeDirective.MatchString(trimmed):
			content, next := block(lines, i+1, indentOf(l))
			lang := firstNonEmpty(codeDirective.FindStringSubmatch(trimmed)[1], "python")
			md = append(md, fence(lang, dedent(content))...)
			i = next
		case strings.HasSuffix(trimmed, "::") && hasBlock(lines, i):
			content, next := block(lines, i+1, indentOf(l))
			if head := strings.TrimSuffix(l, "::"); strings.TrimSpace(head) != "" {
				if !strings.HasSuffix(head, " ") {
					head += ":"
				}
				md = append(md, markup(strings.TrimRight(head, " ")), "")
			}
			md = append(md, fence("python", dedent(content))...)
			i = next
		default:
			md = append(md, markup(l))
			i++
		}
	}
	return md, doctests
}

// hasBlock reports whether an indented block follows line i.
func hasBlock(lines []string, i int) bool {
	content, _ := block(lines, i+1, indentOf(lines[i]))
	return strings.TrimSpace(strings.Join(content, "")) != ""
}

// fence wraps code lines, outer blank lines dropped, in a fenced block.
func fence(lang string, code []string) []string {
	for len(code) > 0 && strings.TrimSpace(code[0]) == "" {
		code = code[1:]
	}
	for len(code) > 0 && strings.TrimSpace(code[len(code)-1]) == "" {
		code = code[:len(code)-1]
	}
	return append(append([]string{"```" + lang}, code...), "```")
}

// role matches a Sphinx cross-reference role: :class:`Foo`, :py:meth:`~a.B.c`.
var role = regexp.MustCompile("(?::\\w+)?:(\\w+):`([^`]+)`")

// linkRoles are the roles that name a documented object.
var linkRoles = map[string]bool{
	"class": true, "func": true, "meth": true, "attr": true, "mod": true, "obj": true,
	"data": true, "exc": true, "const": true, "any": true, "type": true,
}

// doubleTick matches reST inline literals: “code“.
var doubleTick = regexp.MustCompile("``([^`]+)``")

// bracketRef matches a Markdown-style reference: [`Foo`].
var bracketRef = regexp.MustCompile("\\[`([A-Za-z_][\\w.]*)`\\]")

// markup converts reST inline markup to Markdown with {@link} references.
func markup(s string) string {
	s = role.ReplaceAllStringFunc(s, func(m string) string {
		sub := role.FindStringSubmatch(m)
		return roleLink(sub[1], sub[2])
	})
	s = doubleTick.ReplaceAllString(s, "`$1`")
	var b strings.Builder
	last := 0
	for _, loc := range bracketRef.FindAllStringSubmatchIndex(s, -1) {
		if loc[1] < len(s) && (s[loc[1]] == '(' || s[loc[1]] == '[') {
			continue
		}
		b.WriteString(s[last:loc[0]] + "{@link " + s[loc[2]:loc[3]] + "}")
		last = loc[1]
	}
	return b.String() + s[last:]
}

// roleLink renders one role: a link for object roles, inline code else.
func roleLink(name, target string) string {
	target = strings.TrimSpace(target)
	if !linkRoles[name] || strings.HasPrefix(target, "!") {
		return "`" + strings.TrimPrefix(target, "!") + "`"
	}
	if title, ref, ok := strings.Cut(target, "<"); ok && strings.HasSuffix(ref, ">") {
		return "{@link " + strings.TrimSuffix(strings.TrimPrefix(ref, "~"), ">") + " | " + strings.TrimSpace(title) + "}"
	}
	short := strings.HasPrefix(target, "~")
	target = strings.TrimSuffix(strings.TrimPrefix(target, "~"), "()")
	if short && strings.Contains(target, ".") {
		return "{@link " + target + " | " + target[strings.LastIndex(target, ".")+1:] + "}"
	}
	return "{@link " + target + "}"
}
