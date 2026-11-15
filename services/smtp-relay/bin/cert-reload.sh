#!/bin/sh
# cert-reload.sh — checks the certificate pair and reloads Postfix when it is
# usable. Run by certwatch.sh in the image and by smtp-relay-cert.service on
# a native install; an ACME client's renewal hook can run it too. A pair
# that fails the checks is not loaded: Postfix keeps serving the old one.
set -eu

SMTP_RELAY_HOME=${SMTP_RELAY_HOME:-/usr/local/share/smtp-relay}
# shellcheck source=lib.sh
. "$SMTP_RELAY_HOME/bin/lib.sh"
# shellcheck source=cert.sh
. "$SMTP_RELAY_HOME/bin/cert.sh"

cert=${TLS_CERT_FILE:-/etc/smtp-relay/tls/fullchain.pem}
key=${TLS_KEY_FILE:-/etc/smtp-relay/tls/privkey.pem}
if ! checkCert "$cert" "$key" "${SMTP_HOSTNAME:-}"; then
	log "the new certificate is not usable; keeping the one already loaded"
	exit 1
fi
log "certificate changed; reloading Postfix"
postfix reload >/dev/null 2>&1 || die "postfix reload failed"
