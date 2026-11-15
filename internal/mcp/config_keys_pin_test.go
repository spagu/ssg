package mcp

import (
	"reflect"
	"testing"
)

// pinnedDesignerKeys is designer_config_set's allow-list exactly as the hand
// list in config_keys.go held it before the keys came from the configuration
// schema (GO-177), minus templates_dir and static_dir, removed in 1.9.10: they
// name directories the build reads and publishes. Changing the set, a kind or a description is a deliberate
// act: edit the schema entry and this table together.
var pinnedDesignerKeys = map[string]configKey{
	"title":                  {kindString, "the site's name, used in <title>, og:site_name and feeds"},
	"description":            {kindString, "the site's description, used in the meta description and feeds"},
	"template":               {kindString, "theme name used to render the site"},
	"mermaid":                {kindBool, "render mermaid diagrams"},
	"mermaid_theme":          {kindString, "mermaid built-in theme (default, neutral, dark, forest, base)"},
	"mermaid_background":     {kindString, "mermaid background colour, e.g. \"#ffffff\""},
	"highlight":              {kindBool, "syntax-highlight fenced code blocks"},
	"highlight_style":        {kindString, "syntax-highlight colour scheme"},
	"highlight_line_numbers": {kindBool, "show line numbers in highlighted code"},
	"math":                   {kindBool, "render KaTeX math"},
	"toc":                    {kindBool, "generate a table of contents"},
	"toc_depth":              {kindInt, "heading depth the table of contents covers"},
	"minify_html":            {kindBool, "minify generated HTML"},
	"minify_css":             {kindBool, "minify generated CSS"},
	"minify_js":              {kindBool, "minify generated JS"},
	"minify_all":             {kindBool, "minify HTML, CSS and JS together"},
	"pretty_html":            {kindBool, "indent generated HTML for readability"},
	"sourcemap":              {kindBool, "emit source maps for compiled assets"},
	"fingerprint":            {kindBool, "content-hash asset filenames for cache busting"},
	"paginate":               {kindInt, "posts per page in listings"},
	"webp":                   {kindBool, "generate WebP variants of images"},
	"webp_quality":           {kindInt, "WebP encoder quality (1-100)"},
	"image_formats":          {kindList, "image formats to generate, e.g. \"webp,avif\""},
	"image_sizes_attr":       {kindString, "default sizes attribute for responsive images"},
	"clean_special_chars":    {kindBool, "turn curly quotes, dashes and invisible spaces into plain ASCII"},
	"autolink_list_items":    {kindBool, "link a list item whose whole text is a page title (default on)"},
	"strip_md_link_text":     {kindBool, "drop \".md\" from link text that is a bare filename"},

	"story":           {kindSwitch, "story mode: reading companion and fullscreen presentation of posts"},
	"listen":          {kindSwitch, "the Listen (read aloud) button"},
	"print":           {kindSwitch, "the Print / PDF button"},
	"cook_mode":       {kindSwitch, "the Cook Mode button on recipes"},
	"heading_anchors": {kindSwitch, "a # link beside each heading"},
	"og_cards":        {kindSwitch, "a generated share card per page (needs text_color and a background)"},
	"breadcrumbs":     {kindSwitch, "the BreadcrumbList JSON-LD trail (off when a theme supplies its own)"},

	"markdown_publish": {kindBool, "publish each page's Markdown as index.md, plus llms.txt, for agents"},
	"site_graph":       {kindBool, "publish site-graph.json (pages, sections, links) for agents"},
	"webmcp":           {kindBool, "declare search and navigation tools to in-browser agents (WebMCP); turns on search_index"},
	"search_index":     {kindBool, "publish search-index.json for a client-side search"},
	"outputs":          {kindList, "formats written for every page, e.g. \"html,json\""},

	"marketing.og_site_name":     {kindString, "og:site_name, when it differs from the title"},
	"marketing.og_image":         {kindString, "the share image for pages without their own"},
	"marketing.twitter_site":     {kindString, "the site's X/Twitter handle, e.g. \"@example\""},
	"marketing.theme_color":      {kindString, "the browser's theme colour, e.g. \"#1a73e8\""},
	"marketing.logo":             {kindString, "the site's logo"},
	"marketing.favicon":          {kindString, "the favicon"},
	"marketing.apple_touch_icon": {kindString, "the home-screen icon"},
}

func TestDesignerKeysPinnedToPreSchemaList(t *testing.T) {
	if !reflect.DeepEqual(designerConfigKeys, pinnedDesignerKeys) {
		for k, v := range pinnedDesignerKeys {
			if got, ok := designerConfigKeys[k]; !ok || got != v {
				t.Errorf("%s: got %+v (present %v), want %+v", k, got, ok, v)
			}
		}
		for k := range designerConfigKeys {
			if _, ok := pinnedDesignerKeys[k]; !ok {
				t.Errorf("%s: writable now, was not before", k)
			}
		}
	}
}
