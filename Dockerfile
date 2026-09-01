# SSG - Static Site Generator
# Multi-stage build for minimal image size

# Stage 1: Build
# Pinned to 1.27.0-alpine. Not below 1.26.7: earlier 1.26.x stdlib carries
# advisories this code reaches (encoding/asn1 GO-2026-5972, net/http
# GO-2026-5026).
#
# Cross-compile from the native BUILDPLATFORM to the requested TARGETPLATFORM so
# ARM images build fast without QEMU emulation (supports amd64, arm64 and armv7).
FROM --platform=$BUILDPLATFORM golang:1.27.0-alpine@sha256:4c9fe60190a2a3350ddc51de80d0224b8a6698d12bdfc999fee45ea9d6c46dbc AS builder

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
    go build -ldflags="-s -w -X main.Version=1.8.53" -o ssg ./cmd/ssg

# Stage 2: Minimal runtime image
# Pinned by digest as well as tag: a tag is a moving target, so a digest is
# what makes a build reproducible. Dependabot bumps both (.github/dependabot.yml)
# — a digest nobody updates stops receiving security patches silently.
FROM alpine:3.24@sha256:28bd5fe8b56d1bd048e5babf5b10710ebe0bae67db86916198a6eec434943f8b

# Install runtime dependencies (cwebp)
RUN apk add --no-cache libwebp-tools

# Labels
LABEL org.opencontainers.image.title="SSG - Static Site Generator"
LABEL org.opencontainers.image.description="Fast static site generator written in Go"
LABEL org.opencontainers.image.version="1.8.53"
LABEL org.opencontainers.image.source="https://github.com/spagu/ssg"
LABEL org.opencontainers.image.licenses="BSD-3-Clause"
LABEL maintainer="spagu <spagu@github.com>"

# Create non-root user
RUN adduser -D -u 1000 ssg

# Copy binary from builder
COPY --from=builder /build/ssg /usr/local/bin/ssg

# Set working directory
WORKDIR /site

# Change ownership
RUN chown -R ssg:ssg /site

# Switch to non-root user
USER ssg

# Health check: ssg is a batch CLI (no long-running process by default), so the
# check verifies the binary is present and runnable (OPS-004).
HEALTHCHECK --interval=30s --timeout=5s --retries=3 CMD ["ssg", "--version"]

# Default command shows help
ENTRYPOINT ["ssg"]
CMD ["--help"]
