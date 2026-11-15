# Changelog — smtp-relay

All notable changes to `services/smtp-relay` are recorded here. Images are
released from tags `smtp-relay-vX.Y.Z`.

## [Unreleased] — ships with ssg 1.9.11

### Added
- Postfix 3.11.7 submission server on Alpine 3.24.4 (pinned by tag and
  digest): port 465 implicit TLS and 587 STARTTLS, TLS 1.2 or later, Cyrus
  SASL login (PLAIN, LOGIN) required on both, no listener on port 25.
- `RELAY_MODE`: `direct` (recipients' MX), `smarthost` (`RELAYHOST` with a
  verified certificate and an optional login), `hold` (accept, sign, keep).
- OpenDKIM 2.11.0 signing: a 2048-bit RSA key per `DKIM_DOMAINS` entry made
  on the first start, its DNS record printed; `List-Unsubscribe` and
  `List-Unsubscribe-Post` signed and oversigned; mail deferred, never sent
  unsigned, when the signer is down.
- Not an open relay: relaying only for a logged-in client with a `MAIL FROM`
  in `SENDER_DOMAINS`. Per-client limits (connections, messages,
  recipients, logins) through anvil.
- Refuses to start without a matching, unexpired certificate; reloads Postfix
  when the mounted certificate changes and still passes the checks.
- Configuration from environment variables, passwords from `*_FILE`;
  `bin/render-config.sh` writes every file from `config/*.tmpl`, for the
  image and for a native Debian / Raspberry Pi OS install (systemd units in
  `config/systemd/`).
- tini as PID 1, logs on stdout, `HEALTHCHECK`, graceful stop.
- docker compose file (read-only root, seven capabilities, limits and
  reservations, 10 MB logs), `test/smoke.sh` end-to-end test, images for
  linux/amd64, linux/arm64 and linux/arm/v7.
