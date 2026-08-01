class GoproYank < Formula
  desc "Export every GoPro cloud original and verify the archive offline"
  homepage "https://github.com/azohra/gopro-yank"
  url "https://github.com/azohra/gopro-yank/releases/download/v1.0.0/gopro-yank_source.tar.gz"
  sha256 "dee29d42a908684033bc2307734556ab20b9814c334b1ec3022d13ac052ba67d"
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
