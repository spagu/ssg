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
wrangler pages secret put TURNSTILE_SECRET
wrangler pages secret put CONTACT_TO
wrangler pages secret put CONTACT_FROM
# optional, enables the Resend path instead of MailChannels:
wrangler pages secret put RESEND_API_KEY
```

## Front-end

Post a `multipart/form-data` body to `/api/contact` with `name`, `email`,
`message` and the Turnstile-injected `cf-turnstile-response` field.
