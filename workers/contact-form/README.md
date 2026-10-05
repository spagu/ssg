# contact-form worker

A Cloudflare Pages Function that receives a contact / job-application form,
verifies a Turnstile token and sends the message by email (MailChannels by
default, Resend optional). No npm dependencies — deployable via Direct Upload.

## Config

Add to your `.ssg.yaml`:

```yaml
worker:
  dir: workers/contact-form
  mode: functions
  routes_include:
    - /api/*
```

## Secrets

```sh
wrangler pages secret put CONTACT_TURNSTILE_SECRET
wrangler pages secret put CONTACT_TO
wrangler pages secret put CONTACT_FROM
# optional, enables the Resend path instead of MailChannels:
wrangler pages secret put RESEND_API_KEY
```

`CONTACT_TURNSTILE_SECRET` is this form's own Turnstile secret. When it is
unset the form falls back to the shared `TURNSTILE_SECRET`, so a project set up
before the prefix existed keeps working unchanged. Prefer the prefixed name:
every worker in one Pages project sees the same environment, and an unprefixed
secret meant for one template is read by all of them — see *Secrets* in
[docs/WORKERS.md](https://github.com/spagu/ssg/blob/main/docs/WORKERS.md#secrets).

## Front-end

Post a `multipart/form-data` body to `/api/contact` with `name`, `email`,
`message` and the Turnstile-injected `cf-turnstile-response` field.
