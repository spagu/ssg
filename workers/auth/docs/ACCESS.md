# auth — signing in through Cloudflare Access

> **Not tried against a live Access application.** This bridge was built and
> tested against a scripted fake of an Access team — its certs endpoint and
> tokens minted the way Access documents its application tokens
> (`test/access-fake.ts`) — not against a real Zero Trust account. The
> dashboard click-paths below are written as known at the time and **may have
> changed**; the paths, the variables and the checks the worker makes are what
> matter. Try it on a test host first, and decode one real token (it is a JWT)
> to see which claims your team actually sends.

With `AUTH_ACCESS_TEAM` and `AUTH_ACCESS_AUD` set, **Cloudflare Access becomes
one more way to sign in to `auth`**, next to the password and the login
providers. Access says *who* the person is (through the identity provider
your team uses); `auth` decides *what they may do*, from its own roles and
groups, and gives them the same session as any other sign-in.

## Plain Access mode or the bridge?

Every worker with a panel (`comments`, `newsletter`, `newsletter-ui`) can
already sit behind Access on its own (`<PREFIX>_ACCESS_TEAM` + `_ACCESS_AUD`).
That stays fully supported. The difference:

| | Plain Access mode in a worker | The bridge (this page) |
|---|---|---|
| Answers | "this person may enter this panel" | "this person may enter **and** may do exactly these actions" |
| Permissions | all or nothing per panel | per action (`newsletter:send`, `comments:moderate`, …) from roles and groups |
| Audit | the worker records the address, at most | every sign-in, refusal, link and group change in `auth_events` |
| Sessions | one Access session per application | one `auth` session across every panel on the host, revocable here |
| Needs | the worker only | the `auth` worker and its D1 database |

One person running one panel already behind Access: plain Access mode is
enough. Several people, different rights, one place to see who did what: the
bridge.

## How it works

1. The login page shows **Sign in with Cloudflare Access** — a plain link to
   `GET /auth/access?next=…`.
2. That path sits behind an Access application. Access signs the person in at
   the identity provider and forwards the request with its signed token
   (`Cf-Access-Jwt-Assertion` header; the `CF_Authorization` cookie is read
   too).
3. The worker verifies the token itself, so a request that reaches it some
   other way gets nothing: RS256 only; a key from **your team's** certs
   endpoint (the host comes from `AUTH_ACCESS_TEAM`, never from the request);
   `aud` must contain **this application's** AUD — a token Access issued for
   another application of the same team is refused; `iss` is your team;
   `exp` is **required** and not passed (`nbf`/`iat`, when present, not in
   the future beyond 60 s); `type`, when present, is `app`; `sub` is present.
4. It finds the account (below), synchronises mapped groups when the token
   carries groups, and starts the session through the same single entry every
   sign-in uses — so the second factor is asked for when the
   account has one. Access is never assumed to have checked a second factor.

The team's keys are cached per isolate for an hour. An unknown key id (Access
rotated its keys) fetches them again, but at most once a minute, so made-up
key ids cannot turn into a stream of requests. If the certs endpoint is down,
keys already held keep working for up to a day; with no usable key the
sign-in is refused (`access_unavailable`) — never accepted unchecked.

## Setting it up

In the Zero Trust dashboard, under **Access → Applications**:

1. **Add an application → Self-hosted.** Application domain: your site's host,
   path **`auth/access`**. That covers `/auth/access` (sign in) and
   `/auth/access/link` (connect Access to an existing account) and nothing
   else. Add a policy with action **Allow** naming who may use it — the people
   or groups who should be able to sign in this way.
2. **Do not** put `/auth/*` or `/api/auth/*` behind that application. The rest
   of `auth` must stay reachable without Access: the login page (for everyone
   who signs in another way), the provider callbacks
   (`/api/auth/oauth/<provider>/callback` — Apple answers with a cross-site
   POST that would never carry an Access cookie), the public keys
   (`/api/auth/jwks`), the token exchange for scripts (`/api/auth/token`),
   refresh, logout, invitations and reset links.
3. **If the whole host is already behind Access** (a staging site, say), add
   applications with one policy each — action **Bypass**, include
   **Everyone** — for the public parts, as the newsletter panel's README does
   for its hooks ("Putting the panel behind Cloudflare Access"): at least
   `api/auth/jwks`, `api/auth/token` and `api/auth/oauth`. Access applies the
   most specific path, so these get through while the rest stays protected.
   Each of them checks its own credentials. Without the bypass, a script's
   token exchange and a provider's answer get Access's login page instead.
