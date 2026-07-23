---
title: "Cookie policy"
slug: "cookie-policy"
link: "/cookie-policy/"
status: publish
type: page
hide_from_lists: true
description: "How this documentation site uses cookies and the choices you control."
---

This documentation site is deliberately light on cookies. The banner you saw is
the SSG `cookie-consent` worker running on its own docs — a live demo of the
feature, showing the same choices any SSG site can offer.

<p><button type="button" data-cookie-settings class="btn btn--primary">Manage cookies</button></p>

## The categories

### Strictly necessary — always on

Required for the site to work and to remember your consent choice. No consent
needed; cannot be switched off.

| Cookie | Purpose | Retention |
|---|---|---|
| `ssg_consent` | Stores your consent choice | 180 days |
| `ssgtheme-scheme` | Remembers your light/dark preference | until you clear it |

### Analytics — off until you allow it

This site ships a Google Tag Manager placeholder that is **inert** — no
container is configured, so nothing is loaded. Were analytics enabled, they
would sit in this category and load only after you consent to it.

### Marketing — off until you allow it

None in use on this site. The category is shown to demonstrate the banner's
granular choices.

## Your rights

Under the GDPR (EU/EEA) and UK GDPR you can withdraw consent as easily as you
gave it — use **Manage cookies** above. To remove cookies already stored, clear
this site's cookies in your browser.

This page is built from `pages/cookie-policy.md` in the repository; the worker
ships a generic starter you edit for your own site.
