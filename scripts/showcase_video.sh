#!/usr/bin/env bash
# showcase_video.sh — record the landing page's showcase videos (spec
# docs/superpowers/specs/2026-10-23-landing-page-design.md, "The recording").
#
# For each cut (desktop 16:9, phone portrait) it runs the real game's scripted
# tour (-tour <cut> -record <dir>: real input, master bus through the Rec path,
# frames piped at a constant 25 fps of the recording clock), then encodes:
#   src/js/landing/<cut>.mp4   H.264 + AAC, faststart
#   src/js/landing/<cut>.webp  poster (the first frame)
#   src/js/landing/<cut>.vtt   cue file (captions, chapters, themes, beat clock)
#
# Manual by design: no UI change requires a re-record and CI never checks it.
#
# Display: the real display when $DISPLAY is set (GPU: ~2% repeated frames,
# measured 2026-10-23), else xvfb (software GL: 24-52% repeated frames on the
# heavy Synth/theme moments). SHOWCASE_XVFB=1 forces xvfb.
#
# A recording is published into src/js/landing only when it is clean: every
# step reached its state, at most SHOWCASE_MAX_REPEAT_PCT (5) % of the frames
# are repeats and no freeze is longer than SHOWCASE_MAX_FREEZE frames (12 =
# 0.48 s). Otherwise the committed media stay as they were. A window the
# compositor hides (screen off / locked, covered) is throttled to ~1 frame/s:
# measured 2026-10-24, 81% repeated over a 134 s phone run.
#
# Env: CUTS="desktop phone"  SHOWCASE_CRF=23  SHOWCASE_WORK=<dir>  SHOWCASE_XVFB=1
#      SHOWCASE_MAX_REPEAT_PCT=5  SHOWCASE_MAX_FREEZE=12
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
GO="${GO:-$ROOT/.tools/go/bin/go}"
OUT="$ROOT/src/js/landing"
WORK="${SHOWCASE_WORK:-$(mktemp -d -t beatmo-showcase-XXXXXX)}"
CUTS="${CUTS:-desktop phone}"
CRF="${SHOWCASE_CRF:-23}"
MAX_REPEAT_PCT="${SHOWCASE_MAX_REPEAT_PCT:-5}"
MAX_FREEZE="${SHOWCASE_MAX_FREEZE:-12}"

command -v ffmpeg >/dev/null || { echo "showcase-video: ffmpeg is required" >&2; exit 1; }
mkdir -p "$OUT" "$WORK/xdg"

echo "[showcase] building the desktop binary…"
(cd "$ROOT/src/go" && CGO_ENABLED=1 "$GO" build -o "$WORK/beatmo" ./cmd)

runner=()
if [[ -z "${DISPLAY:-}" || "${SHOWCASE_XVFB:-0}" == "1" ]]; then
  runner=(xvfb-run -a -s "-screen 0 1400x900x24")
  echo "[showcase] no display (or SHOWCASE_XVFB=1): recording under xvfb — expect repeated frames on heavy moments"
fi

for cut in $CUTS; do
  case "$cut" in
    desktop) size=(-win-w 1280 -win-h 720) ;;
    phone) size=(-mobile -win-w 390 -win-h 844) ;;
    *) echo "showcase-video: unknown cut $cut" >&2; exit 1 ;;
  esac
  rec="$WORK/$cut"
  rm -rf "$rec"
  echo "[showcase] recording the $cut tour…"
  # XDG_CONFIG_HOME isolates prefs: the tour switches themes and must never
  # touch the developer's real ~/.config/beatmo/prefs.json.
  XDG_CONFIG_HOME="$WORK/xdg" "${runner[@]}" "$WORK/beatmo" -log ERROR -tour "$cut" "${size[@]}" -record "$rec"
  [[ -f "$rec/report.json" && -f "$rec/frames.mkv" ]] || { echo "showcase-video: $cut tour produced no recording" >&2; exit 1; }
  if ! grep -q '"stepErrors": null' "$rec/report.json"; then
    echo "showcase-video: $cut tour steps did not reach their state:" >&2
    cat "$rec/report.json" >&2
    exit 1
  fi
  pct="$(sed -n 's/.*"repeatedPct": \([0-9.]*\).*/\1/p' "$rec/report.json")"
  freeze="$(sed -n 's/.*"longestFreezeFrames": \([0-9]*\).*/\1/p' "$rec/report.json")"
  if awk "BEGIN{exit !($pct > $MAX_REPEAT_PCT || $freeze > $MAX_FREEZE)}"; then
    echo "showcase-video: $cut repeated ${pct}% of its frames (max ${MAX_REPEAT_PCT}%), longest freeze ${freeze} frames (max ${MAX_FREEZE}):" >&2
    echo "  the game stalled — is the window visible and the screen on? src/js/landing/$cut.* left unchanged (work dir $rec)" >&2
    exit 1
  fi
  wav="$(ls "$rec"/audio/*/master.wav)"
  offset="$(cat "$rec/audio_offset.txt")"
  echo "[showcase] encoding $cut (CRF $CRF, audio offset ${offset}s)…"
  mkdir -p "$rec/out"
  ffmpeg -v error -y -i "$rec/frames.mkv" -itsoffset "$offset" -i "$wav" -map 0:v -map 1:a \
    -c:v libx264 -preset veryslow -tune animation -crf "$CRF" -pix_fmt yuv420p -profile:v high \
    -c:a aac -b:a 80k -ac 2 -shortest -movflags +faststart "$rec/out/$cut.mp4"
  ffmpeg -v error -y -i "$rec/out/$cut.mp4" -frames:v 1 -c:v libwebp -quality 70 "$rec/out/$cut.webp"
  cp "$rec/$cut.vtt" "$rec/out/$cut.vtt"
  cp "$rec/out/$cut.mp4" "$rec/out/$cut.webp" "$rec/out/$cut.vtt" "$OUT/"
  printf '[showcase] %-7s mp4 %8d B  poster %6d B  repeated frames %s%%  longest freeze %s frames\n' \
    "$cut" "$(stat -c%s "$OUT/$cut.mp4")" "$(stat -c%s "$OUT/$cut.webp")" "$pct" "$freeze"
done
echo "[showcase] done → $OUT (work dir: $WORK)"
