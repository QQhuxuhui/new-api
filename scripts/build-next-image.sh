#!/usr/bin/env bash
# Build and push the rebuilt site's image from the current commit.
#
# The upstream Dockerfile is used unchanged except for package mirrors that
# are reachable from mainland China (goproxy.cn, npmmirror, Aliyun Debian);
# the patched copy lives in a temporary file, so upgrading upstream never
# conflicts with this script.
#
# Tag: next-<short commit>, so the image never takes a legacy v<N> tag.
# Version shown by the site: git describe (e.g. v1.0.0-rc.49-38-g332e8d9ac).
#
# Usage: scripts/build-next-image.sh [--push] [--no-cache]
set -euo pipefail

REGISTRY="registry.cn-shanghai.aliyuncs.com/hxh_ai/new-api"
PUSH=false
CACHE_FLAG=""
for arg in "$@"; do
  case "$arg" in
    --push) PUSH=true ;;
    --no-cache) CACHE_FLAG="--no-cache" ;;
    *) echo "unknown option: $arg" >&2; exit 2 ;;
  esac
done

cd "$(git rev-parse --show-toplevel)"
if [ -n "$(git status --porcelain --untracked-files=no)" ]; then
  echo "working tree has uncommitted changes; commit them so the tag matches the code" >&2
  exit 1
fi

SHORT_SHA="$(git rev-parse --short HEAD)"
VERSION="$(git describe --tags --always)"
IMAGE="${REGISTRY}:next-${SHORT_SHA}"
DOCKERFILE="$(mktemp)"

restore() {
  git checkout -- VERSION 2>/dev/null || true
  rm -f "$DOCKERFILE"
}
trap restore EXIT

awk '
  { print }
  /^FROM oven\/bun/ { print "ENV BUN_CONFIG_REGISTRY=https://registry.npmmirror.com" }
  /^FROM golang/   { print "ENV GOPROXY=https://goproxy.cn,direct" }
  /^FROM debian/   { print "RUN sed -i \x27s|deb.debian.org|mirrors.aliyun.com|g\x27 /etc/apt/sources.list.d/debian.sources" }
' Dockerfile > "$DOCKERFILE"

printf '%s' "$VERSION" > VERSION
echo "building ${IMAGE} (version ${VERSION})"
docker build $CACHE_FLAG -f "$DOCKERFILE" -t "$IMAGE" .

if [ "$PUSH" = true ]; then
  docker push "$IMAGE"
  echo "pushed ${IMAGE}"
else
  echo "built ${IMAGE}; run again with --push to publish"
fi
