package config

// Schema entries: AI, text to speech, notifications, the MCP server.

var _ = describe(Config{}, typeDocs{
	"ai":            p("models and agents the [ai …] shortcode queries at build time").in(gIntegr),
	"tts":           p("build-time MP3 of each article through a text-to-speech API, and a podcast feed").in(gIntegr),
	"notifications": x("webhooks each new or changed post is sent to (with --notify)").in(gIntegr),
	"notify":        p("send notifications for this build").in(gIntegr),
	"notify_state":  x("state file of sent notifications (default .ssg-notifications.json)").in(gIntegr),
	"mcp":           x("the ssg mcp server: git write-back, search, OAuth").in(gIntegr),
})

var _ = describe(AIConfig{}, typeDocs{
	"default_model": p("model used when a shortcode names none"),
	"default_agent": p("agent used when a shortcode names none"),
	"cache_dir":     x("answer cache directory (default .ssg-cache/ai)"),
	"timeout":       p("per-query timeout, e.g. 30s"),
	"models":        p("named model endpoints"),
	"agents":        p("named roles built on a model"),
})

var _ = describe(AIModel{}, typeDocs{
	"url":         x("chat-completions endpoint the content is sent to"),
	"key":         s("bearer token of the endpoint"),
	"model":       p("provider model id"),
	"system":      p("base system prompt"),
	"max_tokens":  p("answer length cap"),
	"temperature": p("sampling temperature"),
})

var _ = describe(AIAgent{}, typeDocs{
	"model":       p("model it runs on (empty = default)"),
	"system":      p("persona, layered on the model's prompt"),
	"rules":       p("constraints it follows"),
	"skills":      p("capabilities it applies"),
	"max_tokens":  p("answer length cap, overriding the model"),
	"temperature": p("sampling temperature, overriding the model"),
})

var _ = describe(TTSConfig{}, typeDocs{
	"enabled":      p("generate audio at build time"),
	"provider":     p("text-to-speech provider").oneOf("", "generic", "openai", "elevenlabs", "google"),
	"api_url":      x("API endpoint the article text is sent to"),
	"api_key":      s("API key"),
	"voice":        p("voice name"),
	"lang":         p("language (default the page's)"),
	"model":        p("provider model"),
	"speed":        p("speaking speed"),
	"instructions": p("delivery instructions (openai)"),
	"jingle_url":   x("MP3 fetched once and played before every article"),
	"timeout":      p("per request, e.g. 60s"),
	"retries":      p("extra attempts (default 2, -1 none)"),
	"max_chars":    p("characters per request"),
	"breaker":      p("failures in a row before the API is skipped (default 3, -1 never)"),
	"on_failure":   p("what a page gets when its audio fails").oneOf("", "stale", "skip", "fail"),
	"sections":     p("posts (default), pages"),
	"dir":          p("output directory (default audio)"),
	"feed":         p("write a podcast feed of the audio"),
	"feed_path":    p("podcast feed path (default podcast.xml)"),
	"feed_title":   p("podcast title"),
	"feed_author":  p("podcast author"),
	"feed_image":   p("podcast image"),
	"feed_limit":   p("episodes in the feed (default 50)"),
})

var _ = describe(NotifyDest{}, typeDocs{
	"name":          p("destination name"),
	"url":           x("webhook URL the post payload is sent to"),
	"method":        x("HTTP method (default POST)"),
	"headers":       s("request headers; values must hold $NAME references").secretAs(SecretInline),
	"allow_private": x("permit a private or loopback destination"),
})

var _ = describe(MCPConfig{}, typeDocs{
	"git":                 x("git write-back: branch, commit, pull request"),
	"search":              x("MDDB index the find tools consult"),
	"media_allow_private": x("let media_upload fetch private addresses"),
	"oauth":               x("OAuth 2.1 protection of ssg mcp --listen"),
})

var _ = describe(MCPGit{}, typeDocs{
	"account":        x("GitHub owner the pull request is attributed to"),
	"token":          s("token for opening pull requests"),
	"repo":           x("owner/name (default from the remote)"),
	"remote":         x("git remote pushed to (default origin)"),
	"default_branch": x("pull request base (default main)"),
	"branch_prefix":  x("working-branch prefix (default mcp/)"),
})

var _ = describe(MCPSearch{}, typeDocs{
	"mddb_url":        x("MDDB base URL of the index"),
	"mddb_api_key":    s("MDDB API key of the index"),
	"mddb_collection": x("collection holding the theme"),
	"mddb_lang":       p("query tokenisation language"),
	"mddb_fuzzy":      p("typo tolerance: 0 off, 1 or 2 edits"),
	"mddb_validate":   p("validate documents before storing (default on)").defaults(true),
	"mddb_allow_http": x("send the API key over plain http"),
})

var _ = describe(MCPOAuth{}, typeDocs{
	"issuer":   x("authorization server whose tokens are accepted"),
	"resource": x("this endpoint's public URI, the tokens' audience"),
	"scopes":   x("scopes a token must carry"),
	"jwks_url": x("signing keys URL (default discovered)"),
})
