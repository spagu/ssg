# Homebrew formula for SSG - Static Site Generator
# Install: brew install spagu/tap/ssg
# Or: brew tap spagu/tap && brew install ssg

class Ssg < Formula
  desc "Fast static site generator written in Go"
  homepage "https://github.com/spagu/ssg"
  version "1.7.13"
  license "BSD-3-Clause"

  on_macos do
    on_arm do
      url "https://github.com/spagu/ssg/releases/download/v1.7.13/ssg-darwin-arm64.tar.gz"
      sha256 "66b16a4c7190cfad2273c956d681bbaac859df3b01cc897e1fe4192d4ded0687"
    end
    on_intel do
      url "https://github.com/spagu/ssg/releases/download/v1.7.13/ssg-darwin-amd64.tar.gz"
      sha256 "4249df263492c2b8936238b345cfc6d9e36591669e70fc6468b285ed0f603aed"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/spagu/ssg/releases/download/v1.7.13/ssg-linux-arm64.tar.gz"
      sha256 "90040595646a70f964f11be42074092d314c453c185774f20981c0b5757aa075"
    end
    on_intel do
      url "https://github.com/spagu/ssg/releases/download/v1.7.13/ssg-linux-amd64.tar.gz"
      sha256 "63dde71b6fcb7fb934bbe6cad4b44bf92c20dcf38f1d93f1f6c1e6a7fb15ee6f"
    end
  end

  def install
    bin.install "ssg"
  end

  test do
    system "#{bin}/ssg", "--help"
  end
end
