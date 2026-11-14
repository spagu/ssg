# auth — the sign-in pages

What a person sees: sign in, "Your panels", the account page, accepting an
invitation, setting a new password, asking for a reset link. The guide is the
[README](../README.md); the routes behind the forms are in [API.md](API.md).

| Page | For | Without a session |
|---|---|---|
| `/auth/login` | sign in with a password, a login provider and/or Cloudflare Access; `?next=` (a path on this host only), `?error=`, `?ok=`, `?reauth=1` (shown even when signed in: "sign in again" before changing sign-in methods) | — |
| `/auth/` | "Your panels": what the account may use, and where | to the login page, back here afterwards |
| `/auth/account` | name and address, **how you sign in** (password, linked providers and Cloudflare Access; add, remove, set a first password), change the password, where you are signed in, sign out one / everywhere | same |
| `/auth/invite?token=…` | create the account an invitation offers | — |
| `/auth/reset?token=…` | set a new password with a reset link | — |
| `/auth/forgot` | ask for a reset link by mail — only when mail is configured (otherwise `404`, and the login page shows no link) | — |

Add `/auth/*` to the worker's routes next to `/api/auth/*`, and keep the static
files out of the Functions (they are plain files from `public/auth/assets/`):

```yaml
workers:
  - dir: workers/auth
    routes_include: [/api/auth/*, /auth/*]
    routes_exclude: [/auth/assets/*]
```

## How they work

