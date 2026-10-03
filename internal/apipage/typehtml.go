package apipage

import (
	"html"
	"strings"

	"github.com/spagu/ssg/internal/apimodel"
)

// TypeHTML renders a type as HTML text with every documented name linked:
// Promise<<a href="…">Token</a>[]>. href maps a symbol ID to its URL ("" for
// none). Everything else is escaped. It is what the api* template functions
// give themes that draw their own signatures (GO-109).
func TypeHTML(t *apimodel.TypeRef, href func(id string) string) string {
	var b strings.Builder
	writeTypeHTML(&b, t, href)
	return b.String()
}

// writeTypeHTML walks the tree the way TypeRef.String does, linking names.
func writeTypeHTML(b *strings.Builder, t *apimodel.TypeRef, href func(id string) string) {
	if t == nil {
		b.WriteString("unknown")
		return
	}
	switch t.Kind {
	case apimodel.TypeUnion, apimodel.TypeIntersection, apimodel.TypeTuple:
		sep := map[apimodel.TypeKind]string{apimodel.TypeUnion: " | ", apimodel.TypeIntersection: " &amp; ", apimodel.TypeTuple: ", "}[t.Kind]
		if t.Kind == apimodel.TypeTuple {
			b.WriteString("[")
		}
		for i, a := range t.Args {
			if i > 0 {
				b.WriteString(sep)
			}
			writeTypeHTML(b, a, href)
		}
		if t.Kind == apimodel.TypeTuple {
			b.WriteString("]")
		}
	case apimodel.TypeArray:
		if len(t.Args) == 0 {
			b.WriteString("unknown[]")
			return
		}
		el := t.Args[0]
		paren := el.Kind == apimodel.TypeUnion || el.Kind == apimodel.TypeIntersection || el.Kind == apimodel.TypeFunction
		if paren {
			b.WriteString("(")
		}
		writeTypeHTML(b, el, href)
		if paren {
			b.WriteString(")")
		}
		b.WriteString("[]")
	case apimodel.TypeName:
		writeName(b, t, href)
	default: // function, object, literal, verbatim: text, escaped
		b.WriteString(html.EscapeString(t.String()))
	}
}

// writeName writes a name, linked when it is a documented symbol, with its
// type arguments.
func writeName(b *strings.Builder, t *apimodel.TypeRef, href func(id string) string) {
	name := html.EscapeString(t.Name)
	if url := linkFor(t.Ref, href); url != "" {
		b.WriteString(`<a href="` + html.EscapeString(url) + `">` + name + `</a>`)
	} else {
		b.WriteString(name)
	}
	if len(t.Args) > 0 {
		b.WriteString("&lt;")
		for i, a := range t.Args {
			if i > 0 {
				b.WriteString(", ")
			}
			writeTypeHTML(b, a, href)
		}
		b.WriteString("&gt;")
	}
}

// linkFor is href(ref), or "" without a ref or a resolver.
func linkFor(ref string, href func(string) string) string {
	if ref == "" || href == nil {
		return ""
	}
	return href(ref)
}
