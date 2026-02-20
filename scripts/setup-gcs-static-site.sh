#!/usr/bin/env bash
# Set up GCP resources for static site hosting with Cloud CDN.
#
# Usage:
#   GCP_PROJECT=beatmo GCS_BUCKET=www-beatmo-io-static DOMAIN=beatmo.io ./scripts/setup-gcs-static-site.sh
#
# This script is idempotent - it checks if resources exist before creating them.

set -euo pipefail

GCP_PROJECT="${GCP_PROJECT:-beatmo}"
GCS_BUCKET="${GCS_BUCKET:-www-beatmo-io-static}"
DOMAIN="${DOMAIN:-beatmo.io}"
DOMAIN_WWW="www.${DOMAIN}"

# Resource naming (derived from bucket name)
BACKEND_BUCKET="${GCS_BUCKET}"
URL_MAP="${GCS_BUCKET}-lb"
HTTP_PROXY="${GCS_BUCKET}-lb-target-proxy"
HTTPS_PROXY="${GCS_BUCKET}-lb-https-proxy"
SSL_CERT="${DOMAIN//./-}-cert"
IP_NAME="${DOMAIN//./-}-ip"
HTTP_FORWARDING_RULE="${GCS_BUCKET}-lb-forwarding-rule-ipv4"
HTTP_FORWARDING_RULE_IPV6="${GCS_BUCKET}-lb-forwarding-rule-ipv6"
HTTPS_FORWARDING_RULE="${GCS_BUCKET}-lb-https-rule"

echo "=== GCP Static Site Setup ==="
echo "Project:  ${GCP_PROJECT}"
echo "Bucket:   ${GCS_BUCKET}"
echo "Domain:   ${DOMAIN}"
echo ""

# Check for gcloud
if ! command -v gcloud >/dev/null 2>&1; then
  echo "ERROR: gcloud CLI not found. Install: https://cloud.google.com/sdk/docs/install"
  exit 1
fi

# Set project
gcloud config set project "${GCP_PROJECT}" >/dev/null

# Helper: check if a resource exists
resource_exists() {
  local type="$1"
  local name="$2"
  local extra="${3:-}"

  case "${type}" in
    bucket)
      gcloud storage buckets describe "gs://${name}" >/dev/null 2>&1
      ;;
    backend-bucket)
      gcloud compute backend-buckets describe "${name}" >/dev/null 2>&1
      ;;
    url-map)
      gcloud compute url-maps describe "${name}" --global >/dev/null 2>&1
      ;;
    target-http-proxy)
      gcloud compute target-http-proxies describe "${name}" --global >/dev/null 2>&1
      ;;
    target-https-proxy)
      gcloud compute target-https-proxies describe "${name}" --global >/dev/null 2>&1
      ;;
    ssl-certificate)
      gcloud compute ssl-certificates describe "${name}" --global >/dev/null 2>&1
      ;;
    address)
      gcloud compute addresses describe "${name}" --global >/dev/null 2>&1
      ;;
    forwarding-rule)
      gcloud compute forwarding-rules describe "${name}" --global >/dev/null 2>&1
      ;;
    *)
      echo "Unknown resource type: ${type}"
      return 1
      ;;
  esac
}

# 1. Create GCS bucket
echo "[1/9] GCS Bucket..."
if resource_exists bucket "${GCS_BUCKET}"; then
  echo "      Bucket gs://${GCS_BUCKET} already exists"
else
  echo "      Creating bucket gs://${GCS_BUCKET}..."
  gcloud storage buckets create "gs://${GCS_BUCKET}" \
    --location=US \
    --uniform-bucket-level-access
fi

# 2. Configure bucket for static website hosting
echo "[2/9] Bucket website configuration..."
gcloud storage buckets update "gs://${GCS_BUCKET}" \
  --web-main-page-suffix=index.html \
  --web-error-page=index.html

# 3. Make bucket publicly readable
echo "[3/9] Bucket public access..."
gcloud storage buckets add-iam-policy-binding "gs://${GCS_BUCKET}" \
  --member=allUsers \
  --role=roles/storage.objectViewer 2>/dev/null || true

# 4. Create backend bucket with CDN
echo "[4/9] Backend bucket..."
if resource_exists backend-bucket "${BACKEND_BUCKET}"; then
  echo "      Backend bucket ${BACKEND_BUCKET} already exists"
else
  echo "      Creating backend bucket ${BACKEND_BUCKET}..."
  gcloud compute backend-buckets create "${BACKEND_BUCKET}" \
    --gcs-bucket-name="${GCS_BUCKET}" \
    --enable-cdn
