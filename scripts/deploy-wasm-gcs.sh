#!/usr/bin/env bash
# Usage: GCP_PROJECT=... GCS_BUCKET=... ./scripts/deploy-wasm-gcs.sh [--purge-cdn] (gcloud auth login, or a service account).
# More than an rsync: streaming compile needs Content-Type application/wasm and gcloud's inferred MIME depends on the host;
# every runtime import must ship together; and Cache-Control must keep users off half-old/half-new bundles.

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

GCP_PROJECT="${GCP_PROJECT:-}"
GCS_BUCKET="${GCS_BUCKET:-}"
DRY_RUN="${DRY_RUN:-0}"          # Set to 1 for a no-op preview of the sync.
PURGE_CDN="${PURGE_CDN:-0}"      # Set to 1 (or pass --purge-cdn) to invalidate.

for arg in "$@"; do
  case "${arg}" in
    --purge-cdn) PURGE_CDN=1 ;;
    --dry-run)   DRY_RUN=1 ;;
    -h|--help)
      cat <<EOF
Usage:
  GCP_PROJECT=<project-id> GCS_BUCKET=<bucket-name> $0 [--purge-cdn] [--dry-run]

Environment:
  GCP_PROJECT   GCP project ID (required)
  GCS_BUCKET    Target GCS bucket (required), e.g. www-beatmo-io-static
  DRY_RUN       If "1" (or --dry-run), perform a dry-run sync (no changes)
  PURGE_CDN     If "1" (or --purge-cdn), invalidate Cloud CDN after upload
EOF
      exit 0
      ;;
  esac
done

if [[ -z "${GCP_PROJECT}" || -z "${GCS_BUCKET}" ]]; then
  echo "GCP_PROJECT and GCS_BUCKET must be set." >&2
  exit 1
fi

if ! command -v gcloud >/dev/null 2>&1 && ! command -v gsutil >/dev/null 2>&1; then
  echo "Neither gcloud nor gsutil is installed." >&2
  echo "Install Google Cloud SDK first: https://cloud.google.com/sdk/docs/install" >&2
  exit 1
fi

URL_MAP="${GCS_BUCKET}-lb"

echo "[deploy] Project:   ${GCP_PROJECT}"
echo "[deploy] Bucket:    gs://${GCS_BUCKET}"
echo "[deploy] URL map:   ${URL_MAP}"
echo "[deploy] Dry run:   ${DRY_RUN}"
echo "[deploy] Purge CDN: ${PURGE_CDN}"

cd "${ROOT_DIR}"

echo "[deploy] Building WASM via 'make wasm'..."
make wasm

BUILD_DIR="${ROOT_DIR}/build/wasm_site"
echo "[deploy] Staging static site into ${BUILD_DIR}..."
rm -rf "${BUILD_DIR}"
mkdir -p "${BUILD_DIR}"

# Every file the browser fetches at runtime; a new runtime dependency goes here AND in manifest_drift_test.go. ENTRY =
# always revalidated; ASSET = 5-minute TTL; WASM = content-hashed, immutable, brotli; MEDIA = uploaded uncompressed,
# since gzip would break the Range requests video playback relies on.
ENTRY_FILES=(
  "index.html"
  "synth_param_abi.gen.js"
  "chain_spec.gen.js"
)

ASSET_FILES=(
  "landing.js"
  "landing_crystal.js"
  "audio.js"
  "wasm_exec.js"
  "drums.single.js"
  "recording_capture_worklet.js"
  "recording_encoder_worker.js"
  "synth_render_worker.js"
  "voice_post.js"
  "dsp_host.js"
  "dsp_worklet.js"
  "played_voice.js"
  "audio_probe_worklet.js"
  "audio_probe_analysis.js"
)

WASM_FILES=(
  "main.wasm"
)

MEDIA_FILES=(
  "landing/desktop.mp4"
  "landing/desktop.webp"
  "landing/desktop.vtt"
  "landing/phone.mp4"
  "landing/phone.webp"
  "landing/phone.vtt"
)

declare -a MISSING=()
stage_file() {
  local rel="$1"
  local src="src/js/${rel}"
  if [[ ! -f "${src}" ]]; then
    MISSING+=("${src}")
    return
  fi
  mkdir -p "$(dirname "${BUILD_DIR}/${rel}")"
  cp "${src}" "${BUILD_DIR}/${rel}"
}

for f in "${ENTRY_FILES[@]}" "${ASSET_FILES[@]}" "${WASM_FILES[@]}" "${MEDIA_FILES[@]}"; do
  stage_file "${f}"
done

