// Package generator - templates.go contains the generic fallback templates
// scaffolded when a theme has no local files and is not one of the embedded
// starter themes (DOC-013). No external CDN references — system font stack
// only (FE-011), neutral English copy. The document language is the page's
// own, falling back to the site default and to "en" only when nothing says
// otherwise — an export knows its language, and declaring every migrated
// site English is wrong for every site that is not (#208).
package generator

// partialsTemplate holds the document every page template used to repeat.
//
// The scaffold wrote four standalone documents, so its header and footer
// existed in four copies — the shape #208 removed a broken `base.html` for, and
// #216 removed from the bundled theme. The difference now is that this one
// works: Go templates cannot dispatch {{template}} on a computed name, so this
// is a skeleton the page templates wrap themselves in rather than a base they
// extend, which is exactly why the old base.html could never have rendered.
const partialsTemplate = `{{/* Shared blocks. This file defines only — it
     renders nothing on its own and is never addressed as a page template. */}}

{{define "site-name"}}{{if .Site.Title}}{{.Site.Title}}{{else}}{{.Domain}}{{end}}{{end}}

{{define "site-open"}}<!DOCTYPE html>
<html lang="{{if .Ctx.Lang}}{{.Ctx.Lang}}{{else if .Ctx.Site.DefaultLanguage}}{{.Ctx.Site.DefaultLanguage}}{{else}}en{{end}}">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>{{if .PageTitle}}{{.PageTitle}} - {{end}}{{template "site-name" .Ctx}}{{if .Suffix}} - {{.Suffix}}{{end}}</title>
    <meta name="description" content="{{if .Description}}{{.Description}}{{else if .Welcome}}Welcome to {{template "site-name" .Ctx}}{{end}}">
    <link rel="canonical" href="{{.Canonical}}">
    <link rel="stylesheet" href="/css/style.css">
    <style>body{font-family:system-ui,-apple-system,"Segoe UI",Roboto,"Helvetica Neue",Arial,sans-serif}</style>
</head>
<body>
    <a href="#main-content" class="skip-link">Skip to content</a>
    <header class="site-header" id="site-header">
        <div class="container">
            <nav class="main-nav" id="main-nav">
                <a href="/" class="logo" id="site-logo">{{template "site-name" .Ctx}}</a>
                <div class="nav-links" id="nav-links">
                    {{range .Ctx.Site.Pages}}
                    <a href="/{{.Slug}}/" class="nav-link">{{.Title}}</a>
                    {{end}}
                </div>
                <button class="menu-toggle" id="menu-toggle" aria-label="Toggle menu">
                    <span></span>
                    <span></span>
                    <span></span>
                </button>
            </nav>
        </div>
    </header>
{{end}}

{{define "site-close"}}    <footer class="site-footer" id="site-footer">
        <div class="container">
            <p>&copy; {{.Domain}}</p>
        </div>
    </footer>

    <script src="/js/main.js"></script>
</body>
</html>{{end}}`

const indexTemplate = `{{define "index-title"}}{{if .Site.Title}}{{.Site.Title}}{{else}}{{.Domain}}{{end}} - Home{{end}}
{{define "index-description"}}{{if .Site.Description}}{{.Site.Description}}{{else}}Welcome to {{if .Site.Title}}{{.Site.Title}}{{else}}{{.Domain}}{{end}}{{end}}{{end}}
{{define "index-canonical"}}/{{end}}
{{define "index-content"}}
<section class="hero" id="hero">
    <div class="container">
        <h1 class="hero-title">Welcome to {{if .Site.Title}}{{.Site.Title}}{{else}}{{.Domain}}{{end}}</h1>
        <p class="hero-subtitle">Latest articles and updates</p>
    </div>
</section>

<section class="posts-section" id="posts-section">
    <div class="container">
        <h2 class="section-title">Latest posts</h2>
        <div class="posts-grid" id="posts-grid">
            {{range .Posts}}
            <article class="post-card" id="post-card-{{.Slug}}">
                <h3 class="post-card-title">
                    <a href="/{{.Slug}}/">{{.Title}}</a>
                </h3>
                <time class="post-date" datetime="{{.Date.Format "2006-01-02"}}">
                    {{.Date.Format "2 January 2006"}}
                </time>
                <p class="post-excerpt">{{.Excerpt}}</p>
                <a href="/{{.Slug}}/" class="read-more">Read more →</a>
            </article>
            {{end}}
        </div>
    </div>
</section>
{{end}}

{{define "index.html"}}{{template "site-open" (dict "Ctx" . "Suffix" "Home" "Description" .Site.Description "Welcome" true "Canonical" (printf "https://%s/" .Domain))}}
    <main class="main-content" id="main-content">
        <section class="hero" id="hero">
            <div class="container">
                <h1 class="hero-title">Welcome to {{if .Site.Title}}{{.Site.Title}}{{else}}{{.Domain}}{{end}}</h1>
                <p class="hero-subtitle">Latest articles and updates</p>
            </div>
        </section>

        <section class="posts-section" id="posts-section">
            <div class="container">
                <h2 class="section-title">Latest posts</h2>
                <div class="posts-grid" id="posts-grid">
                    {{range .Posts}}
                    <article class="post-card" id="post-card-{{.Slug}}">
                        <h3 class="post-card-title">
                            <a href="/{{.Slug}}/">{{.Title}}</a>
                        </h3>
                        <time class="post-date" datetime="{{.Date.Format "2006-01-02"}}">
                            {{.Date.Format "2 January 2006"}}
                        </time>
                        <p class="post-excerpt">{{.Excerpt}}</p>
                        <a href="/{{.Slug}}/" class="read-more">Read more →</a>
                    </article>
                    {{end}}
                </div>
            </div>
        </section>
    </main>

{{template "site-close" .}}{{end}}`

