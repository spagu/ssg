# Licensing

ssg 1.9.0 and later are licensed under the **Business Source License 1.1**
(`BUSL-1.1`). The full text with ssg's parameters is in [LICENSE](https://github.com/spagu/ssg/blob/main/LICENSE).
Versions up to 1.8.85 stay under the BSD 3-Clause License they were released
with; nothing about them changes.

This page says what the license means in practice. It is not legal advice; the
license text wins where the two differ.

## In one paragraph

You may use ssg for anything, including building commercial sites and sites for
clients, at no charge, as long as **every site you generate keeps the "Built
with ssg" notice** the generator writes. Your content and the sites you generate
are yours. Each version of ssg becomes BSD-3-Clause again four years after its
release (1.9.0 on 2030-10-31).

## What you may do

- Use ssg in production, for personal, non-profit and commercial sites.
- Build sites for clients and charge for that work.
- Modify ssg and redistribute it, with this license and its notices intact.
- Use the themes, workers and scripts in this repository under the same terms.

## The one condition: the notice

Every page ssg writes carries two things:

- `<meta name="generator" content="ssg">` in `<head>`, and
- a visible `Built with <a href="https://ssg.tradik.com/">ssg</a>` before
  `</body>` — unless the theme already has a notice of its own.

A theme places its own notice wherever it fits (a footer, usually) and marks it
with `data-ssg-attribution`; the generator then adds nothing. The bundled
`apidoc` and `ssgtheme` do this. A link to `https://ssg.tradik.com/` anywhere in
the page also counts. Style the generated one through `.ssg-attribution`.

AMP pages and web stories carry no notice, because AMP allows no extra markup;
the rest of the site does. Redirect stubs carry only the meta tag.

`attribution: false` in the configuration removes both. **Using it requires a
commercial license.** There is no license key and no check in the binary — the
condition is a term of the license, not a technical lock.

## Your sites are yours

The license covers ssg, not what you make with it. Your Markdown, your
templates, your images and the HTML, CSS and JavaScript ssg generates from them
are yours, under whatever terms you choose. The only part of the output the
license speaks about is the notice above. This holds for output that includes
code from the bundled themes too.

## Commercial licenses and the price list

There is no price list. The launch grant above is free for everyone, including
companies, and every version released under it stays free under it: a later
change can only apply to later versions, never to the one you already run.

If paid variants appear — removing the notice, support, priority fixes — they
will be announced ahead of time in the [CHANGELOG](https://github.com/spagu/ssg/blob/main/CHANGELOG.md) and the
release notes, and they will apply to new versions only. To hear about it,
watch the repository's releases on GitHub.

To remove the notice today, or for any other arrangement, open a discussion at
https://github.com/spagu/ssg/discussions.

## Why BUSL and not BSD

BSD let anyone build a business on ssg without the project seeing any of it.
BUSL keeps the code open to read, change and redistribute, makes production use
free on one visible condition, and guarantees that every version returns to BSD
on a known date. It is the license MariaDB, HashiCorp and CockroachDB used for
the same reason, and it has an SPDX identifier, so license scanners and package
indexes recognise it.

## Questions

**Is ssg open source?** Not in the OSI sense, from 1.9.0: a license that
conditions production use is not an open source license. The source is public,
and each version becomes open source (BSD-3-Clause) four years after release.

**I pinned `latest` in the GitHub Action or `install.sh`. Did my license change?**
Yes, when it resolved to 1.9.0 or later. Both print a notice saying so. Pin
`version: v1.8.85` to stay on BSD.

**Does the notice have to be visible?** Yes — a link in the footer is enough.
The meta tag alone is not.

**I forked ssg before 1.9.0.** Your fork is BSD-3-Clause and stays so.

**Does this cover the MDDB client, workers or the TTS server in the repository?**
Yes, everything in this repository is under the one LICENSE file. Dependencies
keep their own permissive licenses.