if (( ${#MISSING[@]} > 0 )); then
  echo "[deploy] ERROR: declared manifest files are missing from src/js/:" >&2
  for m in "${MISSING[@]}"; do echo "  - ${m}" >&2; done
  echo "Run 'make wasm' (and the codegen targets) before deploying." >&2
  exit 1
fi

# No audio file ever ships, even if a manifest edit or a stray copy put one in the staging dir.
AUDIO_STAGED="$(find "${BUILD_DIR}" -type f -regextype posix-extended \
  -iregex '.*\.(wav|wave|mp3|aif|aiff|aifc|flac|ogg|oga|opus|m4a|aac|wma|caf)$' -print)"
if [[ -n "${AUDIO_STAGED}" ]]; then
  echo "[deploy] ERROR: audio files must never be deployed:" >&2
  printf '%s\n' "${AUDIO_STAGED}" | sed 's/^/  - /' >&2
  exit 1
fi

# index.html is the content-hashed name's only reference.
WASM_HASH="$(sha256sum "${BUILD_DIR}/main.wasm" | cut -c1-16)"
WASM_NAME="main.${WASM_HASH}.wasm"
mv "${BUILD_DIR}/main.wasm" "${BUILD_DIR}/${WASM_NAME}"
sed -i "s/\"main\.wasm\"/\"${WASM_NAME}\"/g" "${BUILD_DIR}/index.html"
if [[ "$(grep -c "${WASM_NAME}" "${BUILD_DIR}/index.html")" -lt 1 ]] || grep -q '"main\.wasm"' "${BUILD_DIR}/index.html"; then
  echo "[deploy] ERROR: could not stamp ${WASM_NAME} into index.html" >&2
  exit 1
fi
WASM_FILES[0]="${WASM_NAME}"   # (not an array literal: the manifest drift test parses those)
echo "[deploy] main.wasm -> ${WASM_NAME}"

echo "[deploy] Contents staged:"
(cd "${BUILD_DIR}" && find . -type f | sort)

# Uploaded gzip-compressed under the same names and stamped Content-Encoding: gzip below, so the browser downloads the
# compressed bytes: the biggest load-time win.
echo "[deploy] Gzip-compressing staged objects..."
for f in "${ENTRY_FILES[@]}" "${ASSET_FILES[@]}"; do
  staged="${BUILD_DIR}/${f}"
  [[ -f "${staged}" ]] || continue
  raw_bytes=$(wc -c < "${staged}")
  gzip -9 -c "${staged}" > "${staged}.gz"
  mv "${staged}.gz" "${staged}"
  gz_bytes=$(wc -c < "${staged}")
  printf '  %-30s %10d -> %10d bytes\n' "${f}" "${raw_bytes}" "${gz_bytes}"
done

# main.wasm: brotli (quality 11, 16 MiB window) through Node's zlib — no extra
# tool to install. Every browser that can run the game accepts br over HTTPS.
echo "[deploy] Brotli-compressing ${WASM_NAME}..."
WASM_ENCODING="br"
staged="${BUILD_DIR}/${WASM_NAME}"
raw_bytes=$(wc -c < "${staged}")
node -e '
const z = require("zlib"), fs = require("fs"), f = process.argv[1];
const b = fs.readFileSync(f);
fs.writeFileSync(f + ".br", z.brotliCompressSync(b, { params: {
  [z.constants.BROTLI_PARAM_QUALITY]: 11, [z.constants.BROTLI_PARAM_LGWIN]: 24,
  [z.constants.BROTLI_PARAM_SIZE_HINT]: b.length } }));' "${staged}"
mv "${staged}.br" "${staged}"
printf '  %-30s %10d -> %10d bytes (br)\n' "${WASM_NAME}" "${raw_bytes}" "$(wc -c < "${staged}")"

# Sanity-check that the bucket exists before attempting rsync.
if command -v gcloud >/dev/null 2>&1 && gcloud storage --help >/dev/null 2>&1; then
  gcloud config set project "${GCP_PROJECT}" >/dev/null
  if ! gcloud storage buckets describe "gs://${GCS_BUCKET}" >/dev/null 2>&1; then
    echo "[deploy] ERROR: Bucket gs://${GCS_BUCKET} does not exist." >&2
    echo "[deploy] Run scripts/setup-gcs-static-bucket.sh first, e.g.:" >&2
    echo "  GCP_PROJECT=${GCP_PROJECT} GCS_BUCKET=${GCS_BUCKET} ./scripts/setup-gcs-static-bucket.sh" >&2
    exit 1
  fi
fi

if command -v gcloud >/dev/null 2>&1 && gcloud storage --help >/dev/null 2>&1; then
  USE_GCLOUD=1
else
  USE_GCLOUD=0
fi

if (( USE_GCLOUD == 1 )); then
  echo "[deploy] Using 'gcloud storage rsync'..."
  if [[ "${DRY_RUN}" == "1" ]]; then
    gcloud storage rsync --recursive "${BUILD_DIR}" "gs://${GCS_BUCKET}" \
      --dry-run \
      --delete-unmatched-destination-objects
  else
    gcloud storage rsync --recursive "${BUILD_DIR}" "gs://${GCS_BUCKET}" \
      --delete-unmatched-destination-objects
  fi
else
  echo "[deploy] Using 'gsutil rsync'..."
  if [[ "${DRY_RUN}" == "1" ]]; then
    gsutil -m rsync -n -r "${BUILD_DIR}" "gs://${GCS_BUCKET}"
  else
    gsutil -m rsync -r -d "${BUILD_DIR}" "gs://${GCS_BUCKET}"
  fi
fi

# Stamped explicitly: gcloud's inferred MIME is unreliable for .wasm. no-transform stops GCS from decompressively
# transcoding, which strips Content-Length and breaks instantiateStreaming.
ENTRY_CACHE="no-cache, must-revalidate, no-transform"
ASSET_CACHE="public, max-age=300, must-revalidate, no-transform"
WASM_CACHE="public, max-age=31536000, immutable, no-transform"   # content-hashed name

set_headers() {
  local rel="$1" ct="$2" cc="$3" ce="${4-gzip}"   # ce "" = stored uncompressed
  local obj="gs://${GCS_BUCKET}/${rel}"
  if [[ "${DRY_RUN}" == "1" ]]; then
    echo "[deploy:dry] would set ${obj} -> Content-Type=${ct} Content-Encoding=${ce:-<none>} Cache-Control=${cc}"
    return
  fi
  if (( USE_GCLOUD == 1 )); then
    local enc=(--clear-content-encoding)
    [[ -n "${ce}" ]] && enc=(--content-encoding="${ce}")
    gcloud storage objects update "${obj}" \
      --content-type="${ct}" \
      "${enc[@]}" \
      --cache-control="${cc}" >/dev/null
  else
    local enc=()
    [[ -n "${ce}" ]] && enc=(-h "Content-Encoding:${ce}")
    gsutil -h "Content-Type:${ct}" "${enc[@]}" -h "Cache-Control:${cc}" setmeta "${obj}" >/dev/null
  fi
}

for f in "${ENTRY_FILES[@]}"; do
  case "${f}" in
    *.html) set_headers "${f}" "text/html; charset=utf-8" "${ENTRY_CACHE}" ;;
    *.js)   set_headers "${f}" "application/javascript; charset=utf-8" "${ENTRY_CACHE}" ;;
    *) echo "[deploy] WARN: no Content-Type rule for entry file ${f}" >&2 ;;
  esac
