#!/usr/bin/env bash
# Build the WASM bundle and sync it to a GCS bucket for CDN/static hosting.
#
# Usage:
#   GCP_PROJECT=my-project \
#   GCS_BUCKET=www-beatmo-io-static \
#   ./scripts/deploy-wasm-gcs.sh
#
# Auth:
#   - Locally: run `gcloud auth login` (and `gcloud config set project` if desired),
#     or set up a service account and run
#       gcloud auth activate-service-account --key-file=key.json
#   - In CI: activate a service account before invoking this script.
#
# The script only handles build + upload; Cloud CDN / load balancer wiring is a
# one-time GCP configuration step.

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

GCP_PROJECT="${GCP_PROJECT:-}"
GCS_BUCKET="${GCS_BUCKET:-}"
DRY_RUN="${DRY_RUN:-0}"          # Set to 1 for a no-op preview of the sync.

usage() {
  cat <<EOF
Usage:
  GCP_PROJECT=<project-id> GCS_BUCKET=<bucket-name> $0

Environment:
  GCP_PROJECT   GCP project ID (required)
  GCS_BUCKET    Target GCS bucket (required), e.g. www-beatmo-io-static
  DRY_RUN       If "1", perform a dry-run sync (no changes)
EOF
  exit 1
}

if [[ -z "${GCP_PROJECT}" || -z "${GCS_BUCKET}" ]]; then
  echo "GCP_PROJECT and GCS_BUCKET must be set."
  usage
fi

if ! command -v gcloud >/dev/null 2>&1 && ! command -v gsutil >/dev/null 2>&1; then
  echo "Neither gcloud nor gsutil is installed."
  echo "Install Google Cloud SDK first: https://cloud.google.com/sdk/docs/install"
  exit 1
fi

echo "[deploy] Project: ${GCP_PROJECT}"
echo "[deploy] Bucket:  gs://${GCS_BUCKET}"
echo "[deploy] Dry run: ${DRY_RUN}"

cd "${ROOT_DIR}"

echo "[deploy] Building WASM via 'make wasm'..."
make wasm

BUILD_DIR="${ROOT_DIR}/build/wasm_site"
echo "[deploy] Staging static site into ${BUILD_DIR}..."
rm -rf "${BUILD_DIR}"
mkdir -p "${BUILD_DIR}"

# Minimal files required to run the WASM game in a browser.
cp src/js/index.html "${BUILD_DIR}/"
cp src/js/audio.js "${BUILD_DIR}/"
cp src/js/wasm_exec.js "${BUILD_DIR}/"
cp src/js/drums.single.js "${BUILD_DIR}/"
cp src/js/main.wasm "${BUILD_DIR}/"

echo "[deploy] Contents staged:"
ls -1 "${BUILD_DIR}"

# Sanity-check that the bucket exists before attempting rsync, so we can emit
# a clearer error instead of a generic 404 from the storage API.
if command -v gcloud >/dev/null 2>&1 && gcloud storage --help >/dev/null 2>&1; then
  gcloud config set project "${GCP_PROJECT}" >/dev/null
  if ! gcloud storage buckets describe "gs://${GCS_BUCKET}" >/dev/null 2>&1; then
    echo "[deploy] ERROR: Bucket gs://${GCS_BUCKET} does not exist."
    echo "[deploy] Run scripts/setup-gcs-static-bucket.sh first, e.g.:"
    echo "  GCP_PROJECT=${GCP_PROJECT} GCS_BUCKET=${GCS_BUCKET} ./scripts/setup-gcs-static-bucket.sh"
    exit 1
  fi
fi

if command -v gcloud >/dev/null 2>&1 && gcloud storage --help >/dev/null 2>&1; then
  echo "[deploy] Using 'gcloud storage rsync'..."
  # Ensure the correct project is active for this operation.
  gcloud config set project "${GCP_PROJECT}" >/dev/null
  if [[ "${DRY_RUN}" == "1" ]]; then
    gcloud storage rsync "${BUILD_DIR}" "gs://${GCS_BUCKET}" \
      --dry-run \
      --delete-unmatched-destination-objects
  else
    gcloud storage rsync "${BUILD_DIR}" "gs://${GCS_BUCKET}" \
      --delete-unmatched-destination-objects
  fi
elif command -v gsutil >/dev/null 2>&1; then
  echo "[deploy] Using 'gsutil rsync'..."
  if [[ "${DRY_RUN}" == "1" ]]; then
    gsutil -m rsync -n -r "${BUILD_DIR}" "gs://${GCS_BUCKET}"
  else
    gsutil -m rsync -r -d "${BUILD_DIR}" "gs://${GCS_BUCKET}"
  fi
else
  echo "[deploy] No supported GCS sync tool found (gcloud/gsutil)."
  exit 1
fi

echo "[deploy] Deploy complete."
