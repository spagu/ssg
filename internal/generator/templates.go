// Package generator - templates.go contains the generic fallback templates
// scaffolded when a theme has no local files and is not one of the embedded
// starter themes (DOC-013). No external CDN references — system font stack
// only (FE-011), neutral English copy, lang="en".
package generator

// baseTemplate is the base HTML layout template with embedded content blocks
const baseTemplate = `{{define "base"}}<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>{{template "title" .}}</title>
    <meta name="description" content="{{template "description" .}}">
    <link rel="canonical" href="https://{{.Domain}}{{template "canonical" .}}">
    <link rel="stylesheet" href="/css/style.css">
    <style>body{font-family:system-ui,-apple-system,"Segoe UI",Roboto,"Helvetica Neue",Arial,sans-serif}</style>
</head>
<body>
    <a href="#main-content" class="skip-link">Skip to content</a>
    <header class="site-header" id="site-header">
        <div class="container">
            <nav class="main-nav" id="main-nav">
                <a href="/" class="logo" id="site-logo">{{.Domain}}</a>
                <div class="nav-links" id="nav-links">
                    {{range .Site.Pages}}
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

    <main class="main-content" id="main-content">
        {{template "content" .}}
    </main>

    <footer class="site-footer" id="site-footer">
        <div class="container">
            <p>&copy; {{.Domain}}</p>
        </div>
    </footer>

    <script src="/js/main.js"></script>
</body>
</html>{{end}}`

// indexTemplate is the homepage template
const indexTemplate = `{{define "index-title"}}{{.Domain}} - Home{{end}}
{{define "index-description"}}Welcome to {{.Domain}}{{end}}
{{define "index-canonical"}}/{{end}}
{{define "index-content"}}
<section class="hero" id="hero">
    <div class="container">
        <h1 class="hero-title">Welcome to {{.Domain}}</h1>
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

{{define "index.html"}}<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>{{.Domain}} - Home</title>
    <meta name="description" content="Welcome to {{.Domain}}">
    <link rel="canonical" href="https://{{.Domain}}/">
    <link rel="stylesheet" href="/css/style.css">
    <style>body{font-family:system-ui,-apple-system,"Segoe UI",Roboto,"Helvetica Neue",Arial,sans-serif}</style>
</head>
<body>
    <a href="#main-content" class="skip-link">Skip to content</a>
    <header class="site-header" id="site-header">
        <div class="container">
            <nav class="main-nav" id="main-nav">
                <a href="/" class="logo" id="site-logo">{{.Domain}}</a>
                <div class="nav-links" id="nav-links">
                    {{range .Site.Pages}}
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

    <main class="main-content" id="main-content">
        <section class="hero" id="hero">
            <div class="container">
                <h1 class="hero-title">Welcome to {{.Domain}}</h1>
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

    <footer class="site-footer" id="site-footer">
        <div class="container">
            <p>&copy; {{.Domain}}</p>
        </div>
    </footer>

    <script src="/js/main.js"></script>
</body>
</html>{{end}}`

// pageTemplate is the static page template
const pageTemplate = `{{define "page.html"}}<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>{{.Page.Title}} - {{.Domain}}</title>
    <meta name="description" content="{{.Page.Excerpt}}">
    <link rel="canonical" href="https://{{.Domain}}/{{.Page.Slug}}/">
    <link rel="stylesheet" href="/css/style.css">
    <style>body{font-family:system-ui,-apple-system,"Segoe UI",Roboto,"Helvetica Neue",Arial,sans-serif}</style>
</head>
<body>
    <a href="#main-content" class="skip-link">Skip to content</a>
    <header class="site-header" id="site-header">
        <div class="container">
            <nav class="main-nav" id="main-nav">
                <a href="/" class="logo" id="site-logo">{{.Domain}}</a>
                <div class="nav-links" id="nav-links">
                    {{range .Site.Pages}}
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

    <footer class="site-footer" id="site-footer">
        <div class="container">
            <p>&copy; {{.Domain}}</p>
        </div>
    </footer>

    <script src="/js/main.js"></script>
</body>
</html>{{end}}`

// postTemplate is the blog post template
const postTemplate = `{{define "post.html"}}<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>{{.Post.Title}} - {{.Domain}}</title>
    <meta name="description" content="{{.Post.Excerpt}}">
    <link rel="canonical" href="https://{{.Domain}}/{{.Post.Slug}}/">
    <link rel="stylesheet" href="/css/style.css">
    <style>body{font-family:system-ui,-apple-system,"Segoe UI",Roboto,"Helvetica Neue",Arial,sans-serif}</style>
</head>
<body>
    <a href="#main-content" class="skip-link">Skip to content</a>
    <header class="site-header" id="site-header">
        <div class="container">
            <nav class="main-nav" id="main-nav">
                <a href="/" class="logo" id="site-logo">{{.Domain}}</a>
                <div class="nav-links" id="nav-links">
                    {{range .Site.Pages}}
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

    <footer class="site-footer" id="site-footer">
        <div class="container">
            <p>&copy; {{.Domain}}</p>
        </div>
    </footer>

    <script src="/js/main.js"></script>
</body>
</html>{{end}}`

// categoryTemplate is the category listing template
const categoryTemplate = `{{define "category.html"}}<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>{{.Category.Name}} - {{.Domain}}</title>
    <meta name="description" content="Posts in category {{.Category.Name}}">
    <link rel="canonical" href="https://{{.Domain}}/category/{{.Category.Slug}}/">
    <link rel="stylesheet" href="/css/style.css">
    <style>body{font-family:system-ui,-apple-system,"Segoe UI",Roboto,"Helvetica Neue",Arial,sans-serif}</style>
</head>
<body>
    <a href="#main-content" class="skip-link">Skip to content</a>
    <header class="site-header" id="site-header">
        <div class="container">
            <nav class="main-nav" id="main-nav">
                <a href="/" class="logo" id="site-logo">{{.Domain}}</a>
                <div class="nav-links" id="nav-links">
                    {{range .Site.Pages}}
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

    <footer class="site-footer" id="site-footer">
        <div class="container">
            <p>&copy; {{.Domain}}</p>
        </div>
    </footer>

    <script src="/js/main.js"></script>
</body>
</html>{{end}}`
