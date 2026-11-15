#!/bin/sh
# render-config.sh — writes Postfix, Cyrus SASL and OpenDKIM configuration
# from config/*.tmpl and the environment. The image runs it on every start;
# a native install (docs/SMTP-SERVER-PI.md) runs it from a systemd unit before
# Postfix and OpenDKIM. Same templates, same checks, both ways.
#
# Every setting is an environment variable (see README.md, "Configuration");
# passwords are read from files named by *_FILE variables. Anything missing
# or malformed stops here with a message: the server never starts half
# configured.
#
# Many variables below are only read by render(), through eval.
# shellcheck disable=SC2034
set -eu

SMTP_RELAY_HOME=${SMTP_RELAY_HOME:-/usr/local/share/smtp-relay}
# shellcheck source=lib.sh
. "$SMTP_RELAY_HOME/bin/lib.sh"
# shellcheck source=cert.sh
. "$SMTP_RELAY_HOME/bin/cert.sh"
# shellcheck source=dkim-keys.sh
. "$SMTP_RELAY_HOME/bin/dkim-keys.sh"

configDir="$SMTP_RELAY_HOME/config"

# ---------------------------------------------------------------- paths
POSTFIX_DIR=${POSTFIX_DIR:-/etc/postfix}
OPENDKIM_CONF=${OPENDKIM_CONF:-/etc/opendkim/opendkim.conf}
OPENDKIM_USER=${OPENDKIM_USER:-opendkim}
POSTFIX_GROUP=${POSTFIX_GROUP:-postfix}
RUN_DIR=${RUN_DIR:-/run/smtp-relay}
STATE_DIR=${STATE_DIR:-/var/lib/smtp-relay}
LOG_TO_STDOUT=${LOG_TO_STDOUT:-no}
for p in POSTFIX_DIR OPENDKIM_CONF RUN_DIR STATE_DIR; do
	eval "v=\$$p"
	# shellcheck disable=SC2154 # set by the eval above
	isPath "$v" || die "$p must be an absolute path, got '$v'"
done

# ---------------------------------------------------------------- identity
SMTP_HOSTNAME=${SMTP_HOSTNAME:-}
isHostname "$SMTP_HOSTNAME" ||
	die "SMTP_HOSTNAME must be this server's fully qualified name (the one its PTR record and certificate name), got '$SMTP_HOSTNAME'"
INET_PROTOCOLS=${INET_PROTOCOLS:-ipv4}
case "$INET_PROTOCOLS" in ipv4 | ipv6 | all) ;; *) die "INET_PROTOCOLS must be ipv4, ipv6 or all" ;; esac

SMTP_USER=${SMTP_USER:-}
isLogin "$SMTP_USER" || die "SMTP_USER must be 1-64 characters of A-Z a-z 0-9 . _ + @ -, got '$SMTP_USER'"
smtpPassword=$(readSecret SMTP_PASSWORD)
[ "${#smtpPassword}" -ge 16 ] ||
	die "the password in SMTP_PASSWORD_FILE is shorter than 16 characters; this login faces the internet (try: openssl rand -base64 24)"

# ---------------------------------------------------------------- relay mode
RELAY_MODE=${RELAY_MODE:-}
RELAYHOST=${RELAYHOST:-}
RELAYHOST_WRAPPERMODE=no
RELAYHOST_AUTH=no
case "$RELAY_MODE" in
direct | hold) ;;
smarthost)
	hostPart=$(printf '%s' "$RELAYHOST" | sed -E 's/^\[([^]]*)\](:[0-9]+)?$/\1/')
	if ! printf '%s' "$RELAYHOST" | grep -Eq '^\[[A-Za-z0-9.-]+\](:[0-9]{1,5})?$' || ! isHostname "$hostPart"; then
		die "RELAYHOST must look like [smtp.example.net]:587 (brackets: no MX lookup), got '$RELAYHOST'"
	fi
	case "$RELAYHOST" in *:25) die "RELAYHOST on port 25 sends the login in a network that blocks 25 anyway; use 587 or 465" ;; esac
	case "$RELAYHOST" in *:465) RELAYHOST_WRAPPERMODE=yes ;; esac
	if [ -n "${RELAYHOST_USER:-}" ]; then
		isLogin "$RELAYHOST_USER" || die "RELAYHOST_USER has characters outside A-Z a-z 0-9 . _ + @ -"
		RELAYHOST_AUTH=yes
	fi
	;;
*) die "RELAY_MODE must be direct, smarthost or hold (see docs/SMTP-SERVER.md, \"Choosing a relay mode\")" ;;
esac

