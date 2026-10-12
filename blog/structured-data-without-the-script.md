---
title: "Three Releases Without a Post: Structured Data, Languages and a Security Fix"
slug: "structured-data-without-the-script"
status: publish
type: post
date: 2026-10-12
tags: [release, seo, structured-data, i18n, security, workers]
excerpt: "What changed in ssg 1.8.70, 1.8.71 and 1.8.72: recipes and FAQs as structured data without a script, a CV page, a front page per language, a newsletter Worker, and a Go security release."
---

The last post here covered the API documentation in 1.8.69. Three releases
have gone out since without one. Most of what changed came from building real
sites on ssg, a multilingual marketing site and a recipe blog, where we had to
script around something ssg did not do. This post goes through those gaps and
what ssg does about them now.

## Recipes without the backfill script

A recipe site migrated from WordPress had 26 posts and a Python script. The
script read each post, found the ingredients list and the method, and copied
both into the post's `schema:` front matter so the page could carry a Recipe
rich result. It had to run again every time a recipe was added.

In 1.8.72 ssg reads them itself. When a page's structured data is a Recipe and
its `schema:` has no `recipeIngredient` or `recipeInstructions`, ssg takes
them from the post:

- the list under a heading called "Ingredients", "Składniki", "Zutaten",
  "Ingrédients", "Ingredientes" or "Ingredienti";
- the steps under "Method", "Instructions", "Preparation", "Przygotowanie",
  "Zubereitung" and the like.

A heading can be `## Ingredients` or a bold line. It can go on ("Ingredients
for 16 servings"). The steps can be a numbered list, plain paragraphs, or
paragraphs under `**Step 2**`. `recipe_headings` adds a site's own names.
`recipeCategory` now comes from the post's categories. Anything you do write
in `schema:` still wins, field by field.

`{{ duration "PT1H25M" }}` prints "1 hr 25 min", in six languages. Structured
data keeps times as ISO 8601, so a theme no longer needs a second,
hand-written copy to show them. `check_schema_recommended: true` lists what a
recipe card would show if it were there, such as yield, time and nutrition,
as advice that never fails a build.

## More structured data, no extra front matter

- **FAQ.** A `faq:` list in front matter becomes an FAQPage block.
- **Listings.** With `seo: true`, every listing and archive page is an
  ItemList of the posts it shows, in order.
- **Authors.** An author's archive is that author's ProfilePage.
- **The site.** The front page names the Organization behind it, using the
  name, logo and social profiles already set in `marketing:`.

A theme that writes its own JSON-LD keeps full control and gets these blocks as
`.StructuredData` to include if it wants.

## A CV page from Schema Resume

[Schema Resume](https://schema-resume.org) is a JSON format for a CV. Point
ssg at one:

```yaml
resume:
  file: cv.json
```

ssg builds a page at `/cv/` that renders in any theme, with ProfilePage
structured data that lists your roles, education, credentials, skills and
languages in schema.org terms. It also publishes the source document next to
the page as `resume.json`. Your e-mail, phone and street address stay out of
the page, the structured data and that file unless you set `contact: true`.

## A front page per language

On a site in Hindi, English and Polish, each language needed its own home
page. A page with `link: "/en/"` worked, but ssg did not know it was the
English front page. It wrote the English post listing to the same file, and
whichever was rendered last won. Since 1.8.70, a page at a language's root is
that language's front page, and its listing moves to `/en/blog/`. The listing
pages also gained what a theme needs for its `<head>`: the language, the
canonical address of each paginated page, and hreflang for every language.

## Workers

- **Newsletter.** `ssg new worker newsletter` is a sign-up form backed by
  Cloudflare D1. It stores the consent text with each address, checks
  Turnstile, and supports one-click unsubscribe and double opt-in. The
  admin view exports CSV.
- **Turnstile secrets.** Workers in one Cloudflare Pages project share one
  environment. Setting `TURNSTILE_SECRET` for the comments Worker used to turn
  on a check in the cookie-consent log that the banner never passed, so every
  consent record was dropped without an error. Each Worker now reads its own
  secret first.
- **Cookie dialog focus.** The consent dialog now keeps keyboard focus inside
  it and hands it back to the element that opened it when it closes.

## Smaller fixes worth knowing

- **Render hooks** (1.8.70) are now looked up in the theme first, so a theme
  can ship them.
- **The snap's private `/tmp`.** The snap build now warns when you point it
  at `/tmp`, which is the snap's own private copy. Before 1.8.70 it reported
  success, and the files were nowhere you could find them.
- **Docker preview.** In the Docker image, which has no Node, the preview now
  says once that Functions are not served, instead of failing every time it
  starts. `watch_runner: none` silences it.

## 1.8.71: update now

1.8.71 is a security release. Dependabot did not report this: it watches
dependencies, and these vulnerabilities were in Go itself. `govulncheck` found
13 advisories in Go 1.27.1's standard library and in `golang.org/x/net` that
ssg's own code calls, in HTTP/2, `net/http`, `crypto/tls`, `html/template` and
`os`. That covers the preview server, every remote fetch and every page ssg
renders. ssg 1.8.71 is built with Go 1.27.2. Nothing in a site changes; replace
the binary, package or image. Building ssg from source now needs Go 1.27.2.

The [changelog](https://github.com/spagu/ssg/blob/main/CHANGELOG.md) has
everything, and [UPGRADING](/upgrading/) has the one thing in 1.8.72 to check:
recipe pages may now publish ingredients, steps and a category they did not
publish before.
