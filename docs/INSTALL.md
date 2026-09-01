# SSG Installation Guide

This document provides installation instructions for SSG on all supported platforms.

SSG ships as a single static binary of roughly **34 MB** (release builds,
stripped). Since v1.8.4 it embeds pure-Go SQL drivers (MySQL/MariaDB,
PostgreSQL, SQLite) for [external sources](EXTERNAL_SOURCES.md), which
accounts for most of that size; no external libraries or runtimes are needed.

## Table of Contents

- [Quick Install](#quick-install)
- [Linux - Debian/Ubuntu (DEB)](#linux---debianubuntu-deb)
- [Linux - Fedora/RHEL/CentOS (RPM)](#linux---fedorarhelcentos-rpm)
- [Linux - Snap (Ubuntu)](#linux---snap-ubuntu)
- [macOS - Homebrew](#macos---homebrew)
- [macOS - Binary](#macos---binary)
- [FreeBSD](#freebsd)
- [OpenBSD](#openbsd)
- [Windows](#windows)
- [From Source](#from-source)
- [Verify Installation](#verify-installation)
- [Staying up to date](#staying-up-to-date)

---

> **Upgrading an existing install?** See
> [UPGRADING.md](UPGRADING.md) for the steps between your version and this one.
> Most releases are drop-in; the few that are not are listed there.

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
curl -fsSL https://github.com/spagu/ssg/releases/latest/download/ssg-apt.gpg | sudo gpg --dearmor -o /usr/share/keyrings/ssg-keyring.gpg

# Add repository
echo "deb [signed-by=/usr/share/keyrings/ssg-keyring.gpg] https://apt.ssg.dev stable main" | sudo tee /etc/apt/sources.list.d/ssg.list

# Update and install
sudo apt update
sudo apt install ssg
```

### Manual Download

```bash
# Pick the version you want — see all releases (incl. previous versions):
# https://github.com/spagu/ssg/releases
VERSION=1.8.54

# AMD64 (x86_64)
wget https://github.com/spagu/ssg/releases/download/v${VERSION}/ssg_${VERSION}_amd64.deb
sudo dpkg -i ssg_${VERSION}_amd64.deb

# ARM64 (aarch64)
wget https://github.com/spagu/ssg/releases/download/v${VERSION}/ssg_${VERSION}_arm64.deb
sudo dpkg -i ssg_${VERSION}_arm64.deb

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
gpgkey=https://github.com/spagu/ssg/releases/latest/download/ssg-rpm.gpg
EOF

# Install
sudo dnf install ssg
```

### Manual Download

```bash
# Pick the version you want — see all releases (incl. previous versions):
# https://github.com/spagu/ssg/releases
VERSION=1.8.54

# AMD64 (x86_64)
wget https://github.com/spagu/ssg/releases/download/v${VERSION}/ssg-${VERSION}-1.x86_64.rpm
sudo rpm -i ssg-${VERSION}-1.x86_64.rpm

# ARM64 (aarch64)
wget https://github.com/spagu/ssg/releases/download/v${VERSION}/ssg-${VERSION}-1.aarch64.rpm
sudo rpm -i ssg-${VERSION}-1.aarch64.rpm
```

### Install WebP tools

```bash
sudo dnf install libwebp-tools
```

---

## Linux - Snap (Ubuntu)

> **On WSL, prefer the [DEB package](#linux---debianubuntu-deb).** The snap is
> strictly confined, so running it needs snapd to build a private mount
> namespace — something WSL's kernel does not always allow. The symptom is not
> an ssg error but a snapd one, before ssg starts at all:
>
> ```
> cannot preserve mount namespace of process NNNNN as static-site-generator.mnt: Invalid argument
> unexpected eof from helper process
> ```
>
> It typically appears right after a `snap refresh`, when a namespace from the
> previous revision is left behind. Clear it and retry:
>
> ```bash
> sudo umount /run/snapd/ns/static-site-generator.mnt 2>/dev/null
> sudo rm -f /run/snapd/ns/static-site-generator.mnt
> sudo systemctl restart snapd.service
> ```
>
> If it persists, `wsl --shutdown` from Windows clears every namespace at once.
> snapd also needs systemd, which WSL enables only with `systemd=true` under
> `[boot]` in `/etc/wsl.conf`. The DEB has none of these moving parts.

### Install from Snap Store

```bash
sudo snap install static-site-generator
```

### Create short alias

```bash
sudo snap alias static-site-generator ssg
```

Now you can use `ssg` instead of `static-site-generator`.

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
curl -LO https://github.com/spagu/ssg/releases/latest/download/ssg-darwin-arm64.tar.gz
tar -xzf ssg-darwin-arm64.tar.gz
sudo mv ssg /usr/local/bin/

# Intel
curl -LO https://github.com/spagu/ssg/releases/latest/download/ssg-darwin-amd64.tar.gz
tar -xzf ssg-darwin-amd64.tar.gz
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
fetch https://github.com/spagu/ssg/releases/latest/download/ssg-freebsd-amd64.tar.gz
tar -xzf ssg-freebsd-amd64.tar.gz
mv ssg /usr/local/bin/

# ARM64
fetch https://github.com/spagu/ssg/releases/latest/download/ssg-freebsd-arm64.tar.gz
tar -xzf ssg-freebsd-arm64.tar.gz
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
ftp https://github.com/spagu/ssg/releases/latest/download/ssg-openbsd-amd64.tar.gz
tar -xzf ssg-openbsd-amd64.tar.gz
doas mv ssg /usr/local/bin/

# ARM64
ftp https://github.com/spagu/ssg/releases/latest/download/ssg-openbsd-arm64.tar.gz
tar -xzf ssg-openbsd-arm64.tar.gz
doas mv ssg /usr/local/bin/
```

---

## Windows

### Download and Install

1. Download the latest release:
   - [ssg-windows-amd64.zip](https://github.com/spagu/ssg/releases/latest/download/ssg-windows-amd64.zip)
   - [ssg-windows-arm64.zip](https://github.com/spagu/ssg/releases/latest/download/ssg-windows-arm64.zip)

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

- Go 1.27.0 or later
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
ssg --version

# Quick test
mkdir -p test-site/{content/my-site,templates/simple}
ssg my-site simple example.com --http --port=3000
```

Every build now names the binary that produced it on its first line, so a log
read later answers "which ssg was this?" without anyone being asked to run
`--version` separately:

```
🧱 ssg 1.8.48
🔄 Loading content...
```

`--quiet` suppresses it with everything else.

---

## Staying up to date

```bash
ssg self-update-check
```

It prints the running version, how this binary was installed, and whether a
newer release exists:

```
🧱 ssg 1.8.30 (installed via snap)
🆕 A newer release is out: 1.8.48 (you have 1.8.30)
   https://github.com/spagu/ssg/releases/tag/v1.8.48

   Upgrade with:
     sudo snap refresh static-site-generator
```

The upgrade command is the one for **your** install — snap, Homebrew, apt, dnf,
Docker, or the releases page for a downloaded binary — worked out from where the
running executable actually sits, not from the operating system. A Homebrew ssg
on Linux is still Homebrew's, and a binary you copied into `/usr/bin` yourself is
nobody's to upgrade, so that one is sent to the releases page.

Two things it deliberately does not do:

- **It never updates anything.** It reports; you decide. A tool that rewrites
  its own binary asks for more trust than a static site generator has any
  business asking for.
- **It never runs on its own.** A build does not contact the network — that
  would be slow, would fail in air-gapped and CI environments, and nobody asked
  for it. The check happens only when you run it.

It exits 0 whether or not an update exists, so it is safe in a script; exit 1
means the check itself could not be made.

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
sudo snap remove static-site-generator
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
