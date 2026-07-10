# SSG - Static Site Generator
# Multi-stage build for minimal image size

# Stage 1: Build
# Pinned to 1.26.5-alpine: go1.26.4 stdlib crypto/tls is affected by
# GO-2026-5856 (Encrypted Client Hello privacy leak), fixed in go1.26.5.
FROM golang:1.26.5-alpine AS builder

WORKDIR /build

# Install build dependencies
RUN apk add --no-cache git

# Copy go mod files first for better caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build static binary
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w -X main.Version=1.8.0" \
    -o ssg ./cmd/ssg

# Stage 2: Minimal runtime image
FROM alpine:3.23

# Install runtime dependencies (cwebp)
RUN apk add --no-cache libwebp-tools

# Labels
LABEL org.opencontainers.image.title="SSG - Static Site Generator"
LABEL org.opencontainers.image.description="Fast static site generator written in Go"
LABEL org.opencontainers.image.version="1.8.0"
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
