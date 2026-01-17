# SSG Installation Guide

This document provides installation instructions for SSG on all supported platforms.

## Table of Contents

- [Quick Install](#quick-install)
- [Linux - Debian/Ubuntu (DEB)](#linux---debianubuntu-deb)
- [Linux - Fedora/RHEL/CentOS (RPM)](#linux---fedorarhel-rpm)
- [Linux - Snap (Ubuntu)](#linux---snap-ubuntu)
- [macOS - Homebrew](#macos---homebrew)
- [macOS - Binary](#macos---binary)
- [FreeBSD](#freebsd)
- [OpenBSD](#openbsd)
- [Windows](#windows)
- [From Source](#from-source)
- [Verify Installation](#verify-installation)

---

## Quick Install

### One-liner (Linux/macOS)

```bash
curl -sSL https://raw.githubusercontent.com/spagu/ssg/main/install.sh | bash
```

---

## Linux - Debian/Ubuntu (DEB)

### Add Repository (Recommended)

```bash
# Add GPG key
curl -fsSL https://github.com/spagu/ssg/releases/download/v1.3.0/ssg-apt.gpg | sudo gpg --dearmor -o /usr/share/keyrings/ssg-keyring.gpg

# Add repository
echo "deb [signed-by=/usr/share/keyrings/ssg-keyring.gpg] https://apt.ssg.dev stable main" | sudo tee /etc/apt/sources.list.d/ssg.list

# Update and install
sudo apt update
sudo apt install ssg
```

### Manual Download

```bash
# AMD64 (x86_64)
wget https://github.com/spagu/ssg/releases/download/v1.3.0/ssg_1.3.0_amd64.deb
sudo dpkg -i ssg_1.3.0_amd64.deb

# ARM64 (aarch64)
wget https://github.com/spagu/ssg/releases/download/v1.3.0/ssg_1.3.0_arm64.deb
sudo dpkg -i ssg_1.3.0_arm64.deb

# Install dependencies if needed
sudo apt install -f
```

### Recommended: Install WebP tools

```bash
sudo apt install webp
```

---

## Linux - Fedora/RHEL/CentOS (RPM)

### Add Repository (Fedora/RHEL 8+)

```bash
# Add repository
sudo tee /etc/yum.repos.d/ssg.repo << 'EOF'
[ssg]
name=SSG Repository
baseurl=https://rpm.ssg.dev/stable/$basearch
enabled=1
gpgcheck=1
gpgkey=https://github.com/spagu/ssg/releases/download/v1.3.0/ssg-rpm.gpg
EOF

# Install
sudo dnf install ssg
```

### Manual Download

```bash
# AMD64 (x86_64)
wget https://github.com/spagu/ssg/releases/download/v1.3.0/ssg-1.3.0-1.x86_64.rpm
sudo rpm -i ssg-1.3.0-1.x86_64.rpm

# ARM64 (aarch64)
wget https://github.com/spagu/ssg/releases/download/v1.3.0/ssg-1.3.0-1.aarch64.rpm
sudo rpm -i ssg-1.3.0-1.aarch64.rpm
```

### Install WebP tools

```bash
sudo dnf install libwebp-tools
```

---

## Linux - Snap (Ubuntu)

### Install from Snap Store

```bash
sudo snap install ssg
```

### Or from local snap file

```bash
wget https://github.com/spagu/ssg/releases/download/v1.3.0/ssg_1.3.0_amd64.snap
sudo snap install --classic ssg_1.3.0_amd64.snap
```

---

## macOS - Homebrew

### Tap and Install (Recommended)

```bash
# Add tap
brew tap spagu/tap

# Install
brew install ssg
```

### Or direct install

```bash
brew install spagu/tap/ssg
```

### Install WebP tools

```bash
brew install webp
```

---

## macOS - Binary

### Download and Install

```bash
# Apple Silicon (M1/M2/M3)
curl -LO https://github.com/spagu/ssg/releases/download/v1.3.0/ssg-1.3.0-darwin-arm64.tar.gz
tar -xzf ssg-1.3.0-darwin-arm64.tar.gz
sudo mv ssg /usr/local/bin/

# Intel
curl -LO https://github.com/spagu/ssg/releases/download/v1.3.0/ssg-1.3.0-darwin-amd64.tar.gz
tar -xzf ssg-1.3.0-darwin-amd64.tar.gz
sudo mv ssg /usr/local/bin/
```

---

## FreeBSD

### Using pkg (when available)

```bash
pkg install ssg
```

### From Ports

```bash
cd /usr/ports/www/ssg
make install clean
```

### Manual Download

```bash
# AMD64
fetch https://github.com/spagu/ssg/releases/download/v1.3.0/ssg-1.3.0-freebsd-amd64.tar.gz
tar -xzf ssg-1.3.0-freebsd-amd64.tar.gz
mv ssg /usr/local/bin/

# ARM64
fetch https://github.com/spagu/ssg/releases/download/v1.3.0/ssg-1.3.0-freebsd-arm64.tar.gz
tar -xzf ssg-1.3.0-freebsd-arm64.tar.gz
mv ssg /usr/local/bin/
```

---

## OpenBSD

### From Ports

```bash
cd /usr/ports/www/ssg
make install
```

### Manual Download

```bash
# AMD64
ftp https://github.com/spagu/ssg/releases/download/v1.3.0/ssg-1.3.0-openbsd-amd64.tar.gz
tar -xzf ssg-1.3.0-openbsd-amd64.tar.gz
doas mv ssg /usr/local/bin/

# ARM64
ftp https://github.com/spagu/ssg/releases/download/v1.3.0/ssg-1.3.0-openbsd-arm64.tar.gz
tar -xzf ssg-1.3.0-openbsd-arm64.tar.gz
doas mv ssg /usr/local/bin/
```

---

## Windows

### Download and Install

1. Download the latest release:
   - [ssg-1.3.0-windows-amd64.zip](https://github.com/spagu/ssg/releases/download/v1.3.0/ssg-1.3.0-windows-amd64.zip)
   - [ssg-1.3.0-windows-arm64.zip](https://github.com/spagu/ssg/releases/download/v1.3.0/ssg-1.3.0-windows-arm64.zip)

2. Extract the ZIP file

3. Add to PATH:
   ```powershell
   # PowerShell (run as Administrator)
   $env:Path += ";C:\path\to\ssg"
   [System.Environment]::SetEnvironmentVariable("Path", $env:Path, "Machine")
   ```

### Using Scoop (Community)

```powershell
scoop install ssg
```

---

## From Source

### Requirements

- Go 1.21 or later
- Git

### Build and Install

```bash
# Clone repository
git clone https://github.com/spagu/ssg.git
cd ssg

# Build
go build -o ssg ./cmd/ssg

# Install to /usr/local/bin
sudo mv ssg /usr/local/bin/

# Or use make
make build
sudo make install
```

---

## Verify Installation

After installation, verify SSG is working:

```bash
# Check version
ssg --help

# Quick test
mkdir -p test-site/{content/my-site,templates/simple}
ssg my-site simple example.com --http --port=3000
```

---

## Uninstall

### Debian/Ubuntu

```bash
sudo apt remove ssg
```

### Fedora/RHEL

```bash
sudo dnf remove ssg
```

### Snap

```bash
sudo snap remove ssg
```

### Homebrew

```bash
brew uninstall ssg
brew untap spagu/tap
```

### Manual (Binary)

```bash
sudo rm /usr/local/bin/ssg
```

---

## Troubleshooting

### "command not found: ssg"

Ensure `/usr/local/bin` is in your PATH:

```bash
echo $PATH | grep -q "/usr/local/bin" || export PATH=$PATH:/usr/local/bin
```

### WebP conversion fails

Install the `webp` package for your system:

```bash
# Debian/Ubuntu
sudo apt install webp

# Fedora/RHEL
sudo dnf install libwebp-tools

# macOS
brew install webp

# FreeBSD
pkg install graphics/webp
```

### Permission denied

Make sure the binary is executable:

```bash
chmod +x /usr/local/bin/ssg
```

---

## Support

- GitHub Issues: https://github.com/spagu/ssg/issues
- Documentation: https://github.com/spagu/ssg#readme
