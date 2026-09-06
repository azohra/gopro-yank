#!/usr/bin/env bash
set -euo pipefail
script=$(cd "$(dirname "$0")/.." && pwd)/scripts/release-source.sh
scratch=$(mktemp -d)
trap 'rm -rf "$scratch"' EXIT
git init -q -b main "$scratch/repo"
cd "$scratch/repo"
git config user.name test
git config user.email test@example.invalid
git -c commit.gpgsign=false commit --allow-empty -qm first
first=$(git rev-parse HEAD)
git -c tag.gpgsign=false tag v1.2.3
git -c commit.gpgsign=false commit --allow-empty -qm second
git clone -q --bare . "$scratch/origin.git"
git remote add origin "$scratch/origin.git"
[ "$(bash "$script" v1.2.3)" = "$first" ]
if bash "$script" 'refs/heads/main' >/dev/null 2>&1; then exit 1; fi
git switch -qc unmerged "$first"
git -c commit.gpgsign=false commit --allow-empty -qm unmerged
git -c tag.gpgsign=false tag v1.2.4
git push -q origin refs/tags/v1.2.4
if bash "$script" v1.2.4 >/dev/null 2>&1; then exit 1; fi
git switch -q main
git -c tag.gpgsign=false tag -f v1.2.3 >/dev/null
if bash "$script" v1.2.3 >/dev/null 2>&1; then exit 1; fi
echo 'Release source: main ancestor accepted; invalid, unmerged, and moved tags refused'
