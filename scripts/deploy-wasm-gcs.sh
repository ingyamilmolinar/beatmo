#!/usr/bin/env bash
# Build the WASM bundle and sync it to a GCS bucket for CDN/static hosting.
#
# Usage:
#   GCP_PROJECT=my-project \
#   GCS_BUCKET=www-beatmo-io-static \
#   ./scripts/deploy-wasm-gcs.sh [--purge-cdn]
#
# Auth:
#   - Locally: run `gcloud auth login` (and `gcloud config set project` if desired),
#     or set up a service account and run
#       gcloud auth activate-service-account --key-file=key.json
#   - In CI: activate a service account before invoking this script.
#
# Why this is more than a copy/rsync:
#   - index.html uses WebAssembly.instantiateStreaming(fetch("main.wasm"), ...),
#     which mandates Content-Type: application/wasm. Letting gcloud infer the
#     MIME is non-deterministic (depends on the host's Python mimetypes db) and
#     a wrong MIME breaks the streaming compile on Safari + Chrome.
#   - audio.js fetches worklets and workers via relative URLs at runtime
#     (recording_capture_worklet.js, recording_encoder_worker.js,
#     synth_render_worker.js — the off-thread synth renderer, which in turn
#     imports drums.single.js and voice_post.js; dsp_worklet.js — the C stages
#     after the voice, which import drums.single.js and dsp_host.js) and
#     statically imports synth_param_abi.gen.js, dsp_host.js and played_voice.js.
#     They must all ship together; the drift test
#     src/go/internal/deploy/manifest_drift_test.go enforces this.
#   - Cloud CDN happily caches both the entry HTML and the assets; we must set
#     Cache-Control so users pick up new bundles instead of half-old/half-new.

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

GCP_PROJECT="${GCP_PROJECT:-}"
GCS_BUCKET="${GCS_BUCKET:-}"
DRY_RUN="${DRY_RUN:-0}"          # Set to 1 for a no-op preview of the sync.
PURGE_CDN="${PURGE_CDN:-0}"      # Set to 1 (or pass --purge-cdn) to invalidate.
STAGE_ONLY="${STAGE_ONLY:-0}"    # Set to 1 (or --stage-only): build + stage, upload nothing.

for arg in "$@"; do
  case "${arg}" in
    --purge-cdn) PURGE_CDN=1 ;;
    --dry-run)   DRY_RUN=1 ;;
    --stage-only) STAGE_ONLY=1 ;;
    -h|--help)
      cat <<EOF
Usage:
  GCP_PROJECT=<project-id> GCS_BUCKET=<bucket-name> $0 [--purge-cdn] [--dry-run]
  $0 --stage-only

Environment:
  GCP_PROJECT   GCP project ID (required)
  GCS_BUCKET    Target GCS bucket (required), e.g. www-beatmo-io-static
  DRY_RUN       If "1" (or --dry-run), perform a dry-run sync (no changes)
  PURGE_CDN     If "1" (or --purge-cdn), invalidate Cloud CDN after upload
  STAGE_ONLY    If "1" (or --stage-only), build and stage build/wasm_site exactly
                as a deploy would, then stop: no GCP project, gcloud or upload
EOF
      exit 0
      ;;
  esac
done

if [[ "${STAGE_ONLY}" != "1" ]]; then
  if [[ -z "${GCP_PROJECT}" || -z "${GCS_BUCKET}" ]]; then
    echo "GCP_PROJECT and GCS_BUCKET must be set." >&2
    exit 1
  fi

  if ! command -v gcloud >/dev/null 2>&1 && ! command -v gsutil >/dev/null 2>&1; then
    echo "Neither gcloud nor gsutil is installed." >&2
    echo "Install Google Cloud SDK first: https://cloud.google.com/sdk/docs/install" >&2
    exit 1
  fi
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

