# The SMTP server on a Raspberry Pi, without Docker

The same server as [SMTP-SERVER.md](SMTP-SERVER.md), installed natively on a
Raspberry Pi Zero 2 W (or any Debian-based system) running **Raspberry Pi OS
Lite 64-bit**. It uses the same templates and scripts as the image
(`services/smtp-relay/bin` and `config`), so both paths stay in step.

Tested for this guide in a Debian 13 (trixie) arm64 container under QEMU
with the distribution's packages (Postfix 3.10.13, OpenDKIM 2.11.0~beta2,
Cyrus SASL 2.1.28) — not on Pi hardware. Check `cat /etc/os-release` on
yours: older releases ship older packages, and the scripts need Postfix 3.4
or later.

## First, the network

A Pi at home usually sits behind a connection with a **dynamic IP**, a **PTR
record the ISP controls** and **outbound port 25 blocked**. Each of these
rules out `RELAY_MODE=direct` on its own (see "Requirements for direct" in
[SMTP-SERVER.md](SMTP-SERVER.md)). What remains:

- `RELAY_MODE=smarthost` — the Pi signs and hands mail to an upstream server
  you have an account with, over 587 or 465;
- or run the server on a host with a static IP, a PTR you can have set and
  port 25 open, and keep the Pi for something else.

The Worker must also reach the Pi on 465 or 587 from the internet: a
forwarded port on the router, and a DNS name (DNS only, not proxied) that
follows the IP. Cloudflare tunnels cannot carry this (Workers refuse
Cloudflare's own addresses).

## Docker or native?

Docker Engine runs on a Pi Zero 2 W with a 64-bit OS, and the image is
published for `linux/arm64`. The board has 512 MB of RAM. The relay itself
is small — in a test container (amd64) the whole image ran in about 12 MiB
of memory at rest — but the Docker daemon and containerd add their own
resident memory on top. Measure on your board (`free -m` before and after
installing Docker) and decide; both paths below configure the same server.

## Install

```sh
sudo apt update
sudo apt install --no-install-recommends postfix sasl2-bin libsasl2-modules opendkim openssl ca-certificates
```

When the Postfix installer asks, choose **No configuration**; the
configuration is written by the script below.

Copy the scripts and templates from a checkout of the repository:

```sh
sudo mkdir -p /usr/local/share/smtp-relay /etc/smtp-relay/secrets /etc/smtp-relay/tls
sudo cp -r services/smtp-relay/bin services/smtp-relay/config /usr/local/share/smtp-relay/
sudo install -m 640 services/smtp-relay/config/smtp-relay.env.example /etc/smtp-relay/smtp-relay.env
openssl rand -base64 24 | sudo install -m 600 /dev/stdin /etc/smtp-relay/secrets/smtp_password
sudo chmod 700 /etc/smtp-relay/secrets
```

Edit `/etc/smtp-relay/smtp-relay.env` (`SMTP_HOSTNAME`, `RELAY_MODE`,
`DKIM_DOMAINS`, and for a smarthost `RELAYHOST`, `RELAYHOST_USER` plus
`/etc/smtp-relay/secrets/relayhost_password`). Every variable is described in
the [service README](../services/smtp-relay/README.md#configuration). Its
last line, `OPENDKIM_CONF=/etc/opendkim.conf`, is where Debian keeps
OpenDKIM's configuration.

## Certificate

Put `fullchain.pem` and `privkey.pem` (mode 600) in `/etc/smtp-relay/tls/`.
With an ACME client, add a renewal (deploy) hook that copies the new files
there, for example:

```sh
install -m 644 "$CERT_DIR/fullchain.pem" /etc/smtp-relay/tls/fullchain.pem
install -m 600 "$CERT_DIR/privkey.pem"   /etc/smtp-relay/tls/privkey.pem
```

`smtp-relay-cert.path` notices the change, checks the pair and reloads
Postfix; a pair that fails the check is not loaded.

## systemd units

```sh
cd /usr/local/share/smtp-relay/config/systemd
sudo cp smtp-relay-config.service smtp-relay-cert.path smtp-relay-cert.service /etc/systemd/system/
sudo mkdir -p /etc/systemd/system/postfix@-.service.d /etc/systemd/system/opendkim.service.d
sudo cp postfix@-.service.d/smtp-relay.conf /etc/systemd/system/postfix@-.service.d/
sudo cp opendkim.service.d/smtp-relay.conf /etc/systemd/system/opendkim.service.d/
sudo systemctl daemon-reload
sudo systemctl enable --now smtp-relay-config.service smtp-relay-cert.path
sudo systemctl restart opendkim postfix
```

| Unit | Does |
|---|---|
| `smtp-relay-config.service` | runs `bin/render-config.sh` at boot: writes `main.cf`, `master.cf`, the SASL config, the login database (in `/run`, rebuilt each boot), `/etc/opendkim.conf`, and creates the DKIM key |
| `postfix@-.service.d/`, `opendkim.service.d/` | make Postfix and OpenDKIM wait for it, and not start when it fails |
| `smtp-relay-cert.path` / `.service` | reload Postfix after a certificate change |

After changing the env file: `sudo systemctl restart smtp-relay-config
opendkim postfix`. The DKIM record to publish:

```sh
sudo journalctl -u smtp-relay-config | grep -A1 "DKIM DNS record"
sudo cat /var/lib/smtp-relay/dkim/*.dns.txt
```

Logs: `journalctl -u postfix@- -u opendkim -f`.

## Memory on 512 MB

- The example env sets `PROCESS_LIMIT=10`: at most ten processes per Postfix
  service instead of 100. A newsletter sending at `NEWSLETTER_SEND_RATE` 60
  per minute uses one or two at a time.
- Postfix and OpenDKIM start as several small processes (master, qmgr,
  postlogd, tlsmgr, anvil, one `smtpd` per connection, one `smtp` per
  delivery). Watch them with `ps -eo rss,comm | sort -n`.
- A swap file helps against a peak; it does not replace RAM.

## Checks

The same as for the image — [SMTP-SERVER.md](SMTP-SERVER.md): FCrDNS,
outbound 25 (direct only), the open-relay test, a test campaign and its
`Authentication-Results`. `RELAY_MODE=hold` works here too while DNS is not
ready.
