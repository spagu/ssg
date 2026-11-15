# DNS for your own SMTP server

The records [SMTP-SERVER.md](SMTP-SERVER.md) refers to, for a server named
`relay.example.com` at `203.0.113.10` sending as `news@example.com`.
Replace the names; keep the shape. Check every record with `dig` after
publishing — the commands are under each one.

## Which records, by relay mode

| Record | `direct` | `smarthost` | Standard |
|---|---|---|---|
| A / AAAA for `SMTP_HOSTNAME` | yes | yes (the Worker connects to it) | RFC 1035 |
| PTR for the server's IP | yes, set by the IP's owner | no | RFC 1912 2.1 |
| SPF on the sending domain | your server | the upstream's servers | RFC 7208 |
| DKIM (printed by the container) | yes | yes | RFC 6376 |
| DMARC | yes | yes | RFC 7489 |
| MX | not for sending | not for sending | RFC 5321 |

## A / AAAA

```dns
relay.example.com.   3600  IN  A     203.0.113.10
```

If the zone is on Cloudflare, this record must be **DNS only** (grey cloud):
Workers cannot connect to Cloudflare's own addresses. Add an AAAA record only
when the server really sends over IPv6 (`INET_PROTOCOLS=all`) and that
address has its own PTR.

```sh
dig +short A relay.example.com
```

## PTR (reverse DNS)

Set by whoever owns the IP — ISP or hosting provider — not in your zone.
Ask for `203.0.113.10 → relay.example.com`, then check both directions:

```sh
dig +short -x 203.0.113.10        # relay.example.com.
dig +short A relay.example.com    # 203.0.113.10
```

## MX — not needed to send

MX says where mail **to** a domain is delivered. This server only sends, so
it needs none. The domain in `NEWSLETTER_MAIL_FROM` still needs somewhere to
receive replies and bounces: keep that domain's existing MX as it is.

## SPF (RFC 7208)

One TXT record on the domain you send from, listing who may send for it.
`direct`:

```dns
example.com.  3600  IN  TXT  "v=spf1 a:relay.example.com ~all"
```

- The domain may have **only one** SPF record. If it already has one (for
  the mailbox provider), add `a:relay.example.com` (or `ip4:203.0.113.10`)
  to it instead of adding a second record.
- An SPF check may use at most **10 DNS lookups** (`a`, `mx`, `include`, …);
  `ip4:` uses none.
- `~all` (soft fail) while testing; `-all` once every legitimate sender is
  listed.
- `smarthost`: list what the upstream's documentation tells you to include,
  not your own server — the upstream is the one connecting to receivers.

```sh
dig +short TXT example.com | grep spf1
```

## DKIM (RFC 6376)

On the first start the container creates a 2048-bit RSA key per domain in
`DKIM_DOMAINS` and prints its record:

```text
smtp-relay: DKIM DNS record for example.com — publish it once, unchanged:
relay._domainkey.example.com. IN TXT ( "v=DKIM1; k=rsa; " "p=MIIBIjAN…" "…IDAQAB" )
```

Publish it as one TXT record named `relay._domainkey.example.com` (selector
`DKIM_SELECTOR`, default `relay`). The value is split into quoted strings
because one string holds at most 255 characters; most DNS panels accept the
strings pasted together without the quotes. Print it again at any time:

```sh
docker compose exec smtp-relay cat /var/lib/smtp-relay/dkim/example.com.relay.dns.txt
dig +short TXT relay._domainkey.example.com
```

The key stays in the `smtp-state` volume. To rotate it, choose a new
selector (`DKIM_SELECTOR=relay2`), restart, publish the new record, and
remove the old one a week later, once no mail signed with it is in flight.

## DMARC (RFC 7489)

DMARC tells receivers what to do when neither SPF nor DKIM passes **aligned**
with the `From` domain. Here DKIM is always aligned (`d=example.com`). Start
by only collecting reports:

```dns
_dmarc.example.com.  3600  IN  TXT  "v=DMARC1; p=none; rua=mailto:dmarc-reports@example.com"
```

Read the aggregate reports for a few weeks; when every source you send from
passes, raise the policy to `p=quarantine`, then `p=reject`. If the domain
already has a DMARC record, keep it and its policy — this server does not
need a weaker one.

```sh
dig +short TXT _dmarc.example.com
```

## MTA-STS and TLS-RPT — optional

MTA-STS (RFC 8461) and TLS reporting (RFC 8460) protect mail **delivered to**
your domain, so they belong to whoever runs your MX. A sending-only server
needs neither. In `direct` mode this server uses TLS opportunistically when
it delivers (RFC 3207); that is what nearly every sending server does.
