package config

// Schema entries: SEO and discovery, feeds, checks, images.

import (
	ssgi18n "github.com/spagu/ssg/internal/i18n"
	"github.com/spagu/ssg/internal/models"
)

var _ = describe(Config{}, typeDocs{
	// SEO and discovery
	"seo":                     p("inject the OpenGraph/Twitter/JSON-LD head partial into pages without their own").in(gSEO),
	"seo_off":                 p("deprecated: use seo: false").in(gSEO),
	"analytics":               x("render the tracking snippets a migration recorded (third-party scripts on every page)").in(gSEO),
	"analytics_ids":           x("tracking ids the site declares, by vendor (gtm, ga4): the vendor's script runs on every page").in(gSEO),
	"schema":                  p("site-wide JSON-LD defaults merged into every page").in(gSEO),
	"schema_defaults":         p("JSON-LD defaults per content section (\"home\" for the root)").in(gSEO),
	"breadcrumbs":             p("the BreadcrumbList JSON-LD trail (off when a theme supplies its own)").in(gSEO).assistant(),
	"sitemap_off":             p("do not write sitemap.xml").in(gSEO),
	"sitemaps":                p("sub-sitemaps, each with its own selection; sitemap.xml becomes their index").in(gSEO),
	"sitemap_max_urls":        p("URLs per sitemap file (0 = the protocol's 50,000)").in(gSEO),
	"sitemap_prune_canonical": p("also drop pages whose canonical points elsewhere from the sitemap").in(gSEO),
	"lastmod_from_git":        p("sitemap <lastmod> from each file's last git commit").in(gSEO),
	"robots_off":              p("do not write robots.txt").in(gSEO),
	"robots_rules":            p("per-crawler robots.txt rules instead of allow-all").in(gSEO),
	"not_found_off":           p("do not write 404.html").in(gSEO),
	"meta_limits":             p("length ranges check_meta reports titles and descriptions against").in(gSEO),
	"markdown_publish":        p("publish each page's Markdown as index.md, plus llms.txt, for agents").in(gSEO).assistant(),
	"site_graph":              p("publish site-graph.json (pages, sections, links) for agents").in(gSEO).assistant(),
	"webmcp":                  p("declare search and navigation tools to in-browser agents (WebMCP); turns on search_index").in(gSEO).assistant(),
	"search_index":            p("publish search-index.json for a client-side search").in(gSEO).assistant(),
	"route_manifest":          p("write routes.json listing every generated route").in(gSEO),

	// Feeds
	"feed":               p("generate Atom feeds for the site, categories and tags").in(gFeeds),
	"feed_items":         p("items per feed (default 20)").in(gFeeds),
	"feed_full_content":  p("full content in feeds instead of summaries").in(gFeeds),
	"feeds":              p("extra feeds, each with its own selection, path and format").in(gFeeds),
	"feed_autodiscovery": p("add a <link rel=\"alternate\"> for every feed to every page (default on)").defaults(true).in(gFeeds),

	// Checks
	"strict":                   p("turn soft build problems into failures").in(gChecks),
	"check_links":              p("validate internal links after the build").in(gChecks).oneOf("", "warn", "strict"),
	"check_links_ignore":       p("site paths the link check skips: exact, or a prefix ending in /").in(gChecks),
	"check_images":             p("report images without an alt attribute").in(gChecks).oneOf("", "warn", "strict", "strict-decorative"),
	"check_markup":             p("report Markdown indented into a literal code block (default warn)").in(gChecks).oneOf("", "off", "warn", "strict"),
	"check_meta":               p("validate each indexable page's title and meta description").in(gChecks).oneOf("", "warn", "strict"),
	"check_schema":             p("validate the JSON-LD each page emits").in(gChecks).oneOf("", "warn", "strict"),
	"check_schema_recommended": p("also report recommended structured-data properties (advisory)").in(gChecks),
	"check_acp":                p("validate the rows of every acp feed").in(gChecks).oneOf("", "warn", "strict"),
	"check_ucp":                p("validate the UCP attributes of google-merchant feeds").in(gChecks).oneOf("", "warn", "strict"),
	"check_orphans":            p("report indexable pages nothing links to").in(gChecks).oneOf("", "warn", "strict"),
	"check_redirects":          p("report internal links the host answers with a redirect").in(gChecks).oneOf("", "warn", "strict"),
	"check_api":                p("report documentation problems in api_docs packages").in(gChecks).oneOf("", "warn", "strict"),
	"check_modules":            p("report problems with module manifests and modules entries (default warn)").in(gChecks).oneOf("", "off", "warn", "strict"),
	"check_admin_theme":        p("report admin_theme files missing from the output and failing contrast (default warn)").in(gChecks).oneOf("", "off", "warn", "strict"),
	"sanitize_output":          p("remove invisible characters from generated HTML, only report them, or neither").in(gChecks).oneOf("", "on", "warn", "report", "off"),

	// Images
	"webp":               p("generate WebP variants of images").in(gImages).assistant(),
	"webp_quality":       p("WebP encoder quality (1-100)").in(gImages).assistant(),
	"webp_keep_original": p("write .webp next to the originals instead of replacing them").in(gImages),
	"reconvert_images":   p("convert images again even when a WebP exists").in(gImages),
	"image_sizes":        p("responsive widths in pixels; each image gets a variant per width").in(gImages),
	"image_formats":      p("image formats to generate, e.g. \"webp,avif\"").in(gImages).assistant(),
	"avif_quality":       p("AVIF encoder quality (0-100, default 45)").in(gImages),
	"image_sizes_attr":   p("default sizes attribute for responsive images").in(gImages).assistant(),
	"image_metadata":     p("strip (default) or keep EXIF, IPTC and XMP in derivatives").in(gImages).oneOf("", "strip", "keep"),
	"images_gc":          p("remove image-cache entries the build no longer uses").in(gImages),
	"images_gc_dry":      p("only report what images_gc would remove").in(gImages),
})

