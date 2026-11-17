#!/usr/bin/env bash
# Re-fetch every reference score from its upstream URL and check it against the
# SHA256 recorded in internal/scoreref/catalogue.json.
#
# The scores are COMMITTED, so this is not needed to build or test — the Go test
# TestGate3_CommittedBytesMatchCatalogue already proves the committed bytes match
# the catalogue offline. What this script adds is the other half of the claim:
# that those bytes are still what upstream serves, i.e. that our copy was not
# silently edited and that the upstream file has not been revised under us.
#
# Usage: scripts/verify_scores.sh [score-id ...]
#        (no arguments = every score in the catalogue)
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cat_json="$repo_root/src/go/internal/scoreref/catalogue.json"
scores_dir="$repo_root/src/go/internal/scoreref/scores"

[[ -f "$cat_json" ]] || { echo "no catalogue at $cat_json" >&2; exit 1; }

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

wanted=("$@")

# One row per committed file: the score itself, plus a MIDI edition's
# LilyPond companion. Beatmo's own groove scores (provider "beatmo") ARE the
# upstream — there is nothing to re-fetch — so they are reported, not fetched.
mapfile -t rows < <(python3 - "$cat_json" <<'PY'
import json, sys
for s in json.load(open(sys.argv[1]))["scores"]:
    prov = s["source"].get("provider", "")
    print("\t".join([s["id"], s["file"], s["sha256"], s["source"]["url"], prov]))
    c = s.get("companion")
    if c:
        print("\t".join([s["id"] + " (source)", c["file"], c["sha256"], c["url"], prov]))
PY
)

ok=0 bad=0 skipped=0
for row in "${rows[@]}"; do
  IFS=$'\t' read -r id file want url provider <<<"$row"

  if (( ${#wanted[@]} )); then
    match=0
    for w in "${wanted[@]}"; do [[ "$w" == "${id% (source)}" ]] && match=1; done
    (( match )) || continue
  fi

  local_sum="$(sha256sum "$scores_dir/$file" | cut -d' ' -f1)"
  if [[ "$local_sum" != "$want" ]]; then
    printf '%-28s LOCAL MISMATCH  committed file does not match catalogue\n' "$id"
    bad=$((bad+1)); continue
  fi

  if [[ "$provider" == "beatmo" ]]; then
    printf '%-28s ok (authored in-repo; the committed file is the original)\n' "$id"
    ok=$((ok+1)); continue
  fi

  if ! curl -fsSL --max-time 120 -o "$tmp/$file" "$url" 2>/dev/null; then
    printf '%-28s UNREACHABLE     %s\n' "$id" "$url"
    skipped=$((skipped+1)); continue
  fi

  up_sum="$(sha256sum "$tmp/$file" | cut -d' ' -f1)"
  if [[ "$up_sum" == "$want" ]]; then
    printf '%-28s ok\n' "$id"
    ok=$((ok+1))
  else
    printf '%-28s UPSTREAM DRIFT  upstream=%s catalogue=%s\n' "$id" "${up_sum:0:16}" "${want:0:16}"
    printf '%-28s                 upstream revised the encoding; review the change, then\n' ""
    printf '%-28s                 re-commit the file and update sha256 in catalogue.json\n' ""
    bad=$((bad+1))
  fi
done

echo
echo "verified: $ok ok, $bad mismatched, $skipped unreachable"
(( bad == 0 ))
