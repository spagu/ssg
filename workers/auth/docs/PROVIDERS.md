# auth — signing in with Google, Microsoft, Apple, GitHub or Facebook

> **Not tried against the real providers.** None of the five was exercised
> against the live provider in this work — only against scripted fakes built
> from their published protocols (`test/providers-fake.ts`). The console
> click-paths below are written as known at the time and **may have changed**;
> the redirect URI, the scopes and the variables are what the worker needs
> whatever the consoles look like. Try each provider on a test site first.

Every provider is off until you give it its variables; any combination can be
on, the password included, and each person picks the way they like. The
password is a method like the others: `AUTH_PASSWORD_LOGIN=off` removes it. At
least one method must stay: with none, every route that needs the database
answers `503 no_sign_in_method` and says which variables to set.

## How it works

1. A provider button is a plain form to `POST /api/auth/oauth/<provider>/start`.
   The worker stores a short-lived flow (state, nonce, PKCE verifier) under a
   random handle and sends the browser to the provider with the handle in a
   `__Host-` cookie.
2. The provider sends the browser back to **one fixed URL per provider**:

   ```
   https://<your site host>/api/auth/oauth/<provider>/callback
   ```

   e.g. `https://example.com/api/auth/oauth/google/callback`. Paste exactly
   this — scheme, host, path, no trailing slash — into the provider's console.
3. The worker uses the flow up (once, whatever the answer), exchanges the code
   on the provider's hard-coded hosts only, verifies the ID token locally
   (Google, Microsoft, Apple) or reads the profile API (GitHub, Facebook), and
   forgets the provider's tokens.

**Who gets in.** Only invited people. The account here is linked to the pair
*(provider, account id at the provider)* — never to an address. An address is
used for one thing only: to find an open invitation, and only when the
provider says it verified that address. So:

| Situation | Result |
|---|---|
| The provider account is linked to an account here | signed in (unless the account is blocked) |
| Not linked; verified address = an open invitation | the invitation is accepted, the provider account linked |
| Not linked; the invitee opened the invitation link and chose this provider | accepted and linked, **whatever** the provider's address — the link is the proof; the difference is recorded in the audit log |
| Not linked; no invitation | refused (`not_invited`); no account is created |
| Not linked; address not verified (always for Facebook) | refused (`email_unverified`): use the invitation link |
| Verified address equal to an existing account's, nothing linked | refused — an address never opens an account |

A signed-in person links more providers on **Your account** (needs a sign-in
from the last `AUTH_RECENT_SIGNIN_MINUTES`, default 10, else they are asked to
sign in again), removes them, or sets a first password. The last way in cannot
be removed — the database refuses it, also when two removals race.

## Google

1. Google Cloud Console → pick or create a project → **APIs & Services →
   OAuth consent screen** (newer consoles: **Google Auth Platform**): app
   name, support address, audience *External* (or *Internal* for one Google
   Workspace). Data access: `openid`, `.../auth/userinfo.email`,
   `.../auth/userinfo.profile` — non-sensitive, no review.
2. **Clients → Create client → Web application.** Authorized redirect URIs:
   `https://example.com/api/auth/oauth/google/callback`.
3. Copy the client ID and secret:

   ```sh
   # wrangler.toml [vars]: AUTH_GOOGLE_CLIENT_ID = "1234-abc.apps.googleusercontent.com"
   npx wrangler pages secret put AUTH_GOOGLE_CLIENT_SECRET
   ```

Quirks: Google asserts `email_verified`; a Workspace domain can be required
with `AUTH_GOOGLE_ALLOWED_DOMAINS = "example.com"`.

## Microsoft (Entra ID, personal accounts)

1. Microsoft Entra admin center → **Identity → Applications → App
   registrations → New registration**. Supported account types decide
   `AUTH_MICROSOFT_TENANT`:

   | Account types | `AUTH_MICROSOFT_TENANT` |
   |---|---|
   | this organizational directory only | the **Directory (tenant) ID** (a GUID) |
   | any organizational directory | `organizations` |
   | any organizational directory and personal accounts | `common` (default) |
   | personal accounts only | `consumers` |

2. Redirect URI: platform **Web**,
   `https://example.com/api/auth/oauth/microsoft/callback`.
3. **Certificates & secrets → New client secret**; copy the *Value* (shown
   once; it expires — put a reminder in your calendar).
4. **Token configuration → Add optional claim → ID**: `email` and `xms_edov`.

```sh
# [vars]: AUTH_MICROSOFT_CLIENT_ID = "<Application (client) ID>", AUTH_MICROSOFT_TENANT = "common"
npx wrangler pages secret put AUTH_MICROSOFT_CLIENT_SECRET
```

Quirks: the issuer is per tenant — the worker accepts
`https://login.microsoftonline.com/<tid>/v2.0` only with the token's own `tid`,
and only a tenant `AUTH_MICROSOFT_TENANT` allows. Microsoft's `email` claim is
**not verified by default**: it counts as verified only with `xms_edov: true`
(step 4). Without it Microsoft works like Facebook — invitation link only.

## Apple

1. Apple Developer → **Certificates, Identifiers & Profiles → Identifiers**:
   an App ID with **Sign in with Apple** enabled (the *primary App ID*).
2. **Identifiers → Services IDs → +**: e.g. `com.example.web` — this is the
   client id. Enable Sign in with Apple → **Configure**: the primary App ID,
   domain `example.com`, Return URL
   `https://example.com/api/auth/oauth/apple/callback` (https only, no
   localhost).
