# SSG - Static Site Generator
# Multi-stage build for minimal image size

# Stage 1: Build
# Pinned to 1.27.3-alpine. Not below 1.27.3: earlier releases carry stdlib
# advisories this code reaches (crypto/x509: GO-2026-6809, fixed in 1.27.3;
# net/http, HTTP/2, crypto/tls and html/template: GO-2026-6599..6613, fixed in
# 1.27.2).
#
# Cross-compile from the native BUILDPLATFORM to the requested TARGETPLATFORM so
# ARM images build fast without QEMU emulation (supports amd64, arm64 and armv7).
# Tag AND digest, deliberately, and SonarCloud's S8431 will keep asking for one
# or the other. The digest is what makes the build reproducible; the tag is what
# lets Dependabot see a newer one exists and open the bump. Keeping only the
# digest means nobody is told when it stops receiving security patches.
#
# The annotation that would silence the rule cannot go here: a trailing comment
# on a FROM line is parsed as extra arguments and fails the build.
FROM --platform=$BUILDPLATFORM golang:1.27.3-alpine@sha256:f2c872bcee949b7be9c490efd02b48a8a2fbf947cdf533f1c58525ec9d8170fd AS builder

# Provided automatically by buildx.
ARG TARGETARCH
ARG TARGETVARIANT

WORKDIR /build

# Install build dependencies
RUN apk add --no-cache git

# Copy go mod files first for better caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build a static binary for the target architecture (GOARM derived from the
# buildx variant, e.g. "v7" -> GOARM=7 for 32-bit ARM).
RUN CGO_ENABLED=0 GOOS=linux GOARCH="${TARGETARCH}" GOARM="${TARGETVARIANT#v}" \
    go build -ldflags="-s -w -X main.Version=1.8.77" -o ssg ./cmd/ssg

# Stage 2: Minimal runtime image
# Pinned by digest as well as tag: a tag is a moving target, so a digest is
# what makes a build reproducible. Dependabot bumps both (.github/dependabot.yml)
# — a digest nobody updates stops receiving security patches silently.
# Tag and digest together, for the reason above the builder stage.
FROM alpine:3.24@sha256:294b683cb724975bec92580e1e685676bd4b50bda910ddb8c51d4cabeaec77e6

# Runtime dependencies (cwebp), the non-root user and its working directory in
# ONE layer: three RUNs became one, and the directory is created already owned
# by ssg instead of being chown'd afterwards (OPS-014).
RUN apk add --no-cache libwebp-tools \
    && adduser -D -u 1000 ssg \
    && install -d -o ssg -g ssg /site

# Labels
LABEL org.opencontainers.image.title="SSG - Static Site Generator"
LABEL org.opencontainers.image.description="Fast static site generator written in Go"
LABEL org.opencontainers.image.version="1.8.77"
LABEL org.opencontainers.image.source="https://github.com/spagu/ssg"
LABEL org.opencontainers.image.licenses="BSD-3-Clause"
LABEL maintainer="spagu <spagu@github.com>"

# Copy binary from builder — root-owned and world-executable, which is what a
# binary in /usr/local/bin should be; no chown needed.
COPY --from=builder /build/ssg /usr/local/bin/ssg

# Set working directory (created above, already owned by ssg)
WORKDIR /site

# Switch to non-root user
USER ssg

# Health check: ssg is a batch CLI (no long-running process by default), so the
# check verifies the binary is present and runnable (OPS-004).
HEALTHCHECK --interval=30s --timeout=5s --retries=3 CMD ["ssg", "--version"]

# Default command shows help
ENTRYPOINT ["ssg"]
CMD ["--help"]
