# Upgrading

Every release is a drop-in replacement unless this page says otherwise. Replace
the binary, rebuild, and the site you had is the site you get.

Where that is not true — where a default changed, a flag moved, or a setting
started meaning something new — it is listed below, with the version it landed
in and what to do about it. Of the 64 releases so far, 53 need nothing at
all, which is why this page is organised by **what changed**, not by release
number: a section per version would be mostly empty and would bury the handful
that matter.

## The compatibility promise

A setting that stops being the right way to do something keeps working. It is
not removed in the release that replaces it — it starts warning in the build
log, naming what to use instead, and stays accepted for roughly five releases
after that. So an upgrade never fails silently: either it just works, or the
build tells you what to change while still producing your site.

Deprecation warnings are worth reading rather than filtering out. They are the
only notice you get, and they expire.

## How to upgrade

The install method decides the command; none of them need the site touched.

```bash
# One-liner (Linux/macOS) — same command installs and upgrades
curl -sSL https://raw.githubusercontent.com/spagu/ssg/main/install.sh | bash

# Debian/Ubuntu, if you added the apt repository
sudo apt update && sudo apt install --only-upgrade ssg

# macOS
brew upgrade ssg

# Docker — pull the tag you want rather than reusing a cached latest
docker pull ghcr.io/spagu/ssg:latest
```

Confirm what you actually have before and after, because a stale binary earlier
in `PATH` is the most common reason an upgrade appears to do nothing:

```bash
ssg --version
```

