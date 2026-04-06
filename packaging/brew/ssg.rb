# Homebrew formula for SSG - Static Site Generator
# Install: brew install spagu/tap/ssg
# Or: brew tap spagu/tap && brew install ssg

class Ssg < Formula
  desc "Fast static site generator written in Go"
  homepage "https://github.com/spagu/ssg"
  version "1.7.9"
  license "BSD-3-Clause"

  on_macos do
    on_arm do
      url "https://github.com/spagu/ssg/releases/download/v1.7.9/ssg-darwin-arm64.tar.gz"
      sha256 "18de20a119702b6f0e83f6bf99c8071d31a3eeff97cd00e8d896f56cebe10f3f"
    end
    on_intel do
      url "https://github.com/spagu/ssg/releases/download/v1.7.9/ssg-darwin-amd64.tar.gz"
      sha256 "b7dc493e82bfcc5568b18a0ea6bc6e02980762c7d54bb83096efafc215aa2e23"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/spagu/ssg/releases/download/v1.7.9/ssg-linux-arm64.tar.gz"
      sha256 "8a22bf0e461ab3cdc8f055a44127fd6366862b33a9c74a3bc8a3753e2898a0a0"
    end
    on_intel do
      url "https://github.com/spagu/ssg/releases/download/v1.7.9/ssg-linux-amd64.tar.gz"
      sha256 "d22259134ba7c6a84b1c6267575a7165283c454de3aa5bb1f5c05165791c229b"
    end
  end

  def install
    bin.install "ssg"
  end

  test do
    system "#{bin}/ssg", "--help"
  end
end