4. **Copy the application's Application Audience (AUD) tag** from its
   overview, and your team name (the `<team>` in
   `<team>.cloudflareaccess.com`):

   ```toml
   [vars]
   AUTH_ACCESS_TEAM = "myteam"
   AUTH_ACCESS_AUD = "<the application's AUD tag>"
   ```

   Both or neither. One alone, a team that is not a team name, or an AUD that
   is not a tag: the login page shows no Access button and `/auth/access`
   answers `503 access_misconfigured` with what to fix.
5. Deploy. Check: `/auth/access` in a private window goes to your identity
   provider; `curl -i https://example.com/auth/access` is answered by Access,
   never by the worker; `/auth/login` still loads without Access.

## Who gets in

Only people this site knows. In order — the first that applies decides:

| Situation | Result |
|---|---|
| `AUTH_ACCESS_ALLOWED_DOMAINS` set and the address is outside it | refused (`domain_not_allowed`) |
| The Access identity (`sub`) is linked to an account here | signed in — unless the account is **blocked**: then refused (`account_blocked`) whatever Access says |
| The address cannot be matched (missing, or not plain ASCII — see below) | refused (`access_email_unusable`) |
| An existing account has exactly this address | **off by default**: refused (`access_not_linked`) — sign in another way and connect Access on the account page. With `AUTH_ACCESS_LINK_BY_EMAIL=on`: linked and signed in, audited |
| An open invitation for this address | accepted: the account with the invitation's roles and groups, Access linked |
| `AUTH_ACCESS_AUTO_CREATE=on` | a new account with `AUTH_ACCESS_DEFAULT_ROLE` only |
| Anything else | refused (`not_invited`); no account row is written |

