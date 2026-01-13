// Package generator - templates.go contains default HTML templates
package generator

// baseTemplate is the base HTML layout template with embedded content blocks
const baseTemplate = `{{define "base"}}<!DOCTYPE html>
<html lang="pl">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>{{template "title" .}}</title>
    <meta name="description" content="{{template "description" .}}">
    <link rel="canonical" href="https://{{.Domain}}{{template "canonical" .}}">
    <link rel="stylesheet" href="/css/style.css">
    <link rel="preconnect" href="https://fonts.googleapis.com">
    <link rel="preconnect" href="https://fonts.gstatic.com" crossorigin>
    <link href="https://fonts.googleapis.com/css2?family=Inter:wght@400;500;600;700&display=swap" rel="stylesheet">
</head>
<body>
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
const indexTemplate = `{{define "index-title"}}{{.Domain}} - Strona główna{{end}}
{{define "index-description"}}Witamy na stronie {{.Domain}}{{end}}
{{define "index-canonical"}}/{{end}}
{{define "index-content"}}
<section class="hero" id="hero">
    <div class="container">
        <h1 class="hero-title">Witamy na {{.Domain}}</h1>
        <p class="hero-subtitle">Najnowsze artykuły i informacje</p>
    </div>
</section>

<section class="posts-section" id="posts-section">
    <div class="container">
        <h2 class="section-title">Najnowsze wpisy</h2>
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
                <a href="/{{.Slug}}/" class="read-more">Czytaj więcej →</a>
            </article>
            {{end}}
        </div>
    </div>
</section>
{{end}}

{{define "index.html"}}<!DOCTYPE html>
<html lang="pl">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>{{.Domain}} - Strona główna</title>
    <meta name="description" content="Witamy na stronie {{.Domain}}">
    <link rel="canonical" href="https://{{.Domain}}/">
    <link rel="stylesheet" href="/css/style.css">
    <link rel="preconnect" href="https://fonts.googleapis.com">
    <link rel="preconnect" href="https://fonts.gstatic.com" crossorigin>
    <link href="https://fonts.googleapis.com/css2?family=Inter:wght@400;500;600;700&display=swap" rel="stylesheet">
</head>
<body>
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
                <h1 class="hero-title">Witamy na {{.Domain}}</h1>
                <p class="hero-subtitle">Najnowsze artykuły i informacje</p>
            </div>
        </section>

        <section class="posts-section" id="posts-section">
            <div class="container">
                <h2 class="section-title">Najnowsze wpisy</h2>
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
                        <a href="/{{.Slug}}/" class="read-more">Czytaj więcej →</a>
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
<html lang="pl">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>{{.Page.Title}} - {{.Domain}}</title>
    <meta name="description" content="{{.Page.Excerpt}}">
    <link rel="canonical" href="https://{{.Domain}}/{{.Page.Slug}}/">
    <link rel="stylesheet" href="/css/style.css">
    <link rel="preconnect" href="https://fonts.googleapis.com">
    <link rel="preconnect" href="https://fonts.gstatic.com" crossorigin>
    <link href="https://fonts.googleapis.com/css2?family=Inter:wght@400;500;600;700&display=swap" rel="stylesheet">
</head>
<body>
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
<html lang="pl">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>{{.Post.Title}} - {{.Domain}}</title>
    <meta name="description" content="{{.Post.Excerpt}}">
    <link rel="canonical" href="https://{{.Domain}}/{{.Post.Slug}}/">
    <link rel="stylesheet" href="/css/style.css">
    <link rel="preconnect" href="https://fonts.googleapis.com">
    <link rel="preconnect" href="https://fonts.gstatic.com" crossorigin>
    <link href="https://fonts.googleapis.com/css2?family=Inter:wght@400;500;600;700&display=swap" rel="stylesheet">
</head>
<body>
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
<html lang="pl">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>{{.Category.Name}} - {{.Domain}}</title>
    <meta name="description" content="Artykuły z kategorii {{.Category.Name}}">
    <link rel="canonical" href="https://{{.Domain}}/category/{{.Category.Slug}}/">
    <link rel="stylesheet" href="/css/style.css">
    <link rel="preconnect" href="https://fonts.googleapis.com">
    <link rel="preconnect" href="https://fonts.gstatic.com" crossorigin>
    <link href="https://fonts.googleapis.com/css2?family=Inter:wght@400;500;600;700&display=swap" rel="stylesheet">
</head>
<body>
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
                        <a href="/{{.Slug}}/" class="read-more">Czytaj więcej →</a>
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
