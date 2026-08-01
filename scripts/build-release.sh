#!/bin/sh
set -eu

release_version=${1:-dev}
project_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
output_dir="$project_root/release"
build_dir=$(mktemp -d "${TMPDIR:-/tmp}/gopro-yank-release.XXXXXX")
trap 'rm -rf "$build_dir"' EXIT HUP INT TERM

if ! printf '%s\n' "$release_version" | grep -Eq '^(dev|[0-9]+\.[0-9]+\.[0-9]+([.-][0-9A-Za-z.-]+)?)$'; then
  echo "invalid version: $release_version" >&2
  exit 2
fi

rm -rf "$output_dir"
mkdir -p "$output_dir"

for target in darwin/arm64 darwin/amd64 linux/arm64 linux/amd64 windows/arm64 windows/amd64; do
  target_os=${target%/*}
  target_arch=${target#*/}
  executable=gopro-yank
  [ "$target_os" = windows ] && executable=gopro-yank.exe
  package_dir="$build_dir/gopro-yank_${target_os}_${target_arch}"
  mkdir -p "$package_dir"
  echo "Building $target_os/$target_arch..."
  (
    cd "$project_root"
    CGO_ENABLED=0 GOOS="$target_os" GOARCH="$target_arch" \
      go build -trimpath -ldflags "-s -w -X main.version=$release_version" \
      -o "$package_dir/$executable" ./cmd/gopro-yank
  )
  cp "$project_root/LICENSE" "$package_dir/LICENSE"
  if [ "$target_os" = windows ]; then
    (cd "$package_dir" && zip -q "$output_dir/gopro-yank_${target_os}_${target_arch}.zip" "$executable" LICENSE)
  else
    tar -C "$package_dir" -czf "$output_dir/gopro-yank_${target_os}_${target_arch}.tar.gz" "$executable" LICENSE
  fi
done

git -C "$project_root" archive \
  --format=tar.gz \
  --mtime=1970-01-01T00:00:00Z \
  --prefix=gopro-yank/ \
  --output="$output_dir/gopro-yank_source.tar.gz" \
  'HEAD^{tree}' -- .env.example .github cmd docs internal scripts go.mod Makefile LICENSE README.md
source_sha=$(shasum -a 256 "$output_dir/gopro-yank_source.tar.gz" | awk '{print $1}')

cat > "$project_root/Formula/gopro-yank.rb" <<FORMULA
class GoproYank < Formula
  desc "Bring every GoPro cloud original home in a verified archive"
  homepage "https://github.com/azohra/gopro-yank"
  url "https://github.com/azohra/gopro-yank/releases/download/v${release_version}/gopro-yank_source.tar.gz"
  sha256 "${source_sha}"
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
FORMULA

(cd "$output_dir" && shasum -a 256 gopro-yank_* > checksums.txt)
echo "Release artifacts: $output_dir"
echo "Homebrew formula: $project_root/Formula/gopro-yank.rb"
