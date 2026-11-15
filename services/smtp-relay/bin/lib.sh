#!/bin/sh
# Shared helpers for smtp-relay's scripts (sourced, never run). POSIX sh, so
# the same files run in the Alpine image (busybox ash) and on Debian (dash).

# Where the scripts and templates live: /usr/local/share/smtp-relay in the
# image and in the native (Raspberry Pi) install.
SMTP_RELAY_HOME=${SMTP_RELAY_HOME:-/usr/local/share/smtp-relay}

log() { printf 'smtp-relay: %s\n' "$*" >&2; }
die() {
	log "error: $*"
	exit 1
}

# readSecret prints the content of the file named by the variable "$1_FILE"
# without its trailing newline. Passwords come from files (Docker secrets, a
# root-only file on a Pi), never from the environment.
readSecret() {
	eval "secretPath=\${$1_FILE:-}"
	# shellcheck disable=SC2154 # set by the eval above
	[ -n "$secretPath" ] || die "$1_FILE is not set"
	[ -r "$secretPath" ] || die "$1_FILE: cannot read $secretPath"
	secretValue=$(cat "$secretPath")
	[ -n "$secretValue" ] || die "$1_FILE: $secretPath is empty"
	printf '%s' "$secretValue"
}

# The checks below keep every value inside a known character set, so a value
# can be pasted into a config file by sed without escaping and can never
# smuggle a second setting or a shell command in.
isHostname() {
	printf '%s' "$1" | grep -Eq '^([A-Za-z0-9]([A-Za-z0-9-]{0,61}[A-Za-z0-9])?\.)+[A-Za-z]{2,63}$'
}
isUInt() { printf '%s' "$1" | grep -Eq '^[0-9]{1,9}$'; }
isSelector() { printf '%s' "$1" | grep -Eq '^[A-Za-z0-9]([A-Za-z0-9-]{0,62})$'; }
isLogin() { printf '%s' "$1" | grep -Eq '^[A-Za-z0-9._+@-]{1,64}$'; }
isPath() { printf '%s' "$1" | grep -Eq '^/[A-Za-z0-9._/-]+$'; }
isDuration() { printf '%s' "$1" | grep -Eq '^[0-9]{1,4}[smhdw]$'; }

# requireUInt NAME DEFAULT: sets NAME to its value or DEFAULT; dies when it
# is not a whole number.
requireUInt() {
	eval "uintValue=\${$1:-$2}"
	# shellcheck disable=SC2154 # set by the eval above
	isUInt "$uintValue" || die "$1 must be a whole number, got '$uintValue'"
	eval "$1=\$uintValue"
}

# splitList prints a comma- or space-separated list one item per line.
splitList() { printf '%s\n' "$1" | tr ',' ' ' | tr -s ' ' '\n' | grep -v '^$' || true; }

# render TEMPLATE: prints the template with every @@NAME@@ replaced by the
# value of the shell variable NAME. All names must be set (validated above);
# an unknown placeholder is an error, never an empty value.
render() {
	renderSed=''
	for name in $(grep -o '@@[A-Z_]*@@' "$1" | sort -u | tr -d '@'); do
		eval "renderValue=\${$name-__unset__}"
		# shellcheck disable=SC2154 # set by the eval above
		[ "$renderValue" != __unset__ ] || die "template $1 needs $name"
		renderSed="${renderSed}s|@@${name}@@|${renderValue}|g;"
	done
	sed "$renderSed" "$1"
}

# installFile MODE OWNER:GROUP TARGET: copies stdin to TARGET atomically.
installFile() {
	installTmp="$3.tmp.$$"
	cat >"$installTmp"
	chmod "$1" "$installTmp"
	chown "$2" "$installTmp"
	mv -f "$installTmp" "$3"
}