Full release notes for every version live in
[CHANGELOG.md](https://github.com/spagu/ssg/blob/main/CHANGELOG.md). This page
covers only the steps; the changelog covers everything else.

## What applies to you

<div class="upgrade-picker" hidden>
  <label for="upgrade-from"><strong>I am upgrading from</strong></label>
  <select id="upgrade-from">
    <option value="">— choose your current version —</option>
    <optgroup label="1.8.x">
      <option value="1.8.21">1.8.21 — 2026-08-07</option>
      <option value="1.8.20">1.8.20 — 2026-08-07</option>
      <option value="1.8.19">1.8.19 — 2026-08-06</option>
      <option value="1.8.18">1.8.18 — 2026-08-04</option>
      <option value="1.8.17">1.8.17 — 2026-08-04</option>
      <option value="1.8.16">1.8.16 — 2026-08-02</option>
      <option value="1.8.15">1.8.15 — 2026-08-01</option>
      <option value="1.8.14">1.8.14 — 2026-08-01</option>
      <option value="1.8.13">1.8.13 — 2026-07-24</option>
      <option value="1.8.12">1.8.12 — 2026-07-22</option>
      <option value="1.8.11">1.8.11 — 2026-07-22</option>
      <option value="1.8.10">1.8.10 — 2026-07-21</option>
      <option value="1.8.9">1.8.9 — 2026-07-21</option>
      <option value="1.8.8">1.8.8 — 2026-07-20</option>
      <option value="1.8.7">1.8.7 — 2026-07-15</option>
      <option value="1.8.6">1.8.6 — 2026-07-15</option>
      <option value="1.8.5">1.8.5 — 2026-07-15</option>
      <option value="1.8.4">1.8.4 — 2026-07-14</option>
      <option value="1.8.3">1.8.3 — 2026-07-14</option>
      <option value="1.8.2">1.8.2 — 2026-07-11</option>
      <option value="1.8.1">1.8.1 — 2026-07-10</option>
      <option value="1.8.0">1.8.0 — 2026-07-10</option>
    </optgroup>
    <optgroup label="1.7.x">
      <option value="1.7.15">1.7.15 — 2026-07-09</option>
      <option value="1.7.14">1.7.14 — 2026-07-08</option>
      <option value="1.7.13">1.7.13 — 2026-04-08</option>
      <option value="1.7.12">1.7.12 — 2026-04-08</option>
      <option value="1.7.11">1.7.11 — 2026-04-06</option>
      <option value="1.7.10">1.7.10 — 2026-04-06</option>
      <option value="1.7.9">1.7.9 — 2026-04-06</option>
      <option value="1.7.8">1.7.8 — 2026-04-06</option>
      <option value="1.7.7">1.7.7 — 2026-04-01</option>
      <option value="1.7.6">1.7.6 — 2026-04-01</option>
      <option value="1.7.4">1.7.4 — 2026-04-01</option>
      <option value="1.7.3">1.7.3 — 2026-03-31</option>
      <option value="1.7.2">1.7.2 — 2026-03-31</option>
      <option value="1.7.1">1.7.1 — 2026-03-30</option>
      <option value="1.7.0">1.7.0 — 2026-03-05</option>
    </optgroup>
    <optgroup label="1.6.x">
      <option value="1.6.2">1.6.2 — 2026-03-05</option>
      <option value="1.6.1">1.6.1 — 2026-03-05</option>
      <option value="1.6.0">1.6.0 — 2026-03-05</option>
    </optgroup>
    <optgroup label="1.5.x">
      <option value="1.5.4">1.5.4 — 2026-02-04</option>
      <option value="1.5.3">1.5.3 — 2026-02-04</option>
      <option value="1.5.2">1.5.2 — 2026-02-03</option>
      <option value="1.5.1">1.5.1 — 2026-02-03</option>
      <option value="1.5.0">1.5.0 — 2026-02-03</option>
    </optgroup>
    <optgroup label="1.4.x">
      <option value="1.4.9">1.4.9 — 2026-01-29</option>
      <option value="1.4.8">1.4.8 — 2026-01-29</option>
      <option value="1.4.7">1.4.7 — 2026-01-29</option>
      <option value="1.4.6">1.4.6 — 2026-01-23</option>
      <option value="1.4.5">1.4.5 — 2026-01-23</option>
      <option value="1.4.4">1.4.4 — 2026-01-18</option>
      <option value="1.4.3">1.4.3 — 2026-01-18</option>
      <option value="1.4.2">1.4.2 — 2026-01-18</option>
      <option value="1.4.1">1.4.1 — 2026-01-18</option>
      <option value="1.4.0">1.4.0 — 2026-01-18</option>
    </optgroup>
    <optgroup label="1.3.x">
      <option value="1.3.4">1.3.4 — 2026-01-17</option>
      <option value="1.3.3">1.3.3 — 2026-01-17</option>
      <option value="1.3.2">1.3.2 — 2026-01-17</option>
      <option value="1.3.1">1.3.1 — 2026-01-17</option>
      <option value="1.3.0">1.3.0 — 2026-01-17</option>
    </optgroup>
    <optgroup label="1.2.x">
      <option value="1.2.0">1.2.0 — 2026-01-16</option>
    </optgroup>
    <optgroup label="1.1.x">
      <option value="1.1.0">1.1.0 — 2026-01-13</option>
    </optgroup>
    <optgroup label="1.0.x">
      <option value="1.0.0">1.0.0 — 2026-01-13</option>
    </optgroup>
  </select>
  <p class="upgrade-count" role="status"></p>
</div>

Pick your current version and the list below narrows to the steps between it and
today. Without JavaScript the selector stays hidden and every step is shown,
which is a longer read but never a wrong one.

<div class="upgrade-steps">

<div class="upgrade-step" data-since="1.8.23">

### 1.8.23 — `related`'s three-argument form is now `relatedIn`

Two different functions were registered under the name `related`, and the
two-argument one won. So the three-argument form the reference documented never
ran: a theme using it failed with *"wrong number of args for related"*, every
post was skipped, and the build still reported success — which on a first build
reads as though the content never loaded.

**If you use `related page n`, do nothing** — that is the form that always ran,
and it is unchanged. **If you followed the old reference and passed a
collection**, the name is now `relatedIn`:

```gotemplate
{{ .Site.Posts | relatedIn .Page 3 }}
```

The two rank differently, which is why both survive rather than one being
dropped: `related` scores shared tags and keywords over the site's own posts,
`relatedIn` scores shared tags (3) > categories (2) > same author (1) over the
collection you hand it.

</div>

<div class="upgrade-step" data-since="1.8.23">

### 1.8.23 — `formatDate` actually formats

It never did: every non-string fell through to Go's `%v`, and `Page.Date` is a
`time.Time`, so themes rendered `2017-05-13 20:36:46 +0000 UTC` — including
inside `datetime` attributes, where it is not valid HTML.

**Your dates will change appearance.** The default is now `13 May 2017`, and a
layout is accepted:

```gotemplate
{{ formatDate .Date }}                {{/* 13 May 2017 */}}
{{ formatDate .Date "2006-01-02" }}   {{/* 2017-05-13 */}}
```

A zero date now renders empty instead of `1 January 0001`. Strings are still
passed through untouched, so a theme that pre-formats its dates is unaffected.

</div>

<div class="upgrade-step" data-since="1.8.23">

### 1.8.23 — a `404.html` is generated

Static hosts answer an unmatched path by falling back to `index.html` **with a
`200`** unless the output contains a `404.html`, so every dead URL read to a
crawler as another live copy of the home page.

**Do nothing** — a minimal one is generated, and a page slugged `404` still
takes precedence. If you deliberately want none:

```yaml
not_found_off: true
```

Note the new file in your output; a deploy diff will show it once.

</div>

<div class="upgrade-step" data-since="1.8.23">

### 1.8.23 — `pretty_urls` now decides what a page says about itself

It used to feed link checking only, so a site on a host that strips extensions
published canonical tags, `og:url`, JSON-LD and a sitemap naming URLs that
`308` — the one thing a canonical must not do. Those now name the URL the host
actually answers.

**Check which host you have.** `pretty_urls: true` still means what it always
meant — strip `.html` *and* add a trailing slash. Cloudflare Pages does **not**
add the slash, so on Pages the accurate setting is:

```yaml
pretty_urls: strip        # /docs/intro.html → /docs/intro
```

| Value | Host behaviour |
|---|---|
| `false` / `off` | Serves files literally |
| `strip` | Drops `.html`, no trailing slash (Cloudflare Pages) |
| `true` / `strip-slash` | Drops `.html`, adds the slash |

If you leave `true` on a host that does not add the slash, your canonical tags
will name URLs that redirect — the situation this release exists to fix. Feed
entry IDs deliberately keep the raw form, so subscribers are not re-delivered
every post.

</div>

<div class="upgrade-step" data-since="1.8.23">

### 1.8.23 — the bundled theme reads `variables.gtm_id`

`ssgtheme` had the Tag Manager container ID hardcoded, so using GTM meant
editing the theme — which put the ID in the theme rather than the site, lost it
on every theme update, and made two sites sharing the theme impossible.

**If you edited the theme to insert your container ID, move it to the config**,
or Tag Manager will stop loading:

```yaml
variables:
  gtm_id: GTM-XXXXXXX
```

When `variables.cookie_consent` is also set the loader is consent-gated rather
than live, since the container request is itself a third-party call made before
any choice.

</div>

<div class="upgrade-step" data-since="1.8.18">

### 1.8.18 — `sitemap.xml` drops pages marked `noindex`

A page whose rendered HTML carries a `noindex` robots directive is no longer
listed in the sitemap. Submitting a URL you have asked search engines to ignore
is a contradiction, so the two now agree.

**Do nothing** unless you were relying on those URLs being listed for something
other than search — a link checker or a warm-up crawler reading `sitemap.xml`
will see fewer URLs than before.

In the same release, `seo: true` fills a missing meta description from the
front-matter excerpt instead of leaving it out. If you deliberately shipped
pages with no description, they now have one.

</div>

<div class="upgrade-step" data-since="1.8.17">

### 1.8.17 — builds render in parallel by default

Rendering now uses a worker pool sized to the machine. Output is byte-identical
to the sequential build, which is enforced by `make determinism` on every
change, so this is a speed change and not an output change.

**Do nothing.** If you need the old behaviour — a constrained CI box, or
debugging — turn it off:

```bash
ssg --workers=0 …          # or build_workers: 0 in the config
```

Unset means "one worker per CPU"; `0` means sequential; any other number is
taken exactly.

</div>

<div class="upgrade-step" data-since="1.8.15">

### 1.8.15 — `rewrite_md_links` defaults to on

A relative `.md` link in your content is rewritten to the final output URL. It
had to be switched on before; now it is on unless you switch it off.

**Do nothing** in almost every case: a raw `.md` link in a built site either
404s or serves the Markdown source, so the old default was rarely what anyone
wanted. Opt out only if you were deliberately shipping `.md` links:

```yaml
rewrite_md_links: false
```

The rewriting itself did not change — only whether it runs by default.

</div>

<div class="upgrade-step" data-since="1.8.7">

### 1.8.7 — `seo_off` does what it says again

Between 1.8.2 and 1.8.6, `seo_off` and `--seo-off` were accepted but did
nothing, because SEO injection had become opt-in and there was nothing left to
turn off. From 1.8.7 the key is honoured again and forces SEO off.

**Check for a stale `seo_off: true`** left in a config from before 1.8.2. It was
harmless for five releases and is not any more: combined with `seo: true` it
wins, and your OpenGraph tags disappear.

Also in this release, every value flag accepts both spellings — `--flag=value`
and `--flag value` — which undoes the 1.8.1 restriction below.

</div>

<div class="upgrade-step" data-since="1.8.2">

### 1.8.2 — SEO injection became opt-in

**This is the one most likely to surprise you.** The generator-level OpenGraph,
Twitter and JSON-LD partial is off by default. SSG will not rewrite your
rendered `<head>` unless asked.

The reasoning is that SEO injection *modifies your HTML*, unlike the sitemap and
`robots.txt`, which write separate files and stay on. Anything that edits your
markup should be something you asked for.

**If you relied on automatic OpenGraph tags, turn them back on:**

```yaml
seo: true
```

```bash
ssg --seo …
```

Skip this and the build still succeeds — it simply stops emitting the tags, so
the symptom shows up later as bare link previews on social platforms rather than
as a failure at build time. It is worth checking one page's `<head>` after
upgrading.

</div>

<div class="upgrade-step" data-since="1.8.1">

### 1.8.1 — boolean and simple string flags wanted `--flag=value`

For six releases, boolean and simple string options had to be written joined:
`--flag=value`, not `--flag value`.

**Applies only if you are landing on 1.8.1 through 1.8.6.** Going to 1.8.7 or
later, both spellings work and there is nothing to do.

</div>

<div class="upgrade-step" data-since="1.8.0">

### 1.8.0 — the mddb API key is not sent over plaintext

The HTTP client refuses to attach `Authorization: Bearer` over `http://` to a
non-loopback host; `https://` and loopback addresses are unaffected. The gRPC
client picks transport security from the scheme — `grpcs://` and `https://` get
TLS, `grpc://` and `http://` do not, a bare host gets TLS unless it is loopback
— and likewise refuses to send a key over an insecure channel to a non-loopback
host.

**If your mddb URL is `http://` on a remote host, move it to `https://`** (or
`grpcs://`). The key is otherwise dropped, and mddb answers as if you were
unauthenticated — so the failure looks like missing content, not like an auth
error.

Local development against `127.0.0.1` or `localhost` keeps working untouched.

</div>

<div class="upgrade-step" data-since="1.7.15">

### 1.7.15 — the dev server binds loopback only

The built-in server listens on `127.0.0.1` instead of `0.0.0.0`. A development
server reachable from the whole network by default is a hazard, so exposing it
is now a deliberate act.

**If you reached the dev server from another machine** — a phone on the same
Wi-Fi, a container, a colleague — say so explicitly:

```bash
ssg --http --host=0.0.0.0 --port=8888 …
```

There is a matching `host:` config key, defaulting to `127.0.0.1`.

</div>

<div class="upgrade-step" data-since="1.7.14">

### 1.7.14 — Go 1.26.5 or newer to build from source

The module's `go` directive was raised to go1.26.5 to pick up a `crypto/tls` fix
(GO-2026-5856).

**Only affects building from source.** Released binaries, packages, the Docker
image and the GitHub Action are all built for you and carry no toolchain
requirement.

</div>

<div class="upgrade-step" data-since="1.3.1">

### 1.3.1 — WebP conversion needs the `cwebp` binary

1.3.0 briefly used a native Go library; 1.3.1 went back to `cwebp`, which is
what makes static builds and cross-compilation possible without CGO.

**Install the WebP tools** if you generate WebP images:

```bash
sudo apt install webp      # Debian/Ubuntu
brew install webp          # macOS
```

From 1.3.4 the GitHub Action installs them for you, so Action users can skip
this.

</div>

</div>

## Everything else

The releases not listed above changed nothing you have to act on: new features
you can ignore, fixes, performance work, security hardening internal to SSG, and
documentation. They are still worth skimming in the
[changelog](https://github.com/spagu/ssg/blob/main/CHANGELOG.md) — features
arrive far more often than steps do.

## If an upgrade goes wrong

Releases are not deleted. Every previous version stays downloadable from the
[releases page](https://github.com/spagu/ssg/releases), so going back is a
matter of installing the older binary. Your content and config are untouched by
an upgrade, so a rollback needs no cleanup.

If a build breaks in a way this page does not explain, that is a bug worth
reporting rather than working around —
[open an issue](https://github.com/spagu/ssg/issues) with the version you came
from, the version you went to, and what the build printed.

<style>
.upgrade-picker { margin: 2rem 0; }
.upgrade-picker select { display: block; margin-top: 0.5rem; padding: 0.5rem; max-width: 22rem; width: 100%; }
.upgrade-count { margin-top: 0.5rem; font-weight: 600; }
.upgrade-step[hidden] { display: none; }
</style>

<script>
/* Progressive enhancement: the picker is hidden until this runs, so with no
   JavaScript every step is shown and the page is still complete and correct.
   Filtering only ever hides steps that do not apply. */
(function () {
  var picker = document.querySelector('.upgrade-picker');
  var select = document.getElementById('upgrade-from');
  var count = document.querySelector('.upgrade-count');
  var steps = Array.prototype.slice.call(document.querySelectorAll('.upgrade-step'));
  if (!picker || !select || !steps.length) return;

  /* Compare dotted numeric versions of any length — 1.7.8.1 sorts after 1.7.8
     and before 1.7.9, so a shorter version is not treated as larger. */
  function cmp(a, b) {
    var x = a.split('.').map(Number), y = b.split('.').map(Number);
    for (var i = 0; i < Math.max(x.length, y.length); i++) {
      var d = (x[i] || 0) - (y[i] || 0);
      if (d) return d < 0 ? -1 : 1;
    }
    return 0;
  }

  function apply() {
    var from = select.value;
    if (!from) {
      steps.forEach(function (s) { s.hidden = false; });
      count.textContent = '';
      return;
    }
    var shown = 0;
    steps.forEach(function (s) {
      /* A step applies when it landed after the version you are on. */
      var applies = cmp(s.getAttribute('data-since'), from) > 0;
      s.hidden = !applies;
      if (applies) shown++;
    });
    count.textContent = shown === 0
      ? 'Nothing to do — upgrading from ' + from + ' is a drop-in replacement.'
      : shown + (shown === 1 ? ' step applies' : ' steps apply') + ' when upgrading from ' + from + '.';
  }

  select.addEventListener('change', apply);
  picker.hidden = false;
})();
</script>
