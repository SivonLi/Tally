#!/usr/bin/env bash
set -euo pipefail

repo="${GITHUB_REPOSITORY:-}"
if [ -z "$repo" ]; then
  echo "缺少 GITHUB_REPOSITORY"
  exit 1
fi

version=$(jq -r .version mygo.json)
if ! [[ "$version" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  echo "版本号 $version 不是 MAJOR.MINOR.PATCH 格式"
  exit 1
fi

tag="v$version"

if ! awk -v version="$version" '$0 == "## " version { found = 1 } END { exit found ? 0 : 1 }' CHANGELOG.md; then
  echo "CHANGELOG.md 缺少 $version 的小节"
  exit 1
fi

latest=$(
  gh release list --repo "$repo" --exclude-drafts --limit 100 --json tagName \
    --jq '.[].tagName' |
    sed -n 's/^v\([0-9][0-9]*\.[0-9][0-9]*\.[0-9][0-9]*\)$/\1/p' |
    sort -t. -k1,1n -k2,2n -k3,3n |
    tail -n 1
)

if [ -n "$latest" ]; then
  if [ "$latest" = "$version" ]; then
    echo "版本 $version 已发布"
    exit 1
  fi
  highest=$(printf '%s\n%s\n' "$latest" "$version" | sort -t. -k1,1n -k2,2n -k3,3n | tail -n 1)
  if [ "$highest" != "$version" ]; then
    echo "版本 $version 不高于已发布版本 v$latest"
    exit 1
  fi
fi

if gh release view "$tag" --repo "$repo" > /dev/null 2>&1; then
  draft=$(gh release view "$tag" --repo "$repo" --json isDraft --jq .isDraft)
  if [ "$draft" != "true" ]; then
    echo "版本 $version 已发布"
    exit 1
  fi
  echo "发现遗留草稿 $tag，将在发布阶段清理"
fi

echo "版本 $version 校验通过"

if [ "${1:-}" = "--github-output" ]; then
  {
    echo "version=$version"
    echo "tag=$tag"
  } >> "${GITHUB_OUTPUT:?缺少 GITHUB_OUTPUT}"
fi
