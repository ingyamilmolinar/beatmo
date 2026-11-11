#!/usr/bin/env bash
#
# bench-desktop.sh — Self-contained desktop benchmark
#
# Runs the startup demo circuit at multiple BPM levels, collects per-BPM
# metrics (including per-event audio scheduling lead/lag), captures a pprof
# CPU profile at the highest BPM, and produces an ASCII summary table +
# machine-readable JSON output.
#
# Usage:
#   ./scripts/bench-desktop.sh
#   BPM_LEVELS="120 200" DURATION=5 ./scripts/bench-desktop.sh
#
set -euo pipefail
cd "$(dirname "$0")/.."

GO="${GO:-$(pwd)/.tools/go/bin/go}"
BPM_LEVELS="${BPM_LEVELS:-120 200 240 300}"
DURATION="${DURATION:-15}"
RESULTS_DIR="${RESULTS_DIR:-bench-results}"
PROF_BPM=""  # will be set to the highest BPM for pprof

# Use absolute path for results dir since Go process runs from src/go
RESULTS_DIR="$(cd "$(dirname "$0")/.." && pwd)/$RESULTS_DIR"
mkdir -p "$RESULTS_DIR"

# Convert BPM_LEVELS string to array
read -ra BPMS <<< "$BPM_LEVELS"

# Determine highest BPM for pprof capture
for b in "${BPMS[@]}"; do
  if [ -z "$PROF_BPM" ] || [ "$b" -gt "$PROF_BPM" ]; then
    PROF_BPM="$b"
  fi
done

echo "╔══════════════════════════════════════════════════════════════════════╗"
echo "║  DESKTOP BENCHMARK: Startup Demo (58 nodes, 7 instruments, EQ+FX)  ║"
echo "║  BPMs: ${BPM_LEVELS}  Duration: ${DURATION}s each"
echo "╚══════════════════════════════════════════════════════════════════════╝"
echo ""

JSON_RESULTS="["
FIRST=true

for BPM in "${BPMS[@]}"; do
  echo "=== BPM=$BPM (${DURATION}s) ==="

  PROF_FLAG=""
  if [ "$BPM" = "$PROF_BPM" ]; then
    PROF_FLAG="-bench-prof $RESULTS_DIR/cpu_${BPM}.prof"
    echo "  (capturing CPU profile → $RESULTS_DIR/cpu_${BPM}.prof)"
  fi

  LOG_FILE="$RESULTS_DIR/desktop_${BPM}.log"

  # Run benchmark under xvfb (Ebiten requires a display server).
  # Disable parity panics — high BPM benchmarks can trigger race-window
  # mismatches that are harmless for performance measurement.
  if command -v xvfb-run >/dev/null 2>&1; then
    xvfb-run -a bash -c "cd src/go && PARITY_FATAL=0 PERF_LOG=1 CGO_ENABLED=1 $GO run ./cmd \
      -log INFO \
      -bench-bpm $BPM \
      -bench-secs $DURATION \
      $PROF_FLAG" 2>&1 | tee "$LOG_FILE"
  else
    echo "  WARNING: xvfb-run not found, trying without virtual display"
    bash -c "cd src/go && PARITY_FATAL=0 PERF_LOG=1 CGO_ENABLED=1 $GO run ./cmd \
      -log INFO \
      -bench-bpm $BPM \
      -bench-secs $DURATION \
      $PROF_FLAG" 2>&1 | tee "$LOG_FILE"
  fi

  # Extract the bench_json line (emitted to stdout by cmd, lowercase tag).
  JSON_LINE=$(grep -i '\[bench_json\]' "$LOG_FILE" | sed 's/.*\[bench_json\] //I' | head -1)

  if [ -z "$JSON_LINE" ]; then
    echo "  ERROR: No [bench_json] output found for BPM=$BPM"
    continue
  fi

  if [ "$FIRST" = true ]; then
    FIRST=false
  else
    JSON_RESULTS+=","
  fi
  JSON_RESULTS+="$JSON_LINE"

  echo ""
done

JSON_RESULTS+="]"
echo "$JSON_RESULTS" > "$RESULTS_DIR/desktop.json"

