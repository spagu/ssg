# Your own SMTP server for the newsletter

`services/smtp-relay` is a small Postfix image (Alpine; amd64, arm64 and
armv7) that the `newsletter` and `newsletter-ui` workers can send through
with their SMTP transport. This page says what it needs from you and how to
check each requirement. DNS records are in
[SMTP-SERVER-DNS.md](SMTP-SERVER-DNS.md); a Raspberry Pi without Docker in
[SMTP-SERVER-PI.md](SMTP-SERVER-PI.md); every setting in the
[service README](../services/smtp-relay/README.md).

No provider is recommended here. Each requirement names the standard behind
it and a command that checks it, so you can judge any network or host
yourself.

## What it is, and what it is not

It **is** a submission server (RFC 6409, RFC 8314) for your own sending:
the Worker logs in on port 465 (TLS from the first byte) or 587 (STARTTLS),
hands over a message, and the server signs it with DKIM and passes it on.

It is **not** a mailbox server. It does not receive mail for your domain, does
not listen on port 25 and stores nothing but its queue and its DKIM keys.
Replies and bounces go to the address in `NEWSLETTER_MAIL_FROM`, so that
address needs a working mailbox somewhere else.

## Choosing a relay mode

`RELAY_MODE` decides where accepted mail goes. There is no default: the
server refuses to start until you choose.

| Mode | Mail goes to | You need |
|---|---|---|
| `direct` | each recipient domain's MX, on port 25 | outbound port 25, a static IP, a PTR record matching `SMTP_HOSTNAME` |
| `smarthost` | one upstream server you have an account with (`RELAYHOST`) | its address, port 587 or 465, a login |
| `hold` | nowhere: signed and kept in the hold queue | nothing; for the first tests |

`direct` is the whole point of running your own server, and also the mode
with the most requirements. `smarthost` is for a network that blocks
outbound port 25 (most home connections); the message is still signed with
your DKIM key before it leaves, so DMARC can pass through DKIM alignment.

## Requirements for `direct`

**A static public IP.** Receivers judge mail by the sending IP's history;
an address that changes loses it, and dynamic ranges are often refused
outright. IPv4 is enough. Turn on IPv6 (`INET_PROTOCOLS=all`) only if the
IPv6 address also has a PTR record — otherwise it fails the next check.

**PTR (reverse DNS) that matches the HELO name, both ways (FCrDNS).** The IP
resolves to a name, and that name resolves back to the same IP. Only the
**owner of the IP** can set the PTR — your ISP or hosting provider, usually
in their control panel or on request. You cannot set it in your own domain's
DNS zone. Check it:

```sh
dig +short -x 203.0.113.10          # -> relay.example.com.
dig +short A relay.example.com      # -> 203.0.113.10   (the same IP)
```

`SMTP_HOSTNAME` must be exactly that name: Postfix uses it in `EHLO`, and
receivers compare the three.

**Outbound port 25.** Many networks block it. Test from the machine that will
run the server, against the MX of a domain you write to:

```sh
dig +short MX example.org                  # pick a host from the answer
nc -vz -w 5 mx.example.org 25              # "succeeded"/"open" = not blocked
```

A timeout means blocked. Then either ask the network's owner to open it, run
the server on a host where it is open, or use `RELAY_MODE=smarthost`.

## Reaching it from Cloudflare Workers

