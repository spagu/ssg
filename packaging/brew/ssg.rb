# Homebrew formula for SSG - Static Site Generator
# Install: brew install spagu/tap/ssg
# Or: brew tap spagu/tap && brew install ssg
#
# NOTE: this file is the reference copy. The published formula lives at
# https://github.com/spagu/homebrew-tap/blob/main/ssg.rb and is regenerated on
# every tag by the "Update Homebrew tap" step in .github/workflows/ci.yml.

class Ssg < Formula
  desc "Fast static site generator written in Go"
  homepage "https://github.com/spagu/ssg"
  version "1.8.11"
  license "BSD-3-Clause"

  on_macos do
    on_arm do
      url "https://github.com/spagu/ssg/releases/download/v1.8.11/ssg-darwin-arm64.tar.gz"
      sha256 "339168be6005853362be1de8c9a38940186b6da17e9a67a3b161f92130e26c12"
    end
    on_intel do
      url "https://github.com/spagu/ssg/releases/download/v1.8.11/ssg-darwin-amd64.tar.gz"
      sha256 "427c02b3ef85449a45eee8990da0afefda59656e0ca8c51f0140fbb9bea700e9"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/spagu/ssg/releases/download/v1.8.11/ssg-linux-arm64.tar.gz"
      sha256 "b0ff8632d19b4d55990f3fd94f089a89690c18a1e6bd9d6cfdd8123afa89cffd"
    end
    on_intel do
      url "https://github.com/spagu/ssg/releases/download/v1.8.11/ssg-linux-amd64.tar.gz"
      sha256 "bfd9c3e5e31a5e676f57105dc604fb74410f0022b86df3d35d6b0e239987ffff"
    end
  end

  def install
    bin.install "ssg"
  end

  test do
    system "#{bin}/ssg", "--help"
  end
end