Access asserts the address its identity provider verified, so the address is
used to find an invitation (as with Google's verified address). It is compared
after the worker's one normalisation — trimmed, lower-cased, nothing else —
and only in **printable ASCII**: lower-casing maps some non-ASCII letters onto
ASCII ones (the Kelvin sign `K` becomes `k`), and a Unicode domain is not the
same string as its punycode form, so such an address matches nothing. A
plus-tag (`kate+admin@`), a dot, a trailing dot after the domain all make a
**different** address. Mixed case is the same address.

`AUTH_ROLE_METHODS` names the method `access` like any other, e.g.
`{"owner": ["password"]}` keeps owner rights out of Access sessions.

### The two opt-ins, and why they are off

**`AUTH_ACCESS_LINK_BY_EMAIL=on`** lets an Access identity that is not linked
yet enter the existing account with the same address. That hands the keys of
**every account here** to whoever controls the identity provider behind your
Access team and its mapping of people to addresses: an IdP administrator, a
misconfigured SAML attribute, a one-time-PIN login to a mailbox someone can
read, or an IdP that lets users choose their own address can sign in as
anyone, including an owner. Off, the only way to attach Access to an existing
account is from that account's own page, after signing in another way and
recently — the same rule as connecting Google or GitHub. Turn it on only when
the IdP is the authority for addresses in your organisation anyway; each such
link is recorded (`method_linked`, `via: "email"`).

**`AUTH_ACCESS_AUTO_CREATE=on`** with `AUTH_ACCESS_DEFAULT_ROLE=<role name>`
gives an account to anyone your Access policy lets through who has neither an
account nor an invitation. It must never make an administrator: the method
refuses to start (`503 access_misconfigured`) while the default role is
`owner`, holds `*` or any wildcard, or holds a sensitive permission (`auth:…`,
any `…:erase` — the same list service tokens use). Without the role, or with a
role that does not exist, likewise. The account gets that one role, no group
and no password; its creation is recorded (`account_created`). Remember that
the role is read at every token: widening the role later widens every account
it was given to. Off (the default), Access opens nothing by itself — it is
still invitation only.

## Group mapping

Optional. When the token carries the person's groups, an administrator maps
an **Access group** (as your identity provider names it) to a **group here**:

```sh
curl -X POST https://example.com/api/auth/admin/access-groups \
  -H 'x-ssg-auth: 1' -H 'content-type: application/json' --cookie '…' \
  -d '{"accessGroup": "newsletter-editors", "groupId": "<group id>"}'
```

(`auth:admin`; like adding a member, you cannot map to a group whose roles you
do not hold yourself — [API.md](API.md#cloudflare-access-go-139).)

**Which claim.** The worker reads a top-level `groups` claim, or
`custom.groups` (an OIDC claim passed through by Access); an entry is a name,
or an object whose `id` and `name` both count. Whether your tokens carry
either **depends on the identity provider and how Access is set up for it**:
Access's application token does not carry groups by default; the groups Access
knows are offered by its identity endpoint, which this worker does not call.
With an OIDC provider you can usually add the groups claim to the claims
Access passes through (OIDC claims in the provider's settings in Zero Trust);
with SAML, an attribute. Decode a token and look before relying on it.

**At each Access sign-in**, the person's memberships in **mapped** groups are
made to match the token — before the session starts, so the first token
already reflects them:

- a mapped group the token names: the person is added, and the membership is
  marked as synchronised;
- a synchronised membership the token no longer earns: removed;
- a membership an administrator added by hand is **never** removed (and is
  not taken over by a mapping that names it too); groups nobody mapped are
  never looked at;
- a token **without** a groups claim changes nothing (an identity provider
  that stopped sending groups must not empty everyone's memberships); an
  **empty** list is a statement and removes the synchronised memberships;
- the owner invariant holds: a removal that would leave no active owner is
  skipped, logged and recorded (`groups_synced`, `keptForOwner`);
- each change is its own transaction, written so that two sign-ins at once
  cannot leave a membership without its mark or the reverse.

**When it shows.** A sign-in applies it at once. A change of the **mapping**
is applied right away to everyone whose groups an Access sign-in recorded
(their last token's groups), and shows in their permissions at their next
token — a refresh, at most `AUTH_TOKEN_MINUTES` later. A change of the
person's groups **at the identity provider** shows at their next Access
sign-in. Deleting a mapping removes the memberships it alone had added.

## Connecting and removing Access on the account page

A signed-in person sees **Connect Cloudflare Access** under "Add a way to sign
in". It opens `/auth/access/link` (behind Access), which shows "Cloudflare
Access says you are …" and asks to confirm; the confirmation is a form POST —
a GET never links anything. Like any change of sign-in methods it needs a
sign-in from the last `AUTH_RECENT_SIGNIN_MINUTES` (or, for an account with a
second factor, a recent step-up). Access then appears in "How you sign in"
and can be removed there — except when it is the account's only way in: an
Access identity is an ordinary row of `identities`, so the last-method rule of
the database refuses that removal (`method_last`), also for an account that
has only Access.

## Signing out

When the session was started through Access, `POST /api/auth/logout` clears
this worker's cookies and then sends the browser to the team's Access logout,
`https://<team>.cloudflareaccess.com/cdn-cgi/access/logout` — a form gets that
as its `303` (instead of `next`), JSON gets `{ok: true, accessLogoutUrl}`.
Without it, the next visit to `/auth/access` would sign straight back in. The
URL is built from `AUTH_ACCESS_TEAM` alone, never from the request, so it is
not an open redirect; the pages that carry the sign-out form allow that host
in their CSP `form-action`. Sessions started any other way sign out as before.
"Sign out everywhere" ends the sessions here and does not touch Access.

## When it goes wrong

The login page says what happened: `access_failed` (no token, or one that did
not verify — the reason is in the worker's log and the audit row, never the
token), `access_unavailable` (the team's keys could not be had), `not_invited`,
`access_not_linked`, `access_email_unusable`, `domain_not_allowed`,
`account_blocked`, `account_exists`, `invitation_invalid`,
`provider_already_linked` (the account is linked to another Access identity),
`permissions_too_large`. Every sign-in, refusal (with its reason), link,
accepted invitation, created account and group change is a row in
`auth_events` with method `access`.

## Variables

| Name | Default | Meaning |
|---|---|---|
| `AUTH_ACCESS_TEAM` | — | your team name (`myteam` for `myteam.cloudflareaccess.com`) |
| `AUTH_ACCESS_AUD` | — | the AUD tag of the application covering `/auth/access` |
| `AUTH_ACCESS_LINK_BY_EMAIL` | `off` | `on`: an unlinked Access identity enters the existing account with its address — read the risks above |
| `AUTH_ACCESS_AUTO_CREATE` | `off` | `on`: an unknown Access identity gets an account … |
| `AUTH_ACCESS_DEFAULT_ROLE` | — | … with this role only (never owner, a wildcard or a sensitive permission) |
| `AUTH_ACCESS_ALLOWED_DOMAINS` | — | only addresses in these domains (like `AUTH_<PROVIDER>_ALLOWED_DOMAINS`) |
