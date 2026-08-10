package generator

// check_schema — validating the structured data a page actually emits (#111).
//
// `schema:` passes any JSON-LD through untouched, which is what lets Recipe,
// Product, Event and the rest work without SSG knowing them. The cost of that
// generality is that nothing checks whether the result is usable: search engines
// reject structured data missing required properties, and they do it silently
// from the author's side — the build succeeds, the page ships, the rich result
// never appears, and the feedback arrives weeks later in Search Console if
// anyone is looking.
//
// Scope is deliberately narrow. schema.org has hundreds of types and encoding
// them all would be a maintenance burden with little return; the ones that earn
// rich results are a short list. An unknown @type passes silently, so the
// generality this validates is not the generality it takes away.

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"golang.org/x/net/html"
)

// requiredSchemaProps lists the properties a type needs before a search engine
// will use it. These are the required set, not the recommended one: warning
// about every optional property would train people to ignore the warning.
var requiredSchemaProps = map[string][]string{
	"Recipe":        {"image", "recipeIngredient", "recipeInstructions"},
	"Product":       {"image", "name"},
	"Offer":         {"price", "priceCurrency"},
	"Event":         {"location", "name", "startDate"},
	"JobPosting":    {"datePosted", "hiringOrganization", "jobLocation", "title"},
	"LocalBusiness": {"address", "name"},
	"HowTo":         {"name", "step"},
	"VideoObject":   {"name", "thumbnailUrl", "uploadDate"},
	"Article":       {"headline"},
	"BlogPosting":   {"headline"},
	"NewsArticle":   {"headline"},
	"FAQPage":       {"mainEntity"},
}

// checkSchemaIfRequested validates every JSON-LD block in the output.
func (g *Generator) checkSchemaIfRequested() error {
	mode := g.resolveMode(g.config.CheckSchema)
	if mode == "" {
		return nil
	}
	g.log("🧩 Checking structured data...")

	var findings []finding
	err := g.walkOutputHTML(func(rel string, doc *html.Node) {
		for _, raw := range jsonLDBlocks(doc) {
			for _, miss := range missingSchemaProps(raw) {
				findings = append(findings, finding{rel, miss})
			}
		}
	})
	if err != nil {
		return err
	}
	sortFindings(findings)
	return g.report(findings, mode, "structured data",
		"structured data carries every required property",
		"%d structured-data problem(s)")
}

// jsonLDBlocks returns the raw text of every application/ld+json script.
func jsonLDBlocks(doc *html.Node) []string {
	var out []string
	forEachElement(doc, "script", func(n *html.Node) {
		if t, ok := attr(n, "type"); !ok || !strings.EqualFold(strings.TrimSpace(t), "application/ld+json") {
			return
		}
		if n.FirstChild != nil {
			out = append(out, n.FirstChild.Data)
		}
	})
	return out
}

// missingSchemaProps reports what a block is missing, one message per typed
// object. Unparseable JSON is reported too: a block a crawler cannot read is
// worse than no block, and it is invisible in the rendered page.
func missingSchemaProps(raw string) []string {
	var v interface{}
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		return []string{"JSON-LD block is not valid JSON: " + err.Error()}
	}
	var out []string
	walkLD(v, func(obj map[string]interface{}) {
		ldType, _ := obj["@type"].(string)
		required, known := requiredSchemaProps[ldType]
		if !known {
			return // an unknown type is not an error — see the file comment
		}
		var missing []string
		for _, prop := range required {
			if !hasLDProp(obj, prop) {
				missing = append(missing, prop)
			}
		}
		if len(missing) > 0 {
			sort.Strings(missing)
			out = append(out, fmt.Sprintf("%s is missing %s", ldType, strings.Join(missing, ", ")))
		}
	})
	return out
}

// walkLD visits every object in a JSON-LD document, including nested ones —
// an Offer inside a Product is the case that matters, since a missing
// priceCurrency there invalidates the Product too.
func walkLD(v interface{}, visit func(map[string]interface{})) {
	switch t := v.(type) {
	case map[string]interface{}:
		visit(t)
		for _, child := range t {
			walkLD(child, visit)
		}
	case []interface{}:
		for _, child := range t {
			walkLD(child, visit)
		}
	}
}

// hasLDProp reports whether a property is present and carries something. An
// empty string or empty list is the same as absent to a consumer, and treating
// it as present would let the check pass on data that still gets rejected.
func hasLDProp(obj map[string]interface{}, prop string) bool {
	v, ok := obj[prop]
	if !ok || v == nil {
		return false
	}
	switch t := v.(type) {
	case string:
		return strings.TrimSpace(t) != ""
	case []interface{}:
		return len(t) > 0
	case map[string]interface{}:
		return len(t) > 0
	}
	return true
}