done

for f in "${ASSET_FILES[@]}"; do
  case "${f}" in
    *.js) set_headers "${f}" "application/javascript; charset=utf-8" "${ASSET_CACHE}" ;;
    *) echo "[deploy] WARN: no Content-Type rule for asset file ${f}" >&2 ;;
  esac
done

for f in "${WASM_FILES[@]}"; do
  set_headers "${f}" "application/wasm" "${WASM_CACHE}" "${WASM_ENCODING}"
done

for f in "${MEDIA_FILES[@]}"; do
  case "${f}" in
    *.mp4)  set_headers "${f}" "video/mp4" "${ASSET_CACHE}" "" ;;
    *.webp) set_headers "${f}" "image/webp" "${ASSET_CACHE}" "" ;;
    *.vtt)  set_headers "${f}" "text/vtt; charset=utf-8" "${ASSET_CACHE}" "" ;;
    *) echo "[deploy] WARN: no Content-Type rule for media file ${f}" >&2 ;;
  esac
done

# Off by default; --purge-cdn after a Content-Type/Cache-Control fix or an index.html change that needs an immediate cutover.
if [[ "${PURGE_CDN}" == "1" ]]; then
  if [[ "${DRY_RUN}" == "1" ]]; then
    echo "[deploy:dry] would purge Cloud CDN: url-map=${URL_MAP} path=/*"
  else
    echo "[deploy] Purging Cloud CDN cache (url-map=${URL_MAP}, path=/*)..."
    if (( USE_GCLOUD == 1 )); then
      gcloud compute url-maps invalidate-cdn-cache "${URL_MAP}" \
        --path "/*" --async
    else
      echo "[deploy] WARN: gcloud not available; cannot purge CDN." >&2
    fi
  fi
fi

echo "[deploy] Deploy complete."