var _ = describe(models.SitemapSpec{}, typeDocs{
	"path":    p("output path, e.g. /sitemap-blog.xml"),
	"source":  p("a content folder, MDDB section or external_sources name"),
	"include": p("kinds of URL: pages, posts, categories, tags, authors, taxonomies, static, home, listing"),
	"prefix":  p("URL path prefix(es), whole segments"),
})

var _ = describe(RobotsRule{}, typeDocs{
	"user_agent":  p("crawler the block applies to, * for all"),
	"allow":       p("allowed path patterns"),
	"disallow":    p("disallowed path patterns"),
	"crawl_delay": p("seconds between requests (0 = omitted)"),
})

var _ = describe(models.MetaLimits{}, typeDocs{
	"title_min":       p("shortest title not reported (0 switches the bound off)"),
	"title_max":       p("longest title not reported"),
	"description_min": p("shortest description not reported"),
	"description_max": p("longest description not reported"),
})

var _ = describe(models.FeedSpec{}, typeDocs{
	"path":         p("output path, e.g. /blog/feed.xml"),
	"name":         p("handle for the feed template helper"),
	"title":        p("feed title (default: the domain)"),
	"format":       p("feed format").oneOf("", "atom", "rss", "json", "google-merchant", "acp"),
	"source":       p("a content folder, MDDB section or external_sources name"),
	"prefix":       p("URL path prefix(es) of the items"),
	"categories":   p("categories, by name or slug"),
	"tags":         p("tags"),
	"type":         p("post (default) or page").oneOf("", "post", "page"),
	"aggregate":    p("inputs merged into one combined feed"),
	"exclude":      p("drop aggregated items by word or tag"),
	"include":      p("keep only aggregated items with these words or tags"),
	"paginate":     p("items per feed page, linked with rel=next/prev"),
	"items":        p("item cap, overriding feed_items"),
	"full_content": p("full content, overriding feed_full_content"),
	"merchant":     p("defaults of a google-merchant feed"),
	"acp":          p("seller block of an acp feed"),
	"ucp":          p("Universal Commerce Protocol attributes of a google-merchant feed"),
})

var _ = describe(models.FeedInput{}, typeDocs{
	"source":  p("an external_sources name"),
	"site":    p("this site: a content folder, or * for all posts"),
	"label":   p("provenance label added to each item"),
	"include": p("keep only items with these words or tags"),
	"exclude": p("drop items with these words or tags"),
})

var _ = describe(models.FeedFilter{}, typeDocs{
	"words": p("words matched in title and summary, case-insensitively"),
	"tags":  p("tags or categories matched"),
})

var _ = describe(models.MerchantDefaults{}, typeDocs{
	"availability": p("in_stock, out_of_stock, preorder or backorder"),
	"condition":    p("new (default), used or refurbished"),
	"brand":        p("brand of every product"),
})

var _ = describe(models.ACPSeller{}, typeDocs{
	"seller_name":             p("seller name (required)"),
	"seller_url":              p("seller's site URL"),
	"seller_privacy_policy":   p("privacy policy URL"),
	"seller_tos":              p("terms of service URL"),
	"return_policy":           p("return policy URL"),
	"accepts_returns":         p("whether returns are accepted"),
	"return_deadline_in_days": p("days to return, when returns are accepted"),
	"shipping_price":          p("shipping price with currency, e.g. \"5.00 EUR\""),
	"target_countries":        p("countries sold to (required)"),
	"store_country":           p("the store's country (required)"),
	"is_digital":              p("the products are digital"),
	"is_eligible_search":      p("eligible for search (default true)").defaults(true),
	"is_eligible_checkout":    p("eligible for checkout (default false)").defaults(false),
})

var _ = describe(models.UCPOptions{}, typeDocs{
	"checkout_eligibility": p("native_commerce checkout eligibility (default true)").defaults(true),
	"cart_eligibility":     p("native_commerce cart eligibility"),
})

var _ = describe(ssgi18n.Config{}, typeDocs{
	"enabled":                 p("turn on translation dictionaries"),
	"prefix_default_language": p("prefix the default language's URLs too"),
	"translations_dir":        x("directory of translation dictionaries (default i18n)"),
	"dictionary_fallback":     p("fall back to the default language's dictionary"),
	"content_fallback":        p("fall back to the default language's content"),
	"missing_translation":     p("what a missing translation does (default warn)"),
	"invalid_language":        p("what an unknown language code does (default fail)"),
	"duplicate_translation":   p("what two translations of one page do"),
	"fallback_languages":      p("fallback languages per language"),
})

var _ = describe(ssgi18n.LanguageConfig{}, typeDocs{
	"code":     p("language code used in URLs"),
	"locale":   p("locale, e.g. en_US"),
	"name":     p("language name shown to readers"),
	"timezone": p("IANA time zone of this language"),
})
