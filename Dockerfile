# SSG - Static Site Generator
# Multi-stage build for minimal image size

# Stage 1: Build
# Stage 1: Build
FROM golang:1.25-alpine AS builder

WORKDIR /build

# Install build dependencies
RUN apk add --no-cache git

# Copy go mod files first for better caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build static binary
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w -X main.Version=1.3.1" \
    -o ssg ./cmd/ssg

# Stage 2: Minimal runtime image
FROM alpine:3.19

# Install runtime dependencies (cwebp)
RUN apk add --no-cache libwebp-tools

# Labels
LABEL org.opencontainers.image.title="SSG - Static Site Generator"
LABEL org.opencontainers.image.description="Fast static site generator written in Go"
LABEL org.opencontainers.image.version="1.3.1"
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

# Default command shows help
ENTRYPOINT ["ssg"]
CMD ["--help"]