- **Server-rendered, plain forms.** Each page is rendered by a Pages Function
  (`functions/auth/*.ts` → `functions/api/auth/_pages*.ts`) and every action is
  an ordinary `<form method="post">` to the API, which answers `303` back to a
  page with `?ok=` or `?error=`. Sign in, change the password, sign out one
  session, sign out everywhere, accept an invitation, reset a password: all
  work with JavaScript switched off (checked in a browser with JavaScript
  disabled, see [Previews](#previews-and-checks)). `auth.js` only adds a "Show
  password" button, moves focus to an error message, and stops a second
  click from sending a form twice.
- **Pages see only the access cookie.** The refresh cookie never leaves
  `/api/auth/`, so after `AUTH_TOKEN_MINUTES` a page sends the browser through
  `GET /api/auth/resume`, which renews the session and comes straight back —
  or goes to the login page when there is none.
- **`next` never leaves the host.** Only a path on this host is accepted (no
  scheme, no `//`, no backslash, no control character, and not a path whose
  dot segments collapse into `//host`); anything else falls back to "Your
  panels" (or `AUTH_AFTER_LOGIN_URL` when set). A table of hostile values is
  tested against the function, the page and the login form.
- **Messages are chosen, never echoed.** `?error=` and `?ok=` pick a sentence
  from a fixed list; an unknown code shows "Something went wrong", never the
  text of the URL. Errors name nothing about the account ("The address or the
  password is not right").
- **"Your panels"** groups the account's permissions by module, with each
  permission's description. A link to a module's panel needs its address: add
  `"panel": "/newsletter-admin/"` (a path on this host, the module's base
  included) and optionally `"title": {"en": "Newsletter", …}` to that module's
  entry in `AUTH_KNOWN_PERMISSIONS`. Without it the page says plainly that
  the site has not told it where the panel is. The future `core` worker, which
  reads the build's module registry, will replace this page.

## Invitations

Accounts exist only by invitation (or the first one, by bootstrap). Someone
holding `auth:invite` (the `owner` role has it) sends one with
`POST /api/auth/admin/invitations` — or the form in the admin panel's
Invitations view ([PANEL.md](PANEL.md)). The invitation names the roles and groups the account will get, never
more than the inviter holds. With a mail transport it is mailed in the
invitee's language; without one the inviter gets the one-time link to pass on.
The invitee opens it, sees the address it is for and until when it is valid,
**chooses how to sign in** — a password, or one of the enabled login
providers (the provider account becomes the way in, whatever its address:
having the link is the proof) — and is signed in. A link works once, for
`AUTH_INVITE_HOURS` (72), and only its hash is stored.

## Login providers

With providers configured ([PROVIDERS.md](PROVIDERS.md)), the login page shows
one button per enabled provider under the password form ("Or use an account
you already have"; alone when `AUTH_PASSWORD_LOGIN=off`), the invitation page
a card of them next to the password, and the account page:

- **How you sign in** — a row per method: the password ("Set"), each linked
  provider with the address it showed when linked and when; the method of this
  session is marked; *Remove* on each, except on the only one ("Your only way
  in").
- **Add a way to sign in** — a button per enabled provider not linked yet.
- **Set a password** — for an account without one, when the password login is
  on. Changing the password stays for accounts that have one.
- A notice when `AUTH_ROLE_METHODS` leaves a role's permissions out of this
  session, naming the role and the methods it needs (also on "Your panels").

Each button is a plain form, so it works with JavaScript off; Apple's answer
(a form POST from Apple) works the same. The buttons are text in the pages'
own style with each provider's own wording ("Sign in with Google", "Continue
with Facebook"); no provider logo, nothing loaded from a provider. Errors —
cancelled consent, an expired or reused attempt, not invited, an unverified
address, a domain not allowed — come back as `?error=` like every other.

## Mail

Optional, and only for these two mails. The sending is the newsletter
template's portable mail code, carried byte for byte in
`functions/api/auth/mail/` (a test keeps the copies identical and runs the
newsletter's own mail tests against them). The settings are this worker's own,
so a newsletter configuration never sends account mail by accident:

| Name | Kind | Meaning |
|---|---|---|
| `AUTH_MAIL_FROM` | var | verified sender — required with any transport |
| `AUTH_MAIL_FROM_NAME` | var | optional display name |
| `AUTH_RESEND_API_KEY` | secret | Resend |
| `AUTH_MAILCHANNELS_API_KEY` | secret | MailChannels Email API |
| `AUTH_SMTP_HOST`, `AUTH_SMTP_PORT` (587), `AUTH_SMTP_USER`, `AUTH_SMTP_HELO` | vars | your own mail server (465 TLS or 587 STARTTLS; Cloudflare blocks 25) |
| `AUTH_SMTP_PASSWORD` | secret | its password |

The first transport set wins: Resend, then MailChannels, then SMTP. A
transport that can never send is an error at once (`503 mail_misconfigured`
with the reason), never a silent fallback. The mails are plain text and say
who invited whom to which site, what the link does, how long it lasts, and
what to do when the mail was not expected. A refused mail is logged with the
provider's reason — never with the link.

## Languages, look, accessibility

- English, Polski, हिन्दी, Deutsch, Français, with the panel's terms. The
  language comes from `?lang=` (the switcher at the bottom of every page —
  plain links, no JavaScript), then the `ssg-auth-lang` cookie the switcher
  sets (`Path=/auth`), then `Accept-Language`, then English; `<html lang>`
  follows. Every page and both mails exist in all five; a test checks that
  every language has every key and the same placeholders.
- The design tokens are the newsletter panel's (`public/auth/assets/tokens.css`
  is a byte-identical copy of `workers/newsletter-ui/public/css/tokens.css`,
  tested): Material Teal on a warm Brown paper, a serif for headings, the
  system sans for the rest, no web fonts; light and dark follow
  `prefers-color-scheme`. The colour pairs are the panel's (its contrast
  test computes them from these tokens: text 4.5:1, control edges 3:1), and
  axe's contrast rule passes on every page in both schemes.
- WCAG 2.2 AA: one `h1`, labels on every field, hints and errors tied to their
  field (`aria-describedby`, `aria-invalid`), errors announced (`role=alert`,
  and "Error:" leads the page title), visible focus, targets of 40–44 px, 360 px
  without horizontal scrolling (the sessions table turns into cards below
  640 px). **Accessible Authentication (3.3.8):** no puzzle, pasting allowed,
  `autocomplete` set (`username`, `current-password`, `new-password`, `name`),
  a hidden `username` field next to every new password so a password manager
  stores it under the right account, and "Show password".
- Meta tags on every page: description, `robots: noindex, nofollow`,
  `referrer: strict-origin` (why not `no-referrer`: [SECURITY.md](SECURITY.md#the-pages-own-headers)),
  `color-scheme`, `theme-color` for both schemes, an icon. No tag manager and
  no analytics: sign-in pages are not measured.

## Cloudflare Access

With `AUTH_ACCESS_TEAM` and `AUTH_ACCESS_AUD` set ([ACCESS.md](ACCESS.md)):

- the login page shows **Sign in with Cloudflare Access** with the provider
  buttons (alone when it is the only method) — a plain link to
  `/auth/access?next=…`, so it needs no JavaScript and no `form-action`;
- the account page offers **Connect Cloudflare Access** under "Add a way to
  sign in" — a link to `/auth/access/link`, a page (behind Access) that says
  "Cloudflare Access says you are …" and links only after a confirming form
  POST. A linked Access identity is a row in "How you sign in" like a
  provider's; it is an ordinary `identities` row, so the last-method rule
  covers an account whose only way in is Access ("Your only way in");
- the pages with a sign-out form ("Your panels", the account page) allow the
  team's Access host in their CSP `form-action`: signing out of an Access
  session goes on to the team's Access logout. Without Access their CSP is
  exactly what it was.

The words are in five languages in `_access-i18n.ts` (kept apart from
`_i18n-*.ts`, one test checks every key and placeholder); the error codes
`access_failed`, `access_unavailable`, `access_not_linked` and
`access_email_unusable` have their own sentences on the login and account
pages.

## Second factor

With `AUTH_MFA_KEY` set ([SECOND-FACTOR.md](SECOND-FACTOR.md)):

- **`/auth/mfa`** — the second step of signing in, reached only with the
  pending cookie a first factor left (otherwise back to the login page with
  `?error=mfa_expired`). It names the account the first factor proved and
  offers what that account has: the authenticator-app code (a plain form;
  `autocomplete="one-time-code"`, `inputmode="numeric"`, paste welcome), a
  passkey (a form `auth.js` completes through WebAuthn — hidden without
  JavaScript or WebAuthn), "Use a backup code" and "Lost your phone or key?"
  (folded `<details>`; the break-glass form only when `AUTH_MFA_RESET_TOKEN`
  is set).
- **`/auth/mfa/enrol`** — set up an authenticator app (a form; the next page
  draws the QR code locally as SVG, shows the key in groups of four to type,
  and asks for the first code and an optional name) or a passkey. After a
  sign-in that left roles out it says which (`?required=1`). The first factor
  answers with the **backup-codes page** (ten codes, shown once, never in a
  URL). Freshness first: the step-up page for an account with a factor, the
  login page (`reauth=1`) for an old sign-in without one.
- **`/auth/mfa/step-up?next=`** — the second factor again before a sensitive
  change; back to `next` after.
- **The account page** gains "Second factor": the factors (kind, name, added,
  last used, Remove), unused backup codes and "New backup codes", "Add a
  second factor". **"Your panels"** and the account page list roles waiting
  for a second factor in this session, with the link that gets it.

No new asset: the passkey part is a few lines appended to `auth.js` (no
request of its own — the options are in the page, the answer goes in a hidden
field of a plain form). The QR code stays dark on white in both themes.

## For code that extends these pages

- Every first factor — `login.ts` included — goes through
  `startSession(ctx, user, method, next)` (`_signin.ts`), which asks for the
  second factor when one is needed; a redirect-style flow ends with
  `signInRedirect()`.
- The admin panel ([PANEL.md](PANEL.md)) puts forms on the invitation and
  reset-link routes and shows a returned link once.

## Previews and checks

```sh
node --import ./workers/auth/test/resolve.mjs workers/auth/test/dev-server.mjs
# → http://localhost:8788/auth/login — owner@example.com or editor@example.com, "demo password 1"
#   http://localhost:8788/__demo — the demo invitation and reset links, and the mails "sent"
node workers/auth/test/auth-shots.mjs <out-dir> [axe.min.js]
```

The dev server runs the worker's own handlers on an in-memory SQLite database
with invented data; mail goes nowhere. The screenshot script (Playwright from
the machine, never a dependency here) captures every page light and dark at
1280 and 390 px wide, screen by screen; with an axe-core build it runs the WCAG
2.0–2.2 A/AA rules on each; and it walks three journeys and prints the result:
sign in → change the password → "Your panels" → sign out → sign out everywhere
with JavaScript **disabled**; signing in with the keyboard alone; and
with JavaScript disabled, signing in through GitHub, a stranger refused at
Apple, and the invitation accepted with Google.

The dev server turns the five providers on against the scripted fakes of
`test/providers-fake.ts`: a provider button leads to a stand-in consent screen
(`/__provider`, a developer page) where you choose who you are there — Eda
(linked to `editor@example.com` at GitHub), Noor (the invited address), a
stranger, or Cancel. `NOPROVIDERS=1` runs without them; `TURNSTILE=1` adds
Cloudflare's always-passing Turnstile test keys (the widget then loads from
`challenges.cloudflare.com`, the one thing that leaves the machine).
