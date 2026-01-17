#!/bin/bash
# =============================================================================
# SSG - Build All Packages Script
# Builds binary packages for all supported platforms
# =============================================================================
set -e

VERSION="${VERSION:-1.3.0}"
BUILD_DIR="$(pwd)/dist"
SOURCE_DIR="$(pwd)"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

log_info() { echo -e "${BLUE}[INFO]${NC} $1"; }
log_success() { echo -e "${GREEN}[OK]${NC} $1"; }
log_warn() { echo -e "${YELLOW}[WARN]${NC} $1"; }
log_error() { echo -e "${RED}[ERROR]${NC} $1"; }

# Create build directory
mkdir -p "$BUILD_DIR"

# Platforms to build
PLATFORMS=(
    "linux/amd64"
    "linux/arm64"
    "darwin/amd64"
    "darwin/arm64"
    "freebsd/amd64"
    "freebsd/arm64"
    "openbsd/amd64"
    "openbsd/arm64"
    "windows/amd64"
    "windows/arm64"
)

build_binary() {
    local os=$1
    local arch=$2
    local output_name="ssg"
    
    if [[ "$os" == "windows" ]]; then
        output_name="ssg.exe"
    fi
    
    local output_dir="$BUILD_DIR/${os}_${arch}"
    mkdir -p "$output_dir"
    
    log_info "Building for $os/$arch..."
    
    GOOS=$os GOARCH=$arch go build -ldflags "-s -w -X main.Version=$VERSION" \
        -o "$output_dir/$output_name" ./cmd/ssg
    
    # Create tarball
    if [[ "$os" == "windows" ]]; then
        (cd "$output_dir" && zip -q "../ssg-${VERSION}-${os}-${arch}.zip" "$output_name")
    else
        tar -czf "$BUILD_DIR/ssg-${VERSION}-${os}-${arch}.tar.gz" -C "$output_dir" "$output_name"
    fi
    
    log_success "Built ssg-${VERSION}-${os}-${arch}"
}

build_all_binaries() {
    log_info "Building binaries for all platforms..."
    
    for platform in "${PLATFORMS[@]}"; do
        IFS='/' read -r os arch <<< "$platform"
        build_binary "$os" "$arch"
    done
    
    log_success "All binaries built successfully!"
}

build_deb() {
    local arch=$1
    local deb_arch="amd64"
    [[ "$arch" == "arm64" ]] && deb_arch="arm64"
    
    log_info "Building DEB package for $deb_arch..."
    
    local pkg_dir="$BUILD_DIR/deb-${deb_arch}"
    mkdir -p "$pkg_dir/DEBIAN"
    mkdir -p "$pkg_dir/usr/bin"
    mkdir -p "$pkg_dir/usr/share/doc/ssg"
    mkdir -p "$pkg_dir/usr/share/man/man1"
    
    # Copy binary
    cp "$BUILD_DIR/linux_${arch}/ssg" "$pkg_dir/usr/bin/"
    chmod 755 "$pkg_dir/usr/bin/ssg"
    
    # Copy docs
    cp README.md "$pkg_dir/usr/share/doc/ssg/"
    cp CHANGELOG.md "$pkg_dir/usr/share/doc/ssg/"
    cp LICENSE "$pkg_dir/usr/share/doc/ssg/" 2>/dev/null || echo "MIT License" > "$pkg_dir/usr/share/doc/ssg/LICENSE"
    
    # Create control file
    cat > "$pkg_dir/DEBIAN/control" << EOF
Package: ssg
Version: ${VERSION}
Section: web
Priority: optional
Architecture: ${deb_arch}
Maintainer: spagu <spagu@github.com>
Homepage: https://github.com/spagu/ssg
Description: Static Site Generator
 Fast static site generator written in Go.
 Converts Markdown content with YAML frontmatter to static HTML.
 Features: WebP conversion, built-in HTTP server, watch mode.
Depends: libc6
Recommends: webp
EOF

    # Build package
    dpkg-deb --build "$pkg_dir" "$BUILD_DIR/ssg_${VERSION}_${deb_arch}.deb"
    log_success "Built ssg_${VERSION}_${deb_arch}.deb"
}

build_rpm() {
    local arch=$1
    local rpm_arch="x86_64"
    [[ "$arch" == "arm64" ]] && rpm_arch="aarch64"
    
    log_info "Building RPM package for $rpm_arch..."
    
    local spec_file="$BUILD_DIR/ssg.spec"
    
    cat > "$spec_file" << EOF
Name:           ssg
Version:        ${VERSION}
Release:        1%{?dist}
Summary:        Static Site Generator

License:        MIT
URL:            https://github.com/spagu/ssg
Source0:        ssg-${VERSION}-linux-${arch}.tar.gz

%description
Fast static site generator written in Go.
Converts Markdown content with YAML frontmatter to static HTML.
Features: WebP conversion, built-in HTTP server, watch mode.

%install
mkdir -p %{buildroot}%{_bindir}
install -m 755 ssg %{buildroot}%{_bindir}/ssg

%files
%{_bindir}/ssg

%changelog
* $(date '+%a %b %d %Y') spagu <spagu@github.com> - ${VERSION}-1
- Release ${VERSION}
EOF

    # Build RPM using rpmbuild if available
    if command -v rpmbuild &> /dev/null; then
        mkdir -p ~/rpmbuild/{BUILD,RPMS,SOURCES,SPECS,SRPMS}
        cp "$BUILD_DIR/ssg-${VERSION}-linux-${arch}.tar.gz" ~/rpmbuild/SOURCES/
        rpmbuild -bb --target "$rpm_arch" "$spec_file"
        cp ~/rpmbuild/RPMS/${rpm_arch}/ssg-${VERSION}*.rpm "$BUILD_DIR/" 2>/dev/null || true
        log_success "Built RPM for $rpm_arch"
    else
        log_warn "rpmbuild not found, skipping RPM build"
        cp "$spec_file" "$BUILD_DIR/ssg-${VERSION}.spec"
    fi
}

build_snap() {
    log_info "Building Snap package..."
    
    if command -v snapcraft &> /dev/null; then
        cd "$SOURCE_DIR"
        snapcraft
        mv *.snap "$BUILD_DIR/" 2>/dev/null || true
        log_success "Built Snap package"
    else
        log_warn "snapcraft not found, skipping Snap build"
    fi
}

create_checksums() {
    log_info "Creating checksums..."
    
    cd "$BUILD_DIR"
    sha256sum *.tar.gz *.zip *.deb *.rpm 2>/dev/null > checksums.sha256 || true
    log_success "Checksums created"
}

main() {
    log_info "=== SSG Package Builder v${VERSION} ==="
    log_info "Build directory: $BUILD_DIR"
    
    case "${1:-all}" in
        binaries)
            build_all_binaries
            ;;
        deb)
            build_deb "amd64"
            build_deb "arm64"
            ;;
        rpm)
            build_rpm "amd64"
            build_rpm "arm64"
            ;;
        snap)
            build_snap
            ;;
        all)
            build_all_binaries
            build_deb "amd64"
            build_deb "arm64"
            build_rpm "amd64"
            build_rpm "arm64"
            build_snap
            create_checksums
            ;;
        *)
            echo "Usage: $0 {binaries|deb|rpm|snap|all}"
            exit 1
            ;;
    esac
    
    log_info "=== Build complete! ==="
    ls -la "$BUILD_DIR"
}

main "$@"
