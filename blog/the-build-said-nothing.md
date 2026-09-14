---
title: "The Build Said Nothing"
slug: "the-build-said-nothing"
status: publish
type: post
date: 2026-09-14
tags: [release, i18n, diagnostics, seo, analytics, headers]
excerpt: "1.8.62 fixes fourteen things that never errored: pages dropped without a word, a 404 in the wrong language, a header that refused geolocation."
---

Most of the bugs that cost an afternoon are not crashes. A crash tells you where
to look. The expensive ones are where the build goes green, the counts look
plausible, and something is missing that nobody notices until a visitor does.

1.8.62 is fourteen of those. Eleven came from building one three-language site
by hand, from an empty directory, without `ssg init` and without a migration to
fill anything in. That is the path with the fewest defaults, so it is where
silence shows.

## Pages that were read and then dropped

The site had ten pages. The build said:

```text
🔄 Loading content...
   📄 Loaded 0 pages
```

Nothing else. The rule is documented: a file with frontmatter is published
only when `status` is exactly `publish`, and an omitted status counts as a
draft. That rule is fine. What was wrong is that nothing said it had fired.

A draft is something somebody chose. An omitted line is the one case nobody
chose, so that is the case the build now names:

```text
   ⚠️  3 Markdown file(s) in content/site/pages were parsed but not published — they have no `status: publish` line: about.md, privacy.md, terms.md
```

A page with `status: draft` still produces no line. That one was a decision.

The second way a page disappeared was worse, because it looked like success. A
description that reads naturally:

```yaml
description: Site sensors that feed verdicts with real readings: humidity and a relay.
```

is not valid YAML. The colon after "readings" starts a nested mapping. The page
was dropped with YAML's own sentence, the build printed its green tick, and CI
exited 0 with half of a section missing. The message now says what an author
would recognise:

```text
   ⚠️  1 Markdown file(s) were not published — their frontmatter could not be parsed:
        content/site/pages/shop.md: yaml: line 3: mapping values are not allowed in this context
          an unquoted ":" inside a value? Wrap the whole value in double quotes.
```

With `strict: true` the build fails on it instead. If your CI already runs
strict, this is the one change in this release that can turn a green build red,
and it will only do that for a page that was already missing from your site.

The third was the first thing a hand-made project hits. Without a
`metadata.json` the build stopped with a bare `open …: no such file or
directory`. A site with no WordPress export has no categories or authors to put
in that file. A missing one is now an empty set, with one line saying so. A
`source:` that names a directory that does not exist still fails, because
building an empty site from a typo would be the same silence again.

## A multilingual site, checked from the outside

The site is English by default, with Polish and Romanian. Everything below was
found by reading the output, not the templates.

**The 404 was Romanian.** The generated `404.html` used whichever language the
build rendered last, and `ro` sorts after `en` and `pl`. One root 404 answers
every dead URL, so a British visitor following an old link got Romanian copy.
It now uses the default language.

**Subscribing from `/pl/` gave you the English posts.** The build wrote
`/feed.xml`, `/pl/feed.xml` and `/ro/feed.xml` correctly, and then every page
advertised `/feed.xml` in its `<head>`. With `prefix_default_language` on, that
file does not exist at all. The injected link now names the feed of the page's
own language:

```html
<!-- /pl/ -->
<link rel="alternate" type="application/atom+xml" title="Example Site (Polski)" href="/pl/feed.xml">
```

The title changed too. It used to be the domain name. The subscribe list in a
feed reader is the one place a person reads that string, so it now uses the
site's `title:`.

**The sitemap disagreed with the page.** On three languages, 21 of 27 sitemap
URLs had `xhtml:link` alternates. The six without were the three front pages and
the three `/blog/` listings, which are the URLs search engines weight most. The
front page's HTML already listed all three languages. The sitemap now says the
same thing, and the listings are each other's alternates.

**A language switcher highlighted nothing.** In a template, `.Translations` and
`.Page.Translations` look interchangeable. Only the second one set `IsCurrent`,
so a switcher built from the first never marked a language as current. Both
mark it now.

That switcher also wanted to print `EN PL RO`. There was no case helper, so the
theme carried an if/else per language, which is exactly the hardcoding the i18n
config is meant to remove. There are now three helpers:

```gotemplate
{{ range .Page.Translations }}
  <a href="{{ .URL }}" hreflang="{{ .Lang }}"{{ if .IsCurrent }} aria-current="true"{{ end }}>{{ upper .Lang }}</a>
{{ end }}
```

`lower` and `title` exist too, named as Hugo names them. CSS `text-transform`
was never a substitute: it changes the glyphs, but not the accessible name.

**A page slugged `404` failed its own link check.** It was written to
`/404.html`, but its canonical, `og:url` and hreflang all pointed at `/404/`.
`check_links: strict` then failed the build on the page's own head. The address
now matches the file.

## Analytics: a log that lied, and a check that was too broad

The log line for a declared tag manager said:

```text
   🎯 Site metadata: gtm (set `analytics: true` to render)
```

The id was already rendering, because an id declared in `analytics_ids:` never
needed that flag. Only the log was wrong. It now reports each id as rendered,
as a placeholder, or as waiting for the flag.

On the placeholder: a value like `GTM-XXXXXXX` is no longer injected. You can
commit the key with a stand-in and fill it in later, without shipping a
container that does not exist to every page.

The second analytics bug is one that would have lasted years. To avoid adding a
second copy when a theme had wired the tag itself, the build skipped any page
where the id appeared *anywhere*. A cookie notice has to list GA4's cookies, and
they are named after the measurement id: `_ga_G-XXXXXXXXXX`. So the one page
explaining the tracking was the one page with no tracking and no consent-mode
script. Now the id only counts as wired when it appears inside a `<script>`.

## Two things a static site could not see

**Links inside CSS.** `check_links` reads HTML. Rename a self-hosted font and
the build stays green while every page falls back to another typeface, because
a missing `@font-face` source is not an error in any browser. `url()` and
`@import` in every stylesheet are now checked, resolved against the stylesheet
the way a browser resolves them:

```text
   ⚠️  broken link in css/site.css:12 → /fonts/inter-v20-latin-wght.woff2
```

**A header that said no before the visitor could.** The default `_headers`
block sent `Permissions-Policy: geolocation=(), microphone=(), camera=()`. An
empty list disables the API for every origin, including your own. A "use my
location" button got `PERMISSION_DENIED` at once, and the browser never showed a
prompt. The same header is served by `ssg --http`, so the button failed in
preview too, which made it look like a bug in the site's JavaScript.

The default is now `(self)`. Third-party frames are still blocked, which is
what the policy is for, and the visitor is asked.

The fix also exposed a trap. Overriding `/*` to change that one policy
*replaced* the whole block, so every other security header had to be restated,
and one you forgot was dropped without a word. Overrides now merge:

```yaml
headers:
  /*:
    Permissions-Policy: "geolocation=(self), microphone=(), camera=()"
    X-XSS-Protection: ""        # an empty value removes a header
```

`X-Frame-Options` and the rest keep their defaults.

## Before you upgrade

Four changes can alter what an existing site produces. Each has a section in
[Upgrading](/upgrading/):

```text
1. strict: true now fails on unparseable frontmatter, and check_links: strict on a stylesheet naming a missing file
2. a headers: override that left a header out to remove it now keeps it — use ""
3. Permissions-Policy allows your own origin; set () back if you want the APIs off everywhere
4. analytics_ids values containing XXXX are not rendered
```

The rest only removes silence. If a 1.8.62 build prints warnings a 1.8.61 build
did not, those warnings describe your site as it already was.
