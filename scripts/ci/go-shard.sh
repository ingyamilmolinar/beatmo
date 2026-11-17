#!/usr/bin/env bash
# Splits `go test ./...` into shard i of n, run from the module directory.
#   go-shard.sh plan i/n [build flags...]
#   go-shard.sh run  i/n [build flags...] -- [test flags...]
# Packages with at least SPLIT_MIN tests are split by test name; the rest are
# assigned whole. Both by a hash, so every (package, test) runs in exactly one
# shard. With COVERDIR set, each test binary writes covdata there; packages
# matching the GO_SHARD_EXCLUDE regex run in no shard.
set -euo pipefail
mode="$1"; shard="$2"; shift 2
i="${shard%/*}"; n="${shard#*/}"
build=(); test=()
while [[ $# -gt 0 && "$1" != "--" ]]; do build+=("$1"); shift; done
[[ $# -gt 0 ]] && shift
test=("$@")
go="${GO:-go}"

listing="$("$go" test "${build[@]}" -list '.' ./...)"
plan="$(awk -v i="$i" -v n="$n" -v min="${SPLIT_MIN:-300}" -v ex="${GO_SHARD_EXCLUDE:-}" '
  BEGIN { chars = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789_./-" }
  function h(s,  x, k) { x = 5381; for (k = 1; k <= length(s); k++) x = (x * 33 + index(chars, substr(s, k, 1))) % 1000003; return x }
  # 60 KB per -run value keeps every argv string under Linux'"'"' 128 KB cap.
  function flush(pkg) { if (re != "") print "RUN " pkg " ^(" re ")$"; re = "" }
  /^(Test|Example|Fuzz)[A-Za-z0-9_]*$/ { names[++cnt] = $0; next }
  /^ok / {
    pkg = $2
    if (ex != "" && pkg ~ ex) { cnt = 0; next }
    if (cnt >= min) {
      re = ""
      for (k = 1; k <= cnt; k++) {
        if (h(names[k]) % n != i - 1) continue
        if (length(re) > 60000) flush(pkg)
        re = (re == "" ? names[k] : re "|" names[k])
      }
      flush(pkg)
    } else if (cnt > 0 && h(pkg) % n == i - 1) {
      whole = whole " " pkg
    }
    cnt = 0
  }
  END { if (whole != "") print "WHOLE" whole }
' <<<"$listing")"

if [[ "$mode" == "plan" ]]; then
  printf '%s\n' "$plan"
  exit 0
fi

cover=()
[[ -n "${COVERDIR:-}" ]] && mkdir -p "$COVERDIR" && cover=(-args "-test.gocoverdir=$COVERDIR")
rc=0
while read -r kind rest; do
  case "$kind" in
    WHOLE) # shellcheck disable=SC2086
      "$go" test "${build[@]}" "${test[@]}" $rest "${cover[@]}" || rc=1 ;;
    RUN) pkg="${rest%% *}"; re="${rest#* }"
      "$go" test "${build[@]}" "${test[@]}" -run "$re" "$pkg" "${cover[@]}" || rc=1 ;;
  esac
done <<<"$plan"
exit $rc
