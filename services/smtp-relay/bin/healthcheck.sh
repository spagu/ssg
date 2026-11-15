#!/bin/sh
# healthcheck.sh — Docker HEALTHCHECK. Healthy means: Postfix's master runs,
# OpenDKIM runs (without it mail is deferred, not sent) and the certificate
# has not expired. It opens no SMTP connection, so it adds nothing to the log.
set -eu
postfix status >/dev/null 2>&1 || { echo "postfix is not running"; exit 1; }
pid=$(cat /run/opendkim/opendkim.pid 2>/dev/null || true)
if [ -z "$pid" ] || ! kill -0 "$pid" 2>/dev/null; then
	echo "opendkim is not running"
	exit 1
fi
openssl x509 -in "${TLS_CERT_FILE:-/etc/smtp-relay/tls/fullchain.pem}" -noout -checkend 0 >/dev/null 2>&1 ||
	{ echo "the TLS certificate has expired"; exit 1; }