3. **Keys → +**: Sign in with Apple, for the primary App ID. Download the
   `.p8` file (once); note its **Key ID** and your **Team ID** (Membership).

```sh
# [vars]: AUTH_APPLE_CLIENT_ID = "com.example.web", AUTH_APPLE_TEAM_ID = "ABCDE12345", AUTH_APPLE_KEY_ID = "XYZ9876543"
npx wrangler pages secret put AUTH_APPLE_PRIVATE_KEY < AuthKey_XYZ9876543.p8
```

Quirks: Apple has no static client secret — the worker signs one per request
(an ES256 JWT, 5 minutes, WebCrypto) with the `.p8` key. Apple answers with a
cross-site form POST (`response_mode=form_post`, needed for name and address);
the callback bounces it to a GET, where the browser's flow cookie is sent. The
**name arrives only on the first consent**, unsigned: it becomes the new
account's display name if the invitation had none, nothing more. With "Hide My
Email" the address is `…@privaterelay.appleid.com`, which never matches an
invitation: use the invitation link. PKCE is not part of Apple's documented
token request, so it is not sent; state and nonce are.

## GitHub

1. GitHub → **Settings → Developer settings → OAuth Apps → New OAuth App**
   (an *OAuth App*, not a GitHub App). Homepage URL `https://example.com`;
   Authorization callback URL
   `https://example.com/api/auth/oauth/github/callback`.
2. Copy the Client ID; **Generate a new client secret**.

```sh
# [vars]: AUTH_GITHUB_CLIENT_ID = "Ov23li…"
npx wrangler pages secret put AUTH_GITHUB_CLIENT_SECRET
```

Quirks: no ID token. The account id is GitHub's numeric user id (a login can
be renamed and reused). The address comes from `GET /user/emails` (scope
`user:email`): the *primary* one, verified only if GitHub says so. Scopes
asked: `read:user user:email`; `allow_signup=false` keeps GitHub's sign-up
link off its consent page.

## Facebook

1. Meta for Developers → **My Apps → Create app**, use case *Authenticate and
   request data from users with Facebook Login*.
2. **Facebook Login → Settings**: Valid OAuth Redirect URIs
   `https://example.com/api/auth/oauth/facebook/callback`; keep *Use Strict
   Mode for redirect URIs* and *Enforce HTTPS* on.
3. Permissions `public_profile` and `email`; switch the app to **Live**.
   **App settings → Basic**: App ID and App Secret. Optional: *Advanced →
   Require App Secret* (the worker sends `appsecret_proof`).

```sh
# [vars]: AUTH_FACEBOOK_CLIENT_ID = "<App ID>", AUTH_FACEBOOK_API_VERSION = "v24.0"
npx wrangler pages secret put AUTH_FACEBOOK_CLIENT_SECRET
```

Quirks: Facebook does **not** say whether an address is verified, so its
address is never matched to anything — a Facebook account accepts an
invitation only through the invitation link. Graph versions retire after about
two years: raise `AUTH_FACEBOOK_API_VERSION` when Meta warns.

## Narrowing

| Variable | Effect |
|---|---|
| `AUTH_<PROVIDER>_ALLOWED_DOMAINS` | e.g. `AUTH_GOOGLE_ALLOWED_DOMAINS = "example.com, example.org"`: that provider is accepted only with a **verified** address in one of these domains (exact; sub-domains are their own entries) — for sign-in, invitations and linking |
| `AUTH_ROLE_METHODS` | JSON, e.g. `{"owner": ["google", "github"]}`: a listed role counts only in sessions started with one of its methods (`password`, a provider id, or `access` — [Cloudflare Access](ACCESS.md)); applied at every token issue. The pages tell the person which roles are left out and why. Not valid JSON of that shape → no role counts (fails closed) |
| `AUTH_PASSWORD_LOGIN` | `off`: no password sign-in (`404 method_disabled`); the login page shows no password form and no "Forgot your password?" link; invitations offer the providers only |
| `AUTH_RECENT_SIGNIN_MINUTES` | how fresh a sign-in must be to add, remove or set a method (default 10) |

The second factor applies to provider sign-ins too, when a role needs one
([SECOND-FACTOR.md](SECOND-FACTOR.md)): the worker never assumes a provider
checked one.

## Buttons and branding

The buttons are text in these pages' own neutral style — "Sign in with Google",
"Sign in with Microsoft", "Sign in with Apple", "Sign in with GitHub",
"Continue with Facebook", the wording each provider's guidelines give, the name
never altered. **No provider logo is shipped** and nothing is loaded from a
provider: drawing logos to each guideline (Google's four-colour "G", Apple's
glyph sizes, Microsoft's squares, Meta's "f") was left out. Add them yourself
if your review requires them, as your own files under `/auth/assets/`.

## When it goes wrong

The login page (the account page for a link) says what happened: `oauth_cancelled`
(consent cancelled), `oauth_state_invalid` (expired, used, or another
browser), `not_invited`, `email_unverified`, `provider_failed` (the answer
could not be verified — the reason is in the worker's log, never a token),
`domain_not_allowed`, `identity_in_use`, `provider_already_linked`,
`account_blocked`, `reauth_required`. A provider route answers `404` when the
provider is not configured, `503 provider_misconfigured` (naming the variable)
when its variables are wrong. Each sign-in, refusal, link and unlink is a row in
`auth_events` (see [API.md](API.md#login-providers-and-sign-in-methods-go-151)).
