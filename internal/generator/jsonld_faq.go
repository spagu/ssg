package generator

// Questions and answers as an FAQPage (1.8.72). A page lists them in front
// matter —
//
//	faq:
//	  - question: Can I freeze it?
//	    answer: Yes, for up to a month.
//
// — and ssg emits the FAQPage beside the page's main entity. A theme renders
// the list from .faq as it likes; the answers are Markdown.

import (
	"strings"

	"github.com/spagu/ssg/internal/models"
)

// faqLD is the page's FAQPage, or nil when it lists no complete pair.
func (g *Generator) faqLD(page models.Page) map[string]interface{} {
	raw, ok := page.Extra["faq"].([]interface{})
	if !ok {
		return nil
	}
	var items []interface{}
	for _, entry := range raw {
		qa, ok := entry.(map[string]interface{})
		if !ok {
			continue
		}
		q := strings.TrimSpace(str(qa["question"]))
		a := strings.TrimSpace(tmplStripHTML(g.convertMarkdownToHTML(str(qa["answer"]))))
		if q == "" || a == "" {
			continue
		}
		items = append(items, map[string]interface{}{
			"@type": "Question", "name": q,
			"acceptedAnswer": map[string]interface{}{"@type": "Answer", "text": a},
		})
	}
	if len(items) == 0 {
		return nil
	}
	return map[string]interface{}{"@context": "https://schema.org", "@type": "FAQPage", "mainEntity": items}
}
