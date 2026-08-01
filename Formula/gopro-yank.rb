class GoproYank < Formula
  desc "Bring every GoPro cloud original home in a verified archive"
  homepage "https://github.com/azohra/gopro-yank"
  url "https://github.com/azohra/gopro-yank/releases/download/v1.0.0/gopro-yank_source.tar.gz"
  sha256 "6f7bec7f05b510ced7955ed51cb2917b2acd37e96b72d1b2e816046b04d6d4b2"
  license "MIT"
  head "https://github.com/azohra/gopro-yank.git", branch: "main"

  depends_on "go" => :build

  def install
    ldflags = "-s -w -X main.version=#{version}"
    system "go", "build", *std_go_args(ldflags: ldflags), "./cmd/gopro-yank"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/gopro-yank --version")
  end
end
