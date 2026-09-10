package components

// Reading a component call out of content (GO-093).
//
//	{{< gallery src="photos/" columns=3 wide=true captions="a, b" >}}
//
// The syntax is deliberately not the one the legacy shortcodes use. `{{name}}`
// and `[name]` keep working exactly as they did, and neither can be mistaken
// for this: a page that happens to contain `{{gallery}}` renders as it always
// has, and one that contains `{{< gallery >}}` is asking for something new.
//
// One rule matters more than the grammar: **an unknown call is left alone.**
// The legacy `{{name}}` form deletes what it does not recognise, which is a
// reasonable thing to do to a shortcode a config once defined and no longer
// does — and a terrible thing to do to prose that happens to look like a call.
// A call this build cannot resolve stays in the page as the author typed it,
// with a warning naming the file.

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// callRe matches a whole component call and captures its name and argument
// text.
//
// A call lives on one line and ends at the first `>}}`. Both bounds are
// deliberate: staying on one line means an unterminated `{{<` swallows a word,
// not the rest of the document, and allowing anything else inside means a prop
// can hold a `>` — an arrow, a closing tag in an example — which excluding the
// character outright would have made unwritable. The one value a call cannot
// carry is the terminator itself.
//
// Arguments must be separated from the name by whitespace, so `{{<name!>}}` is
// not a call at all rather than a call to `name` with nonsense attached.
var callRe = regexp.MustCompile(`\{\{<\s*([a-z0-9_-]+)((?:\s[^\n]*?)?)\s*>\}\}`)

// argRe matches one key=value pair: quoted with either quote, or bare.
var argRe = regexp.MustCompile(`([a-zA-Z_][a-zA-Z0-9_-]*)\s*=\s*(?:"([^"]*)"|'([^']*)'|([^\s"']+))`)

// Call is one occurrence of a component in content.
type Call struct {
	Name string
	Args map[string]string
	// Raw is the exact text matched, so an unresolved call can be put back
	// byte for byte.
	Raw string
	// Start and End bound Raw within the content it came from.
	Start, End int
}

// FindCalls returns every component call in content, in order.
//
// Calls inside code — a fenced block or a span between backticks — are not
// calls. Documentation that shows how to write one is the ordinary reason to
// have the syntax in a file at all, and expanding it there would make it
// impossible to document a component using the component's own site.
func FindCalls(content string) []Call {
	matches := callRe.FindAllStringSubmatchIndex(content, -1)
	code := codeSpans(content)
	out := make([]Call, 0, len(matches))
	for _, m := range matches {
		if inSpans(code, m[0]) {
			continue
		}
		name := content[m[2]:m[3]]
		args := ""
		if m[4] >= 0 {
			args = content[m[4]:m[5]]
		}
		out = append(out, Call{
			Name:  name,
			Args:  parseArgs(args),
			Raw:   content[m[0]:m[1]],
			Start: m[0],
			End:   m[1],
		})
	}
	return out
}

// parseArgs reads the key=value pairs of a call.
func parseArgs(s string) map[string]string {
	args := map[string]string{}
	for _, m := range argRe.FindAllStringSubmatch(s, -1) {
		key := m[1]
		switch {
		case m[2] != "" || strings.Contains(m[0], `=""`):
			args[key] = m[2]
		case m[3] != "" || strings.Contains(m[0], "=''"):
			args[key] = m[3]
		default:
			args[key] = m[4]
		}
	}
	return args
}

// Resolve turns a call's text arguments into the typed props the template
// receives, applying defaults and checking the schema.
//
// Everything a caller can get wrong is reported with the component, the prop
// and the reason — a message that says only "invalid props" sends the author
// back to a template they did not write.
func (c *Component) Resolve(call Call) (map[string]interface{}, error) {
	props := make(map[string]interface{}, len(c.Props))
	for name, def := range c.Props {
		raw, given := call.Args[name]
		if !given {
			switch {
			case def.Required:
				return nil, fmt.Errorf("%s: %q is required", c.Name, name)
			case def.Default != nil:
				props[name] = def.Default
			}
			continue
		}
		value, err := coerce(def, raw)
		if err != nil {
			return nil, fmt.Errorf("%s: %q %w", c.Name, name, err)
		}
		props[name] = value
	}
	// An argument the schema does not declare is a typo often enough to be
	// worth saying so; a component with no schema at all takes anything,
	// because it has promised nothing.
	if len(c.Props) > 0 {
		for name := range call.Args {
			if _, declared := c.Props[name]; !declared {
				return nil, fmt.Errorf("%s: %q is not one of its props (%s)",
					c.Name, name, strings.Join(c.PropNames(), ", "))
			}
		}
	} else {
		for name, raw := range call.Args {
			props[name] = raw
		}
	}
	return props, nil
}

// coerce reads one argument according to its declared type.
func coerce(def Prop, raw string) (interface{}, error) {
	switch def.Type {
	case TypeNumber:
		if n, err := strconv.Atoi(raw); err == nil {
			return n, nil
		}
		f, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			return nil, fmt.Errorf("= %q is not a number", raw)
		}
		return f, nil
	case TypeBool:
		b, err := strconv.ParseBool(raw)
		if err != nil {
			return nil, fmt.Errorf("= %q is not true or false", raw)
		}
		return b, nil
	case TypeArray:
		var out []string
		for _, part := range strings.Split(raw, ",") {
			if p := strings.TrimSpace(part); p != "" {
				out = append(out, p)
			}
		}
		return out, nil
	}
	if len(def.Enum) > 0 && !contains(def.Enum, raw) {
		return nil, fmt.Errorf("= %q is not one of: %s", raw, strings.Join(def.Enum, ", "))
	}
	return raw, nil
}

// fenceRe matches a fenced code block, and inlineCodeRe a backtick span. Both
// are the Markdown the content is written in, read here only well enough to
// know what is quoted.
var (
	fenceRe      = regexp.MustCompile("(?s)^```[^\n]*\n.*?\n```|(?s)\n```[^\n]*\n.*?\n```")
	inlineCodeRe = regexp.MustCompile("`+[^`\n]*`+")
	indentedRe   = regexp.MustCompile(`(?m)^(?: {4}|\t)[^\n]*$`)
)

// span is a half-open byte range of content that is code.
type span struct{ start, end int }

// codeSpans finds the parts of content that are code rather than prose.
func codeSpans(content string) []span {
	var out []span
	for _, m := range fenceRe.FindAllStringIndex(content, -1) {
		out = append(out, span{m[0], m[1]})
	}
	for _, m := range indentedRe.FindAllStringIndex(content, -1) {
		out = append(out, span{m[0], m[1]})
	}
	for _, m := range inlineCodeRe.FindAllStringIndex(content, -1) {
		if !inSpans(out, m[0]) {
			out = append(out, span{m[0], m[1]})
		}
	}
	return out
}

// inSpans reports whether an offset falls inside any span.
func inSpans(spans []span, at int) bool {
	for _, s := range spans {
		if at >= s.start && at < s.end {
			return true
		}
	}
	return false
}
