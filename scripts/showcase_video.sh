#!/usr/bin/env bash
# Records each cut through the real game's scripted tour (-tour <cut> -record) → src/js/landing/<cut>.{mp4,webp,vtt}; manual.
# Published only when clean (every step reached, ≤ SHOWCASE_MAX_REPEAT_PCT repeated frames, no freeze > SHOWCASE_MAX_FREEZE):
# xvfb's software GL and a hidden window both repeat frames. Env: CUTS SHOWCASE_CRF SHOWCASE_WORK SHOWCASE_XVFB=1
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
