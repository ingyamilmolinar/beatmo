#!/usr/bin/env bash
# One-time helper to create and configure a public GCS bucket suitable
# for static site hosting / Cloud CDN origin.
#
# Usage:
#   GCP_PROJECT=my-project \
#   GCS_BUCKET=www-beatmo-io-static \
#   GCS_LOCATION=us-central1 \
#   ./scripts/setup-gcs-static-bucket.sh
#
# This will:
#   - Create the bucket (if it does not exist) with uniform bucket access.
#   - Configure index/error pages.
#   - Grant public read access to objects (storage.objectViewer for allUsers).
#
# You still need to wire Cloud CDN / HTTPS Load Balancer + DNS
# (point www.beatmo.io at the load balancer IP / hostname).

set -euo pipefail

GCP_PROJECT="${GCP_PROJECT:-}"
GCS_BUCKET="${GCS_BUCKET:-}"
GCS_LOCATION="${GCS_LOCATION:-us-central1}"

usage() {
  cat <<EOF
Usage:
  GCP_PROJECT=<project-id> GCS_BUCKET=<bucket-name> [GCS_LOCATION=us-central1] $0

Environment:
  GCP_PROJECT   GCP project ID (required)
  GCS_BUCKET    Target GCS bucket (required), e.g. www-beatmo-io-static
  GCS_LOCATION  Bucket location/region (default: us-central1)
EOF
  exit 1
}

if [[ -z "${GCP_PROJECT}" || -z "${GCS_BUCKET}" ]]; then
  echo "GCP_PROJECT and GCS_BUCKET must be set."
  usage
fi

if ! command -v gcloud >/dev/null 2>&1; then
  echo "gcloud is not installed."
  echo "Install Google Cloud SDK first: https://cloud.google.com/sdk/docs/install"
  exit 1
fi

echo "[gcs-setup] Project:   ${GCP_PROJECT}"
echo "[gcs-setup] Bucket:    gs://${GCS_BUCKET}"
echo "[gcs-setup] Location:  ${GCS_LOCATION}"

gcloud config set project "${GCP_PROJECT}" >/dev/null

echo "[gcs-setup] Creating bucket (if missing)..."
if ! gcloud storage buckets describe "gs://${GCS_BUCKET}" >/dev/null 2>&1; then
  gcloud storage buckets create "gs://${GCS_BUCKET}" \
    --project="${GCP_PROJECT}" \
    --location="${GCS_LOCATION}" \
    --uniform-bucket-level-access
else
  echo "[gcs-setup] Bucket already exists, skipping creation."
fi

echo "[gcs-setup] Configuring website main/error pages..."
gcloud storage buckets update "gs://${GCS_BUCKET}" \
  --web-main-page-suffix=index.html \
  --web-error-page=index.html

echo "[gcs-setup] Granting public read access for objects..."
gcloud storage buckets add-iam-policy-binding "gs://${GCS_BUCKET}" \
  --member="allUsers" \
  --role="roles/storage.objectViewer"

echo "[gcs-setup] Done. You can now deploy with:"
echo "  GCP_PROJECT=${GCP_PROJECT} GCS_BUCKET=${GCS_BUCKET} ./scripts/deploy-wasm-gcs.sh"

