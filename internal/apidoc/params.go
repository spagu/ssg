package apidoc

import "strings"

// splitType reads a leading "{Type}" from s. Braces nest, so
// "{Object<string, {a: number}>}" is one type. Without a type, or when the
// brace is never closed, typ is "" and rest is s unchanged (left-trimmed).
func splitType(s string) (typ, rest string) {
	s = strings.TrimLeft(s, " \t\n")
	if !strings.HasPrefix(s, "{") {
		return "", s
	}
	end := matchBracket(s, '{', '}')
	if end < 0 {
		return "", s
	}
	return strings.TrimSpace(s[1:end]), strings.TrimLeft(s[end+1:], " \t\n")
}

// matchBracket returns the index of the bracket closing the one at s[0],
// counting nested pairs, or -1 when it is never closed.
func matchBracket(s string, open, closing byte) int {
	depth := 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case open:
			depth++
		case closing:
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

// parseParam parses the text of @param, @property and their aliases:
// "{Type} name text", "{Type} [name=default] - text", "{Type=} name",
// "{...Type} name" and the forms without a type.
func parseParam(text string) ParamTag {
	typ, rest := splitType(text)
	var p ParamTag
	p.Type, p.Rest, p.Optional = typeModifiers(typ)
	if strings.HasPrefix(rest, "[") {
		if end := matchBracket(rest, '[', ']'); end > 0 {
			name, def, _ := strings.Cut(rest[1:end], "=")
			p.Name, p.Default, p.Optional = strings.TrimSpace(name), strings.TrimSpace(def), true
			rest = rest[end+1:]
		}
	}
	if p.Name == "" {
		p.Name, rest = firstWord(rest)
	}
	if strings.HasPrefix(p.Name, "...") {
		p.Name, p.Rest = p.Name[3:], true
	}
	p.Text = stripHyphen(rest)
	return p
}

// typeModifiers strips the rest marker ("...T") and the optional marker
// ("T=") from a type expression.
func typeModifiers(typ string) (t string, rest, optional bool) {
	if strings.HasPrefix(typ, "...") {
		typ, rest = strings.TrimSpace(typ[3:]), true
	}
	if strings.HasSuffix(typ, "=") {
		typ, optional = strings.TrimSpace(strings.TrimSuffix(typ, "=")), true
	}
	return typ, rest, optional
}

// parseTypeTag parses "{Type} text" where the type is optional, as written
// after @returns and @throws.
func parseTypeTag(text string) TypeTag {
	typ, rest := splitType(text)
	return TypeTag{Type: typ, Text: stripHyphen(rest)}
}

// parseTemplates parses "@template T", "@template T, U text",
// "@template {Constraint} T" and "@typeParam T - text". Every name gets the
// same constraint (as Type) and text.
func parseTemplates(text string) []ParamTag {
	typ, rest := splitType(text)
	var names []string
	for {
		rest = strings.TrimLeft(rest, " \t")
		end := strings.IndexAny(rest, " \t\n,")
		if end < 0 {
			end = len(rest)
		}
		if end > 0 {
			names = append(names, rest[:end])
		}
		rest = strings.TrimLeft(rest[end:], " \t")
		if !strings.HasPrefix(rest, ",") {
			break
		}
		rest = rest[1:]
	}
	desc := stripHyphen(rest)
	out := make([]ParamTag, 0, len(names))
	for _, n := range names {
		out = append(out, ParamTag{Name: n, Type: typ, Text: desc})
	}
	return out
}

// firstWord splits s into its first whitespace-delimited word and the
// trimmed remainder.
func firstWord(s string) (word, rest string) {
	s = strings.TrimLeft(s, " \t\n")
	end := strings.IndexAny(s, " \t\n")
	if end < 0 {
		return s, ""
	}
	return s[:end], strings.TrimSpace(s[end:])
}

// stripHyphen trims s and removes the optional "-" separating a name from
// its description ("name - text").
func stripHyphen(s string) string {
	s = strings.TrimSpace(s)
	if s == "-" {
		return ""
	}
	if len(s) > 1 && s[0] == '-' && (s[1] == ' ' || s[1] == '\t' || s[1] == '\n') {
		return strings.TrimSpace(s[1:])
	}
	return s
}
