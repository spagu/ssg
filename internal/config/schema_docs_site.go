package config

// Schema entries: site identity, content, URLs, languages, appearance.
// Descriptions are short phrases (they are form labels' help text); the keys
// marked .assistant() keep the wording `ssg mcp` designer_config_read has
// always shown, since that tool now reads it from here.

import (
	"github.com/spagu/ssg/internal/models"
	"github.com/spagu/ssg/internal/taxonomy"
)

var _ = describe(Config{}, typeDocs{
	// Site
	"domain":      x("the site's domain, e.g. example.com; a path after it sets base_path").in(gSite),
	"title":       p("the site's name, used in <title>, og:site_name and feeds").in(gSite).assistant(),
	"description": p("the site's description, used in the meta description and feeds").in(gSite).assistant(),
	"colors":      p("the site palette by role (primary, secondary, accent, text, background), as .Site.Colors and --ssg-color-* CSS properties").in(gSite),
	"marketing":   p("the site's social identity: share image, icons, theme colour, verification tokens").in(gSite),
	"attribution": p("keep the \"Built with ssg\" notice and generator meta tag; off only under a commercial license").defaults(true).in(gSite),

	// Content
	"source":              x("the content directory inside content_dir the site is built from").in(gContent),
	"content_dir":         x("directory holding the content sources").in(gContent),
	"data_dir":            x("directory of YAML/JSON data files loaded into .Data").in(gContent),
	"pages_path":          p("subdirectory of the source holding pages (default \"pages\")").in(gContent),
	"posts_path":          p("subdirectory of the source holding posts (default \"posts\")").in(gContent),
	"posts_page":          p("move the post listing off the root, e.g. \"blog\" publishes it at /blog/").in(gContent),
	"content_sources":     x("extra Markdown roots merged into the site").in(gContent),
	"content_exclude":     p("glob patterns of Markdown files under content_dir that are not pages").in(gContent),
	"content_schemas":     p("per content type front-matter contracts: required fields and field rules").in(gContent),
	"flat_posts":          p("load Markdown sitting directly in posts/ as posts").in(gContent),
	"auto_excerpt":        p("derive a missing excerpt from the opening paragraph").in(gContent),
	"clean_special_chars": p("turn curly quotes, dashes and invisible spaces into plain ASCII").in(gContent).assistant(),
	"preserve_slug_case":  p("keep the file name's case in slugs and URLs").in(gContent),
	"slug_transliterate":  p("fold Latin letters with diacritics to ASCII in slugs made from labels (ż → z, ß → ss); old URLs get a 301").in(gContent),
	"home_pages_limit":    p("pages listed on the home page before a \"see all\" link (0 theme default, negative no limit)").in(gContent),
	"home_posts_limit":    p("posts listed on the home page before a \"see all\" link (0 theme default, negative no limit)").in(gContent),
	"paginate":            p("posts per page in listings").in(gContent).assistant(),
	"date_archives":       p("generate /YYYY/ and /YYYY/MM/ archive listings").in(gContent),
	"type_archives":       p("content types that get a listing at /<type>/ (type: true)").in(gContent),
	"taxonomies":          p("custom taxonomies and overrides of category, tag and series").in(gContent),
	"versions":            p("what a chain of document versions means for search engines").in(gContent),
	"components_dir":      x("directory of typed content components (props schema, template, assets)").in(gContent),

	// URLs and links
	"post_url_format":          p("post URLs by date or by slug").in(gURLs).oneOf("", "date", "slug"),
	"page_format":              p("page files as directories, flat .html files, or both").in(gURLs).oneOf("", "directory", "flat", "both"),
	"permalinks":               p("URL pattern per content type, with :year :month :day :slug :category").in(gURLs),
	"relative_links":           p("rewrite absolute URLs to relative ones").in(gURLs),
	"base_path":                p("path the site is served under, e.g. /docs-site; \"/\" forces the root").in(gURLs),
	"pretty_urls":              p("how the host serves URLs: off, strip (.html dropped) or strip-slash").in(gURLs),
	"rewrite_md_links":         p("rewrite relative .md links to their published URLs (default on)").defaults(true).in(gURLs),
	"strip_md_link_text":       p("drop \".md\" from link text that is a bare filename").in(gURLs).assistant(),
	"autolink_list_items":      p("link a list item whose whole text is a page title (default on)").defaults(true).in(gURLs).assistant(),
	"link_rewrites":            x("href prefix in content → replacement, longest prefix wins").in(gURLs),
	"redirects":                x("_redirects rules: exact paths or /old/* splats with a status").in(gURLs),
	"alias_stubs":              p("also write meta-refresh pages for front-matter aliases (default on)").defaults(true).in(gURLs),
	"language_sections":        p("language of a whole content section, keyed by directory (\"home\" for the root)").in(gLanguages),
	"languages":                p("language codes of a multilingual build, or full entries with locale, name and time zone").in(gLanguages).or(languageConfigSample),
	"default_language":         p("the language of pages that name none; drives URL prefixes and hreflang").in(gLanguages),
	"i18n":                     p("translation dictionaries, fallbacks and how translation problems are treated").in(gLanguages),
	"timezone":                 p("IANA time zone dates are shown in, e.g. Europe/Warsaw").in(gLanguages),
	"language_timezones":       p("time zone per content language").in(gLanguages),
	"output_encoding":          p("character encoding of HTML, published Markdown and llms.txt").in(gOutput).oneOf("", "utf-8", "utf-16le", "utf-16be"),
	"output_encoding_sections": p("output encoding per content section").in(gOutput),

	// Appearance
	"template":               p("theme name used to render the site").in(gAppearance).assistant(),
	"templates_dir":          x("directory themes are loaded from").in(gAppearance),
	"engine":                 p("template engine of the theme").in(gAppearance).oneOf("", "go", "pongo2", "mustache", "handlebars"),
	"online_theme":           x("URL of a theme archive downloaded before the build").in(gAppearance),
	"online_theme_sha256":    x("expected SHA-256 of the online_theme archive").in(gAppearance),
	"highlight":              p("syntax-highlight fenced code blocks").in(gAppearance).assistant(),
	"highlight_style":        p("syntax-highlight colour scheme").in(gAppearance).assistant(),
	"highlight_line_numbers": p("show line numbers in highlighted code").in(gAppearance).assistant(),
	"mermaid":                p("render mermaid diagrams").in(gAppearance).assistant(),
	"mermaid_theme":          p("mermaid built-in theme (default, neutral, dark, forest, base)").in(gAppearance).assistant(),
	"mermaid_background":     p("mermaid background colour, e.g. \"#ffffff\"").in(gAppearance).assistant(),
	"math":                   p("render KaTeX math").in(gAppearance).assistant(),
	"toc":                    p("generate a table of contents").in(gAppearance).assistant(),
	"toc_depth":              p("heading depth the table of contents covers").in(gAppearance).assistant(),
	"shortcodes":             p("reusable content snippets rendered by a theme template").in(gAppearance),
	"shortcode_brackets":     p("also match the [shortcode] syntax").in(gAppearance),
	"shortcode_errors":       p("what a failing shortcode leaves: drop it, keep its source, or fail the build").in(gAppearance).oneOf("", "drop", "keep", "strict"),
	"render_hooks":           x("Markdown node kind → template file that renders it (any path the build can read)").in(gAppearance),
	"variables":              x("values for templates as .Vars and SSG_* environment variables; a $NAME value is read from the build's environment").in(gAppearance),
	"recipe_headings":        p("extra headings a recipe's ingredients and method are read from").in(gAppearance),
})