# ---------------------------------------------------------------------------
# Deploy manifest: every file the browser fetches at runtime.
#
# Buckets:
#   ENTRY_FILES   -> Cache-Control: no-cache, must-revalidate
#                   (index.html + codegen output -- always re-validate so new
#                    bundles take effect on the next visit)
#   ASSET_FILES   -> Cache-Control: public, max-age=300, must-revalidate
#                   (5-minute TTL until we adopt content-hashed filenames)
#   WASM_FILES    -> Content-Type: application/wasm (mandatory for streaming
#                   compile; without it Safari/Chrome reject the WebAssembly
#                   instantiation). main.wasm is renamed to its content hash
#                   (main.<sha16>.wasm, the only reference is stamped into
#                   index.html), cached immutable, and brotli-compressed
#                   (Content-Encoding: br): 6.57 MB gzip -> 4.60 MB br,
#                   cold time-to-ready -11% on the 4G profile (2026-10-23,
#                   src/js/landing_load.browser.test.js).
#   MEDIA_FILES   -> the landing page's recordings, posters and cue files
#                   (spec docs/superpowers/specs/2026-10-23-landing-page-design.md).
#                   Uploaded UNcompressed: gzip would break the HTTP Range
#                   requests video playback relies on, and mp4/webp are
#                   already compressed.
#
# Adding a new runtime dependency? Add it here AND update the drift test
# src/go/internal/deploy/manifest_drift_test.go in the same change.
# ---------------------------------------------------------------------------
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

# No audio file is ever shipped: reference recordings are local dev tooling
# and every instrument is synthesized. Refuse to upload one even if a manifest
# edit or a stray copy put it in the staging dir. Guard:
# TestDeployShipsNoAudioFiles (src/go/internal/deploy).
AUDIO_STAGED="$(find "${BUILD_DIR}" -type f -regextype posix-extended \
  -iregex '.*\.(wav|wave|mp3|aif|aiff|aifc|flac|ogg|oga|opus|m4a|aac|wma|caf)$' -print)"
if [[ -n "${AUDIO_STAGED}" ]]; then
  echo "[deploy] ERROR: audio files must never be deployed:" >&2
  printf '%s\n' "${AUDIO_STAGED}" | sed 's/^/  - /' >&2
  exit 1
fi

# Content-hash the game's wasm and stamp the name into index.html (its only
# reference). The name doubles as the build id the landing's returning-visitor
# skip compares against (localStorage "beatmo.booted").
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

# ---------------------------------------------------------------------------
# Gzip-compress every staged object in place (same filename). The objects are
# then uploaded as gzip and stamped with Content-Encoding: gzip + no-transform
# below, so the browser downloads the compressed bytes and decompresses
# transparently. This is the single biggest load-time win -- main.wasm goes
# from ~28 MiB to ~6.5 MiB on the wire (see internal/deploy/asset_size_test.go).
# The size/streaming guards live in src/js/wasm_load_startup.browser.test.js.
# ---------------------------------------------------------------------------
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

if [[ "${STAGE_ONLY}" == "1" ]]; then
  echo "[deploy] Stage only: ${BUILD_DIR} holds what a deploy would upload; nothing uploaded."
  exit 0
fi

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

# ---------------------------------------------------------------------------
# Stamp Content-Type and Cache-Control on every object after rsync. gcloud's
# inferred MIME is unreliable for .wasm; explicit beats inferred for the few
# files we ship.
# ---------------------------------------------------------------------------
# Every object is uploaded gzip-compressed (see the gzip pass below), so each
# carries Content-Encoding: gzip. no-transform is mandatory: it stops GCS from
# decompressively transcoding the gzipped object, which would strip
# Content-Length and break WebAssembly.instantiateStreaming on main.wasm.
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

# ---------------------------------------------------------------------------
# Optional CDN invalidation. Off by default; use --purge-cdn for the first
# deploy after a Content-Type/Cache-Control fix and any time index.html
# changes shape in a way that needs an immediate cutover.
# ---------------------------------------------------------------------------
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