# ---------------------------------------------------------------- DKIM and senders
DKIM_DOMAINS=${DKIM_DOMAINS:-}
[ -n "$DKIM_DOMAINS" ] || die "DKIM_DOMAINS must list the domain(s) you send from, e.g. example.com"
for d in $(splitList "$DKIM_DOMAINS"); do isHostname "$d" || die "DKIM_DOMAINS: '$d' is not a domain name"; done
DKIM_SELECTOR=${DKIM_SELECTOR:-relay}
isSelector "$DKIM_SELECTOR" || die "DKIM_SELECTOR must be letters, digits and hyphens, got '$DKIM_SELECTOR'"
requireUInt DKIM_KEY_BITS 2048
[ "$DKIM_KEY_BITS" -ge 2048 ] && [ "$DKIM_KEY_BITS" -le 4096 ] || die "DKIM_KEY_BITS must be 2048-4096 (RFC 8301 sets 1024 as the floor; 2048 is the common choice)"
SENDER_DOMAINS=${SENDER_DOMAINS:-$DKIM_DOMAINS}
for d in $(splitList "$SENDER_DOMAINS"); do isHostname "$d" || die "SENDER_DOMAINS: '$d' is not a domain name"; done

# ---------------------------------------------------------------- TLS
TLS_CERT_FILE=${TLS_CERT_FILE:-/etc/smtp-relay/tls/fullchain.pem}
TLS_KEY_FILE=${TLS_KEY_FILE:-/etc/smtp-relay/tls/privkey.pem}
if ! isPath "$TLS_CERT_FILE" || ! isPath "$TLS_KEY_FILE"; then die "TLS_CERT_FILE and TLS_KEY_FILE must be absolute paths"; fi
checkCert "$TLS_CERT_FILE" "$TLS_KEY_FILE" "$SMTP_HOSTNAME" || die "no usable TLS certificate; not starting"

# ---------------------------------------------------------------- limits
requireUInt LIMIT_CONNECTIONS_PER_MINUTE 60
requireUInt LIMIT_CONCURRENT_CONNECTIONS 10
requireUInt LIMIT_MESSAGES_PER_MINUTE 600
requireUInt LIMIT_RECIPIENTS_PER_MINUTE 600
requireUInt LIMIT_AUTH_PER_MINUTE 30
requireUInt MESSAGE_SIZE_LIMIT 10240000
requireUInt OUTBOUND_CONCURRENCY 5
requireUInt PROCESS_LIMIT 20
QUEUE_LIFETIME=${QUEUE_LIFETIME:-2d}
isDuration "$QUEUE_LIFETIME" || die "QUEUE_LIFETIME must look like 2d or 36h"

COMPATIBILITY_LEVEL=$(postconf -dh mail_version | cut -d. -f1,2)
RECIPIENT_DOMAIN_CHECK='reject_unknown_recipient_domain,'
[ "$RELAY_MODE" != hold ] || RECIPIENT_DOMAIN_CHECK=''

# ---------------------------------------------------------------- write
mkdir -p "$RUN_DIR" "$POSTFIX_DIR/sasl" "$STATE_DIR" "$(dirname "$OPENDKIM_CONF")"
chmod 755 "$RUN_DIR"

{
	render "$configDir/main.cf.tmpl"
	render "$configDir/main.cf.$RELAY_MODE.tmpl"
	[ "$LOG_TO_STDOUT" != yes ] || render "$configDir/main.cf.stdout.tmpl"
} | installFile 644 root:root "$POSTFIX_DIR/main.cf"
installFile 644 root:root "$POSTFIX_DIR/master.cf" <"$configDir/master.cf"
render "$configDir/sasl-smtpd.conf.tmpl" | installFile 644 root:root "$POSTFIX_DIR/sasl/smtpd.conf"

# The sasldb is rebuilt every time, so a changed or removed password takes
# effect on the next start and nothing stale survives. smtpd reads it after
# dropping to the postfix user, hence the group.
rm -f "$RUN_DIR/sasldb2" "$RUN_DIR/sasldb2".*
printf '%s' "$smtpPassword" | saslpasswd2 -p -c -f "$RUN_DIR/sasldb2" -u "$SMTP_HOSTNAME" "$SMTP_USER" ||
	die "saslpasswd2 could not write $RUN_DIR/sasldb2"
for f in "$RUN_DIR"/sasldb2*; do
	chown "root:$POSTFIX_GROUP" "$f"
	chmod 640 "$f"
done

splitList "$SENDER_DOMAINS" | sed 's/$/ OK/' | installFile 644 root:root "$RUN_DIR/sender-domains"

if [ "$RELAY_MODE" = smarthost ] && [ "$RELAYHOST_AUTH" = yes ]; then
	# Postfix opens this table before dropping privileges: root-only is fine.
	relayPassword=$(readSecret RELAYHOST_PASSWORD)
	printf '%s %s:%s\n' "$RELAYHOST" "$RELAYHOST_USER" "$relayPassword" | installFile 600 root:root "$RUN_DIR/relay-password"
else
	: | installFile 600 root:root "$RUN_DIR/relay-password"
fi

render "$configDir/opendkim.conf.tmpl" | installFile 644 root:root "$OPENDKIM_CONF"
ensureDkimKeys

# postfix check validates main.cf/master.cf and creates or repairs the queue
# directories (a fresh volume) with the ownership Postfix wants.
postfix check || die "postfix check failed — see the lines above"
log "configuration written: $SMTP_HOSTNAME, RELAY_MODE=$RELAY_MODE, DKIM $DKIM_SELECTOR for $(splitList "$DKIM_DOMAINS" | tr '\n' ' ')"