fi

# 5. Create URL map (load balancer)
echo "[5/9] URL map (load balancer)..."
if resource_exists url-map "${URL_MAP}"; then
  echo "      URL map ${URL_MAP} already exists"
else
  echo "      Creating URL map ${URL_MAP}..."
  gcloud compute url-maps create "${URL_MAP}" \
    --default-backend-bucket="${BACKEND_BUCKET}" \
    --global
fi

# 6. Create HTTP target proxy
echo "[6/9] HTTP target proxy..."
if resource_exists target-http-proxy "${HTTP_PROXY}"; then
  echo "      HTTP proxy ${HTTP_PROXY} already exists"
else
  echo "      Creating HTTP proxy ${HTTP_PROXY}..."
  gcloud compute target-http-proxies create "${HTTP_PROXY}" \
    --url-map="${URL_MAP}" \
    --global
fi

# 7. Create SSL certificate and HTTPS target proxy
echo "[7/9] SSL certificate and HTTPS proxy..."
if resource_exists ssl-certificate "${SSL_CERT}"; then
  echo "      SSL certificate ${SSL_CERT} already exists"
else
  echo "      Creating SSL certificate ${SSL_CERT}..."
  gcloud compute ssl-certificates create "${SSL_CERT}" \
    --domains="${DOMAIN},${DOMAIN_WWW}" \
    --global
fi

if resource_exists target-https-proxy "${HTTPS_PROXY}"; then
  echo "      HTTPS proxy ${HTTPS_PROXY} already exists"
else
  echo "      Creating HTTPS proxy ${HTTPS_PROXY}..."
  gcloud compute target-https-proxies create "${HTTPS_PROXY}" \
    --url-map="${URL_MAP}" \
    --ssl-certificates="${SSL_CERT}" \
    --global
fi

# 8. Reserve static IP address
echo "[8/9] Static IP address..."
if resource_exists address "${IP_NAME}"; then
  echo "      IP address ${IP_NAME} already exists"
else
  echo "      Reserving IP address ${IP_NAME}..."
  gcloud compute addresses create "${IP_NAME}" \
    --ip-version=IPV4 \
    --global
fi

IP_ADDRESS=$(gcloud compute addresses describe "${IP_NAME}" --global --format="value(address)")
echo "      IP Address: ${IP_ADDRESS}"

# 9. Create forwarding rules
echo "[9/9] Forwarding rules..."

# HTTP forwarding rule
if resource_exists forwarding-rule "${HTTP_FORWARDING_RULE}"; then
  echo "      HTTP forwarding rule already exists"
else
  echo "      Creating HTTP forwarding rule..."
  gcloud compute forwarding-rules create "${HTTP_FORWARDING_RULE}" \
    --load-balancing-scheme=EXTERNAL_MANAGED \
    --network-tier=PREMIUM \
    --address="${IP_NAME}" \
    --global \
    --target-http-proxy="${HTTP_PROXY}" \
    --ports=80
fi

# HTTPS forwarding rule
if resource_exists forwarding-rule "${HTTPS_FORWARDING_RULE}"; then
  echo "      HTTPS forwarding rule already exists"
else
  echo "      Creating HTTPS forwarding rule..."
  gcloud compute forwarding-rules create "${HTTPS_FORWARDING_RULE}" \
    --load-balancing-scheme=EXTERNAL_MANAGED \
    --network-tier=PREMIUM \
    --address="${IP_NAME}" \
    --global \
    --target-https-proxy="${HTTPS_PROXY}" \
    --ports=443
fi

echo ""
echo "=== Setup Complete ==="
echo ""
echo "Static IP: ${IP_ADDRESS}"
echo ""
echo "DNS Configuration Required:"
echo "  Add an A record for ${DOMAIN} pointing to ${IP_ADDRESS}"
echo "  Add an A record for ${DOMAIN_WWW} pointing to ${IP_ADDRESS}"
echo ""
echo "SSL Certificate Status:"
gcloud compute ssl-certificates describe "${SSL_CERT}" --global \
  --format="table(managed.status,managed.domainStatus)"
echo ""
echo "Note: SSL certificate provisioning may take 10-20 minutes."
echo "Check status with:"
echo "  gcloud compute ssl-certificates describe ${SSL_CERT} --global --format='yaml(managed)'"
echo ""
echo "Deploy your site with:"
echo "  make deploy"
