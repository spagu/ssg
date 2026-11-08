---
title: "Why ssg Moved to the Business Source License"
slug: "why-ssg-moved-to-the-business-source-license"
status: publish
type: post
date: 2026-11-08
tags: [license, release]
excerpt: "From 1.9.0 ssg is licensed under BUSL-1.1 instead of BSD. Using it stays free for every site; a commercial site keeps a one-line notice, and every version returns to BSD after four years. What changed, what did not, and why."
---

From version 1.9.0, ssg is licensed under the **Business Source License 1.1**
(BUSL-1.1) instead of the BSD 3-Clause License. This post explains what that
means for a site built with ssg and why we did it.

The short version: nobody pays anything, a commercial site keeps a one-line
"Built with ssg" link, and every version becomes BSD again four years after its
release.

## What changes for you

| Your situation | Before (BSD) | From 1.9.0 (BUSL) |
|---|---|---|
| Personal blog, school, charity, public body | Free | Free, no condition |
| Business site, shop, site with ads or sponsors | Free | Free, keep the notice |
| Site you build for a client | Free | Free, the client's site keeps the notice |
| Removing the notice from a commercial site | Allowed | Needs a commercial license |
| Selling ssg itself: hosted service, bundled in a paid product, renamed copy | Allowed | Needs a commercial license |
| Reading, changing, forking, packaging ssg | Allowed | Allowed |
| Your content and the generated site | Yours | Yours |

The notice is two things ssg writes into every page. One is
`<meta name="generator" content="ssg">` in the head. The other is a visible
"Built with ssg" link before `</body>`. A theme can put the link in its own
footer and mark it with `data-ssg-attribution`, and then ssg adds nothing. The
bundled themes already do this. There is no license key and nothing in the
binary checks anything. The condition is part of the license, not a lock.

Versions up to 1.8.85 stay under BSD for good. If you pinned one, nothing
changes for you. If your GitHub Action or install script asks for `latest`, it
now gets 1.9.0, and both say so when they install it.

## Why not stay with BSD

BSD asks for nothing. Anyone may take ssg, put it behind a "static hosting"
product, rename it, and sell it. The project never hears about it, gets no
link back, and has no way to ask for anything in return. That is a deliberate
and generous choice, and for many projects it is the right one.

For ssg it stopped being the right one. Most of the work in the last releases
came from running real sites. One has 37,000 pages generated from data. Others
are WordPress migrations that turned up a new bug every week, plus a
consultancy whose Search Console filled with errors. That work takes time, and
it continues because people use ssg. A license that lets a company build a
product on top of ssg without any trace leaves the project with no way to keep
paying for that time.

We wanted two things:

1. **Visibility.** A commercial site built with ssg says so in one line.
   That is how people find the project, and it costs a site almost nothing.
2. **A way to charge later, if ever needed**, to companies that sell ssg
   itself or want the notice gone. We wanted this without touching people
   who just build their own sites.

## Why BUSL and not something else

**GPL or AGPL would not do it.** Those licenses decide what happens when you
distribute the program or run it as a network service. ssg runs on your own
machine, and what it produces is your site. The GPL makes no claim on output,
so an AGPL ssg would still ask nothing of the sites built with it. It would
also make life harder for anyone who wants to embed it, which was never the
problem.

**A closed binary would lose more than it protects.** Being able to read the
code, file a precise bug and send a fix is a large part of why ssg works for
the people who use it. BUSL keeps all of that: the source is public, you can
change it and redistribute it.

**BUSL is a known license.** MariaDB, HashiCorp and CockroachDB used it for
the same reason. It has an SPDX identifier (`BUSL-1.1`), so license scanners,
package indexes and procurement departments recognise it instead of meeting a
custom text.

**It returns to BSD on a date.** Each version becomes BSD-3-Clause four years
after its release; 1.9.0 does on 31 October 2030. Whatever happens to the
project, the code does not stay restricted forever.

## What we are not doing

- **There is no price list.** The free grant covers everyone, companies too.
- **Terms never change for a version you already run.** If paid options ever
  appear, such as removing the notice, support or priority fixes, they will
  apply to new versions only and will be announced in the changelog first.
- **We do not claim your sites.** The license covers ssg, not your Markdown,
  your templates, your images or the HTML ssg generates. The notice is the
  only part of the output it mentions.

## What this costs, honestly

ssg is no longer open source in the OSI sense. A license that puts a condition
on production use does not meet that definition, and we would rather say so
plainly than argue about it. Some lists that only accept OSI licenses will drop
ssg. We accepted that trade.

If your case does not fit the table above, or you need a site without the
notice, [open a discussion](https://github.com/spagu/ssg/discussions).
[Licensing](https://github.com/spagu/ssg/blob/main/docs/LICENSING.md) answers
the common questions: affiliate links, forks made before 1.9.0, hosting the
sites you build. The [LICENSE](https://github.com/spagu/ssg/blob/main/LICENSE)
file is the text that counts.

What else is in 1.9.0 has [its own post](/blog/ssg-1-9-share-cards-limits-and-migration-fixes/).
