#!/bin/sh
# smoke.sh IMAGE — end-to-end check of a built smtp-relay image.
#
# Starts the image in RELAY_MODE=hold (nothing leaves the container) with a
# throw-away self-signed certificate and checks, against the real server:
#   TLS on 465 and STARTTLS on 587; AUTH offered only after TLS; a login and
#   one message on each port; a wrong password refused; no relaying without a
#   login (open-relay test); a foreign sender refused; the held message carries
#   a DKIM-Signature that verifies and covers List-Unsubscribe(-Post).
# Needs docker, openssl and python3. Removes everything it creates.
# SMOKE_PLATFORM=linux/arm64 runs a foreign-architecture image under QEMU.
set -eu

image=${1:?usage: smoke.sh IMAGE}
here=$(cd "$(dirname "$0")" && pwd)
work=$(mktemp -d)
name="smtp-relay-smoke-$$"
host=relay.example.com
fail() {
	echo "FAIL: $*" >&2
	docker logs "$name" >&2 2>&1 || true
	exit 1
}
cleanup() {
	docker rm -f -v "$name" >/dev/null 2>&1 || true
	rm -rf "$work"
}
trap cleanup EXIT INT TERM

openssl req -x509 -newkey rsa:2048 -nodes -days 2 -subj "/CN=$host" \
	-addext "subjectAltName=DNS:$host" -keyout "$work/privkey.pem" -out "$work/fullchain.pem" 2>/dev/null
password=$(openssl rand -base64 24)
printf '%s\n' "$password" >"$work/password"
chmod 644 "$work"/*

# shellcheck disable=SC2086 # the optional --platform flag is meant to split
docker run -d --name "$name" ${SMOKE_PLATFORM:+--platform $SMOKE_PLATFORM} \
	-p 127.0.0.1::465 -p 127.0.0.1::587 \
	-e SMTP_HOSTNAME="$host" -e RELAY_MODE=hold -e DKIM_DOMAINS=example.com \
	-e SMTP_USER=newsletter -e SMTP_PASSWORD_FILE=/run/secrets/smtp_password \
	-v "$work/password:/run/secrets/smtp_password:ro" \
	-v "$work/fullchain.pem:/etc/smtp-relay/tls/fullchain.pem:ro" \
	-v "$work/privkey.pem:/etc/smtp-relay/tls/privkey.pem:ro" \
	"$image" >/dev/null

for _ in $(seq 1 60); do
	state=$(docker inspect -f '{{.State.Health.Status}}' "$name" 2>/dev/null || echo gone)
	[ "$state" = healthy ] && break
	[ "$state" = gone ] && fail "container exited"
	sleep 1
done
[ "$state" = healthy ] || fail "not healthy after 60 s ($state)"
echo "ok: container healthy"

port465=$(docker port "$name" 465/tcp | head -1 | sed 's/.*://')
port587=$(docker port "$name" 587/tcp | head -1 | sed 's/.*://')

# openssl s_client: the handshake and what the server offers after it.
ehlo465=$( (sleep 2; printf 'EHLO client.example.com\r\n'; sleep 1; printf 'QUIT\r\n') |
	openssl s_client -connect "127.0.0.1:$port465" -servername "$host" -crlf -quiet 2>&1 || true)
printf '%s\n' "$ehlo465" | grep -q '^250-AUTH PLAIN LOGIN' || fail "465: AUTH not offered after TLS: $ehlo465"
echo "ok: 465 implicit TLS, EHLO offers AUTH PLAIN LOGIN"
ehlo587=$( (sleep 2; printf 'EHLO client.example.com\r\n'; sleep 1; printf 'QUIT\r\n') |
	openssl s_client -connect "127.0.0.1:$port587" -starttls smtp -servername "$host" -crlf -quiet 2>&1 || true)
printf '%s\n' "$ehlo587" | grep -q '^250-AUTH PLAIN LOGIN' || fail "587: AUTH not offered after STARTTLS: $ehlo587"
echo "ok: 587 STARTTLS, EHLO offers AUTH PLAIN LOGIN"
protocol=$(openssl s_client -connect "127.0.0.1:$port465" -servername "$host" </dev/null 2>/dev/null | grep -E '^ *Protocol *:' | head -1 || true)
echo "ok: negotiated ${protocol:-?}"

python3 -I "$here/smtp_checks.py" "$port465" "$port587" "$work/password" || fail "SMTP checks"

queueId=$(docker exec "$name" postqueue -j | python3 -I -c 'import json,sys; print(json.loads(sys.stdin.readline())["queue_id"])') ||
	fail "no message in the queue"
docker exec "$name" postqueue -p | grep -q "^$queueId!" || fail "message $queueId is not on hold"
echo "ok: message $queueId is on hold (nothing left the container)"
docker exec "$name" postcat -hbq "$queueId" | sed '/^\*\*\* /d' >"$work/message.eml"
grep '^DKIM-Signature:' "$work/message.eml" | head -1
docker exec "$name" sh -c 'openssl pkey -in /var/lib/smtp-relay/dkim/example.com.relay.key -pubout' >"$work/dkim.pub"
verdict=$(python3 -I "$here/dkim_verify.py" "$work/message.eml" "$work/dkim.pub") || fail "DKIM verification: $verdict"
printf '%s\n' "$verdict"
printf '%s\n' "$verdict" | grep -qi 'list-unsubscribe-post' || fail "the signature does not cover List-Unsubscribe-Post"
echo "ok: DKIM-Signature verifies and covers List-Unsubscribe and List-Unsubscribe-Post"
# The verifier must also be able to say no: change one signed header.
sed 's/^Subject: smoke test/Subject: smoke tesT/' "$work/message.eml" >"$work/tampered.eml"
if python3 -I "$here/dkim_verify.py" "$work/tampered.eml" "$work/dkim.pub" >/dev/null; then
	fail "the DKIM check accepted a tampered message"
fi
echo "ok: a tampered Subject fails DKIM verification"
echo "PASS"
