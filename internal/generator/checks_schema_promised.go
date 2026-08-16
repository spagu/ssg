package generator

// The type a section promised, and whether it actually shipped (#158).
//
// `check_schema` validated the JSON-LD a page emitted and said nothing about
// the JSON-LD a page failed to emit. That gap has a specific shape, and it is
// not hypothetical: SEO injection is gated on what the theme already produced —
// a theme that writes any `application/ld+json` block of its own opts the page
// out of auto-injection entirely. A theme with a hand-written FAQPage partial
// therefore suppressed the Recipe that `schema_defaults` declared for every post
// in that section. Zero Recipe markup shipped for three days while
// `check_schema: warn` reported "structured data carries every required
// property" — because the FAQPage it could see was complete.
//
// So the check now asks the question one level up: a section that declares
// `"@type": Recipe` must produce a Recipe somewhere on the page. Missing
// entirely is a louder failure than present-but-incomplete, and until now it was
// the only one nothing reported.

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/spagu/ssg/internal/models"
)

// promisedSchemaTypes maps each generated file to the @type its schema_defaults
// section promised, so the check can look for it in what shipped.
//
// Only pages a section actually covers appear: a site with no schema_defaults,
// or a page outside every configured prefix, promises nothing and is not
// examined.
func (g *Generator) promisedSchemaTypes() map[string]string {
	if len(g.config.SchemaDefaults) == 0 {
		return nil
	}
	promised := map[string]string{}
	record := func(pages []models.Page) {
		for _, page := range pages {
			want, ok := ldTypeOf(g.sectionSchema(page))
			if !ok {
				continue
			}
			for _, abs := range g.getOutputPaths(page.GetOutputPath()) {
				if rel, err := filepath.Rel(g.config.OutputDir, abs); err == nil {
					promised[filepath.ToSlash(rel)] = want
				}
			}
		}
	}
	record(g.siteData.Pages)
	record(g.siteData.Posts)
	return promised
}

// ldTypeOf returns the single @type a schema block declares. A block with no
// @type promises nothing to look for, and a list of types (`["Recipe",
// "Product"]`) is left alone: which of them a page must carry is the author's
// business, and guessing would produce a warning nobody can act on.
func ldTypeOf(ld map[string]interface{}) (string, bool) {
	t, ok := ld["@type"].(string)
	if !ok {
		return "", false
	}
	t = strings.TrimSpace(t)
	return t, t != ""
}

// ldTypesIn collects every @type a JSON-LD block carries: the node itself, the
// members of a top-level array, and the members of an @graph. All three are
// ordinary shapes — Google's own guidance for a Recipe beside an FAQPage
// produces two sibling blocks or one @graph — so a check that only read the
// root node would report a type that is plainly present.
func ldTypesIn(raw string) []string {
	var node interface{}
	if err := json.Unmarshal([]byte(raw), &node); err != nil {
		return nil // malformed JSON-LD is missingSchemaProps' business, not ours
	}
	var out []string
	collectLDTypes(node, 0, &out)
	return out
}

// collectLDTypes appends every @type reachable from v. The depth bound stops a
// self-referential @graph from spinning the build.
func collectLDTypes(v interface{}, depth int, out *[]string) {
	if depth > 8 {
		return
	}
	switch t := v.(type) {
	case []interface{}:
		for _, item := range t {
			collectLDTypes(item, depth+1, out)
		}
	case map[string]interface{}:
		*out = append(*out, typeNames(t["@type"])...)
		collectLDTypes(t["@graph"], depth+1, out)
	}
}

// typeNames reads an @type value, which schema.org allows to be one name or a
// list of them.
func typeNames(v interface{}) []string {
	switch typed := v.(type) {
	case string:
		return []string{strings.TrimSpace(typed)}
	case []interface{}:
		var out []string
		for _, one := range typed {
			if s, ok := one.(string); ok {
				out = append(out, strings.TrimSpace(s))
			}
		}
		return out
	}
	return nil
}

// missingPromisedType reports the promised type when nothing in blocks carries
// it. An empty promise or a page with no structured data at all is not this
// check's business: the first has nothing to verify, and the second is what
// `seo: true` and the existing checks already speak to.
func missingPromisedType(blocks []string, want string) bool {
	if want == "" || len(blocks) == 0 {
		return false
	}
	for _, raw := range blocks {
		for _, got := range ldTypesIn(raw) {
			if strings.EqualFold(got, want) {
				return false
			}
		}
	}
	return true
}

// promisedTypeFinding explains the miss in the terms the author set it in: the
// section declared a type, the page shipped without it, and the likeliest cause
// is the theme's own JSON-LD block having opted the page out of injection.
func promisedTypeFinding(file, want string, blocks int) finding {
	detail := fmt.Sprintf("schema_defaults promises @type %q and no JSON-LD on the page carries it", want)
	if blocks > 0 {
		detail += fmt.Sprintf(" — the theme emits %d block(s) of its own, which turns auto-injection off for this page"+
			" (emit the derived data yourself with {{ toJSON .Schema }}, or move the hand-written block into an @graph)", blocks)
	}
	return finding{file, detail}
}
