#!/usr/bin/env bash
# compare-bench.sh — diff two record-bench runs (recording-on vs baseline)
# and exit non-zero if any acceptance threshold is breached.
#
# Usage:
#   scripts/compare-bench.sh <recording-perf_stats.json> <baseline-perf_stats.json>
#
# Thresholds (clean run):
#   audioCallMax delta < 5%
#   updateMaxMS  delta < 10%
#   recordingDrops == 0
#   goroutines delta <= 10
#
# Thresholds (noisy run, set NOISY=1):
#   audioCallMax delta < 30%
#   updateMaxMS  delta < 50%
#   recordingDrops < 1% of audio_enq

set -eo pipefail

REC=${1:?recording perf_stats.json path required}
BASE=${2:?baseline perf_stats.json path required}

if ! command -v jq >/dev/null 2>&1; then
  echo "jq is required (apt-get install jq)" >&2
  exit 2
fi

for f in "$REC" "$BASE"; do
  [[ -f "$f" ]] || { echo "missing file: $f" >&2; exit 2; }
done

# Extract metrics. Use 0 fallback for unset fields.
get() { jq -r ".$1 // 0" "$2"; }

rec_audio_max=$(get AudioCallMax "$REC")
base_audio_max=$(get AudioCallMax "$BASE")
rec_update_max=$(get UpdateMaxMS "$REC")
base_update_max=$(get UpdateMaxMS "$BASE")
rec_drops=$(get RecordingDrops "$REC")
rec_goroutines=$(get Goroutines "$REC")
base_goroutines=$(get Goroutines "$BASE")
rec_audio_enq=$(get AudioEnq "$REC")

# Compute relative deltas using bc (handles floats).
delta_pct() {
  # $1 = current, $2 = baseline. Returns abs((cur-base)/base * 100), or 0
  # if base is 0/missing.
  local cur=$1 base=$2
  if [[ -z "$base" || "$base" == "0" || "$base" == "0.0" ]]; then
    echo "0"
    return
  fi
  echo "scale=2; d=($cur-$base)/$base*100; if (d<0) -d else d" | bc -l
}

audio_delta=$(delta_pct "$rec_audio_max" "$base_audio_max")
update_delta=$(delta_pct "$rec_update_max" "$base_update_max")

# Goroutine delta is absolute, not pct.
gor_delta=$(echo "$rec_goroutines - $base_goroutines" | bc -l)
if (( $(echo "$gor_delta < 0" | bc -l) )); then
  gor_delta=$(echo "0 - $gor_delta" | bc -l)
fi

# Drop-rate as percent of total audio enqueues.
drop_rate="0"
if [[ "$rec_audio_enq" != "0" ]]; then
  drop_rate=$(echo "scale=4; $rec_drops / $rec_audio_enq * 100" | bc -l)
fi

# Set thresholds based on NOISY flag.
if [[ "${NOISY:-0}" == "1" ]]; then
  AUDIO_LIMIT=30
  UPDATE_LIMIT=50
  DROP_LIMIT=1.0
else
  AUDIO_LIMIT=5
  UPDATE_LIMIT=10
  DROP_LIMIT=0.0001
fi

cat <<EOF
=== Bench comparison ===
Recording perf_stats: $REC
Baseline  perf_stats: $BASE

Metric             Baseline    Recording   Delta       Limit       Status
-----------------  ---------   ---------   ---------   ---------   ------
AudioCallMax (ms)  ${base_audio_max}     ${rec_audio_max}     ${audio_delta}%     ${AUDIO_LIMIT}%
UpdateMaxMS  (ms)  ${base_update_max}     ${rec_update_max}     ${update_delta}%     ${UPDATE_LIMIT}%
Goroutines         ${base_goroutines}          ${rec_goroutines}          Δ${gor_delta}        ≤10
RecordingDrops     n/a         ${rec_drops}          ${drop_rate}%       ${DROP_LIMIT}%
EOF

fail=0
fail_metric() {
  echo "FAIL: $1" >&2
  fail=1
}

if (( $(echo "$audio_delta > $AUDIO_LIMIT" | bc -l) )); then
  fail_metric "AudioCallMax delta ${audio_delta}% exceeds limit ${AUDIO_LIMIT}%"
fi
if (( $(echo "$update_delta > $UPDATE_LIMIT" | bc -l) )); then
  fail_metric "UpdateMaxMS delta ${update_delta}% exceeds limit ${UPDATE_LIMIT}%"
fi
if (( $(echo "$gor_delta > 10" | bc -l) )); then
  fail_metric "Goroutine delta ${gor_delta} exceeds 10"
fi
if (( $(echo "$drop_rate > $DROP_LIMIT" | bc -l) )); then
  fail_metric "RecordingDrops rate ${drop_rate}% exceeds ${DROP_LIMIT}%"
fi

if (( fail == 0 )); then
  echo
  echo "All metrics within thresholds — PASS"
  exit 0
fi
exit 1
