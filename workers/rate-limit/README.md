# rate-limit worker

A request budget for every Function in the project. Scaffold it with
`ssg new worker rate-limit`.

It exists because the other templates are presented as ready to deploy and one
thing was missing from all of them. `ssg new worker contact-form` gives you a
public, unauthenticated `POST` that sends email; `comments` accepts writes;
`cookie-consent` logs. None of them bounded how often they could be called.
Turnstile raises the cost of abuse without capping it — a solved token can be
replayed inside its validity window, and Turnstile is optional anyway.

`rate_limit` / `rate_burst` in `.ssg.yaml` bound the **built-in preview server**
only. This is the deployed equivalent.

## How it composes

It is `functions/_middleware.ts`, so it wraps whatever else is in `functions/`,
including routes added later. Scaffold it beside another template and both end
up in the same project:

```bash
ssg new worker contact-form
ssg new worker rate-limit
cp -r workers/rate-limit/functions/_middleware.ts workers/contact-form/functions/
```

## Backends

Pick one. Without either, the middleware is a **no-op** — an unconfigured
project behaves exactly as it did, which is the right default for something that
can otherwise turn away real visitors.

| Backend | Accuracy | Cost | When |
|---|---|---|---|
| `RATE_LIMITER` binding | exact, per point of presence | free | the default; nothing to provision |
| `RATE_LIMIT_KV` | approximate | KV pricing | only when the binding is unavailable |

The KV weakness is worth understanding rather than discovering: KV is eventually
consistent, so a burst arriving at several points of presence at once can
overshoot the cap — which is precisely the shape a spam run takes. It is a
fallback, not a choice.

## Settings

All optional, all in `[vars]`:

| Variable | Default | Meaning |
|---|---|---|
| `RATE_LIMIT_MAX` | `20` | requests per key per window |
| `RATE_LIMIT_WINDOW` | `60` | window, in seconds |
| `RATE_LIMIT_BY` | `ip` | `ip`, or `header:<name>` |
| `RATE_LIMIT_PATHS` | `/api/` | comma-separated prefixes this covers |
| `RATE_LIMIT_FAIL` | `open` | `open` or `closed` when the backend errors |

## Decisions worth knowing about

**Fail open by default.** When the backend itself errors, the request goes
through. For a contact form that is right: losing a real enquiry costs more than
letting one extra message past. For anything that moves money it is wrong — set
`RATE_LIMIT_FAIL = "closed"` on a project with a `stripe-checkout` in it. It is
a setting rather than a silent default because the answer differs per site.

**Rejected requests are not counted.** A caller already over the cap does not
push their own window forward by knocking again. Without that, a bot can lock a
real visitor out of a shared address indefinitely — and `cf-connecting-ip`
buckets everyone behind one CGNAT or one office together, so shared addresses
are the normal case, not the edge one. Use `RATE_LIMIT_BY = "header:…"` when the
site has something better to key on.

**The 429 carries `Retry-After`** and `Cache-Control: no-store`. The first so a
well-behaved client backs off instead of retrying immediately; the second
because a 429 is about this caller at this moment, and a cached one would answer
somebody else's request.

## Testing it before it ships

`ssg --watch --wrangler` runs the Functions locally, so the limit can be
exercised the way a redirect can:

```bash
for i in $(seq 1 25); do curl -s -o /dev/null -w '%{http_code}\n' \
  http://localhost:8788/api/contact -X POST; done
```

With `RATE_LIMIT_MAX = "20"` the first twenty answer normally and the rest
answer `429`.
