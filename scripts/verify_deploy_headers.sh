#!/usr/bin/env bash
# Verify a deployed beatmo bundle by HEAD-fetching each runtime asset and
# asserting the HTTP status + Content-Type + Cache-Control come back correctly.
#
# Usage:
#   BEATMO_VERIFY_DEPLOY_URL=https://beatmo.io ./scripts/verify_deploy_headers.sh
#
#   # or pass the URL positionally:
#   ./scripts/verify_deploy_headers.sh https://staging.beatmo.io
#
# Exits non-zero on the first failed assertion and prints a punch list of all
# problems discovered before exiting. Designed to run in CI after a staging
# deploy and on demand against prod.
#
# Why this exists: deploy-wasm-gcs.sh is the only place that stamps
# Content-Type and Cache-Control headers on the GCS objects. If those flags
# regress (or if a CDN cache rule starts overriding them), the user sees a
# broken site -- not the deploy script. This is the after-the-fact ground
# truth.

set -uo pipefail

URL_DEFAULT="${BEATMO_VERIFY_DEPLOY_URL:-}"
URL="${1:-${URL_DEFAULT}}"

if [[ -z "${URL}" ]]; then
  cat <<EOF >&2
Usage: BEATMO_VERIFY_DEPLOY_URL=https://beatmo.io $0
   or: $0 https://beatmo.io
EOF
  exit 2
fi

URL="${URL%/}"   # strip trailing slash

# Same buckets as scripts/deploy-wasm-gcs.sh -- if you change one, change the
# other (and the drift test will fail if you miss a file).
ENTRY_FILES=(
  "index.html"
  "synth_param_abi.gen.js"
)
ASSET_FILES=(
  "audio.js"
  "wasm_exec.js"
  "drums.single.js"
  "insert_fx_worklet.js"
  "recording_capture_worklet.js"
  "recording_encoder_worker.js"
)
WASM_FILES=(
  "main.wasm"
)

ERRORS=()

expected_content_type() {
  case "$1" in
    *.html) echo "text/html" ;;
    *.wasm) echo "application/wasm" ;;
    *.js)   echo "application/javascript" ;;
    *)      echo "" ;;
  esac
}

# fetch_headers prints the raw HEAD response (status line + headers).
fetch_headers() {
  local url="$1"
  # --max-time guards against a hung connection in CI.
  curl --silent --show-error --location --head --max-time 20 "$url"
}

check_one() {
  local rel="$1"
  local want_ct
  want_ct="$(expected_content_type "$rel")"

  local url
  if [[ "$rel" == "index.html" ]]; then
    url="${URL}/"
  else
    url="${URL}/${rel}"
  fi

  local resp
  if ! resp="$(fetch_headers "$url" 2>&1)"; then
    ERRORS+=("${rel}: curl failed (${resp})")
    return
  fi

  # Use grep with -i for case-insensitive matching (POSIX-portable; awk's
  # IGNORECASE is gawk-only).
  local status_line
  status_line="$(printf '%s\n' "$resp" | grep -i '^HTTP/' | tail -n1 | tr -d '\r')"
  local status_code
  status_code="$(printf '%s' "$status_line" | awk '{print $2}')"
  if [[ "$status_code" != "200" ]]; then
    ERRORS+=("${rel}: expected HTTP 200, got '${status_line}'")
    return
  fi

  local ct cc
  ct="$(printf '%s\n' "$resp" | grep -i '^content-type:' | tail -n1 | sed -E 's/^[^:]+:[[:space:]]*//' | tr -d '\r')"
  cc="$(printf '%s\n' "$resp" | grep -i '^cache-control:' | tail -n1 | sed -E 's/^[^:]+:[[:space:]]*//' | tr -d '\r')"

  if [[ -n "$want_ct" ]]; then
    if [[ "$ct" != *"$want_ct"* ]]; then
      ERRORS+=("${rel}: Content-Type must contain '${want_ct}', got '${ct}'")
    fi
  fi
  if [[ -z "$cc" ]]; then
    ERRORS+=("${rel}: Cache-Control header is missing (empty); deploy must set it explicitly")
  fi

  printf '  %-34s 200  ct=%-40s cc=%s\n' "${rel}" "${ct:-<missing>}" "${cc:-<missing>}"
}

echo "[verify] Target: ${URL}"
echo "[verify] Probing ${#ENTRY_FILES[@]} entry files, ${#ASSET_FILES[@]} asset files, ${#WASM_FILES[@]} wasm file(s)..."

for f in "${ENTRY_FILES[@]}" "${ASSET_FILES[@]}" "${WASM_FILES[@]}"; do
  check_one "$f"
done

if (( ${#ERRORS[@]} > 0 )); then
  echo
  echo "[verify] FAIL -- ${#ERRORS[@]} problem(s):" >&2
  for e in "${ERRORS[@]}"; do echo "  - $e" >&2; done
  exit 1
fi

echo "[verify] OK"
