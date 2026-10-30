# contact-form worker

A Cloudflare Pages Function that receives a contact / job-application form,
verifies a Turnstile token and sends the message by email through Resend or the
MailChannels Email API. No npm dependencies; `ssg --deploy cloudflare`
publishes the `functions/` tree through `wrangler pages deploy`.

## Config

Add to your `.ssg.yaml`:

```yaml
worker:
  dir: workers/contact-form
  mode: functions
  routes_include:
    - /api/*
```

## Files

```text
workers/contact-form/
├── functions/api/contact.ts   POST /api/contact: form → Turnstile → mail
├── functions/api/_mail.ts     Resend / MailChannels delivery (not routed)
├── wrangler.snippet.toml      secret list for `ssg new wrangler`
└── test/                      node --test suite
```

## Secrets

```sh
wrangler pages secret put CONTACT_TURNSTILE_SECRET
wrangler pages secret put CONTACT_TO
wrangler pages secret put CONTACT_FROM
# one mail provider is required:
wrangler pages secret put CONTACT_RESEND_API_KEY        # Resend
wrangler pages secret put CONTACT_MAILCHANNELS_API_KEY  # or MailChannels
```

| Secret | Required | Purpose |
|---|---|---|
| `CONTACT_TURNSTILE_SECRET` | yes | Turnstile secret for this form (falls back to `TURNSTILE_SECRET`) |
| `CONTACT_TO` | yes | inbox the messages go to |
| `CONTACT_FROM` | yes | sender address, verified with your mail provider |
| `CONTACT_RESEND_API_KEY` | one of the two | send through [Resend](https://resend.com) |
| `CONTACT_MAILCHANNELS_API_KEY` | one of the two | send through the MailChannels Email API (`X-Api-Key` header) |
| `RESEND_API_KEY` | no | legacy Resend key, read only when neither prefixed key is set |

When both prefixed keys are set, Resend wins. MailChannels ended keyless
sending from Cloudflare Workers in 2024, so every provider now needs a key: with
no mail key, or without `CONTACT_TO` / `CONTACT_FROM`, the form answers
`503 {"error":"contact form mail not configured: …"}` and logs the reason —
before the Turnstile check, so a visitor's token is never spent on a form that
cannot send. A provider error answers `502 {"error":"send failed"}` and logs the
status.

Upgrading: the unprefixed `RESEND_API_KEY` used to do nothing (the Resend path
shipped commented out). It is now honoured as a Resend fallback, so a project
that already set it starts sending through Resend. Prefer
`CONTACT_RESEND_API_KEY`; the shared name is read last, so a `RESEND_API_KEY`
meant for another worker never overrides this form's MailChannels key.

`CONTACT_TURNSTILE_SECRET` is this form's own Turnstile secret. When it is
unset the form falls back to the shared `TURNSTILE_SECRET`, so a project set up
before the prefix existed keeps working unchanged. Prefer the prefixed name:
every worker in one Pages project sees the same environment, and an unprefixed
secret meant for one template is read by all of them — see *Secrets* in
[docs/WORKERS.md](https://github.com/spagu/ssg/blob/main/docs/WORKERS.md#secrets).

## Front-end

Post a `multipart/form-data` body to `/api/contact` with `name`, `email`,
`message` and the Turnstile-injected `cf-turnstile-response` field.

## Tests

From the repository root (Node 23.6+ strips the TypeScript types itself):

```sh
node --import ./workers/contact-form/test/resolve.mjs --test --experimental-test-coverage 'workers/contact-form/test/*.test.ts'
```
