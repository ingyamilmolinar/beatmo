#!/usr/bin/env bash
# fetch_synth_refs.sh — download University of Iowa MIS single-note AIFFs,
# convert to 16-bit mono WAV, and write/refresh MANIFEST.md.
#
# License: UoI MIS samples are "freely downloadable and usable for any
# projects, without restrictions" (public-domain-equivalent).
#   https://theremin.music.uiowa.edu/
#
# Usage (from repo root):
#   bash scripts/fetch_synth_refs.sh
#
# Behaviour:
#   - Verifies each URL with curl -sI before downloading (skips 404s).
#   - Converts AIFF → 16-bit mono WAV via scripts/aiff_to_wav.py.
#   - Writes sha256 checksums into MANIFEST.md.
#   - Idempotent: re-runs recreate MANIFEST header + re-download all entries.
#   - Exits 0 if at least 1 WAV lands; exits 1 only if every fetch fails.

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OUT_DIR="${REPO_ROOT}/src/go/internal/audio/testdata/refs"
MANIFEST="${OUT_DIR}/MANIFEST.md"
CONVERTER="${REPO_ROOT}/scripts/aiff_to_wav.py"
TMP_AIF="$(mktemp /tmp/beatmo_ref_XXXXXX.aif)"
trap 'rm -f "${TMP_AIF}"' EXIT

mkdir -p "${OUT_DIR}"

# ---------------------------------------------------------------------------
# Curated list: (instId  note  aiffURL)
# All URLs verified HTTP 200 before inclusion.
# ---------------------------------------------------------------------------
declare -a ENTRIES=(
  # Verified working (brief-supplied)
  "violin"   "A4"  "https://theremin.music.uiowa.edu/sound%20files/MIS%20Pitches%20-%202014/Strings/Violin/Violin.arco.ff.sulA.A4.stereo.aif"
  "flute"    "A4"  "https://theremin.music.uiowa.edu/sound%20files/MIS%20Pitches%20-%202014/Woodwinds/Flute/Flute.nonvib.ff.A4.stereo.aif"
  # Discovered via category page probe
  "cello"    "A3"  "https://theremin.music.uiowa.edu/sound%20files/MIS%20Pitches%20-%202014/Strings/Cello/Cello.arco.ff.sulA.A3.stereo.aif"
  "oboe"     "A4"  "https://theremin.music.uiowa.edu/sound%20files/MIS%20Pitches%20-%202014/Woodwinds/Oboe/Oboe.ff.A4.stereo.aif"
  "trumpet"  "A4"  "https://theremin.music.uiowa.edu/sound%20files/MIS%20Pitches%20-%202014/Brass/BbTrumpet/Trumpet.novib.ff.A4.stereo.aif"
)

# ---------------------------------------------------------------------------
# Re-write MANIFEST header (idempotent).
# ---------------------------------------------------------------------------
cat > "${MANIFEST}" <<'HEADER'
# Reference WAVs — University of Iowa Musical Instrument Samples

**Source**: University of Iowa Musical Instrument Samples (MIS)
**URL**: https://theremin.music.uiowa.edu/
**License**: "These samples may be downloaded and used for any projects, without restrictions." (public-domain-equivalent)

## Format

Original files are AIFF (24-bit stereo, 44100 Hz, ~3 s single-note recordings).
Each file was converted to **16-bit mono WAV, capped at 2 seconds** using
`scripts/aiff_to_wav.py` (Python stdlib only — no ffmpeg/sox).

Regenerate via:
```
bash scripts/fetch_synth_refs.sh
```

## File table

| file | instrument | note | source URL | sha256 |
|------|------------|------|------------|--------|
HEADER

# ---------------------------------------------------------------------------
# Download, convert, checksum.
# ---------------------------------------------------------------------------
landed=0
n=${#ENTRIES[@]}
i=0
while [ $i -lt $n ]; do
  INST="${ENTRIES[$i]}"
  NOTE="${ENTRIES[$((i+1))]}"
  URL="${ENTRIES[$((i+2))]}"
  i=$((i+3))

  WAV_NAME="${INST}_${NOTE}.wav"
  WAV_PATH="${OUT_DIR}/${WAV_NAME}"

  echo "==> ${INST} ${NOTE}: probing ${URL} ..."
  HTTP_STATUS=$(curl -sI --max-time 15 "${URL}" | head -1 | awk '{print $2}')
  if [ "${HTTP_STATUS}" != "200" ]; then
    echo "    WARNING: HTTP ${HTTP_STATUS} — skipping ${INST} ${NOTE}"
    continue
  fi

  echo "    downloading ..."
  if ! curl -sL --max-time 120 --output "${TMP_AIF}" "${URL}"; then
    echo "    WARNING: download failed for ${INST} ${NOTE} — skipping"
    continue
  fi

  echo "    converting AIFF → WAV ..."
  if ! python3 "${CONVERTER}" "${TMP_AIF}" "${WAV_PATH}"; then
    echo "    WARNING: conversion failed for ${INST} ${NOTE} — skipping"
    rm -f "${WAV_PATH}"
    continue
  fi

  SHA=$(sha256sum "${WAV_PATH}" | awk '{print $1}')
  echo "    OK: ${WAV_NAME} (sha256=${SHA})"

  # Append row to MANIFEST table.
  printf "| %s | %s | %s | %s | %s |\n" \
    "${WAV_NAME}" "${INST}" "${NOTE}" "${URL}" "${SHA}" >> "${MANIFEST}"

  landed=$((landed+1))
done

echo ""
echo "fetch_synth_refs.sh: ${landed} WAV(s) landed in ${OUT_DIR}"
echo "MANIFEST updated: ${MANIFEST}"

if [ "${landed}" -eq 0 ]; then
  echo "ERROR: no WAVs landed — check network / URL list" >&2
  exit 1
fi
exit 0