# ── Summary Table ──
echo ""
echo "╔═════════════════════════════════════════════════════════════════════════════════════════════════════════════════╗"
echo "║  DESKTOP BENCHMARK SUMMARY                                                                                   ║"
echo "╠══════╦═══════╦═════════╦═════════╦═════════╦═════════╦═══════╦═════════╦═════════╦═════════╦═════════╦════════╣"
echo "║ BPM  ║  FPS  ║ Upd Avg ║ Upd Max ║ Drw Avg ║ Drw Max ║ ScCnt ║ Overdue ║ lagP90  ║ lagP99  ║ MinLead ║ Heap   ║"
echo "╠══════╬═══════╬═════════╬═════════╬═════════╬═════════╬═══════╬═════════╬═════════╬═════════╬═════════╬════════╣"

for BPM in "${BPMS[@]}"; do
  LOG_FILE="$RESULTS_DIR/desktop_${BPM}.log"
  JSON_LINE=$(grep -i '\[bench_json\]' "$LOG_FILE" 2>/dev/null | sed 's/.*\[bench_json\] //I' | head -1)
  if [ -z "$JSON_LINE" ]; then
    echo "║ $(printf '%4s' "$BPM") ║  N/A  ║   N/A   ║   N/A   ║   N/A   ║   N/A   ║  N/A  ║   N/A   ║   N/A   ║   N/A   ║   N/A   ║  N/A   ║"
    continue
  fi

  # Parse JSON with simple grep/sed (no jq dependency). Convert null → 0.
  extract() { local v; v=$(echo "$JSON_LINE" | grep -o "\"$1\":[^,}]*" | sed "s/\"$1\"://"); echo "${v:-0}" | sed 's/null/0/'; }

  FPS=$(extract fps)
  UPD_AVG=$(extract updateAvgMS)
  UPD_MAX=$(extract updateMaxMS)
  DRW_AVG=$(extract drawAvgMS)
  DRW_MAX=$(extract drawMaxMS)
  SC_CNT=$(extract schedCount)
  OVERDUE=$(extract schedOverdue)
  LAG_P90=$(extract schedLagP90MS)
  LAG_P99=$(extract schedLagP99MS)
  MIN_LEAD=$(extract schedMinLeadMS)
  HEAP=$(extract heapAllocKB)

  printf "║ %4s ║ %5.1f ║ %5.2fms ║ %5.2fms ║ %5.1fms ║ %5.1fms ║ %5s ║ %7s ║ %5.2fms ║ %5.2fms ║ %5.1fms ║ %4sMB ║\n" \
    "$BPM" "$FPS" "$UPD_AVG" "$UPD_MAX" "$DRW_AVG" "$DRW_MAX" "$SC_CNT" "$OVERDUE" "$LAG_P90" "$LAG_P99" "$MIN_LEAD" "$(echo "scale=0; ${HEAP:-0}/1024" | bc)"
done

echo "╚══════╩═══════╩═════════╩═════════╩═════════╩═════════╩═══════╩═════════╩═════════╩═════════╩═════════╩════════╝"
echo ""