var _ = describe(models.Marketing{}, typeDocs{
	"verification":     p("search-console verification tokens by vendor (public meta tags)"),
	"social_profiles":  p("the site's social profile URLs by network"),
	"og_site_name":     p("og:site_name, when it differs from the title").assistant(),
	"og_image":         p("the share image for pages without their own").assistant(),
	"twitter_site":     p("the site's X/Twitter handle, e.g. \"@example\"").assistant(),
	"favicon":          p("the favicon").assistant(),
	"apple_touch_icon": p("the home-screen icon").assistant(),
	"logo":             p("the site's logo").assistant(),
	"theme_color":      p("the browser's theme colour, e.g. \"#1a73e8\"").assistant(),
	"colors":           p("the migrated theme's palette by role"),
})

var _ = describe(ContentSource{}, typeDocs{
	"name":     p("optional name, so an include file can contribute this source"),
	"path":     x("directory of the extra Markdown root"),
	"type":     p("file every entry as a page or a post").oneOf("", "page", "post"),
	"category": p("category every entry of this source is filed under"),
})

var _ = describe(models.ContentSchema{}, typeDocs{
	"required": p("front-matter fields a page of this type must have"),
	"fields":   p("rule per front-matter field"),
})

var _ = describe(models.FieldRule{}, typeDocs{
	"type":   p("the field's type").oneOf("string", "int", "bool", "date", "url", "list", "enum"),
	"values": p("allowed values when type is enum"),
})

var _ = describe(VersionsConfig{}, typeDocs{
	"noindex_old": p("noindex superseded versions and leave them out of the sitemap"),
})

var _ = describe(RedirectRule{}, typeDocs{
	"from":   p("path or /prefix/* splat redirected"),
	"to":     p("target URL; :splat inserts the matched rest"),
	"status": p("HTTP status: 301, 302, 307, 308 or 410"),
	"force":  p("redirect even when a file exists at the path"),
})

var _ = describe(Shortcode{}, typeDocs{
	"name":     p("shortcode name used in content"),
	"type":     p("type for template logic, e.g. banner"),
	"template": x("template file that renders it, joined to the theme folder (a path the build reads)"),
	"title":    p("title or heading"),
	"text":     p("text content"),
	"url":      p("link URL"),
	"logo":     p("logo or image path"),
	"legal":    p("legal text"),
	"ranking":  p("ranking score, e.g. 3.5"),
	"tags":     p("tags for categorization"),
	"data":     p("additional custom data"),
})

var _ = describe(taxonomy.DefinitionConfig{}, typeDocs{
	"label":          p("plural label, e.g. Categories"),
	"singular":       p("singular label"),
	"path":           p("URL path of the taxonomy"),
	"field":          p("front-matter field holding the terms"),
	"template":       p("theme template of the taxonomy index"),
	"term_template":  p("theme template of one term"),
	"sort":           p("how terms are sorted"),
	"multiple":       p("a page may have several terms"),
	"archive":        p("generate the term listings"),
	"feed":           p("generate a feed per term"),
	"sitemap":        p("list the term pages in the sitemap"),
	"case_sensitive": p("treat terms differing in case as different"),
	"slugify":        p("slugify terms for their URLs"),
	"generate_empty": p("generate pages for terms without content"),
	"paginate":       p("items per term page"),
	"outputs":        p("formats written for the index and the terms"),
	"outputs_index":  p("formats written for the index only"),
	"outputs_term":   p("formats written for the term pages only"),
})
