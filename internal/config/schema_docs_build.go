package config

// Schema entries: output, build, server, deployment.
//
// Class rule: a key is `executes` when it runs a command, deploys, changes
// what a server does, names a program, file or directory the build reads or
// writes outside the output, or weakens a security guard.

import "github.com/spagu/ssg/internal/models"

var _ = describe(Config{}, typeDocs{
	// Output
	"output_dir":                x("directory the site is written to; clean empties it first").in(gOutput),
	"static_dir":                x("directory copied verbatim into the output").in(gOutput),
	"static_sources":            x("extra files or directories copied verbatim into the output").in(gOutput),
	"minify_all":                p("minify HTML, CSS and JS together").in(gOutput).assistant(),
	"minify_html":               p("minify generated HTML").in(gOutput).assistant(),
	"minify_css":                p("minify generated CSS").in(gOutput).assistant(),
	"minify_js":                 p("minify generated JS").in(gOutput).assistant(),
	"minify_html_keep_comments": p("comment openings minify_html keeps, e.g. email_off").in(gOutput),
	"pretty_html":               p("indent generated HTML for readability").in(gOutput).assistant(),
	"sourcemap":                 p("emit source maps for compiled assets").in(gOutput).assistant(),
	"fingerprint":               p("content-hash asset filenames for cache busting").in(gOutput).assistant(),
	"scss":                      p("compile *.scss in the output with the dart-sass CLI").in(gOutput),
	"sass_binary":               x("path of the sass program, instead of looking it up in PATH").in(gOutput),
	"bundles":                   p("concatenated CSS/JS bundles: output file → input files").in(gOutput),
	"outputs":                   p("formats written for every page, e.g. \"html,json\"").in(gOutput).assistant(),
	"outputs_custom":            p("page formats the site defines with a template of its own").in(gOutput),
	"headers":                   x("_headers blocks per path pattern; replaces the default security headers of that pattern").in(gOutput),
	"headers_defaults_off":      x("drop the built-in security and cache header blocks").in(gOutput),
	"host_files":                x("write _headers and _redirects (default on)").defaults(true).in(gOutput),
	"sanitize_html":             x("run rendered content through the HTML sanitizer (stored-XSS guard)").in(gOutput),
	"build_report":              p("file in the output the JSON build report is written to; true means build.json").in(gOutput).or(true),
	"zip":                       p("also pack the output as <domain>.zip").in(gOutput),
	"targz":                     p("also pack the output as <domain>.tar.gz").in(gOutput),
	"tarxz":                     p("also pack the output as <domain>.tar.xz").in(gOutput),

	// Build
	"clean":          p("empty output_dir before building").in(gBuild),
	"quiet":          p("print only errors").in(gBuild),
	"incremental":    p("rebuild only what a change can affect").in(gBuild),
	"markdown_cache": p("keep converted Markdown between builds").in(gBuild),
	"build_workers":  p("workers for parallel build stages (unset: one per CPU, 0: sequential)").in(gBuild),
	"resources":      p("CPU and memory ceiling every command runs under").in(gBuild),
	"mem_limit":      p("alias of resources.memory").in(gBuild),
	"profile":        p("report where the build's time went").in(gBuild).oneOf("", "text", "json"),
	"profile_pprof":  x("directory cpu.prof and heap.prof are written to").in(gBuild),
	"hooks":          x("commands run at build phases: pre_build, post_build, post_page").in(gBuild),

	// Server
	"http":                p("serve the site with the built-in server").in(gServer),
	"host":                x("address the server binds to (default 127.0.0.1)").in(gServer),
	"port":                x("port the server listens on").in(gServer),
	"watch":               p("rebuild when content, theme or config change").in(gServer),
	"auto_reload":         p("reload the browser after each rebuild (default on)").defaults(true).in(gServer),
	"watch_runner":        x("program run beside the watcher, e.g. wrangler; none turns it off").in(gServer),
	"watch_runner_config": x("config file passed to the watch runner").in(gServer),
	"watch_runner_dir":    x("working directory of the watch runner").in(gServer),
	"tls_cert":            x("TLS certificate file of the server").in(gServer),
	"tls_key":             x("TLS private key file of the server").in(gServer),
	"tls_auto":            x("obtain a Let's Encrypt certificate for tls_domain").in(gServer),
	"tls_domain":          x("public domain of the automatic certificate").in(gServer),
	"http3":               x("also serve HTTP/3 (QUIC); needs TLS").in(gServer),
	"gzip":                x("gzip-compress responses").in(gServer),
	"max_conns":           x("cap on concurrent connections (0 = unlimited)").in(gServer),
	"server_auth":         x("access control of the server").in(gServer).oneOf("", "basic", "jwt"),
	"server_users":        s("basic-auth users as \"login:$PASSWORD_VARIABLE\"").in(gServer).secretAs(SecretLogin),
	"jwt_secret":          s("HS256 secret of jwt access").in(gServer),
	"ip_allowlist":        x("IPs or CIDRs allowed to connect").in(gServer),
	"ip_blocklist":        x("IPs or CIDRs refused").in(gServer),
	"rate_limit":          x("requests per second per IP (0 = off)").in(gServer),
	"rate_burst":          x("burst size of the rate limiter").in(gServer),

	// Deployment
	"deploy":         x("deployment provider: cloudflare, github-pages, netlify, vercel, ftp, sftp").in(gDeploy),
	"deploy_project": x("Pages, site or project name").in(gDeploy),
	"deploy_branch":  x("branch deployed to").in(gDeploy),
	"deploy_target":  x("ftp/sftp URL or git remote").in(gDeploy),
})

var _ = describe(models.StaticSource{}, typeDocs{
	"path":     x("file or directory copied"),
	"dest":     p("prefix it is placed under in the output"),
	"sitemap":  p("list it in sitemap.xml"),
	"priority": p("its sitemap priority (0 = 0.8)"),
})

var _ = describe(CustomOutput{}, typeDocs{
	"name":     p("format name used in outputs"),
	"suffix":   p("file suffix, e.g. xml"),
	"mime":     p("media type"),
	"template": x("template file that renders it (any path the build can read)"),
})

var _ = describe(ResourcesConfig{}, typeDocs{
	"cpu":        p("CPUs used (0 = no cap)"),
	"memory":     p("soft memory limit, e.g. 512MiB; none turns the cgroup fit off"),
	"gc_percent": p("Go GC percent (default 100)"),
})
