#!/bin/sh
set -eu

release_version=${1:-dev}
case "$release_version" in
v[0-9]*) release_version=${release_version#v} ;;
esac
project_root=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)
output_dir="$project_root/release"

if ! printf '%s\n' "$release_version" | grep -Eq '^(dev|[0-9]+\.[0-9]+\.[0-9]+([.-][0-9A-Za-z.-]+)?)$'; then
  echo "invalid version: $release_version" >&2
  exit 2
fi

rm -rf "$output_dir"
mkdir -p "$output_dir"

export RELEASE_VERSION="$release_version"
(cd "$project_root" && goreleaser release --snapshot --skip=publish --clean)
for archive in "$project_root"/dist/gopro-yank_*.tar.gz "$project_root"/dist/gopro-yank_*.zip; do
  [ -f "$archive" ] && cp "$archive" "$output_dir/"
done

git -C "$project_root" archive \
  --format=tar.gz \
  --mtime=1970-01-01T00:00:00Z \
  --prefix=gopro-yank/ \
  --output="$output_dir/gopro-yank_source.tar.gz" \
  'HEAD^{tree}' -- .goreleaser.yaml mise.toml .env.example .github cmd docs internal scripts site CONTRIBUTING.md go.mod go.sum Makefile LICENSE README.md

darwin_amd64_sha=$(shasum -a 256 "$output_dir/gopro-yank_darwin_amd64.tar.gz" | awk '{print $1}')
darwin_arm64_sha=$(shasum -a 256 "$output_dir/gopro-yank_darwin_arm64.tar.gz" | awk '{print $1}')
linux_amd64_sha=$(shasum -a 256 "$output_dir/gopro-yank_linux_amd64.tar.gz" | awk '{print $1}')
linux_arm64_sha=$(shasum -a 256 "$output_dir/gopro-yank_linux_arm64.tar.gz" | awk '{print $1}')

cat > "$output_dir/gopro-yank.rb" <<CASK
cask "gopro-yank" do
  arch arm: "arm64", intel: "amd64"
  os macos: "darwin", linux: "linux"

  version "${release_version}"
  sha256 arm:          "${darwin_arm64_sha}",
         intel:        "${darwin_amd64_sha}",
         arm64_linux:  "${linux_arm64_sha}",
         x86_64_linux: "${linux_amd64_sha}"

  on_macos do
    postflight do
      system_command "/usr/bin/xattr",
                     args: ["-d", "com.apple.quarantine", "#{staged_path}/gopro-yank"]
    end
  end

  url "https://github.com/azohra/gopro-yank/releases/download/v#{version}/gopro-yank_#{os}_#{arch}.tar.gz"
  name "GoPro Yank"
  desc "Download and verify available GoPro cloud originals"
  homepage "https://gopro-yank.azohra.com/"

  binary "gopro-yank"
end
CASK

(cd "$output_dir" && shasum -a 256 gopro-yank_* > checksums.txt)
echo "Release artifacts: $output_dir"
echo "Homebrew cask asset: $output_dir/gopro-yank.rb"