// pageTemplate is the static page template
const pageTemplate = `{{define "page.html"}}{{template "site-open" (dict "Ctx" . "PageTitle" .Page.Title "Description" .Page.Excerpt "Canonical" (or .Canonical (.Page.GetCanonical .Domain)))}}
    <main class="main-content" id="main-content">
        <article class="page-content" id="page-{{.Page.Slug}}">
            <div class="container">
                <header class="page-header">
                    <h1 class="page-title">{{.Page.Title}}</h1>
                </header>
                <div class="page-body" id="page-body">
                    {{.Page.Content | safeHTML}}
                </div>
            </div>
        </article>
    </main>

{{template "site-close" .}}{{end}}`

// postTemplate is the blog post template
const postTemplate = `{{define "post.html"}}{{template "site-open" (dict "Ctx" . "PageTitle" .Post.Title "Description" .Post.Excerpt "Canonical" (or .Canonical (.Post.GetCanonical .Domain)))}}
    <main class="main-content" id="main-content">
        <article class="post-content" id="post-{{.Post.Slug}}">
            <div class="container">
                <header class="post-header">
                    <h1 class="post-title">{{.Post.Title}}</h1>
                    <div class="post-meta">
                        <time class="post-date" datetime="{{.Post.Date.Format "2006-01-02"}}">
                            {{.Post.Date.Format "2 January 2006"}}
                        </time>
                        {{if .Post.Categories}}
                        <div class="post-categories">
                            {{range .Post.Categories}}
                            <a href="/category/{{getCategorySlug .}}/" class="category-tag">{{getCategoryName .}}</a>
                            {{end}}
                        </div>
                        {{end}}
                    </div>
                </header>
                <div class="post-body" id="post-body">
                    {{.Post.Content | safeHTML}}
                </div>
            </div>
        </article>
    </main>

{{template "site-close" .}}{{end}}`

// categoryTemplate is the category listing template
const categoryTemplate = `{{define "category.html"}}{{template "site-open" (dict "Ctx" . "PageTitle" .Category.Name "Description" (printf "Posts in category %s" .Category.Name) "Canonical" (printf "https://%s/category/%s/" .Domain .Category.Slug))}}
    <main class="main-content" id="main-content">
        <section class="category-page" id="category-{{.Category.Slug}}">
            <div class="container">
                <header class="category-header">
                    <h1 class="category-title">{{.Category.Name}}</h1>
                    {{if .Category.Description}}
                    <p class="category-description">{{.Category.Description}}</p>
                    {{end}}
                </header>
                <div class="posts-grid" id="category-posts">
                    {{range .Posts}}
                    <article class="post-card" id="post-card-{{.Slug}}">
                        <h3 class="post-card-title">
                            <a href="/{{.Slug}}/">{{.Title}}</a>
                        </h3>
                        <time class="post-date" datetime="{{.Date.Format "2006-01-02"}}">
                            {{.Date.Format "2 January 2006"}}
                        </time>
                        <p class="post-excerpt">{{.Excerpt}}</p>
                        <a href="/{{.Slug}}/" class="read-more">Read more →</a>
                    </article>
                    {{end}}
                </div>
            </div>
        </section>
    </main>

{{template "site-close" .}}{{end}}`
