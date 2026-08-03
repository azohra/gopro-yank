#!/bin/sh
set -eu

release_url=${GOPRO_YANK_RELEASE_URL:-https://github.com/azohra/gopro-yank/releases/latest/download}
install_dir=${GOPRO_YANK_INSTALL_DIR:-"$HOME/.local/bin"}
temporary_dir=''
staged_binary=''

cleanup() {
  [ -z "$staged_binary" ] || rm -f "$staged_binary"
  [ -z "$temporary_dir" ] || rm -rf "$temporary_dir"
}
trap cleanup EXIT HUP INT TERM

command -v curl >/dev/null 2>&1 || {
  printf '%s\n' 'GoPro Yank needs curl to install.' >&2
  exit 1
}

case $(uname -s) in
  Darwin) target_os=darwin ;;
  Linux) target_os=linux ;;
  *)
    printf '%s\n' 'This installer supports macOS and Linux. See the releases page for other systems.' >&2
    exit 1
    ;;
esac

case $(uname -m) in
  arm64 | aarch64) target_arch=arm64 ;;
  x86_64 | amd64) target_arch=amd64 ;;
  *)
    printf 'GoPro Yank does not publish a build for %s.\n' "$(uname -m)" >&2
    exit 1
    ;;
esac

asset="gopro-yank_${target_os}_${target_arch}.tar.gz"
temporary_dir=$(mktemp -d "${TMPDIR:-/tmp}/gopro-yank-install.XXXXXX")

printf 'Downloading GoPro Yank for %s/%s...\n' "$target_os" "$target_arch"
curl -fsSL "$release_url/$asset" -o "$temporary_dir/$asset"
curl -fsSL "$release_url/checksums.txt" -o "$temporary_dir/checksums.txt"

expected=$(awk -v asset="$asset" '$2 == asset { print $1 }' "$temporary_dir/checksums.txt")
[ -n "$expected" ] || {
  printf '%s\n' 'The release checksum list does not include this build.' >&2
  exit 1
}

if command -v sha256sum >/dev/null 2>&1; then
  actual=$(sha256sum "$temporary_dir/$asset" | awk '{ print $1 }')
elif command -v shasum >/dev/null 2>&1; then
  actual=$(shasum -a 256 "$temporary_dir/$asset" | awk '{ print $1 }')
else
  printf '%s\n' 'A SHA-256 checksum tool is required (sha256sum or shasum).' >&2
  exit 1
fi

[ "$actual" = "$expected" ] || {
  printf '%s\n' 'Checksum verification failed. Nothing was installed.' >&2
  exit 1
}

tar -xzf "$temporary_dir/$asset" -C "$temporary_dir"
[ -f "$temporary_dir/gopro-yank" ] || {
  printf '%s\n' 'The release archive did not contain gopro-yank.' >&2
  exit 1
}

mkdir -p "$install_dir"
staged_binary="$install_dir/.gopro-yank.new.$$"
install -m 0755 "$temporary_dir/gopro-yank" "$staged_binary"
mv -f "$staged_binary" "$install_dir/gopro-yank"
staged_binary=''

if [ "${GOPRO_YANK_NO_PATH_UPDATE:-0}" != 1 ]; then
  case ":${PATH:-}:" in
    *":$install_dir:"*) ;;
    *)
      case ${SHELL##*/} in
        zsh) profile="$HOME/.zprofile" ;;
        bash)
          if [ -f "$HOME/.bash_profile" ]; then profile="$HOME/.bash_profile"; else profile="$HOME/.profile"; fi
          ;;
        *) profile="$HOME/.profile" ;;
      esac
      path_line='export PATH="$HOME/.local/bin:$PATH"'
      if [ "$install_dir" = "$HOME/.local/bin" ] && ! grep -F "$path_line" "$profile" >/dev/null 2>&1; then
        printf '\n# GoPro Yank\n%s\n' "$path_line" >> "$profile"
        printf 'Added %s to your PATH in %s.\n' "$install_dir" "$profile"
      fi
      ;;
  esac
fi

printf 'Installed GoPro Yank at %s/gopro-yank\n' "$install_dir"
