#!/usr/bin/env bash
# Prints the image ref for a kind (build|browser). The tag hashes everything the
# images are built from, so an unchanged image is never rebuilt.
set -euo pipefail
kind="${1:?usage: image-tag.sh build|browser}"
root="$(cd "$(dirname "$0")/../.." && pwd)"
hash="$(cat "$root/ci/versions.env" "$root/ci/images/build.Dockerfile" "$root/ci/images/browser.Dockerfile" | sha256sum | cut -c1-16)"
echo "ghcr.io/ingyamilmolinar/beatmo-ci-${kind}:${hash}"
