# auth — HTTP API

Every route of the `auth` worker, with its request and answer. The guide is
the [README](../README.md); the pages people use are in [PAGES.md](PAGES.md),
the permission model in [PERMISSIONS.md](PERMISSIONS.md), the security
decisions in [SECURITY.md](SECURITY.md), and what another worker needs to
accept these sessions in [CONTRACT.md](CONTRACT.md). Service accounts, their
tokens and `POST /api/auth/token` are in
[SERVICE-TOKENS.md](SERVICE-TOKENS.md#admin-api).

## Sessions and passwords

Every `POST` needs the CSRF defence (see [CSRF](SECURITY.md#csrf)); JSON clients send
`x-ssg-auth: 1`. Errors are `{"error": "<code>"}`.

| Method + path | Body | Success | Failure |
|---|---|---|---|
| `POST /api/auth/login` | `{email, password, next?, "cf-turnstile-response"?}` or a form | `200 {user, expiresAt, sessionExpiresAt}` + cookies; a form gets `303` to `next` | `401 invalid_credentials`, `429 too_many_attempts` (+ `retry-after`), `403 turnstile_required`, `400 invalid_request`, `404 method_disabled` (`AUTH_PASSWORD_LOGIN=off`); a form gets `303` to `AUTH_LOGIN_URL?error=<code>&next=…` |
| `POST /api/auth/refresh` | — | `200 {user, expiresAt, sessionExpiresAt}` + new cookies | `401 session_ended` (cookies cleared) |
| `POST /api/auth/logout` | — or a form (`next?`) | `200 {ok: true}`, cookies cleared; a form gets `303` | — |
| `POST /api/auth/logout-all` | — or a form (`next?`) | `200 {ok: true, revoked: n}` | `401 not_authenticated` |
| `POST /api/auth/password` | `{current, password}` or a form (`next?`) | `200 {ok: true, revoked: n}` (other sessions ended); a form gets `303 next?ok=password_changed` | `401 not_authenticated` / `invalid_credentials`, `400 <rule>`, `429` |
| `POST /api/auth/bootstrap` | `{token, email, password, name?}` | `201 {user, …}` + cookies | `404 not_found`, `401 invalid_token`, `400 invalid_email` / `<rule>`, `429` |
| `GET /api/auth/session` | — | `200 {user, expiresAt, sessionExpiresAt}` | `401 not_authenticated` |
| `GET /api/auth/jwks` | — | `200 {keys: [JWK…]}`, `cache-control: public, max-age=300` | `503` |

Login, refresh and bootstrap can also answer `500 {error:
"permissions_too_large", detail}` — see [The token ceiling](PERMISSIONS.md#the-token-ceiling).
The permission and admin routes are in [Admin API](#admin-api).

`user` is `{id, email, name, mustChangePassword}`; `expiresAt` is the access
token's expiry (Unix seconds), `sessionExpiresAt` the session's (ISO 8601).
Password rules: `password_required`, `password_too_short`, `password_too_long`,
`password_equals_email`. Without `AUTH_DB` or a usable `AUTH_SIGNING_KEY`
every route answers `503 {error: "not_configured", hint}` — the worker never
signs with a default key or one generated at runtime.

**No account enumeration.** An unknown address, a wrong password, a blocked
account and an account without a password get the same status and body, and
the same hashing work: when there is no hash to check, a dummy derivation runs
at the configured cost.

## Invitations, reset links, your own sessions

| Method + path | Needs | Body | Success | Failure |
|---|---|---|---|---|
| `GET /api/auth/admin/invitations` | `auth:invite` | — | `{items: [invitation], nextCursor: null}` | |
| `POST /api/auth/admin/invitations` | `auth:invite` | `{email, name?, roleIds?, groupIds?, lang?, delivery?}` | `201 {invitation, delivery: "mail"}` or `{invitation, delivery: "link", link}` | `403 escalation {permissions}`, `409 account_exists`, `404 role_not_found` / `group_not_found`, `409 mail_not_configured`, `503 mail_misconfigured {detail}`, `502 mail_failed {detail, temporary}`, `400 invalid_request` |
| `DELETE /api/auth/admin/invitations/:id` | `auth:invite` | — | `{ok: true}` (again: still `200`) | `409 invitation_accepted`, `404 invitation_not_found` |
| `POST /api/auth/admin/invitations/:id/resend` | `auth:invite` | `{delivery?}` or none | `{invitation, delivery, link?}` — a new link, the old one dead | `409 invitation_closed` / `account_exists`, `403 escalation`, `404`, `502 mail_failed` |
| `POST /api/auth/admin/users/:id/reset-link` | `auth:admin` | `{delivery?, lang?}` or none | `201 {expiresAt, delivery: "mail"}` or `{expiresAt, delivery: "link", link}` | `403 escalation` (the account holds more than you), `404 user_not_found`, mail errors as above |
| `POST /api/auth/invite` | the token | `{token, method?: "password", password, name?}` or a form | `201 {user, expiresAt, sessionExpiresAt, skipped: {roles, groups}}` + cookies; a form gets `303 /auth/?welcome=1(&skipped=n)` | `410 invitation_invalid`, `400 unsupported_method` / `<password rule>`, `409 account_exists`, `503 hashing_unavailable`; a form gets `303 /auth/invite?token=…&error=<code>` |
| `POST /api/auth/reset` | the token | `{token, password}` or a form | `200 {ok: true, revoked: n}`; a form gets `303 AUTH_LOGIN_URL?ok=password_reset` | `410 reset_invalid`, `400 <password rule>`, `503 hashing_unavailable`; a form gets `303 /auth/reset?token=…&error=<code>` |
| `POST /api/auth/forgot` | — (only with mail) | `{email}` or a form | `200 {ok: true}` for every address; a form gets `303 /auth/forgot?sent=1` | `404 not_found` without a mail transport, `503 mail_misconfigured`, `429 too_many_requests` (+ `retry-after`), `400 invalid_email` |
| `GET /api/auth/sessions` | a session | — | `{items: [{id, createdAt, lastUsedAt, expiresAt, current}], nextCursor: null}` | `401 not_authenticated` |
| `DELETE /api/auth/sessions/:id` | a session | — | `{ok: true, current}` (`current: true` also clears the cookies) | `404 session_not_found` (anyone else's, ended or unknown), `401` |
| `POST /api/auth/sessions/:id` | a session | a form (`next?`) | `303 next?ok=session_ended`, or `303 AUTH_LOGIN_URL?ok=signed_out` for this one | `303 next?error=session_not_found` / `not_authenticated` |
| `GET /api/auth/resume?next=` | the refresh cookie | — | `303 next` with new cookies | `303 AUTH_LOGIN_URL?next=…(&error=session_ended)` |

An **invitation** is `{id, email, name, roleIds, groupIds, lang, status,
invitedBy, invitedByEmail, createdAt, expiresAt, acceptedAt, acceptedUserId,
revokedAt, revokedBy, revokeReason}`; `status` is `pending`, `accepted`,
`expired` or `revoked` (`revokeReason` `revoked` or `replaced` — inviting an
address again replaces its open invitation). The inviter cannot hand out more
than they hold: the union of the roles' permissions and the groups' roles must
be covered by the caller's own (`*` covers everything); resending checks the
one resending again. At acceptance the recorded roles and groups are checked
again: one deleted meanwhile is skipped and listed in `skipped`, never granted.

`delivery`: absent = by mail when a transport is configured, the link
otherwise; `"link"` = always the link (shown once, never stored or logged);
`"mail"` = mail or `409 mail_not_configured`. A transport that can never send
(no `AUTH_MAIL_FROM`, SMTP on port 25, a user without a password) is `503
mail_misconfigured` up front. `lang` (`en`, `pl`, `hi`, `de`, `fr`) is the
language of the mail and of the page the link opens; default: the caller's
page language (`ssg-auth-lang` cookie, then `Accept-Language`).

`resume` exists for the pages: they see only the access cookie, while the
refresh cookie is scoped to `/api/auth/`. A page without a valid access token
sends the browser to `resume`, which rotates the refresh token like `POST
/api/auth/refresh` and sends it back, or on to the login page. The refresh
cookie is `SameSite=Strict`, and a request marked `Sec-Fetch-Site` other than
`same-origin` / `none` is sent to the login page without touching anything.

## Login providers and sign-in methods

How to switch providers on, and what each one needs, is in
[PROVIDERS.md](PROVIDERS.md). `<provider>` is `google`, `microsoft`, `apple`,
`github` or `facebook`; a provider without its variables answers `404
not_found` on every route below (as if absent), one with wrong variables `503
provider_misconfigured {hint}`.

| Method + path | Needs | Body | Success | Failure |
|---|---|---|---|---|
| `POST /api/auth/oauth/<provider>/start` | — (`link`: a session from the last `AUTH_RECENT_SIGNIN_MINUTES`) | a form, or JSON with `x-ssg-auth: 1`: `{purpose?: "signin" \| "invite" \| "link", next?, token?}` | a form gets `303` to the provider + the flow cookie `__Host-ssg-auth-flow-<provider>`; JSON `200 {authorizationUrl}` + the cookie | `400 invalid_request` (purpose), `410 invitation_invalid`, `401 not_authenticated`, `403 reauth_required`, `409 provider_already_linked`; a form gets `303` to the page that explains |
| `GET /api/auth/oauth/<provider>/callback` | the flow cookie | the provider's `code`, `state` (or `error`) | `303` to `next` / `AUTH_AFTER_LOGIN_URL` (sign-in), `/auth/?welcome=1` (invitation), `/auth/account?ok=method_linked` (link), with session cookies; the flow cookie cleared | `303 AUTH_LOGIN_URL?error=<code>` (`/auth/account?error=<code>` for a link): `oauth_state_invalid`, `oauth_cancelled`, `provider_failed`, `not_invited`, `email_unverified`, `domain_not_allowed`, `identity_in_use`, `provider_already_linked`, `invitation_invalid`, `account_exists`, `account_blocked`, `reauth_required`, `permissions_too_large` |
| `POST /api/auth/oauth/apple/callback` | — | Apple's form post | `303` to the `GET` above with the same fields (no check here: the cookie is not sent cross-site) | `404` for a provider that does not post |
| `GET /api/auth/methods` | a session | — | `{password, identities: [{id, provider, name, email, linkedAt, lastUsedAt}], available: [{id, name}], sessionMethod, recentSignIn, recentMinutes}` | `401 not_authenticated` |
| `POST /api/auth/methods/set-password` | a recent session | `{password}` or a form | `200 {ok: true}`; a form `303 /auth/account?ok=password_set` | `409 password_exists`, `404 method_disabled`, `400 <password rule>`, `403 reauth_required {minutes}`, `401`, `503 hashing_unavailable` |
| `DELETE /api/auth/methods/:id` | a recent session | — | `200 {ok: true}` | `409 method_last`, `404 method_not_found` (anyone else's, unknown, removed), `403 reauth_required`, `401` |
| `POST /api/auth/methods/:id` | a recent session | a form | `303 /auth/account?ok=method_removed` | `303 /auth/account?error=<code>`; not signed in / not recently: `303` to the login page (`reauth=1`) |

`:id` is `password` or a linked identity's `id`. `email` of an identity is the
address the provider showed when it was linked — shown, never used to decide
anything. Every route that needs the database answers `503 no_sign_in_method
{hint}` when `AUTH_PASSWORD_LOGIN=off` and no provider is configured.

**Audit trail.** Every sign-in (any method), refused provider sign-in, link,
unlink, first password and provider invitation acceptance is a row in
`auth_events {id, at, kind, user_id, method, detail}` — `kind`: `signin`,
`signin_refused`, `method_linked`, `method_unlinked`, `password_set`,
`invitation_accepted` (its `detail` records `invitedEmail`, `providerEmail`,
`emailVerified` and `mismatch`). Written by one function, `emitEvent`
(`functions/api/auth/_audit.ts`); never a token, a code or a secret. Failed
password logins stay in the lockout counters only.

## Cloudflare Access

How to switch it on, which path goes behind Access and who gets in:
[ACCESS.md](ACCESS.md). Without `AUTH_ACCESS_TEAM` + `AUTH_ACCESS_AUD` the two
page routes answer `404 not_found` (as if absent); half or wrongly configured
— or `AUTH_ACCESS_AUTO_CREATE=on` with a default role that could make an
administrator — `503 access_misconfigured {hint}`.

| Method + path | Needs | Body | Success | Failure |
|---|---|---|---|---|
| `GET /auth/access?next=` | Access's token (header `Cf-Access-Jwt-Assertion` or cookie `CF_Authorization`) — the path sits behind Access | — | `303` to `next` / `AUTH_AFTER_LOGIN_URL` (`/auth/?welcome=1` after accepting an invitation) with session cookies — or to `/auth/mfa` when the account has a second factor | `303 AUTH_LOGIN_URL?error=<code>`: `access_failed`, `access_unavailable`, `not_invited`, `access_not_linked`, `access_email_unusable`, `domain_not_allowed`, `account_blocked`, `account_exists`, `invitation_invalid`, `identity_in_use`, `provider_already_linked`, `permissions_too_large` |
| `GET /auth/access/link` | a recent session + Access's token | — | `200` a confirmation page ("Cloudflare Access says you are …") | `303 /auth/account?error=<code>` (`access_failed`, `access_unavailable`, `domain_not_allowed`, `identity_in_use`, `provider_already_linked`); not signed in: through `/api/auth/resume`; not recently: `303 AUTH_LOGIN_URL?reauth=1` (or the step-up page for an account with a second factor) |
| `POST /auth/access/link` | the same, a plain form (CSRF) | — | `303 /auth/account?ok=method_linked` | as above; not signed in: `303 AUTH_LOGIN_URL?next=/auth/account` |
| `GET /api/auth/admin/access-groups` | `auth:admin` | — | `{items: [mapping], nextCursor: null}` | |
| `POST /api/auth/admin/access-groups` | `auth:admin` | `{accessGroup, groupId}` | `201 {mapping, resynced}` | `400 invalid_request`, `404 group_not_found`, `403 escalation {permissions}`, `409 mapping_exists` |
| `DELETE /api/auth/admin/access-groups/:id` | `auth:admin` | — | `{ok: true, resynced}` | `404 mapping_not_found` |

A **mapping** is `{id, accessGroup, groupId, groupName, createdBy, createdAt}`
(`groupName` null once the group is deleted — the mapping then does nothing).
`resynced`: how many people whose groups an Access sign-in recorded had a
membership change when the mapping was applied to them at once; permissions
follow at their next token. `POST /api/auth/logout` answers `{ok: true,
accessLogoutUrl}` (a form: `303` to that URL) for a session started through
Access. `GET /api/auth/methods` lists `{id: "access", name: "Cloudflare
Access"}` under `available` while Access is on and not linked; a linked Access
identity is an ordinary identity (`provider: "access"`), removed with
`DELETE /api/auth/methods/:id` like any other (`409 method_last` when it is
the only way in). Audit kinds added: `account_created` (auto-provisioning) and
`groups_synced` (`detail`: `via` signin|mapping, `added`, `removed`,
`keptForOwner`); links by address carry `via: "email"`.

## Second factor

How it works, the policy and recovery: [SECOND-FACTOR.md](SECOND-FACTOR.md).
While `AUTH_MFA_KEY` is unset every route below answers `404 not_found` (as if
absent). POSTs need the CSRF defence (`x-ssg-auth: 1`, or a same-origin plain
form where "form" is noted). "fresh" = an account with a factor needs one from
the last `AUTH_STEP_UP_MINUTES` (else `403 step_up_required {stepUpUrl,
minutes}`, a form → `303 /auth/mfa/step-up?next=…`); one without needs a
sign-in from the last `AUTH_RECENT_SIGNIN_MINUTES` (else `403
reauth_required`, a form → the login page with `reauth=1`).

`POST /api/auth/login` (and every other first factor) for an account with a
factor answers `401 {error: "second_factor_required", factors: ["totp" |
"passkey", "backup"], challengeUrl: "/auth/mfa", expiresAt}` with only the
`__Host-ssg-auth-mfa` cookie (5 minutes, single use; a form: `303 /auth/mfa`).
A success may add `enrol: true` (a role needs a factor the account has not set
up; a form goes to `/auth/mfa/enrol?required=1&next=…`); `POST
/api/auth/invite` likewise.

| Method + path | Needs | Body | Success | Failure |
|---|---|---|---|---|
| `POST /api/auth/mfa/verify` | the pending cookie; form | `{code}` \| `{backupCode}` \| `{passkey: <assertion>}` | `200 {user, expiresAt, sessionExpiresAt, next}` + session cookies (form: `303` to the sign-in's `next`) | `401 mfa_pending_invalid` (form: `303 AUTH_LOGIN_URL?error=mfa_expired`), `401 invalid_code` (form: `303 /auth/mfa?error=…`), `429 too_many_attempts` + `retry-after`, `503 mfa_unavailable`, `500 permissions_too_large` |
| `POST /api/auth/mfa/break-glass` | the pending cookie of an owner; `AUTH_MFA_RESET_TOKEN`; form | `{token}` | `200 {user, expiresAt, sessionExpiresAt, enrol: true}` (form: `303 /auth/mfa/enrol?required=1&next=…`) | `404 not_found` (unset or used), `401 mfa_pending_invalid` \| `invalid_token`, `403 not_owner`, `429` |
| `POST /api/auth/mfa/step-up` | a session; form | as verify, + `next` | `200 {ok, verifiedAt, minutes}` + a new access cookie (form: `303 next`) | `409 no_factor`, `401 invalid_code`, `429`, `503` |
| `POST /api/auth/mfa/totp/start` | a session, fresh; form | `{next?}` | `201 {factorId, secret, uri, qrSvg}` (form: `303 /auth/mfa/enrol?factor=…`) | `503 mfa_unavailable` |
| `POST /api/auth/mfa/totp/confirm` | a session, fresh; form | `{factorId, code, name?, next?}` | `200 {factor, backupCodes?}` + a new access cookie (form: the backup-codes page, or `303 next?ok=factor_added`) | `400 invalid_code`, `410 enrolment_expired`, `503` |
| `POST /api/auth/mfa/passkey/options` | `register`: a session, fresh; `signin`: the pending cookie; `step-up`: a session | `{purpose}` | `200 {publicKey}` (binary fields base64url) | `400 invalid_request`, `404 no_passkey`, `401` |
| `POST /api/auth/mfa/passkey/register` | a session, fresh; form | `{credential, name?, next?}` (form: `credential` as JSON text) | `201 {factor, backupCodes?}` (form: as confirm) | `400 invalid_passkey` |
| `GET /api/auth/mfa/factors` | a session | — | `{enabled, state, factors: [{id, kind, name, createdAt, lastUsedAt}], backupCodesLeft, requiredBy, sessionFactor, stepUpMinutes}` | |
| `POST /api/auth/mfa/backup-codes` | a session, fresh; form | `{next?}` | `200 {backupCodes}` (form: the backup-codes page) | `409 no_factor` |
| `DELETE /api/auth/mfa/factors/:id` (`POST` for a form) | a session, fresh | — | `{ok: true}` (form: `303 next?ok=factor_removed`) | `404 factor_not_found`, `409 factor_required {roles}` (the last one, while a role needs it) |
| `POST /api/auth/mfa/reset` | `*`, fresh | `{userId}` | `{ok: true, removed, revoked}` | `400 own_account`, `403 forbidden {permission: "*"}`, `404 user_not_found` |

`GET /api/auth/me/permissions` adds `secondFactor: {roles, hasFactor, url}`
when roles are left out of this session for want of a second factor. Step-up
also guards `POST /api/auth/admin/grants` (a role holding `*` or
`auth:*`), `POST …/groups/:id/members` (a group holding either), `POST
/api/auth/admin/invitations` (carrying either), `PUT …/roles/:id` (adding
either), `POST …/users/:id/revoke-sessions` (another account), `POST
…/service-accounts/:id/tokens` (with `allowSensitive`), `/api/auth/methods/*`,
the provider `link` start, and the admin panel's routes marked "step-up" below. Audit kinds added: `mfa_enrolled`,
`mfa_removed`, `mfa_passed`, `mfa_failed`, `mfa_backup_code_used`,
`mfa_backup_codes_regenerated`, `mfa_reset`; `signin` carries `secondFactor`.

## Admin API

Every route answers `401 {error: "not_authenticated"}` without a valid access
token (and live session, active account), and `403 {error: "forbidden",
permission}` without the permission named below, checked with `can` against the
token. Every non-GET route needs the [CSRF](SECURITY.md#csrf) defence (`x-ssg-auth: 1`,
JSON only). Lists are `{items, nextCursor: null}` (no paging yet). Errors:
`400 invalid_request {detail}`, `404 <thing>_not_found`, `409 name_taken`,
`409 owner_required`.

**Escalation.** An administrator cannot hand out a permission they do not hold
— unless they hold `*`. That covers creating a role, adding a permission to a
role, granting a role, and adding someone to a group (which hands them the
group's roles): `403 {error: "escalation", permissions: [the ones missing]}`.
Only a `*` holder can make someone an owner.

| Method + path | Needs | Body | Success |
|---|---|---|---|
| `GET /api/auth/permissions` | a session | — | `{items: [{name, module, description, known}], nextCursor, problems}` |
| `GET /api/auth/me/permissions` | a session | — | `{userId, permissions: [{name, known, sources}], token: [perms in the current token]}` |
| `GET /api/auth/module` | `auth:admin` | — | the module manifest |
| `GET /api/auth/admin/roles` | `auth:admin` | — | `{items: [role]}` |
| `POST /api/auth/admin/roles` | `auth:admin` | `{name, description?, permissions}` | `201 {role}` |
| `GET /api/auth/admin/roles/:id` | `auth:admin` | — | `{role}` |
| `PUT /api/auth/admin/roles/:id` | `auth:admin` | `{name?, description?, permissions?}` | `{role}`; `409 built_in_role` on renaming a built-in |
| `DELETE /api/auth/admin/roles/:id` | `auth:admin` | — | `{ok: true}` (its grants go too) |
| `GET /api/auth/admin/groups` | `auth:admin` | — | `{items: [group]}` |
| `POST /api/auth/admin/groups` | `auth:admin` | `{name, description?}` | `201 {group}` |
| `GET /api/auth/admin/groups/:id` | `auth:admin` | — | `{group, members: [member], grants: [grant]}` |
| `PUT /api/auth/admin/groups/:id` | `auth:admin` | `{name?, description?}` | `{group}` |
| `DELETE /api/auth/admin/groups/:id` | `auth:admin` | — | `{ok: true}` (members and grants go too) |
| `POST /api/auth/admin/groups/:id/members` | `auth:admin` | `{userId}` | `201 {ok, added: true}`, `200 {added: false}` if already in |
| `DELETE /api/auth/admin/groups/:id/members/:userId` | `auth:admin` | — | `{ok: true}` |
| `GET /api/auth/admin/grants?roleId=&userId=&groupId=` | `auth:admin` | — | `{items: [grant]}` |
| `POST /api/auth/admin/grants` | `auth:admin` | `{roleId, userId}` or `{roleId, groupId}` — `userId` may be a service account's id | `201 {grant}`; `409 grant_exists` |
| `DELETE /api/auth/admin/grants/:id` | `auth:admin` | — | `{ok: true}` |
| `GET /api/auth/admin/users/:id/permissions` | `auth:admin` | — | `{userId, permissions: [{name, known, sources}]}` |
| `POST /api/auth/admin/users/:id/revoke-sessions` | `auth:admin` | — | `{ok: true, revoked: n}` |

Shapes: a **role** is `{id, name, description, builtIn, permissions,
unknownPermissions, createdAt, updatedAt}` (`name` a slug `[a-z][a-z0-9-]*`, ≤
64; at most 64 permissions); a **group** `{id, name, description, memberCount,
createdAt, updatedAt}` (`name` 1–100 characters); a **member** `{userId, email,
name, status, addedBy, addedAt}`; a **grant** `{id, roleId, roleName, userId,
groupId, grantedBy, grantedAt}` (`grantedBy` null = the system); a **source**
`{roleId, roleName, via: "user"}` or `{roleId, roleName, via: "group",
groupId, groupName}`.

## The admin panel's routes

What the panel at `/auth/admin/` ([PANEL.md](PANEL.md)) stands on, next to the
routes above. All need `auth:admin` (401 / 403 as above, CSRF on non-GET).
"Step-up" = `403 step_up_required {stepUpUrl, minutes}` while the caller's
account has a second factor not passed in the last `AUTH_STEP_UP_MINUTES`.
Security review (SR-04, SR-05): every route that takes `*` or `auth:*` away
(DELETE a grant, a group member, a group, a role, an Access mapping; PUT a
role without one it held) answers `403 escalation` unless the caller holds
it, and asks for a step-up; so do the routes that end the sessions of,
disable, delete or revoke the tokens of an account holding `*` / `auth:*`
(escalation only). `POST …/users/:id/reset-link` for another account,
`POST …/invitations/:id/resend` of an invitation carrying `*` / `auth:*`, and
`POST …/access-groups` onto a group holding one need a step-up.

| Method + path | Body | Success | Failure |
|---|---|---|---|
| `GET /api/auth/admin/users?q=&status=&roleId=&groupId=&cursor=&limit=` | — | `{items: [user], nextCursor}` — by address; `nextCursor` is the last address (null at the end); `q` part of the address or name; `roleId` held directly or through a group; `limit` 1–200 (50) | `400 invalid_request` |
| `GET /api/auth/admin/users/:id` | — | `{user, methods, secondFactor, groups, grants, permissions, sessions, events}` | `404 user_not_found` |
| `POST /api/auth/admin/users/:id/status` | `{status: "blocked" \| "active"}` | `{ok, status, revoked}` — blocking ends every session at once | `400 own_account`, `403 escalation`, step-up (blocking an owner), `409 owner_required`, `404` |
| `DELETE /api/auth/admin/users/:id` | — | `{ok: true}` — the account, its grants, memberships, sessions and sign-in methods; audit events stay | step-up (always), `400 own_account`, `403 escalation`, `409 owner_required`, `404` |
| `GET /api/auth/admin/matrix?…` (as users) | — | `{items: [{id, email, name, status, permissions}], nextCursor}` — effective permissions per account | `400` |
| `GET /api/auth/admin/sessions?userId=&cursor=&limit=` | — | `{items: [session], nextCursor}` — live sessions, newest use first; `limit` 1–200 (100) | `400` |
| `DELETE /api/auth/admin/sessions/:id` | — | `{ok: true, current}` | step-up (another account's), `403 escalation` (the account holds `*` / `auth:*` the caller does not), `404 session_not_found` |
| `POST /api/auth/admin/sessions/revoke-all` | `{confirm: "revoke-all"}` | `{ok: true, revoked}` — every session but the caller's | step-up (always), `400 invalid_request` |
| `GET /api/auth/admin/events?kind=&user=&from=&to=&cursor=` | — | `{items: [event], nextCursor, retentionDays}` — newest first, 100 a page | `400 invalid_request` |
| `GET /api/auth/admin/events/export?kind=&user=&from=&to=` | — | `text/csv` attachment, at most 10 000 rows | `400` |
| `GET /api/auth/admin/status` | — | `{keys, rotationDays, tokens, hashing, signIn, access, mfa, mail, audit, invitations, modules, registryProblems}` — never a secret's value | |
| `GET /api/auth/module/summary` | — | the module summary ([format](../../module-summary.schema.json)): accounts, open invitations, live sessions, accounts missing a needed second factor; an old signing key; configuration problems | |

Shapes: a **user** (list) is `{id, email, name, status, createdAt,
lastLoginAt, mustChangePassword, hasPassword, owner}`; the person page adds
`methods: {password, identities: [{id, provider, email, linkedAt,
lastUsedAt}]}`, `secondFactor: {enabled, factors: [{id, kind, name,
createdAt, lastUsedAt}], backupCodesLeft}` (never a secret),
`groups: [{id, name, addedAt}]`, `grants` (direct), `permissions` (with
sources, as `/admin/users/:id/permissions`), `sessions` and the last 20
`events`. A **session** is `{id, userId, email, name, method, secondFactor,
createdAt, lastUsedAt, expiresAt}` (`method` how it started, `secondFactor`
`totp` \| `backup` \| `passkey` \| null) — never a token or a hash. An
**event** is `{id, at, kind, userId, email, by, byEmail, method, detail}`:
`user` in the filter matches the account the event is about or the
administrator who acted (`by`); `from` is inclusive, `to` exclusive (ISO 8601
dates or times). The CSV columns are `at, kind, account_id, account_email,
by_id, by_email, method, detail`; a cell starting with `= + - @`, a tab or CR
gets a leading apostrophe; UTF-8 with a byte-order mark, CRLF.

**Audit kinds of the admin panel:** `account_blocked`, `account_unblocked`,
`account_deleted` (`detail.email`), `sessions_revoked` (`scope` account \|
everyone, `revoked`), `session_revoked`, `grant_added` / `grant_removed`
(`role`, `group`), `member_added` / `member_removed`, `role_created` /
`role_changed` / `role_deleted`, `group_deleted` (`group`),
`access_mapping_added` / `access_mapping_removed` (`accessGroup`, `group`),
`invitation_sent` (`email`, `delivery`) /
`invitation_revoked`, `reset_link_issued` (`delivery`, never the link),
`service_token_created` / `service_token_revoked` (the token's public id),
`signing_key_seen` (`kid` — a new signing key in use). `detail.by` is the
administrator. Every write also deletes events older than
`AUTH_AUDIT_RETENTION_DAYS` (365).
