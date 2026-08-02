package generator

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/spagu/ssg/internal/ai"
	"github.com/spagu/ssg/internal/models"
)

// aiShortcodeRe matches a self-closing [ai …attrs] content shortcode.
var aiShortcodeRe = regexp.MustCompile(`(?s)\[ai\s+([^\]]*)\]`)

// aiAttrRe matches key="value" / key='value' attribute pairs.
var aiAttrRe = regexp.MustCompile(`(\w+)\s*=\s*("(?:[^"]*)"|'(?:[^']*)')`)

// resolveAIContent replaces every [ai …] shortcode in every page and post with
// the (cached) AI answer before rendering. It runs sequentially, before the
// parallel render, so it has full page context for the `ifs` guard and never
// races the render caches. A no-op unless ai.models are configured (#1.8.16).
func (g *Generator) resolveAIContent() {
	if g.config.AI == nil || !g.config.AI.Enabled() {
		return
	}
	g.log("🤖 Resolving AI content shortcodes...")
	for i := range g.siteData.Posts {
		g.siteData.Posts[i].Content = g.resolveAIIn(g.siteData.Posts[i])
	}
	for i := range g.siteData.Pages {
		g.siteData.Pages[i].Content = g.resolveAIIn(g.siteData.Pages[i])
	}
}

// resolveAIIn resolves the [ai …] shortcodes in one page's content.
func (g *Generator) resolveAIIn(page models.Page) string {
	if !strings.Contains(page.Content, "[ai ") {
		return page.Content
	}
	vars := aiVars(page, g.config.Variables)
	return aiShortcodeRe.ReplaceAllStringFunc(page.Content, func(m string) string {
		attrs := parseAIAttrs(aiShortcodeRe.FindStringSubmatch(m)[1])
		return g.runAIShortcode(attrs, vars)
	})
}

// runAIShortcode evaluates one [ai …] shortcode: the ifs guard first (fall back
// when it fails), then the cached query (fall back on any error).
func (g *Generator) runAIShortcode(attrs, vars map[string]string) string {
	fallback := firstOf(attrs, "fallback", "fallback_answer")
	if cond := attrs["ifs"]; cond != "" {
		ok, err := ai.Eval(cond, vars)
		if err != nil {
			fmt.Printf("   ⚠️  ai ifs: %v\n", err)
			return fallback
		}
		if !ok {
			return fallback
		}
	}
	question := attrs["question"]
	if question == "" {
		return fallback
	}
	answer, err := g.config.AI.Query(
		firstOf(attrs, "agent", "ai_agent"),
		firstOf(attrs, "model", "ai_model"),
		question, parseAITimeout(attrs["timeout"]))
	if err != nil {
		fmt.Printf("   ⚠️  ai query: %v\n", err)
		return fallback
	}
	return answer
}

// parseAIAttrs parses key="value" pairs from a shortcode's attribute string.
func parseAIAttrs(s string) map[string]string {
	out := map[string]string{}
	for _, m := range aiAttrRe.FindAllStringSubmatch(s, -1) {
		out[strings.ToLower(m[1])] = m[2][1 : len(m[2])-1]
	}
	return out
}

// firstOf returns the first non-empty attribute among the given keys.
func firstOf(attrs map[string]string, keys ...string) string {
	for _, k := range keys {
		if v := attrs[k]; v != "" {
			return v
		}
	}
	return ""
}

// parseAITimeout parses a duration attribute (e.g. "10s"); 0 uses the default.
func parseAITimeout(s string) time.Duration {
	if s == "" {
		return 0
	}
	if d, err := time.ParseDuration(s); err == nil {
		return d
	}
	return 0
}

// aiVars builds the field map the `ifs` guard evaluates against: the page's
// well-known fields plus custom frontmatter and site variables.
func aiVars(page models.Page, siteVars map[string]interface{}) map[string]string {
	v := map[string]string{
		"lang":     page.Lang,
		"status":   page.Status,
		"type":     page.Type,
		"category": page.Category,
		"series":   page.Series,
		"slug":     page.Slug,
		"title":    page.Title,
		"tags":     strings.Join(page.Tags, ","),
	}
	for key, val := range page.Extra {
		v[key] = fmt.Sprintf("%v", val)
	}
	for key, val := range siteVars {
		if _, taken := v[key]; !taken {
			v[key] = fmt.Sprintf("%v", val)
		}
	}
	return v
}
