#!/usr/bin/env bash
# Install cwebp (and any extra packages named as arguments, e.g. libavif-bin)
# on a Linux CI runner.
#
# The distribution's package comes first. When its mirror is unreachable — a
# DNS outage of azure.archive.ubuntu.com took every apt-based job down for over
# an hour on 2026-11-11 — cwebp comes from Google's own libwebp release
# instead, pinned by version and SHA-256. The extra packages have no such
# fallback: ssg skips a format whose encoder is missing, with a warning, so
# the build still finishes.
#
# Used by action.yml and .github/workflows/docs-site.yml.
# Bump: take the newest vX.Y.Z from https://github.com/webmproject/libwebp/tags
# (the same pin as snap/snapcraft.yaml) and the tarballs' sha256sum.
set -euo pipefail

webpVersion="1.6.0"
case "$(uname -m)" in
  x86_64) webpArch="linux-x86-64"; webpSum="1c5ffab71efecefa0e3c23516c3a3a1dccb45cc310ae1095c6f14ae268e38067" ;;
  aarch64 | arm64) webpArch="linux-aarch64"; webpSum="69f5eebe203e0f3942fe37986209a1725741be19c152950a4283b376c95ec798" ;;
  *) webpArch="" ; webpSum="" ;;
esac

aptOpts=(-o Acquire::Retries=3 -o Acquire::http::Timeout=20)
if sudo timeout 300 apt-get update "${aptOpts[@]}" &&
  sudo timeout 300 apt-get install -y --no-install-recommends "${aptOpts[@]}" webp "$@"; then
  exit 0
fi

echo "::warning::The distribution mirror is unreachable; installing cwebp ${webpVersion} from Google's libwebp release instead."
if [[ $# -gt 0 ]]; then
  echo "::warning::Not installed: $*. ssg skips the formats they encode."
fi
if [[ -z "$webpArch" ]]; then
  echo "::error::No libwebp release for $(uname -m); re-run the job when the mirror is back."
  exit 1
fi

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
name="libwebp-${webpVersion}-${webpArch}"
curl -fsSL --retry 3 --connect-timeout 10 --max-time 120 \
  -o "$tmp/$name.tar.gz" "https://storage.googleapis.com/downloads.webmproject.org/releases/webp/$name.tar.gz"
echo "$webpSum  $tmp/$name.tar.gz" | sha256sum -c --quiet -
tar -xzf "$tmp/$name.tar.gz" -C "$tmp"
sudo install -m 0755 "$tmp/$name/bin/cwebp" /usr/local/bin/cwebp
cwebp -version