# ── GC Summary Table ──
echo "╔══════════════════════════════════════════════════════════════════════════════════╗"
echo "║  GARBAGE COLLECTOR (whole-run window; STW pause approximations from /gc/pauses)   ║"
echo "╠══════╦════════╦════════╦════════╦══════════╦══════════╦══════════╦══════════════╣"
echo "║ BPM  ║ Cycles ║ Forced ║ Pauses ║ Pause50  ║ Pause99  ║ PauseMax ║ HeapHigh(MB) ║"
echo "╠══════╬════════╬════════╬════════╬══════════╬══════════╬══════════╬══════════════╣"
for BPM in "${BPMS[@]}"; do
  LOG_FILE="$RESULTS_DIR/desktop_${BPM}.log"
  JSON_LINE=$(grep -i '\[bench_json\]' "$LOG_FILE" 2>/dev/null | sed 's/.*\[bench_json\] //I' | head -1)
  if [ -z "$JSON_LINE" ]; then
    echo "║ $(printf '%4s' "$BPM") ║  N/A   ║  N/A   ║  N/A   ║   N/A    ║   N/A    ║   N/A    ║     N/A      ║"
    continue
  fi
  gextract() { local v; v=$(echo "$JSON_LINE" | grep -o "\"$1\":[^,}]*" | sed "s/\"$1\"://"); echo "${v:-0}" | sed 's/null/0/'; }
  GC_CYC=$(gextract gcCycles); GC_FRC=$(gextract gcForced); GC_PAU=$(gextract gcPauses)
  GC_P50=$(gextract gcPauseP50MS); GC_P99=$(gextract gcPauseP99MS); GC_MAX=$(gextract gcPauseMaxMS)
  GC_HI=$(gextract gcHeapHighKB)
  printf "║ %4s ║ %6s ║ %6s ║ %6s ║ %6.3fms ║ %6.3fms ║ %6.3fms ║ %12s ║\n" \
    "$BPM" "$GC_CYC" "$GC_FRC" "$GC_PAU" "$GC_P50" "$GC_P99" "$GC_MAX" "$(echo "scale=1; ${GC_HI:-0}/1024" | bc)"
done
echo "╚══════╩════════╩════════╩════════╩══════════╩══════════╩══════════╩══════════════╝"
echo ""

# ── pprof summary ──
if [ -f "$RESULTS_DIR/cpu_${PROF_BPM}.prof" ]; then
  echo "=== CPU Profile (BPM=$PROF_BPM) — Top 15 ==="
  "$GO" tool pprof -top -nodecount=15 "$RESULTS_DIR/cpu_${PROF_BPM}.prof" 2>/dev/null || true
  echo ""
  echo "Full profile: $RESULTS_DIR/cpu_${PROF_BPM}.prof"
  echo "  Interactive: $GO tool pprof -http=:8080 $RESULTS_DIR/cpu_${PROF_BPM}.prof"
fi

echo ""
echo "Results saved to $RESULTS_DIR/desktop.json"
echo "Per-BPM logs: $RESULTS_DIR/desktop_*.log"

# ── Optional synth param-mutation storm pass ──
# Gated by BENCH_SYNTH_STORM so the default run's output is unchanged.
# Usage: BENCH_SYNTH_STORM=combined STORM_BPM=200 DURATION=15 ./scripts/bench-desktop.sh
if [ -n "${BENCH_SYNTH_STORM:-}" ]; then
  echo ""
  echo "== synth storm pass: $BENCH_SYNTH_STORM =="
  if command -v xvfb-run >/dev/null 2>&1; then
    xvfb-run -a bash -c "cd src/go && PARITY_FATAL=0 PERF_LOG=1 CGO_ENABLED=1 $GO run ./cmd \
      -log INFO \
      -bench-bpm ${STORM_BPM:-200} \
      -bench-secs $DURATION \
      -bench-synth-storm $BENCH_SYNTH_STORM \
      -bench-prof $RESULTS_DIR/cpu_storm_${BENCH_SYNTH_STORM}.prof" \
      2>&1 | tee "$RESULTS_DIR/desktop_storm_${BENCH_SYNTH_STORM}.log"
  else
    echo "  WARNING: xvfb-run not found, trying without virtual display"
    bash -c "cd src/go && PARITY_FATAL=0 PERF_LOG=1 CGO_ENABLED=1 $GO run ./cmd \
      -log INFO \
      -bench-bpm ${STORM_BPM:-200} \
      -bench-secs $DURATION \
      -bench-synth-storm $BENCH_SYNTH_STORM \
      -bench-prof $RESULTS_DIR/cpu_storm_${BENCH_SYNTH_STORM}.prof" \
      2>&1 | tee "$RESULTS_DIR/desktop_storm_${BENCH_SYNTH_STORM}.log"
  fi
  echo ""
  echo "Storm log: $RESULTS_DIR/desktop_storm_${BENCH_SYNTH_STORM}.log"
  echo "Storm CPU profile: $RESULTS_DIR/cpu_storm_${BENCH_SYNTH_STORM}.prof"
fi
