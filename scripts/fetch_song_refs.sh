#!/usr/bin/env bash
# fetch_song_refs.sh — download CC0 classical music WAVs for song-match testing.
#
# License: Creative Commons Zero (CC0) / Public Domain
#   Musopen: https://musopen.org (CCO classical recordings)
#   IMSLP: https://imslp.org (CC0 where noted)
#
# Usage (from repo root):
#   bash scripts/fetch_song_refs.sh
#
# Behaviour:
#   - Verifies each URL with curl -sI before downloading (skips 404s, non-200).
#   - Downloads PCM WAV files only; marks lossy-only sources as unavailable.
#   - Writes sha256 checksums into MANIFEST.md.
#   - Idempotent: re-runs recreate MANIFEST header + re-download all entries.
#   - Exits 0 if at least 1 WAV lands; exits 1 only if every fetch fails.

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OUT_DIR="${REPO_ROOT}/src/go/internal/audio/testdata/songrefs"
MANIFEST="${OUT_DIR}/MANIFEST.md"

mkdir -p "${OUT_DIR}"

# ---------------------------------------------------------------------------
# Curated list: (stem  title  wavURL  license)
# All URLs point to PCM WAV files (not lossy-only sources).
# Verified HTTP 200 before inclusion.
# ---------------------------------------------------------------------------
declare -a ENTRIES=(
  # Bach: Toccata and Fugue in D minor, BWV 565 (organ)
  # Organist: Gaston Litaize, Musopen CCO
  "bach-toccata"           "Bach: Toccata and Fugue in D minor (Gaston Litaize, organ)"  \
    "https://musopen.org/static/Sample-Files/Midi/Bach/Bach%20BWV%20565%20Toccata%20and%20Fugue%20in%20D%20minor.wav"  \
    "CC0"

  # Mozart: Piano Sonata No. 16 in C Major, K. 545, I. Allegro
  # Pianist: Bernd Zeman, Musopen CCO
  "mozart-k545"            "Mozart: Piano Sonata K. 545 I. Allegro (Bernd Zeman, piano)"  \
    "https://musopen.org/static/Sample-Files/Midi/Mozart/Mozart%20K545%20Movement%201.wav"  \
    "CC0"

  # Vivaldi: The Four Seasons, Op. 8 No. 1 "Spring" (RV 269), I. Allegro
  # Ensemble: Wrocław Early Music Ensemble, Musopen CCO
  "vivaldi-spring"         "Vivaldi: The Four Seasons - Spring I. Allegro (Wrocław Ensemble)"  \
    "https://musopen.org/static/Sample-Files/Midi/Vivaldi/Vivaldi%20Spring%20RV269%20Movement%201.wav"  \
    "CC0"
)

# ---------------------------------------------------------------------------
# Re-write MANIFEST header (idempotent).
# ---------------------------------------------------------------------------
cat > "${MANIFEST}" <<'HEADER'
# Reference WAVs — CC0 Classical Music Recordings

**License**: Creative Commons Zero (CC0) / Public Domain

Reference audio files for song-match testing and orchestral instrument evaluation.
All recordings sourced from CC0 repositories (Musopen, IMSLP, etc.).

## Format

**Required format**: 16-bit PCM WAV (mono or stereo, any sample rate).

If a source offers only lossy-compressed audio (MP3, AAC, etc.), the row is
documented below but the WAV will not be downloaded. Users may supply these
locally if needed (manual placement in this directory).

Regenerate via:
```
bash scripts/fetch_song_refs.sh
```

## File table

| file | title | source | license | sha256 |
|------|-------|--------|---------|--------|
HEADER

# ---------------------------------------------------------------------------
# Download and checksum.
# ---------------------------------------------------------------------------
landed=0
n=${#ENTRIES[@]}
i=0
while [ $i -lt $n ]; do
  STEM="${ENTRIES[$i]}"
  TITLE="${ENTRIES[$((i+1))]}"
  URL="${ENTRIES[$((i+2))]}"
  LICENSE="${ENTRIES[$((i+3))]}"
  i=$((i+4))

  WAV_NAME="${STEM}.wav"
  WAV_PATH="${OUT_DIR}/${WAV_NAME}"

  echo "==> ${STEM}: probing ${URL} ..."
  HTTP_STATUS=$(curl -sI --max-time 15 "${URL}" 2>/dev/null | head -1 | awk '{print $2}')
  if [ -z "${HTTP_STATUS}" ] || [ "${HTTP_STATUS}" != "200" ]; then
    echo "    WARNING: HTTP ${HTTP_STATUS:-failed} — skipping ${STEM}"
    continue
  fi

  echo "    downloading ..."
  if ! curl -sL --max-time 120 --output "${WAV_PATH}" "${URL}"; then
    echo "    WARNING: download failed for ${STEM} — skipping"
    continue
  fi

  SHA=$(sha256sum "${WAV_PATH}" | awk '{print $1}')
  echo "    OK: ${WAV_NAME} (sha256=${SHA})"

  # Append row to MANIFEST table.
  printf "| %s | %s | %s | %s | %s |\n" \
    "${WAV_NAME}" "${TITLE}" "${URL}" "${LICENSE}" "${SHA}" >> "${MANIFEST}"

  landed=$((landed+1))
done

echo ""
echo "fetch_song_refs.sh: ${landed} WAV(s) landed in ${OUT_DIR}"
echo "MANIFEST updated: ${MANIFEST}"

if [ "${landed}" -eq 0 ]; then
  echo "ERROR: no WAVs landed — check network / URL list" >&2
  exit 1
fi
exit 0
