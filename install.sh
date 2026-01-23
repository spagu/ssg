#!/bin/bash
# =============================================================================
# SSG Quick Installer
# Usage: curl -sSL https://raw.githubusercontent.com/spagu/ssg/main/install.sh | bash
# =============================================================================
set -e

VERSION="${SSG_VERSION:-1.4.5}"
INSTALL_DIR="${SSG_INSTALL_DIR:-/usr/local/bin}"

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

log_info() { echo -e "${BLUE}[INFO]${NC} $1"; }
log_success() { echo -e "${GREEN}[OK]${NC} $1"; }
log_error() { echo -e "${RED}[ERROR]${NC} $1"; exit 1; }

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

download_and_install() {
    local url="https://github.com/spagu/ssg/releases/download/v${VERSION}/ssg-${VERSION}-${OS}-${ARCH}.tar.gz"
    local tmp_dir=$(mktemp -d)
    
    log_info "Downloading SSG v${VERSION}..."
    
    if command -v curl &> /dev/null; then
        curl -sL "$url" -o "$tmp_dir/ssg.tar.gz"
    elif command -v wget &> /dev/null; then
        wget -q "$url" -O "$tmp_dir/ssg.tar.gz"
    else
        log_error "curl or wget is required"
    fi
    
    log_info "Extracting..."
    tar -xzf "$tmp_dir/ssg.tar.gz" -C "$tmp_dir"
    
    log_info "Installing to $INSTALL_DIR..."
    if [[ -w "$INSTALL_DIR" ]]; then
        mv "$tmp_dir/ssg" "$INSTALL_DIR/"
    else
        sudo mv "$tmp_dir/ssg" "$INSTALL_DIR/"
    fi
    
    chmod +x "$INSTALL_DIR/ssg"
    rm -rf "$tmp_dir"
    
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
