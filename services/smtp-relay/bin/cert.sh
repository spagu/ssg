#!/bin/sh
# Certificate checks (sourced after lib.sh). Used at start, where a failure
# stops the server, and by certwatch.sh, where a failure keeps the old,
# still-loaded certificate.

# checkCert CERT KEY HOSTNAME: returns 0 when CERT and KEY are a usable pair.
# Prints why not otherwise. A name mismatch is only a warning: some people
# connect by a different name than the one in HELO.
checkCert() {
	if [ ! -r "$1" ] || [ ! -s "$1" ]; then
		log "no TLS certificate at $1 — mount one (see docs/SMTP-SERVER.md, \"Certificate\"); refusing to offer AUTH without TLS"
		return 1
	fi
	if [ ! -r "$2" ] || [ ! -s "$2" ]; then
		log "no TLS private key at $2"
		return 1
	fi
	if ! certPub=$(openssl x509 -in "$1" -noout -pubkey 2>/dev/null); then
		log "$1 is not a PEM certificate"
		return 1
	fi
	if ! keyPub=$(openssl pkey -in "$2" -pubout 2>/dev/null); then
		log "$2 is not a PEM private key (or it is encrypted)"
		return 1
	fi
	if [ "$certPub" != "$keyPub" ]; then
		log "the private key $2 does not belong to the certificate $1"
		return 1
	fi
	if ! openssl x509 -in "$1" -noout -checkend 0 >/dev/null 2>&1; then
		log "the certificate $1 has expired"
		return 1
	fi
	if ! openssl x509 -in "$1" -noout -checkend 1209600 >/dev/null 2>&1; then
		log "warning: the certificate $1 expires within 14 days — is renewal running?"
	fi
	if ! openssl x509 -in "$1" -noout -checkhost "$3" 2>/dev/null | grep -q 'does match'; then
		log "warning: the certificate $1 does not name $3; clients connecting to $3 will reject it"
	fi
	return 0
}

# certFingerprint CERT KEY: one line that changes whenever either file does.
certFingerprint() { cat "$1" "$2" 2>/dev/null | openssl dgst -sha256 2>/dev/null; }
