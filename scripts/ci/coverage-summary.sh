#!/usr/bin/env bash
# Go and C coverage tables for the CI run summary (stdout when run locally).
set -uo pipefail
root="$(cd "$(dirname "$0")/../.." && pwd)"
out="${GITHUB_STEP_SUMMARY:-/dev/stdout}"
go="${GO:-go}"
if [[ -f "$root/coverage/c-summary.txt" ]]; then
  { echo "## C Coverage (own DSP sources, miniaudio excluded)"; echo '```'; cat "$root/coverage/c-summary.txt"; echo '```'; } >> "$out"
fi
if [[ -f "$root/coverage/go-percent.txt" ]]; then
  {
    echo "## Go Test Coverage"; echo
    echo "| Package | Coverage |"; echo "|---------|----------|"
    sed 's|github.com/ingyamilmolinar/beatmo/||' "$root/coverage/go-percent.txt" |
      awk '/coverage:/ { printf "| `%s` | %s |\n", $1, $3 }'
    echo
    [[ -f "$root/coverage/go.out" ]] &&
      echo "**Total: $(cd "$root/src/go" && "$go" tool cover -func=../../coverage/go.out | tail -1 | awk '{print $NF}')**"
  } >> "$out"
elif [[ -f "$root/coverage/test-output.txt" ]]; then
  {
    echo "## Go Test Coverage"; echo
    echo "| Package | Coverage |"; echo "|---------|----------|"
    grep -E '^ok\s' "$root/coverage/test-output.txt" | sed 's|github.com/ingyamilmolinar/beatmo/||' |
      awk '{ pkg=$2; for(i=1;i<=NF;i++) if($i=="coverage:") { printf "| `%s` | %s |\n", pkg, $(i+1); break } }'
    echo
    if [[ -f "$root/coverage/go.out" ]]; then
      echo "**Total: $(cd "$root/src/go" && "$go" tool cover -func=../../coverage/go.out | tail -1 | awk '{print $NF}')**"
    fi
  } >> "$out"
fi
exit 0
