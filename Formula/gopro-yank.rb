class GoproYank < Formula
  desc "Download and verify every GoPro cloud original"
  homepage "https://github.com/azohra/gopro-yank"
  url "https://github.com/azohra/gopro-yank/releases/download/v1.0.0/gopro-yank_source.tar.gz"
  sha256 "0b58640bf691eac7e27780634eb8d89023c3e4d02904e92e3c03a8f2553c859a"
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
