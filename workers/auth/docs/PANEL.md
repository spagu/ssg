# auth — the accounts admin panel

`/auth/admin/` is where an administrator sees who may do what, and changes or
takes it away in a minute: people, invitations, groups, roles, an access
matrix, sessions, service accounts, Cloudflare Access mappings, the audit log
and the worker's status. The guide is the [README](../README.md); the routes
behind every button are in [API.md](API.md) (the panel's own in
[The admin panel's routes](API.md#the-admin-panels-routes-go-138)).

## Opening it

Nothing to configure: the panel ships with the worker. `routes_include:
[/api/auth/*, /auth/*]` already covers `/auth/admin/` (a Pages Function
serves the shell), and `routes_exclude: [/auth/assets/*]` keeps its scripts
and stylesheets — `public/auth/assets/admin/` — out of the Functions.

- **No session** → the browser goes through `/api/auth/resume` to the login
  page and comes back.
- **A session holding neither `auth:admin` nor `auth:invite`** → a `403` page
  in words, with a link to "Your panels".
- **`auth:invite` only** → the Invitations view alone (roles and groups are
  chosen by an `auth:admin` holder).
- **`auth:admin`** → everything below. "Your panels" (`/auth/`) links here,
  and `core`'s shared bar lists the views (the `nav` entries of
  `module.json`).

The panel hides what the account cannot use; the API refuses it regardless,
so a hidden button is courtesy, never the protection.

## The views

| View | Needs | What it does |
|---|---|---|
| **People** `#/people` | `auth:admin` | every account, by address, a page at a time; search by address or name; filter by status, role (held directly or through a group) and group |
| **A person** `#/people/<id>` | `auth:admin` | the account; **what it may do and where each permission comes from** ("role X, granted directly" / "role X, through group Y"); groups (join, leave) and direct roles (grant, take away); how it signs in; its second factor (never a secret); its sessions (end one); its last 20 audit events. Actions: block / unblock, password reset link, sign out everywhere, reset second factor (owners only, while the second factor is on), delete |
| **Invitations** `#/invitations` | `auth:invite` | the form — address, name, roles and groups (with `auth:admin`), language, by mail or as a link — and the invitations sent (open by default, all on request) with Send again and Withdraw |
| **Groups** `#/groups`, `#/groups/<id>` | `auth:admin` | each group with its members and the roles it grants; add a member by address, remove one; grant or take away a role; rename; delete |
| **Roles** `#/roles`, `#/roles/<id>` | `auth:admin` | each role with built-in ones marked, wildcards and unknown names flagged; the editor picks permissions from the registry grouped by module with their descriptions in the viewer's language, offers `worker:*` per module and `*` behind a warning, keeps (and flags) names no module declares, and lists who holds the role |
| **Access matrix** `#/matrix` | `auth:admin` | people × modules: all, some (*n of m*) or none of each module's permissions — a real table with row and column headers, the person's column sticky — and the same in sentences under it; filter by person, status, module and level |
| **Sessions** `#/sessions` | `auth:admin` | every live session: who, how they signed in, whether a second factor was passed, started, last used; End one; **Sign everyone out** (break-glass) |
| **Service accounts** `#/services`, `#/services/<id>` | `auth:admin` | accounts for scripts: enable / disable, roles, what their tokens can reach, tokens (expiry, last use, revoke), a new token (narrowed permissions; `allowSensitive` offered only to `*` holders) |
| **Access mappings** `#/mappings` | `auth:admin` | only while Cloudflare Access is configured: an Access group → an `auth` group |
| **Audit log** `#/audit` | `auth:admin` | sign-ins, refusals, second-factor events and every administrative change — when, what, about whom, by whom, how, detail; filter by event kind, account (about it or done by it) and dates; Export CSV |
| **Status** `#/status` | `auth:admin` | signing keys and their age, token lifetime and the longest wait for a permission change, password hashing, ways in, Access, second factor and its policy (and how many accounts lack a needed factor), mail, open invitations, the audit log's retention, the modules the registry knows; what needs attention first |

### Things the panel never does

- **Shows a secret twice.** An invitation or reset link given as a link, and a
  service token's secret, appear once, in a box that says so ("This link will
  not be shown again"), with a Copy button; the panel keeps no copy, and the
  worker stores only hashes.
- **Builds HTML from data.** Every value is inserted as text (no
  `innerHTML`); the shell's data block is JSON that cannot close its element.
- **Replays a change.** See step-up.

### Confirmations

Every change that takes something away asks first, naming the object and the
consequence — "Delete anna@example.com? Removes the account, its sessions and
its sign-in methods … Audit entries stay." The dialog is a modal `<dialog>`:
the rest of the page is inert (Tab stays inside), Escape cancels, focus starts
on Cancel and returns to the button that opened it. Signing everyone out also
needs a ticked "I understand that everyone will be signed out".

A refusal the API explains is shown in words: the owner invariant ("this
would leave no active account holding the owner role — grant owner to someone
else first"), escalation (the permissions you do not hold), your own account,
a name taken, a mail that cannot go out.

## Step-up

These changes need a second factor from the last `AUTH_STEP_UP_MINUTES` (10)
when your account has one ([SECOND-FACTOR.md](SECOND-FACTOR.md#step-up)):
handing out `*` or `auth:*` (a grant, a group membership, an invitation sent
or resent, an Access group mapping, a role edited to hold one) and taking
either away (revoking such a grant, removing such a member, deleting such a
role, group or mapping, editing it out of a role), a password reset link for
another account, deleting an account, blocking an owner, ending another
account's sessions (one, all, or everyone's), resetting someone's second
factor, a service token with `allowSensitive`.

Taking `*` or `auth:*` away also needs the caller to hold it (403
`escalation`), as handing it out does; so does ending the sessions of,
disabling or deleting an account — a service account included — that holds
`*` or `auth:*`. An `auth:admin` cannot strip, sign out or switch off the
owners (security review SR-04).

When the API answers `step_up_required`, the panel says so ("Confirm it is
you") and, on consent, goes to `/auth/mfa/step-up`, which brings the browser
back to the **same view** — with a notice that nothing has been changed yet.
**The action is never replayed:** you press it again, with the factor fresh,
knowingly (`js/models/stepup.js`: idle → needed → leaving → returned).

## Freshness

A changed grant reaches a session at its next token refresh — at most
`AUTH_TOKEN_MINUTES` plus 30 s (10 min 30 s by default; Status shows it).
The panel says so after every grant, removal or role edit. To stop someone at
once, also end their sessions (or block the account): this worker refuses
them immediately; other workers accept an already-issued token until it
expires ([PERMISSIONS.md](PERMISSIONS.md#freshness--how-long-a-change-takes)).

## Audit log

`auth_events` holds sign-ins and refusals (password, providers, Access),
second-factor events and every administrative change: blocked,
unblocked, deleted, sessions ended, roles granted or taken away, group
memberships, roles created, changed or deleted, invitations sent or
withdrawn, reset links made (never the link), service tokens made or revoked,
and a new signing key in use. Addresses and names only — never a password, a
token, a link or an IP address. Failed password logins stay in the lockout
counters, not the log (an attacker must not grow it one guess at a time).

**Retention:** `AUTH_AUDIT_RETENTION_DAYS` (default **365**). Every write
deletes events older than that, in the same batch — no cron; an idle site
keeps its last year until something happens.

**CSV export** uses the view's filter (at most 10 000 rows — narrow the dates
for more). A cell a spreadsheet would run as a formula (`=`, `+`, `-`, `@`, a
tab or CR first) gets a leading apostrophe; UTF-8 with a byte-order mark.

## Signing keys and their age

A JWK carries no date, so the worker records when it first signed with each
key id (`signing_keys`). Status shows that age and advises rotating at least
every **365 days** (README, *Rotating the key*); a key used before this
release counts from the first token after the upgrade.

## Languages, look, accessibility

- **Five languages** (English, Polski, हिन्दी, Deutsch, Français), the
  worker's own terms; the language starts as the page's (`?lang=` on the sign-in
  pages, the cookie, `Accept-Language`) and the panel's own choice is
  remembered in the browser. Light, dark or the system's choice.
- **Look:** the worker's design tokens (`public/auth/assets/tokens.css` — the
  newsletter panel's, byte for byte) and `auth.css`, plus the panel's
  `admin/css/admin.css` (shell) and `admin/css/views.css` (views). **Every
  colour is a token**; no colour is written in the panel's CSS (a test
  refuses one). Links use `--link` when the site theme defines it, the
  primary colour otherwise. The colour guidelines are the tokens file's
  header and, site-wide, [`workers/core/README.md`](../../core/README.md).
- **The shared bar:** with the `core` worker on the host, `/core/nav.js` draws
  the site's admin bar (`<ssg-admin-nav current="auth">`) and `/core/theme.css`
  (after the panel's tokens) brings the site's colours; when the bar shows its
  own sign-out (`data-signout`) the panel hides its "Signed in as / Sign out".
  Without `core` both answer 404 and nothing changes.
- **WCAG 2.2 AA:** one `h1` per view, focused after navigation; landmarks and
  labelled sections; tables with captions and row/column headers; a table
  wider than the screen scrolls inside its own focusable region with a
  "scroll sideways" hint — the page never scrolls sideways at 360 px; targets
  of 40–44 px; visible focus; status messages in a live region; chips say
  their state in words (the colour only repeats it); text 4.5:1 and control
  edges 3:1 in both themes (`test/admin-contrast.test.ts` computes them).
- **CSP:** the pages' own (`default-src 'none'; script-src 'self'; style-src
  'self' …`) plus `connect-src 'self'` for the API; no inline script or style,
  nothing from another host. Meta tags complete (`noindex, nofollow,
  noarchive`); no tag manager and no analytics — an admin tool is not measured.

## Files

| Where | What |
|---|---|
| `functions/auth/admin/index.ts` → `functions/api/auth/_admin-shell.ts` | the shell: session, permission, CSP, the JSON data block |
| `functions/api/auth/admin/{users,sessions,events,matrix,status}…` | the routes added for the panel |
| `functions/api/auth/_admin-*.ts` | their data layer: users, sessions, events, keys, status, summary, what is sensitive |
| `public/auth/assets/admin/js/core/` | fetch wrapper, router, strings, formatting, preferences, DOM builder |
| `public/auth/assets/admin/js/models/` | the logic: access, step-up, permissions, matrix, filters, status, audit vocabulary (tested, 100 %) |
| `public/auth/assets/admin/js/views/`, `main.js` | DOM glue, checked in the browser |
| `public/auth/assets/admin/js/strings/` | the five languages |

## Previews and checks

```sh
node --import ./workers/auth/test/resolve.mjs workers/auth/test/dev-server.mjs
# → http://localhost:8788/auth/admin/ — owner@example.com, "demo password 1"
node workers/auth/test/admin-shots.mjs <out-dir> [axe.min.js]
MFA=1 STEP_UP_MINUTES=1 node --import ./workers/auth/test/resolve.mjs workers/auth/test/dev-server.mjs &
node workers/auth/test/admin-walk.mjs <out-dir>
```

The dev server adds invented data (`test/admin-demo-seed.ts`): about forty
people, groups, built-in and custom roles, open invitations, service accounts
with tokens, sessions started with a password, providers and Access — some
with a second factor —, ninety days of audit events and an Access mapping.
`admin-shots.mjs` captures every view light and dark at 1440×900 and 390×844,
runs axe-core (WCAG 2.0–2.2 A/AA) on each, checks that nothing scrolls the
page sideways and logs CSP refusals. `admin-walk.mjs` uses the keyboard alone:
sign in with the password and the app code, invite someone with a role and
get the one-time link, grant a role, end a session after the step-up window —
the dialog, the step-up page, the way back with nothing done, the action
repeated.
