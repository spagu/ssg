# Homebrew formula for SSG - Static Site Generator
# Install: brew install spagu/tap/ssg
# Or: brew tap spagu/tap && brew install ssg

class Ssg < Formula
  desc "Fast static site generator written in Go"
  homepage "https://github.com/spagu/ssg"
  version "1.3.2"
  license "BSD-3-Clause"

  on_macos do
    on_arm do
      url "https://github.com/spagu/ssg/releases/download/v1.3.0/ssg-1.3.0-darwin-arm64.tar.gz"
      sha256 "PLACEHOLDER_SHA256_DARWIN_ARM64"
    end
    on_intel do
      url "https://github.com/spagu/ssg/releases/download/v1.3.0/ssg-1.3.0-darwin-amd64.tar.gz"
      sha256 "PLACEHOLDER_SHA256_DARWIN_AMD64"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/spagu/ssg/releases/download/v1.3.0/ssg-1.3.0-linux-arm64.tar.gz"
      sha256 "PLACEHOLDER_SHA256_LINUX_ARM64"
    end
    on_intel do
      url "https://github.com/spagu/ssg/releases/download/v1.3.0/ssg-1.3.0-linux-amd64.tar.gz"
      sha256 "PLACEHOLDER_SHA256_LINUX_AMD64"
    end
  end

  def install
    bin.install "ssg"
  end

  test do
    system "#{bin}/ssg", "--help"
  end
end
