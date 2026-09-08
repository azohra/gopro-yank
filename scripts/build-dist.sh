#!/bin/sh
set -eu

release_version=${RELEASE_VERSION:-dev}
project_root=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)
output_dir="$project_root/release"
scratch=$(mktemp -d)
trap 'rm -rf "$scratch"' EXIT HUP INT TERM
rm -rf "$output_dir"
mkdir -p "$output_dir"
cd "$project_root"

for os in darwin linux windows; do
  for arch in arm64 amd64; do
    [ "$os/$arch" != darwin/amd64 ] || continue
    binary=gopro-yank
    [ "$os" != windows ] || binary=gopro-yank.exe
    dir="$scratch/${os}_${arch}"
    mkdir -p "$dir"
    CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" go build -trimpath -buildvcs=false \
      -ldflags="-s -w -X main.version=$release_version" \
      -o "$dir/$binary" ./cmd/gopro-yank
    cp LICENSE "$dir/"
    if [ "$os" = windows ]; then
      (cd "$dir" && zip -q "$output_dir/gopro-yank_${os}_${arch}.zip" "$binary" LICENSE)
    else
      COPYFILE_DISABLE=1 tar -czf "$output_dir/gopro-yank_${os}_${arch}.tar.gz" -C "$dir" "$binary" LICENSE
    fi
  done
done
[ "$("$scratch/$(go env GOOS)_$(go env GOARCH)/gopro-yank" --version)" = "gopro-yank $release_version" ]

darwin_arm64_sha=$(shasum -a 256 "$output_dir/gopro-yank_darwin_arm64.tar.gz" | awk '{print $1}')
linux_amd64_sha=$(shasum -a 256 "$output_dir/gopro-yank_linux_amd64.tar.gz" | awk '{print $1}')
linux_arm64_sha=$(shasum -a 256 "$output_dir/gopro-yank_linux_arm64.tar.gz" | awk '{print $1}')

cat > "$output_dir/gopro-yank.rb" <<CASK
cask "gopro-yank" do
  arch arm: "arm64", intel: "amd64"
  os macos: "darwin", linux: "linux"

  version "${release_version}"
  sha256 arm:          "${darwin_arm64_sha}",
         arm64_linux:  "${linux_arm64_sha}",
         x86_64_linux: "${linux_amd64_sha}"

  on_macos do
    depends_on arch: :arm64

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

(cd "$output_dir" && shasum -a 256 gopro-yank_* gopro-yank.rb > checksums.txt)
echo "Release artifacts: $output_dir"
echo "Homebrew cask asset: $output_dir/gopro-yank.rb"
