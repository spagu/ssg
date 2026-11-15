package config

// Schema entries: workers, endpoints, modules, admin look, API docs.

var _ = describe(Config{}, typeDocs{
	"worker":             x("one Cloudflare Pages Functions directory or prebuilt Worker wired into the output").in(gWorkers),
	"workers":            x("several independent workers; supersedes worker").in(gWorkers),
	"modules":            p("modules deployed outside this build, for the core worker's registry").in(gWorkers),
	"admin_theme":        p("the admin panels' name, logo, colours and fonts").in(gWorkers),
	"endpoints":          x("server endpoints: redirects, proxies, forms, protected sections").in(gWorkers),
	"endpoints_platform": x("platform endpoints are compiled to").in(gWorkers).oneOf("", "cloudflare", "netlify", "vercel"),
	"api_docs":           x("code or OpenAPI packages documented as pages").in(gWorkers),
})

var _ = describe(WorkerConfig{}, typeDocs{
	"name":            x("the worker's name in a workers list"),
	"dir":             x("source directory: a Functions tree or a prebuilt _worker.js"),
	"mode":            x("functions (default) or a prebuilt worker").oneOf("", "functions", "worker"),
	"source":          x("repository or zip URL the worker is fetched from into dir"),
	"auth":            x("authentication of the source fetch"),
	"routes_include":  x("routes sent to the worker (default /api/*)"),
	"routes_exclude":  x("routes served as static files"),
	"wrangler_config": x("wrangler config file outside the project root"),
	"config":          x("free-form settings the worker phase hands to the worker"),
})

var _ = describe(WorkerAuth{}, typeDocs{
	"type":     x("how the source fetch authenticates").oneOf("", "bearer", "basic", "header"),
	"token":    s("bearer token"),
	"username": x("basic-auth user"),
	"password": s("basic-auth password"),
	"header":   x("header name of header auth"),
	"value":    s("header value of header auth"),
})

var _ = describe(ModuleConfig{}, typeDocs{
	"id":  p("the module's id, as in its module.json"),
	"url": x("the module's base path on this host, e.g. /newsletter-admin"),
	"dir": x("the module's source, read to validate its module.json"),
})

var _ = describe(AdminThemeConfig{}, typeDocs{
	"name":       p("replaces \"Site admin\" in the shared navigation"),
	"logo":       p("logo image on this host (.svg, .png, .webp)"),
	"logo_dark":  p("logo for the dark bar"),
	"main":       p("main brand colour, #rgb or #rrggbb"),
	"accent":     p("accent colour, #rgb or #rrggbb"),
	"font":       p("headings' font stack"),
	"font_body":  p("body font stack"),
	"font_files": p("@font-face files on this host"),
	"tokens":     p("colour token overrides per scheme (light, dark)"),
})

var _ = describe(AdminFontFile{}, typeDocs{
	"family": p("font family name"),
	"src":    p("font file URL on this host"),
	"weight": p("font weight"),
	"style":  p("font style"),
})

var _ = describe(Endpoint{}, typeDocs{
	"path":          x("request path, e.g. /api/quote"),
	"type":          x("endpoint type: redirect, proxy, form or auth"),
	"to":            x("redirect target, or the webhook a form is delivered to"),
	"status":        x("redirect status (default 302)"),
	"target":        x("upstream a proxy forwards to"),
	"methods":       x("allowed HTTP methods (empty = any)"),
	"allow_private": x("permit a private or loopback upstream"),
	"fields":        x("form fields forwarded (empty = all)"),
	"honeypot":      x("form field that must stay empty"),
	"redirect":      x("where the browser goes after a form submit"),
	"user":          x("basic-auth user of a protected section"),
	"password":      s("basic-auth password of a protected section"),
})

var _ = describe(APIDocsConfig{}, typeDocs{
	"name":       p("package name (default from package.json)"),
	"root":       x("directory with the package and its sources"),
	"language":   p("source language; empty detects it").oneOf("", "javascript", "typescript", "go", "php", "python"),
	"entry":      x("entry files"),
	"include":    x("globs relative to root"),
	"exclude":    x("globs left out"),
	"url":        p("page prefix (default /api/<name>/)"),
	"visibility": p("which symbols are shown").oneOf("", "public", "internal", "all"),
	"stability":  p("stability levels shown: beta, alpha, experimental (empty = all)"),
	"readme":     p("show the package README (default on)").defaults(true),
	"source_url": p("link to code with {path}, {line}, {ref}"),
	"source_ref": p("the {ref}: tag, branch, commit, or auto"),
	"playground": x("ES module URL the browser runs examples against"),
	"openapi":    x("OpenAPI 3 file documented instead of code"),
	"try_it":     p("the Try it console on OpenAPI operations (default on)").defaults(true),
})
