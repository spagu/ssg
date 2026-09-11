#!/bin/bash
# =============================================================================
# SSG Quick Installer
# Usage: curl -sSL https://raw.githubusercontent.com/spagu/ssg/main/install.sh | bash
#
# What it checks before it installs anything (SEC-013):
#   - the download is fetched over HTTPS only, and an HTTP error is an error
#     (a 404 page is not a tarball);
#   - the tarball's SHA-256 is verified against the release's checksums.sha256,
#     which the release workflow publishes beside every binary — a corrupted or
#     substituted download stops here, before sudo is ever asked for;
#   - `set -u` and `pipefail`, so an unset variable or a failed download in a
#     pipe cannot be quietly treated as success.
# =============================================================================
set -euo pipefail

VERSION="${SSG_VERSION:-1.8.61}"
INSTALL_DIR="${SSG_INSTALL_DIR:-/usr/local/bin}"
RELEASE_URL="https://github.com/spagu/ssg/releases/download/v${VERSION}"

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

log_info() { local message="$1"; echo -e "${BLUE}[INFO]${NC} ${message}"; }
log_success() { local message="$1"; echo -e "${GREEN}[OK]${NC} ${message}"; }
log_warn() { local message="$1"; echo -e "${YELLOW}[WARN]${NC} ${message}" >&2; }
log_error() { local message="$1"; echo -e "${RED}[ERROR]${NC} ${message}" >&2; exit 1; }

TMP_DIR=""
cleanup() { if [[ -n "$TMP_DIR" ]]; then rm -rf "$TMP_DIR"; fi; }
trap cleanup EXIT

detect_platform() {
    OS=$(uname -s | tr '[:upper:]' '[:lower:]')
    ARCH=$(uname -m)
    
    case "$ARCH" in
        x86_64|amd64) ARCH="amd64" ;;
        aarch64|arm64) ARCH="arm64" ;;
        *) log_error "Unsupported architecture: $ARCH" ;;
    esac
    
    case "$OS" in
        linux|darwin|freebsd|openbsd) ;;
        mingw*|msys*|cygwin*) OS="windows" ;;
        *) log_error "Unsupported OS: $OS" ;;
    esac
    
    log_info "Detected platform: $OS/$ARCH"
}

# fetch downloads one release asset to a path; HTTPS only, no HTTP fallback,
# and an HTTP error status fails instead of saving the error page.
fetch() {
    local url="$1" dest="$2"
    curl --proto '=https' --proto-redir =https --fail -sSL "${url}" -o "${dest}"
}

# sha256_of prints the SHA-256 of a file with whichever tool the platform has.
sha256_of() {
    local file="$1"
    if command -v sha256sum &> /dev/null; then
        sha256sum "${file}" | awk '{print $1}'
    elif command -v shasum &> /dev/null; then
        shasum -a 256 "${file}" | awk '{print $1}'
    else
        return 1
    fi
}

# verify_checksum compares the tarball against the release's checksums.sha256.
# A missing checksum tool is a warning and a continue — the download is still
# HTTPS-only from the release — but a MISMATCH is always fatal.
verify_checksum() {
    local archive="$1" name="$2" sums="$TMP_DIR/checksums.sha256"
    if ! fetch "${RELEASE_URL}/checksums.sha256" "$sums"; then
        log_error "Could not download checksums.sha256 for v${VERSION}; refusing to install an unverified binary"
    fi
    local expected
    expected=$(awk -v n="$name" '$2 == n {print $1}' "$sums")
    [[ -n "$expected" ]] || log_error "checksums.sha256 for v${VERSION} has no entry for ${name}"
    local actual
    if ! actual=$(sha256_of "$archive"); then
        log_warn "No sha256sum or shasum on this system — skipping checksum verification"
        return 0
    fi
    [[ "$actual" == "$expected" ]] || log_error "Checksum mismatch for ${name}: expected ${expected}, got ${actual}"
    log_success "Checksum verified"
}

download_and_install() {
    local name="ssg-${OS}-${ARCH}.tar.gz"
    TMP_DIR=$(mktemp -d)

    command -v curl &> /dev/null || log_error "curl is required"

    log_info "Downloading SSG v${VERSION}..."
    fetch "${RELEASE_URL}/${name}" "$TMP_DIR/$name" \
        || log_error "Download failed: ${RELEASE_URL}/${name} (does release v${VERSION} exist for ${OS}/${ARCH}?)"

    verify_checksum "$TMP_DIR/$name" "$name"

    log_info "Extracting..."
    tar -xzf "$TMP_DIR/$name" -C "$TMP_DIR"
    [[ -f "$TMP_DIR/ssg" ]] || log_error "The archive did not contain an ssg binary"

    log_info "Installing to $INSTALL_DIR..."
    # install(1) sets the mode in the same step as the copy, so there is no
    # window where the binary sits in place without its permissions.
    if [[ -w "$INSTALL_DIR" ]]; then
        install -m 0755 "$TMP_DIR/ssg" "$INSTALL_DIR/ssg"
    else
        sudo install -m 0755 "$TMP_DIR/ssg" "$INSTALL_DIR/ssg"
    fi

    log_success "SSG v${VERSION} installed successfully!"
}

verify_installation() {
    if command -v ssg &> /dev/null; then
        log_success "SSG is available at: $(which ssg)"
        echo ""
        echo "Quick start:"
        echo "  ssg my-site simple example.com --http --watch"
        echo ""
        echo "Documentation: https://github.com/spagu/ssg"
    else
        log_error "Installation failed. Please check $INSTALL_DIR is in your PATH."
    fi
}

main() {
    echo ""
    echo "================================"
    echo "  SSG Installer v${VERSION}"
    echo "================================"
    echo ""
    
    detect_platform
    download_and_install
    verify_installation
}

main
