#!/bin/sh
# entrypoint.sh — the container's main process, under tini (PID 1).
#
# Runs three long-lived processes and supervises them the simplest way that
# is still correct: if any one exits, the others are stopped and the
# container exits non-zero, so Docker's restart policy starts everything
# again from a clean state. Postfix's own master already restarts its
# daemons; this script only has to watch master, OpenDKIM and the
# certificate watcher. SIGTERM/SIGINT (docker stop) stop Postfix gracefully.
set -eu

SMTP_RELAY_HOME=${SMTP_RELAY_HOME:-/usr/local/share/smtp-relay}
# shellcheck source=lib.sh
. "$SMTP_RELAY_HOME/bin/lib.sh"

# A read-only root file system mounts /etc/postfix as an empty tmpfs: put the
# distribution's files (postfix-files, dynamicmaps.cf, ...) back first.
if [ ! -e /etc/postfix/postfix-files ]; then
	cp -a "$SMTP_RELAY_HOME/postfix-dist/." /etc/postfix/
fi
# The same for Postfix's data directory (master.lock, TLS caches): a fresh
# tmpfs belongs to root, and Postfix will not start until it is its own.
mkdir -p /var/lib/postfix
chown postfix:postfix /var/lib/postfix
chmod 700 /var/lib/postfix

"$SMTP_RELAY_HOME/bin/render-config.sh"

mkdir -p /run/opendkim
chown opendkim:opendkim /run/opendkim

# OpenDKIM only logs to syslog; busybox syslogd forwards it to stdout.
# Postfix logs to stdout itself (maillog_file).
syslogd -n -O - &
syslogPid=$!
opendkim -f -x "${OPENDKIM_CONF:-/etc/opendkim/opendkim.conf}" &
dkimPid=$!
"$SMTP_RELAY_HOME/bin/certwatch.sh" &
watchPid=$!
postfix start-fg &
postfixPid=$!

stopping=no
shutdown() {
	stopping=yes
	log "stopping"
	postfix stop >/dev/null 2>&1 || true
	kill "$dkimPid" "$watchPid" 2>/dev/null || true
	wait "$postfixPid" "$dkimPid" "$watchPid" 2>/dev/null || true
	kill "$syslogPid" 2>/dev/null || true
}
trap 'shutdown; exit 0' TERM INT

# alive PID: the process exists and is not a zombie waiting to be reaped.
# (busybox ash's "wait -n" waits for every job, so it cannot be used here.)
alive() { [ -r "/proc/$1/stat" ] && ! grep -q '^[0-9]* (.*) Z ' "/proc/$1/stat"; }

# Sleeping in a background job keeps the shell interruptible: a trapped
# SIGTERM runs shutdown at once instead of after the sleep.
while alive "$postfixPid" && alive "$dkimPid" && alive "$watchPid" && alive "$syslogPid"; do
	sleep 2 &
	wait $! || true
done
[ "$stopping" = yes ] && exit 0
log "a supervised process exited; stopping the rest so the container restarts"
shutdown
exit 1
