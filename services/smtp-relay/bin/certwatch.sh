#!/bin/sh
# certwatch.sh — the image's certificate watcher: runs cert-reload.sh when
# the mounted certificate or key changes.
#
# Polls a checksum every CERT_CHECK_INTERVAL seconds (default 300) instead of
# using inotify: ACME clients replace files through symlinks, and bind mounts
# do not always pass inotify events into a container. A checksum sees every
# kind of change and needs no extra package.
set -eu

SMTP_RELAY_HOME=${SMTP_RELAY_HOME:-/usr/local/share/smtp-relay}
# shellcheck source=lib.sh
. "$SMTP_RELAY_HOME/bin/lib.sh"
# shellcheck source=cert.sh
. "$SMTP_RELAY_HOME/bin/cert.sh"

cert=${TLS_CERT_FILE:-/etc/smtp-relay/tls/fullchain.pem}
key=${TLS_KEY_FILE:-/etc/smtp-relay/tls/privkey.pem}
interval=${CERT_CHECK_INTERVAL:-300}
isUInt "$interval" && [ "$interval" -ge 10 ] || die "CERT_CHECK_INTERVAL must be a whole number of seconds, at least 10"

last=$(certFingerprint "$cert" "$key")
while :; do
	sleep "$interval"
	now=$(certFingerprint "$cert" "$key")
	[ "$now" != "$last" ] || continue
	# Remember the new state either way: a broken pair is reported once,
	# not every interval, and the next good one is picked up.
	last=$now
	"$SMTP_RELAY_HOME/bin/cert-reload.sh" || true
done
