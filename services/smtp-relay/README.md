# smtp-relay

[![smtp-relay](https://github.com/spagu/ssg/actions/workflows/smtp-relay.yml/badge.svg)](https://github.com/spagu/ssg/actions/workflows/smtp-relay.yml)
![Alpine 3.24.4](https://img.shields.io/badge/Alpine-3.24.4-0D597F)
![Postfix 3.11](https://img.shields.io/badge/Postfix-3.11-blue)
![platforms](https://img.shields.io/badge/platforms-amd64%20%7C%20arm64%20%7C%20armv7-informational)

A Postfix **submission server** with DKIM signing, for the SMTP transport of
ssg's `newsletter` and `newsletter-ui` workers: ports 465 (implicit TLS) and
587 (STARTTLS), a login required on both, mail relayed to recipients' MX, to
a smarthost, or held for testing. Not a mailbox server; it does not listen
on port 25.

The guide — requirements, DNS, firewall, connecting the Worker, testing,
operations, risks — is [docs/SMTP-SERVER.md](../../docs/SMTP-SERVER.md), with
[DNS records](../../docs/SMTP-SERVER-DNS.md) and a
[Raspberry Pi install without Docker](../../docs/SMTP-SERVER-PI.md).

```sh
cp .env.example .env && mkdir -p secrets tls
openssl rand -base64 24 > secrets/smtp_password && : > secrets/relayhost_password
cp /path/to/fullchain.pem /path/to/privkey.pem tls/
docker compose up -d
```

Images, the same in both registries: `ghcr.io/spagu/ssg-smtp-relay` and
`tradik/ssg-smtp-relay` on Docker Hub — `:X.Y.Z`, `:X.Y`, `:X`, `:latest`
from tags `smtp-relay-vX.Y.Z`; `:edge` from `main`. Set `SMTP_RELAY_IMAGE`
to use the Docker Hub name in the compose file.

## Configuration

Environment variables only; passwords from files (`*_FILE`, Docker
secrets), never from the environment. A missing or malformed value stops
the start with a message naming it.

| Variable | Default | Meaning |
|---|---|---|
| `SMTP_HOSTNAME` | — (required) | the server's FQDN: HELO name, PTR, certificate name, `NEWSLETTER_SMTP_HOST` |
| `RELAY_MODE` | — (required) | `direct`, `smarthost` or `hold` |
| `DKIM_DOMAINS` | — (required) | domains you send from; one key each |
| `DKIM_SELECTOR` | `relay` | DKIM selector |
| `DKIM_KEY_BITS` | `2048` | RSA key size for new keys (2048–4096) |
| `SENDER_DOMAINS` | `DKIM_DOMAINS` | allowed `MAIL FROM` domains |
| `SMTP_USER` | — (required) | the login (`NEWSLETTER_SMTP_USER`) |
| `SMTP_PASSWORD_FILE` | — (required) | file with its password, at least 16 characters |
| `RELAYHOST` | — | smarthost as `[host]:port`, port 587 or 465 |
| `RELAYHOST_USER`, `RELAYHOST_PASSWORD_FILE` | — | smarthost login |
| `TLS_CERT_FILE`, `TLS_KEY_FILE` | `/etc/smtp-relay/tls/fullchain.pem`, `privkey.pem` | certificate chain and key (PEM) |
| `CERT_CHECK_INTERVAL` | `300` | seconds between certificate checks (image) |
| `INET_PROTOCOLS` | `ipv4` | `ipv4`, `ipv6` or `all` |
| `QUEUE_LIFETIME` | `2d` | how long undeliverable mail is retried |
| `MESSAGE_SIZE_LIMIT` | `10240000` | bytes per message |
| `PROCESS_LIMIT` | `20` | processes per Postfix service |
| `OUTBOUND_CONCURRENCY` | `5` | parallel deliveries per recipient domain |
| `LIMIT_*` | see below | per-client limits |

## Limits

Per client IP and minute (Postfix `anvil`), sized for the newsletter
workers' defaults — `NEWSLETTER_SEND_RATE` 60 messages a minute in batches
of `NEWSLETTER_SEND_BATCH` 10, one connection and one login per batch:

| Variable | Default | Postfix setting |
|---|---|---|
| `LIMIT_CONNECTIONS_PER_MINUTE` | `60` | `smtpd_client_connection_rate_limit` |
| `LIMIT_CONCURRENT_CONNECTIONS` | `10` | `smtpd_client_connection_count_limit` |
| `LIMIT_MESSAGES_PER_MINUTE` | `600` | `smtpd_client_message_rate_limit` |
| `LIMIT_RECIPIENTS_PER_MINUTE` | `600` | `smtpd_client_recipient_rate_limit` |
| `LIMIT_AUTH_PER_MINUTE` | `30` | `smtpd_client_auth_rate_limit` |

Raising the send rate means raising these with it (connections ≈ rate ÷
batch). Workers do not connect from one fixed address, so the limits are a
brake on abuse rather than an exact quota. Also fixed: five errors end a
session (`smtpd_hard_error_limit`), 100 recipients per message, and a
newsletter gives up after `QUEUE_LIFETIME` instead of Postfix's five days.

## Design

| Choice | Why |
|---|---|
| Alpine 3.24.4, by tag and digest | the current Alpine release (Docker Hub, 2026-11-08); the image is about 45 MB |
| Postfix 3.11.7 (Alpine package) | `start-fg`, `maillog_file=/dev/stdout`, `smtpd_tls_chain_files`, anvil limits |
| **Cyrus SASL** with sasldb, not Dovecot | Postfix on Alpine already links libsasl2, so it adds about 0.15 MB (Dovecot: about 10 MB, and a second daemon); and Postfix's *client* SASL — the smarthost login — exists only for Cyrus |
| **OpenDKIM** 2.11.0 | the signing milter Alpine and Debian both package; small (about 0.1 MB); dkimproxy needs Perl, rspamd is a whole filter |
| **tini** + a shell supervisor | tini (PID 1) forwards signals and reaps zombies; the entrypoint starts Postfix (`postfix start-fg`), OpenDKIM, syslogd (OpenDKIM logs only to syslog) and the certificate watcher, and exits when any of them dies so the restart policy starts all from scratch. s6-overlay would add a package set for the same result |
| Root master, unprivileged daemons | Postfix's master must start as root (ports, privilege separation); every network-facing daemon runs as `postfix`, OpenDKIM as `opendkim`. Compose runs it read-only with seven capabilities |
| Certificate polling, not inotify | ACME clients swap symlinks and bind mounts do not pass every inotify event; a checksum sees all changes with no extra package |
| DKIM signs `List-Unsubscribe(-Post)`, oversigned | RFC 8058 one-click unsubscribe counts only when signed |
| `milter_default_action = tempfail` | if OpenDKIM is down, mail waits instead of leaving unsigned |
| one source for Docker and native | `bin/render-config.sh` writes every file from `config/*.tmpl` in both |

## Tests

`make smtp-relay-test` (or `sh test/smoke.sh IMAGE`) starts the image in
`hold` mode with a self-signed certificate and checks: TLS on 465 and
STARTTLS on 587, no `AUTH` and no mail before STARTTLS, a login and one
message per port, a wrong password, relaying without a login (open-relay
test), a foreign sender, the message on hold, and a DKIM signature that
verifies (`test/dkim_verify.py`) and covers `List-Unsubscribe-Post` — and
that a tampered copy fails. `SMOKE_PLATFORM=linux/arm64` runs another
architecture under QEMU. CI runs it on every change to this directory
(`.github/workflows/smtp-relay.yml`).

Changes: [CHANGELOG.md](CHANGELOG.md).
