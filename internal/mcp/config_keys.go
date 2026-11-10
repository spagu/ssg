package mcp

// The designer's allow-list: which settings an assistant may change through
// designer_config_set, and how each is written.

// Kinds of writable setting. A switch is a feature configured by a mapping
// (`story:`, `listen:`…): the assistant turns it on or off, and the mapping's
// other settings stay with the file's author.
const (
	kindBool   = "bool"
	kindInt    = "int"
	kindString = "string"
	kindList   = "list"   // comma-separated in, a YAML list out
	kindSwitch = "switch" // true/false, written to <key>.enabled
)

// configKey is one presentation setting the designer may change.
type configKey struct {
	kind string
	desc string
}

// designerConfigKeys is the complete set of writable keys. Anything absent here
// is refused — notably every secret (`ai.*.key`, `mcp.git.token`, `jwt_secret`,
// auth), deployment, server, endpoint, hook and URL-structure setting, plus
// `sass_binary` (an executable path).
//
// The test for membership is not "is it about rendering" but: does it carry a
// secret, change where the site deploys, what the server does, or what a URL
// is? The site's own name and description fail all four, and they are what a
// designer or content manager most obviously expects to edit (#212). So do the
// presentation features added since — story mode, the Listen, Print and Cook
// Mode buttons, heading anchors, share cards — and the files published for
// agents, `markdown_publish` and `site_graph` among them (#410).
//
// `language` and `author` are absent because no such top-level key exists.
// `default_language` does, but it drives i18n, URL prefixes and hreflang — a
// URL-structure setting, refused by the rule above rather than by oversight.
// The site's identity for sharing lives under `marketing.` and is writable key
// by key; `marketing.verification` holds search-console tokens and is not.
var designerConfigKeys = map[string]configKey{
	"title":                  {kindString, "the site's name, used in <title>, og:site_name and feeds"},
	"description":            {kindString, "the site's description, used in the meta description and feeds"},
	"template":               {kindString, "theme name used to render the site"},
	"templates_dir":          {kindString, "directory themes are loaded from"},
	"static_dir":             {kindString, "directory copied verbatim into the output"},
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
