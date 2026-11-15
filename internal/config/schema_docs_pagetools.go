package config

// Schema entries: page tools — story mode, Listen, Print, Cook Mode, heading
// anchors, share cards, Web Push, Web Stories, résumé.

var _ = describe(Config{}, typeDocs{
	"story":           p("story mode: reading companion and fullscreen presentation of posts").in(gPageTools).assistant(),
	"listen":          p("the Listen (read aloud) button").in(gPageTools).assistant(),
	"print":           p("the Print / PDF button").in(gPageTools).assistant(),
	"cook_mode":       p("the Cook Mode button on recipes").in(gPageTools).assistant(),
	"heading_anchors": p("a # link beside each heading").in(gPageTools).assistant(),
	"og_cards":        p("a generated share card per page (needs text_color and a background)").in(gPageTools).assistant(),
	"web_push":        p("web app manifest and subscribe script for the web-push worker").in(gPageTools),
	"web_stories":     p("a Google Web Story (AMP) per selected post, and a hub").in(gPageTools),
	"resume":          p("a Schema Resume JSON CV published as a page").in(gPageTools),
})

var _ = describe(storyShape{}, typeDocs{
	"enabled":  p("turn story mode on or off"),
	"sections": p("content it applies to: posts (default), pages, or a directory").or(""),
	"rules":    p("settings added to the pages a rule matches, in order"),
})

var _ = describe(storyRuleShape{}, typeDocs{
	"when": p("conditions a page must meet; every one given must hold"),
	"set":  p("settings the matching pages get"),
})

var _ = describe(storyWhenShape{}, typeDocs{
	"category":   p("the page's category").or(""),
	"categories": p("alias of category").or(""),
	"tags":       p("the page's tags").or(""),
	"tag":        p("alias of tags").or(""),
	"section":    p("a content directory, e.g. posts/recipes").or(""),
	"sections":   p("alias of section").or(""),
	"contains":   p("text the article contains"),
})

var _ = describe(ListenConfig{}, typeDocs{
	"enabled":  p("show the Listen button"),
	"label":    p("button text (default \"Listen\")"),
	"voice":    p("preferred browser voice, matched by name"),
	"player":   p("compact button or full audio bar for an article's MP3").oneOf("", "compact", "full"),
	"auto":     p("place the button on posts whose theme does not (default on)").defaults(true),
	"sections": p("posts, pages, or both (default posts)"),
})

var _ = describe(PrintConfig{}, typeDocs{
	"enabled":  p("show the Print / PDF button"),
	"label":    p("button text (default \"Print / PDF\")"),
	"sections": p("posts, pages, or both"),
	"auto":     p("place the button where the theme does not (default on)").defaults(true),
	"labels":   p("texts of the print view, by name"),
})

var _ = describe(cookModeFields{}, typeDocs{
	"enabled": p("show the Cook Mode button on recipes"),
	"label":   p("button text (default \"Cook mode\")"),
	"auto":    p("place the button where the theme does not (default on)").defaults(true),
	"labels":  p("texts of the cook view, by name"),
})

var _ = describe(headingAnchorsFields{}, typeDocs{
	"enabled": p("turn heading links on or off"),
	"levels":  p("heading levels that get a link (default 1, 2, 3)"),
	"symbol":  p("link text (default #)"),
	"class":   p("link class (default heading-anchor)"),
	"label":   p("aria-label; {text} is the heading's text"),
})

var _ = describe(ogCardsFields{}, typeDocs{
	"enabled":          p("turn share cards on or off"),
	"dir":              p("output directory (default og)"),
	"sections":         p("posts, pages (default both)"),
	"width":            p("card width in pixels"),
	"height":           p("card height in pixels"),
	"background":       p("background colour, #rrggbb"),
	"text_color":       p("text colour, #rrggbb (required)"),
	"accent_color":     p("accent colour, #rrggbb"),
	"background_image": p("background image, a path from the project root"),
	"overlay":          p("how strongly the background colour covers the image, 0 to 1 (default 0.6)"),
	"logo":             p("logo, a path from the project root"),
	"font":             p("font file, a path from the project root"),
	"format":           p("image format").oneOf("", "png", "jpeg"),
	"quality":          p("JPEG quality (default 85)"),
})

var _ = describe(webPushFields{}, typeDocs{
	"enabled":  p("turn Web Push on or off"),
	"endpoint": p("where the web-push worker answers (default /api/push)"),
})

var _ = describe(webStoriesFields{}, typeDocs{
	"enabled":          p("generate Web Stories"),
	"sections":         p("content the stories are made from (default posts)"),
	"url":              p("URL of the stories hub"),
	"publisher_logo":   p("publisher logo shown in each story"),
	"pages":            p("most pages per story"),
	"min_images":       p("images a post needs, the featured one included"),
	"cta":              p("call-to-action text on the last page"),
	"narration":        p("add the article's audio as narration"),
	"background_audio": p("background music file or URL"),
})

var _ = describe(breadcrumbsFields{}, typeDocs{
	"enabled":  p("emit the BreadcrumbList"),
	"home":     p("name of the first step (the home page)"),
	"sections": p("name and URL per section directory"),
})

var _ = describe(BreadcrumbSection{}, typeDocs{
	"name": p("step name"),
	"url":  p("step URL"),
})

var _ = describe(ResumeConfig{}, typeDocs{
	"file":    x("the Schema Resume JSON document; empty = off"),
	"url":     p("the page's URL (default /cv/)"),
	"title":   p("the page title (default the person's name)"),
	"contact": p("publish e-mail, phone and street address"),
})
