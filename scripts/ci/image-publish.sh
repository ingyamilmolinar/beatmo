#!/usr/bin/env bash
# Makes sure both CI images exist in GHCR (build + push the missing ones) and
# writes their refs to $GITHUB_OUTPUT. Needs docker and, to push, GITHUB_TOKEN.
set -euo pipefail
root="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$root"
build_args=()
while IFS='=' read -r k v; do
  [[ -z "$k" || "$k" == \#* ]] && continue
  build_args+=(--build-arg "$k=$v")
done < ci/versions.env

if [[ -n "${GITHUB_TOKEN:-}" ]]; then
  echo "$GITHUB_TOKEN" | docker login ghcr.io -u "${GITHUB_ACTOR:-ci}" --password-stdin >/dev/null
fi

publish() {
  local kind="$1"; shift
  local ref; ref="$(scripts/ci/image-tag.sh "$kind")"
  local cache="${ref%%:*}:buildcache"
  if docker manifest inspect "$ref" >/dev/null 2>&1; then
    echo "$kind: $ref already published"
  else
    if [[ "${CI_IMAGE_PUSH:-1}" != "1" ]]; then
      echo "::error::$ref is not published and this run cannot push it (a fork PR that changes ci/); a maintainer must push the change" >&2
      exit 1
    fi
    echo "$kind: building $ref"
    docker buildx inspect beatmo >/dev/null 2>&1 || docker buildx create --name beatmo --driver docker-container >/dev/null
    docker buildx build --builder beatmo --push "${build_args[@]}" "$@" \
      --cache-from "type=registry,ref=$cache" --cache-to "type=registry,ref=$cache,mode=max" \
      -f "ci/images/$kind.Dockerfile" -t "$ref" ci/images
  fi
  [[ -n "${GITHUB_OUTPUT:-}" ]] && echo "$kind=$ref" >> "$GITHUB_OUTPUT"
  return 0
}

publish build
publish browser --build-arg "BUILD_IMAGE=$(scripts/ci/image-tag.sh build)"
