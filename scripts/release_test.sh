#!/usr/bin/env bash
set -euo pipefail

repo_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
test_root=$(mktemp -d)
trap 'rm -rf "$test_root"' EXIT

make_fixture() {
  local fixture="$test_root/$1"
  mkdir -p "$fixture/scripts" "$fixture/release" "$test_root/$1-bin"
  cp "$repo_root/scripts/release-source.sh" "$fixture/scripts/"
  cp "$repo_root/mise.toml" "$fixture/mise.toml"
  printf 'version "%s"\n' '1.2.3' >"$fixture/release/gopro-yank.rb"
  printf 'asset\n' >"$fixture/release/gopro-yank-linux-amd64.tar.gz"
  (cd "$fixture/release" && shasum -a 256 gopro-yank-linux-amd64.tar.gz >checksums.txt)
  git -C "$fixture" init -q -b main
  git -C "$fixture" config user.email test@example.invalid
  git -C "$fixture" config user.name test
  git -C "$fixture" add mise.toml release scripts
  git -C "$fixture" commit -q -m fixture
  git -C "$fixture" tag v1.2.3
  git -C "$fixture" clone -q --bare . "$test_root/$1-origin.git"
  git -C "$fixture" remote add origin "$test_root/$1-origin.git"
  git -C "$fixture" push -q origin refs/tags/v1.2.3
  cat >"$test_root/$1-bin/gh" <<'GH'
#!/usr/bin/env bash
set -euo pipefail
log=${GH_LOG:?}
printf '%s\n' "$*" >>"$log"
case "$*" in
  "auth status") exit 0 ;;
  "repo view --json nameWithOwner --jq .nameWithOwner") printf '%s\n' 'azohra/gopro-yank'; exit 0 ;;
  "release view v1.2.3 --json isDraft --jq .isDraft")
    if [[ -f ${GH_STATE:?} ]]; then printf '%s\n' false; exit 0; fi
    if [[ ${GH_SCENARIO:?} == auth-failure ]]; then printf '%s\n' 'HTTP 403: Forbidden' >&2; exit 1; fi
    if [[ ${GH_SCENARIO:?} == existing ]]; then printf '%s\n' true; exit 0; fi
    if [[ ${GH_SCENARIO:?} == published ]]; then printf '%s\n' false; exit 0; fi
    printf '%s\n' 'HTTP 404: release not found' >&2
    exit 1
    ;;
  "release create v1.2.3"*) : >"$GH_STATE"; exit 0 ;;
  "release upload v1.2.3"*) exit 0 ;;
  "release download v1.2.3 --dir "*)
    download_dir=${*: -1}
    for asset in "${GH_FIXTURE:?}"/release/*; do
      if [[ ${GH_SCENARIO:?} == missing && ${asset##*/} == gopro-yank-linux-amd64.tar.gz ]]; then continue; fi
      cp "$asset" "$download_dir/"
    done
    if [[ ${GH_SCENARIO:?} == extra ]]; then printf "obsolete\n" >"$download_dir/obsolete.zip"; fi
    if [[ ${GH_SCENARIO:?} == corrupt ]]; then printf 'corrupt\n' >"$download_dir/gopro-yank-linux-amd64.tar.gz"; fi
    exit 0
    ;;
  "release edit v1.2.3 --draft=false") : >"$GH_STATE"; exit 0 ;;
esac
printf 'unexpected gh invocation: %s\n' "$*" >&2
exit 2
GH
  chmod +x "$test_root/$1-bin/gh"
  printf '%s\n' "$fixture"
}

run_release() {
  local fixture=$1 scenario=$2
  GH_LOG="${fixture}-gh.log" GH_STATE="${fixture}-state" GH_SCENARIO="$scenario" GH_FIXTURE="$fixture" PATH="$test_root/${scenario}-bin:$PATH" \
    mise --cd "$fixture" run release v1.2.3
}

new_release=$(make_fixture new)
run_release "$new_release" new >/dev/null 2>&1
grep -Eq '^release create v1.2.3 --draft ' "$new_release-gh.log"
grep -Eq '^release edit v1.2.3 --draft=false' "$new_release-gh.log"

existing_release=$(make_fixture existing)
run_release "$existing_release" existing >/dev/null 2>&1
if grep -Eq '^release create v1.2.3' "$existing_release-gh.log"; then
  echo 'existing draft was recreated' >&2
  exit 1
fi
grep -Eq '^release upload v1.2.3' "$existing_release-gh.log"
grep -Eq '^release edit v1.2.3 --draft=false' "$existing_release-gh.log"

lookup_failure=$(make_fixture auth-failure)
if run_release "$lookup_failure" auth-failure >"$lookup_failure-output" 2>&1; then
  echo 'release lookup failure unexpectedly succeeded' >&2
  exit 1
fi
grep -Fq 'could not inspect release v1.2.3: HTTP 403: Forbidden' "$lookup_failure-output"
if grep -Eq '^release create v1.2.3' "$lookup_failure-gh.log"; then
  echo 'release lookup failure was masked by create' >&2
  exit 1
fi

published=$(make_fixture published)
if run_release "$published" published >/dev/null 2>&1; then
  echo 'published release unexpectedly succeeded' >&2
  exit 1
fi
if grep -Eq 'release (upload|create|edit)' "$published-gh.log"; then
  echo 'published release was modified' >&2
  exit 1
fi

missing=$(make_fixture missing)
if run_release "$missing" missing >"$missing-output" 2>&1; then
  echo 'missing upload unexpectedly succeeded' >&2
  exit 1
fi
grep -Fq 'release is missing uploaded asset gopro-yank-linux-amd64.tar.gz' "$missing-output"
if grep -Eq '^release edit v1.2.3 --draft=false' "$missing-gh.log"; then
  echo 'missing upload published the draft' >&2
  exit 1
fi

corrupt=$(make_fixture corrupt)
if run_release "$corrupt" corrupt >"$corrupt-output" 2>&1; then
  echo 'corrupt upload unexpectedly succeeded' >&2
  exit 1
fi
grep -Fq 'uploaded asset gopro-yank-linux-amd64.tar.gz does not match the built release' "$corrupt-output"
if grep -Eq '^release edit v1.2.3 --draft=false' "$corrupt-gh.log"; then
  echo 'corrupt upload published the draft' >&2
  exit 1
fi

extra=$(make_fixture extra)
if run_release "$extra" extra >"$extra-output" 2>&1; then
  echo 'unexpected draft asset was published' >&2
  exit 1
fi
grep -Fq 'draft contains unexpected asset obsolete.zip' "$extra-output"
if grep -Eq '^release edit v1.2.3 --draft=false' "$extra-gh.log"; then
  echo 'unexpected draft asset published the draft' >&2
  exit 1
fi

echo 'release contract tests passed'
