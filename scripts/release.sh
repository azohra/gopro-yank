#!/usr/bin/env bash
set -euo pipefail

tag=${1:?Provide a Release Please tag}
case "$tag" in
  site/v*) component=site; latest=false ;;
  v*) component=app; latest=true ;;
  *) echo 'release: expected v… or site/v…' >&2; exit 1 ;;
esac

draft=$(gh release view "$tag" --json isDraft --jq .isDraft)
if [ "$draft" = false ]; then
  echo "release: $tag is already published"
  exit 0
fi

git fetch --quiet origin "refs/tags/$tag"
released=$(git rev-parse 'FETCH_HEAD^{commit}')
tree=$(git rev-parse 'FETCH_HEAD^{tree}')
version=$(git show "$released:$component/VERSION")
[ "${tag##*/}" = "v$version" ] || { echo 'release: tag and component version differ' >&2; exit 1; }
pr=$(gh api "repos/{owner}/{repo}/commits/$released/pulls" --jq ".[] | select(.merged_at != null and .merge_commit_sha == \"$released\" and .base.ref == \"main\") | .number")
[ -n "$pr" ] || { echo 'release: no merged PR found for this tag' >&2; exit 1; }
head=$(gh pr view "$pr" --json headRefOid --jq .headRefOid)
git fetch --quiet origin "refs/pull/$pr/head"
[ "$(git rev-parse FETCH_HEAD)" = "$head" ] && [ "$(git rev-parse 'FETCH_HEAD^{tree}')" = "$tree" ] || {
  echo 'release: the tag differs from the checked candidate' >&2
  exit 1
}

run=$(gh api "repos/{owner}/{repo}/actions/artifacts?name=release-$head" --jq '.artifacts | map(select(.expired == false)) | first.workflow_run.id')
[ -n "$run" ] && [ "$run" != null ] || { echo 'release: candidate artifacts are missing or expired' >&2; exit 1; }
result=$(gh run view "$run" --json conclusion,headSha,workflowName --jq '.conclusion + " " + .headSha + " " + .workflowName')
[ "$result" = "success $head Check" ] || { echo 'release: candidate Check did not pass' >&2; exit 1; }

scratch=$(mktemp -d)
trap 'rm -rf "$scratch"' EXIT
gh run download "$run" --name "release-$head" --dir "$scratch"
files=("$scratch/$component/release/"*)
[ -f "${files[0]}" ] || { echo "release: no $component artifacts in the candidate" >&2; exit 1; }
gh release upload "$tag" "${files[@]}" --clobber
gh release edit "$tag" --draft=false --latest="$latest"
