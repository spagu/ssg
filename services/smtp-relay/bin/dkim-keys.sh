#!/bin/sh
# DKIM keys (sourced after lib.sh). One RSA key per domain, created on the
# first start in STATE_DIR/dkim and kept there: losing it means publishing a
# new DNS record, so back the directory up.

# dkimRecord KEYFILE: prints the TXT value for KEYFILE's public key, split
# into quoted strings of at most 255 characters (RFC 1035 3.3.14, RFC 6376
# 3.6.2.2): a 2048-bit key does not fit in one.
dkimRecord() {
	pub=$(openssl pkey -in "$1" -pubout -outform DER 2>/dev/null | openssl base64 -A) ||
		die "cannot read the DKIM key $1"
	printf '"v=DKIM1; k=rsa; "'
	pub="p=$pub"
	while [ -n "$pub" ]; do
		chunk=$(printf '%s' "$pub" | cut -c1-250)
		pub=$(printf '%s' "$pub" | cut -c251-)
		printf ' "%s"' "$chunk"
	done
	printf '\n'
}

# ensureDkimKeys: creates missing keys, writes the OpenDKIM key and signing
# tables into RUN_DIR and prints the DNS record of every key.
ensureDkimKeys() {
	keyDir="$STATE_DIR/dkim"
	mkdir -p "$keyDir"
	chown "$OPENDKIM_USER:$OPENDKIM_USER" "$keyDir"
	chmod 700 "$keyDir"
	keyTable=''
	signingTable=''
	for domain in $(splitList "$DKIM_DOMAINS"); do
		key="$keyDir/$domain.$DKIM_SELECTOR.key"
		if [ ! -s "$key" ]; then
			log "creating a $DKIM_KEY_BITS-bit DKIM key for $domain (selector $DKIM_SELECTOR)"
			(umask 077 && openssl genpkey -algorithm RSA -pkeyopt "rsa_keygen_bits:$DKIM_KEY_BITS" -out "$key.tmp" 2>/dev/null) ||
				die "openssl could not create $key"
			mv -f "$key.tmp" "$key"
		fi
		chown "$OPENDKIM_USER:$OPENDKIM_USER" "$key"
		chmod 600 "$key"
		record="$DKIM_SELECTOR._domainkey.$domain. IN TXT ( $(dkimRecord "$key") )"
		printf '%s\n' "$record" >"$keyDir/$domain.$DKIM_SELECTOR.dns.txt"
		log "DKIM DNS record for $domain — publish it once, unchanged:"
		printf '%s\n' "$record" >&2
		keyTable="$keyTable$domain $domain:$DKIM_SELECTOR:$key
"
		signingTable="$signingTable$domain $domain
"
	done
	printf '%s' "$keyTable" | installFile 644 "root:root" "$RUN_DIR/dkim-keytable"
	printf '%s' "$signingTable" | installFile 644 "root:root" "$RUN_DIR/dkim-signingtable"
}
