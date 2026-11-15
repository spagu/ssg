# syntax=docker/dockerfile:1
#
# smtp-relay: Postfix submission server (465 implicit TLS, 587 STARTTLS, SASL
# login required) with OpenDKIM signing, for the newsletter workers' SMTP
# transport. See README.md and docs/SMTP-SERVER.md.
#
#   docker build -t smtp-relay .
#
# The base image is pinned by tag AND digest, like the repository's own
# Dockerfile: the digest (of the multi-arch index) makes the build
# reproducible, the tag lets Dependabot (.github/dependabot.yml) see a newer
# release. Packages come from that Alpine release's repository; rebuild to
# pick up its security updates.
FROM alpine:3.24.4@sha256:f59abe656b692d901e184be3445e084786602108599d2a51d229c0f5a48c71bf

# postfix          the MTA (links libsasl2 already)
# cyrus-sasl       sasldb plugin + saslpasswd2 (server logins)
# cyrus-sasl-login AUTH LOGIN, for clients that do not offer PLAIN
# opendkim         DKIM signing milter
# openssl          certificate checks and DKIM key generation
# ca-certificates  verifying a smarthost's certificate
# tini             PID 1: forwards signals, reaps zombies
#
# Not pinned to package versions: an Alpine branch keeps only the newest
# build of each package, so a pin breaks the build at the first security
# update. The digest above fixes the release; a rebuild takes its fixes.
# hadolint ignore=DL3018
RUN apk add --no-cache \
      postfix \
      cyrus-sasl \
      cyrus-sasl-login \
      opendkim \
      openssl \
      ca-certificates \
      tini \
 && mkdir -p /usr/local/share/smtp-relay \
 && cp -a /etc/postfix /usr/local/share/smtp-relay/postfix-dist \
 && mkdir -p /etc/smtp-relay/tls /var/lib/smtp-relay \
 && rm -f /etc/postfix/main.cf.proto /etc/postfix/master.cf.proto

COPY bin/ /usr/local/share/smtp-relay/bin/
COPY config/ /usr/local/share/smtp-relay/config/

ENV SMTP_RELAY_HOME=/usr/local/share/smtp-relay \
    LOG_TO_STDOUT=yes \
    TLS_CERT_FILE=/etc/smtp-relay/tls/fullchain.pem \
    TLS_KEY_FILE=/etc/smtp-relay/tls/privkey.pem

ARG VERSION=dev
LABEL org.opencontainers.image.title="smtp-relay" \
      org.opencontainers.image.description="Postfix submission server (465/587, SASL, DKIM) for ssg's newsletter workers" \
      org.opencontainers.image.version="${VERSION}" \
      org.opencontainers.image.source="https://github.com/spagu/ssg/tree/main/services/smtp-relay"

# The queue and the DKIM keys are the only state worth keeping.
VOLUME ["/var/spool/postfix", "/var/lib/smtp-relay"]

# 587 submission (STARTTLS), 465 submissions (implicit TLS). Not 25.
EXPOSE 465 587

# Postfix's master runs as root by design (it binds the ports and starts each
# daemon as the unprivileged "postfix" user); OpenDKIM drops to "opendkim".
STOPSIGNAL SIGTERM
HEALTHCHECK --interval=30s --timeout=5s --start-period=30s --retries=3 \
  CMD ["/usr/local/share/smtp-relay/bin/healthcheck.sh"]
ENTRYPOINT ["/sbin/tini", "--", "/usr/local/share/smtp-relay/bin/entrypoint.sh"]