The Worker connects with `connect()` from `cloudflare:sockets`. Cloudflare's
[TCP sockets page](https://developers.cloudflare.com/workers/runtime-apis/tcp-sockets/)
(last updated 2026-10-13, read 2026-11-15) says:

- outbound **port 25 is blocked** — the server listens on 465 and 587;
- connections **to Cloudflare's own IP ranges are refused** — so the DNS
  record for `SMTP_HOSTNAME` must point straight at your server (**DNS
  only**, not proxied), and a Cloudflare tunnel cannot carry this traffic;
- outbound connections come from a prefix that is **not** part of
  Cloudflare's published IP ranges. There is no fixed list to allow, so an IP
  allow-list for 465/587 would either block the Worker or have to be so wide
  it protects nothing. The protection is TLS plus a login with a long random
  password, plus the rate limits.

The same page does not say how the Worker checks the server's certificate.
Use a certificate a public CA issued for exactly `SMTP_HOSTNAME`; expect a
self-signed one to be refused (not tested against a live Worker).

## Firewall

| Direction | Port | Why |
|---|---|---|
| in | 465, 587 TCP | the Worker (from anywhere — see above) |
| in | 25 | **closed.** Open it only if you deliberately run an MX, which this is not |
| out | 25 TCP | `direct` only: recipients' MX servers |
| out | 587 or 465 TCP | `smarthost` only: the upstream |
| out | 53 UDP/TCP | DNS (MX lookups) |

## Certificate

The server refuses to start without a usable certificate and key, a key that
does not match, or an expired certificate: it never offers a login over an
unencrypted connection. Put `fullchain.pem` (leaf first, then
intermediates) and `privkey.pem` in the directory `TLS_DIR` names, or point
`TLS_CERT_FILE` / `TLS_KEY_FILE` elsewhere.

Any ACME client (RFC 8555) can issue one. The SMTP host usually serves no
website, so use the **DNS-01** challenge, or **HTTP-01** with port 80 opened
only during issuance. After each renewal copy the two files into `TLS_DIR`
(or mount the client's directory read-only). The image checks the files
every `CERT_CHECK_INTERVAL` seconds (300) and reloads Postfix when they
change and still pass the checks; a broken pair is logged and the old one
stays in use. 14 days before expiry it logs a warning.

## Connect newsletter-ui (and newsletter)

```toml
# wrangler.toml [vars]
NEWSLETTER_MAIL_FROM = "news@example.com"        # its domain in DKIM_DOMAINS
NEWSLETTER_SMTP_HOST = "relay.example.com"       # = SMTP_HOSTNAME, DNS only
NEWSLETTER_SMTP_PORT = "465"                     # or "587"
NEWSLETTER_SMTP_USER = "newsletter"              # = SMTP_USER
```

```sh
npx wrangler secret put NEWSLETTER_SMTP_PASSWORD   # = secrets/smtp_password
```

Port 465 is what RFC 8314 prefers (TLS before any SMTP is spoken); 587
works the same. `NEWSLETTER_SMTP_HELO` can stay unset. Set the same values on
the `newsletter` template for its confirmation mail. In the panel,
**Settings → Test the SMTP connection** checks TLS and the login.

Start with `RELAY_MODE=hold`: messages are accepted and signed but never
sent. `docker compose exec smtp-relay postqueue -p` lists them;
`postcat -hq <id>` shows the headers, `DKIM-Signature` included. Switch to
`direct` or `smarthost` once DNS is in place.

## Testing deliverability

1. Send a test campaign to a mailbox you control **at another provider**.
2. Open the raw message and find `Authentication-Results` (RFC 8601):
   `spf=pass`, `dkim=pass header.d=example.com`, `dmarc=pass`. A `fail` names
   the record to fix ([SMTP-SERVER-DNS.md](SMTP-SERVER-DNS.md)).
3. Check that the `DKIM-Signature` `h=` list contains `List-Unsubscribe` and
   `List-Unsubscribe-Post` — receivers honour one-click unsubscribe (RFC
   8058) only when it does. The image signs both.
4. Blocklists: many receivers consult DNS-based lists (RFC 5782). A list is
   queried by reversing the IP's octets under the list's zone, e.g.
   `dig +short 10.113.0.203.<list zone>`; an answer means listed. Look up the
   lists' own removal procedures; none is named here.

## Proving it is not an open relay

An open relay sends mail for anyone and is abused within hours. This one
relays only for a logged-in client and only from `SENDER_DOMAINS`. From a
machine outside your network:

```sh
openssl s_client -connect relay.example.com:465 -crlf -quiet
EHLO test.example.org
MAIL FROM:<someone@example.org>
RCPT TO:<someone-else@example.org>     # expect: 554 5.7.1 ... Access denied
QUIT
nc -vz -w 5 relay.example.com 25       # expect: refused / timed out
```

`services/smtp-relay/test/smoke.sh` runs this check (and a wrong password, a
foreign sender, and DKIM verification) against a fresh container.

## Operations

| Task | Command (`docker compose exec smtp-relay …`) |
|---|---|
| queue | `postqueue -p` |
| retry deferred mail now | `postqueue -f` |
| delete one / all held | `postsuper -d <id>` / `postsuper -d ALL hold` |
| logs | `docker compose logs -f smtp-relay` |
| the DKIM record again | `cat /var/lib/smtp-relay/dkim/*.dns.txt` |

**Back up the DKIM key** (`smtp-state` volume, `/var/lib/smtp-relay/dkim`):
losing it means a new key and a new DNS record. The queue volume needs no
backup. **Updates**: `docker compose pull && docker compose up -d`; the image
follows Alpine's security updates when rebuilt.

## Risks

- **Open relay** — see above; re-run the test after any configuration change.
- **A new IP has no reputation.** Receivers rate-limit or junk mail from an
  address they have never seen. Warm up: send to your most engaged readers
  first, a small volume per day, and raise it over weeks while watching
  bounces and complaints (`NEWSLETTER_SEND_RATE` keeps the pace).
- **One complaint-heavy send** can undo that work. Keep double opt-in, honour
  unsubscribes at once, remove hard bounces (the worker does both).
- **A home connection** usually has a dynamic IP without a PTR you control and
  a blocked port 25; that is what `smarthost` mode is for.
