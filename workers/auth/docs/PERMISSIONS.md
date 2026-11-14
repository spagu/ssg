# auth — permissions

Roles, groups, grants, what ends up in the token, and how fresh it is. The
guide is the [README](../README.md); the routes that manage all this are in
[API.md](API.md#admin-api).

A permission is `worker:action`, lower case — `newsletter:send`,
`comments:moderate`, `auth:admin`. A **role** is a named set of permissions,
and it may also hold a trailing wildcard: `newsletter:*` (every action of one
worker) or `*` (everything). Nothing else is a pattern — `*:erase`,
`newsletter:*x`, upper case, spaces and empty parts are refused. A role is
**granted** to a user or to a **group** (a set of users). The model is flat on
purpose: roles never inherit from roles and groups never contain groups, so you
can work out anyone's rights by hand.

**Effective permissions** of a user = the union of the permissions of every
role granted to the user directly or to a group the user belongs to,
de-duplicated and sorted. Wildcards stay wildcards (`*` is not expanded into a
list). That list is the token's `perms`. A worker asks one question, the pure
function `can(perms, required)` in `functions/api/auth/_can.ts`: it allows
`required` only when `perms` holds exactly that name, its `worker:*`, or `*`.

**Narrowed by the way in**. With `AUTH_ROLE_METHODS` — e.g.
`{"owner": ["google", "github"]}` — a listed role counts only in sessions
started with one of its methods (`password` or a login provider's id): its
permissions are left out of the token of any other session, unless another
role of the account gives them too. Applied at every login and every refresh
(the session remembers how it started, `session_methods`); "Your panels" and
the account page name the roles left out and the methods they need. A value
that is not such a JSON object grants nothing from any role until it is fixed
— a broken policy never opens. Details: [PROVIDERS.md](PROVIDERS.md#narrowing).

## Two worked examples

**An editor who can send but not erase.** Grant Eva the built-in
`newsletter-editor` role (`newsletter:edit`, `newsletter:read`,
`newsletter:send`). Her token's `perms` is exactly that list:
`can(perms, "newsletter:send")` is true, `can(perms, "newsletter:erase")` is
false — erasing a person, importing, exporting and suppressions stay with
whoever holds those names. Had she been given `newsletter:*` instead, she
could erase too: a wildcard is a deliberate choice, never a default.

**A moderator limited to comments.** Create a group *Moderators*, grant it the
built-in `comments-moderator` role (`comments:moderate`, `comments:read`) and
add Max. His `perms` is `["comments:moderate", "comments:read"]`: he can read
the queue, approve and mark spam, but not delete, import, or touch anything of
the newsletter. Removing him from the group takes both away at his next
refresh; `GET /api/auth/admin/users/<id>/permissions` shows each permission
"via group Moderators, role comments-moderator".

## Built-in roles

Created once, with the schema; afterwards ordinary roles — their permissions
and descriptions can be edited, non-owner ones deleted — except that their
names never change.

| Role | Permissions | For |
|---|---|---|
| `owner` | `*` | everything, in every worker; the bootstrap account gets it |
| `newsletter-editor` | `newsletter:edit`, `newsletter:read`, `newsletter:send` | writes and sends campaigns; no import, export, erase or suppressions |
| `newsletter-viewer` | `newsletter:read` | looks, changes nothing |
| `comments-moderator` | `comments:moderate`, `comments:read` | approves and marks spam; no delete or import |

The names come from the `permissions` of `workers/newsletter-ui/module.json`
and `workers/comments/module.json`; a test fails if a built-in role names a
permission no manifest declares. This worker declares its own two:
`auth:admin` (roles, groups, grants, anyone's sessions) and `auth:invite`
(invitations).

## The owner invariant

There is always at least one **active** account holding `owner` — directly or
through a group — and `owner` keeps `*`. The database enforces it, not just the
API: triggers (`auth_owner_*` in `schema.sql`) undo any statement that would end
it — revoking the grant, removing the member, deleting the group, deleting the
role or taking `*` out of it, renaming it, blocking or deleting the account —
and the API answers `409 {error: "owner_required"}` with nothing changed. D1
runs one statement or batch at a time and a trigger runs inside it, so two
simultaneous removals of the last two owners cannot both succeed (tested).
Grant `owner` to a second person before removing the first. A service
account ([SERVICE-TOKENS.md](SERVICE-TOKENS.md)) holding `owner` never
counts: it has no `users` row, so it is never an active owner.

A database created by an older build of this worker (accounts, no roles) makes
its oldest active account the owner when it upgrades to schema version 2.

## Freshness — how long a change takes

Permissions travel in the access token, and every worker trusts the token until
it expires; refresh always reads the grants afresh. So a granted or revoked
role, a group membership or an edited role reaches an account at its **next
refresh** — at most `AUTH_TOKEN_MINUTES` (default **10 minutes**) plus the 30 s
clock leeway verifiers allow: **10 min 30 s** by default. This worker's own
admin API checks the token too, with the same bound.

For an immediate stop, `POST /api/auth/admin/users/<id>/revoke-sessions` ends
every session of the account: this worker refuses its tokens at once, its next
refresh fails, and it must log in again — getting the permissions as they are
now. Other workers still accept an already-issued token until it expires (the
same 10 min 30 s at most); blocking an account works the same way.

## The token ceiling

The access cookie must fit in a browser: RFC 6265 asks for at least 4096
bytes per cookie (name, value and attributes), and Chrome and Firefox stop at
4096 for name + value. This worker caps the whole `Set-Cookie` value at
**3800 bytes** (296 bytes of margin). Measured with the default issuer and
audience (`https://example.com/api/auth`, `example.com`) and a 29-character
address, that leaves room for about **104 permissions of 20 characters**
(`newsletter:settings` is 19) or **36 of the longest allowed, 64 characters**
(a test pins both). A long `AUTH_ISSUER` or address leaves fewer.

An account whose permissions do not fit gets no token: login answers `500
{error: "permissions_too_large", detail}` (a form login goes back with
`?error=permissions_too_large`), the session just made is ended, and the
worker logs `auth: no access token for account <id>: …` for the administrator.
A refresh answers the same but keeps the session alive (it sets the rotated
refresh cookie), so it works again as soon as the account's roles are
narrowed — fewer roles, or `worker:*` instead of long lists. The list is never
cut short.

## The registry of known permissions

Each worker declares its permissions in its `module.json`. A Pages Function
cannot read other templates' files, so this worker learns the names from:

1. its own manifest (`functions/api/auth/_module.ts`, the generated copy of
   `module.json`): `auth:admin`, `auth:invite`;
2. the var **`AUTH_KNOWN_PERMISSIONS`**: a JSON array of modules in the
   manifest's shape — `[{"id": "newsletter-ui", "permissions": [{"name":
   "newsletter:send", "description": {"en": "…"}}]}]` (a bare name and a
   plain-string description are accepted too). Print it from the manifests of
   the workers you run:

   ```sh
   node workers/auth/scripts/auth-permissions.mjs --base newsletter-ui=/newsletter-admin workers/newsletter-ui/module.json workers/comments/module.json
   ```

   English descriptions only by default — Cloudflare caps one variable at
   5 KB; `--all-languages` keeps all five. Each module's title and panel go
   along, so "Your panels" can name and link it; a standalone Worker's panel
   is relative to its own base path, which `--base <id>=<path>` supplies
   (without it the module is listed unlinked, with a note).

The build-time module registry that `core` reads is not a source for `auth`:
list other modules' permissions in `AUTH_KNOWN_PERMISSIONS`. The registry is advisory: a role may hold a name nobody declares (a
private worker); it is kept and flagged — `unknownPermissions` on a role,
`known: false` in the lists — never dropped. Malformed entries are skipped,
listed in `problems` and logged once.
