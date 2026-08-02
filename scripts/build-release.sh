#!/bin/sh
set -eu

release_version=${1:-dev}
project_root=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)
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
  'HEAD^{tree}' -- .env.example .github cmd docs internal scripts site CONTRIBUTING.md go.mod go.sum Makefile LICENSE README.md

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
