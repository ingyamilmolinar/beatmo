#!/bin/bash
# gcov over our own src/c through the native audio tests → coverage/c/*.c.gcov, c-summary.txt, go-native.out.
# .gcno/.gcda paths are baked from -o at compile time; CGO_LDFLAGS=--coverage also forces a relink (the build cache does
# not hash static-lib content); .gcda is cleared up front; a trap restores build/.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
cd "$REPO_ROOT"

GO="${GO:-$REPO_ROOT/.tools/go/bin/go}"
CC="${CC:-cc}"

# miniaudio is third-party, so it is left out.
OWN_SRC=(drums fmsynth insert_fx fx_host mix_dsp wavetable adsr noise modular modular_stages modal_bank)

OUT="$REPO_ROOT/coverage/c"
SUMMARY="$REPO_ROOT/coverage/c-summary.txt"
mkdir -p "$OUT" "$REPO_ROOT/coverage"

# Always, even on failure, so the next make never links instrumented objects.
cleanup() {
    rm -f "$REPO_ROOT"/build/*.o "$REPO_ROOT"/build/*.gcno \
        "$REPO_ROOT"/build/*.gcda "$REPO_ROOT/build/libdrums.a"
}
trap cleanup EXIT

echo "========================================"
echo "C coverage (gcov) — own sources only"
echo "========================================"

# --- 1. Clean stale instrumentation + accumulated counters -------------------
rm -f build/*.gcno build/*.gcda build/*.o build/libdrums.a
rm -f "$OUT"/*.gcov "$OUT"/*.gcov.json.gz "$SUMMARY"

# --- 2. Compile instrumented own sources; miniaudio without coverage ---------
mkdir -p build
for f in "${OWN_SRC[@]}"; do
    extra=""
    [ "$f" = drums ] && extra="-DMA_ENABLE_MP3"
    # shellcheck disable=SC2086
    $CC -O0 -g --coverage $extra -c "src/c/$f.c" -o "build/$f.o"
done
$CC -O2 -DMA_ENABLE_MP3 -c src/c/miniaudio.c -o build/miniaudio.o
# A STRONG __gcov_dump reference (-DBEATMO_GCOV) so the member is pulled out of libgcov.a; a weak one would not, and
# the dump would silently no-op. Tooling, so not instrumented.
$CC -O2 -DBEATMO_GCOV -c src/c/gcov_flush.c -o build/gcov_flush.o
# The test oracle the native bridge links: tooling, so not instrumented (as C_SRC_TEST in the Makefile).
$CC -O2 -c src/c/noise_ma_oracle.c -o build/noise_ma_oracle.o

# --- 3. Archive at the path hardcoded in the #cgo LDFLAGS --------------------
ar rcs build/libdrums.a build/miniaudio.o build/gcov_flush.o build/noise_ma_oracle.o \
    $(printf 'build/%s.o ' "${OWN_SRC[@]}")

# --- 4. Run the native (no -tags test) audio tests through the bridge --------
export CGO_LDFLAGS="--coverage"
export CGO_ENABLED=1

# -linkmode=external: gcov registers counters through C constructors, which Go's internal linker entry skips. The
# native suite runs minutes under -covermode=atomic, and a timeout kills it before __gcov_dump.
COVERAGE_C_TIMEOUT="${COVERAGE_C_TIMEOUT:-1500s}"
test_rc=0
(
    cd src/go
    "$GO" test -count=1 -ldflags '-linkmode=external' \
        -coverpkg=./internal/audio \
        -coverprofile="$REPO_ROOT/coverage/go-native.out" \
        -covermode=atomic -timeout "$COVERAGE_C_TIMEOUT" ./internal/audio
) || test_rc=$?

# Optional xvfb pass picks up mixer tests that need a display; their counters
# accumulate additively into the same build/*.gcda (more coverage, desired).
if [ "${COVERAGE_C_XVFB:-0}" = 1 ] && command -v xvfb-run >/dev/null 2>&1; then
    (
        cd src/go
        xvfb-run -a "$GO" test -count=1 -ldflags '-linkmode=external' \
            -timeout "$COVERAGE_C_TIMEOUT" -run 'Mixer|Signal' ./internal/audio
    ) || echo "WARNING: xvfb mixer pass failed — coverage still collected"
fi

if [ $test_rc -ne 0 ]; then
    echo "WARNING: Some native tests failed — coverage data still collected"
fi

# --- 5. Report: annotated .gcov + per-file table + TOTAL ----------------------
echo ""
echo "=== C Coverage (own sources, miniaudio excluded) ==="
{
    printf '%-14s %10s %8s %8s\n' "FILE" "COVERED" "LINES" "PCT"
} | tee "$SUMMARY"

total_lines=0
total_exec=0
missing=0
for f in "${OWN_SRC[@]}"; do
    if [ ! -f "build/$f.gcda" ]; then
        printf '%-14s %10s %8s %8s\n' "$f.c" "-" "-" "NO DATA" | tee -a "$SUMMARY"
        missing=1
        continue
    fi
    # From the repo root: the .gcno records src/c/<f>.c relative to it.
    summary_line=$(gcov --object-directory "$REPO_ROOT/build" \
        "src/c/$f.c" 2>/dev/null |
        grep -A1 "File 'src/c/$f\.c'" | grep 'Lines executed' | head -1)
    pct=$(echo "$summary_line" | sed -n 's/.*Lines executed:\([0-9.]*\)% of \([0-9]*\).*/\1/p')
    lines=$(echo "$summary_line" | sed -n 's/.*Lines executed:\([0-9.]*\)% of \([0-9]*\).*/\2/p')
    if [ -z "$pct" ] || [ -z "$lines" ]; then
        printf '%-14s %10s %8s %8s\n' "$f.c" "-" "-" "PARSE?" | tee -a "$SUMMARY"
        missing=1
        continue
    fi
    exec_lines=$(awk -v p="$pct" -v l="$lines" 'BEGIN { printf "%d", (p/100)*l + 0.5 }')
    total_lines=$((total_lines + lines))
    total_exec=$((total_exec + exec_lines))
    printf '%-14s %10s %8s %7s%%\n' "$f.c" "$exec_lines" "$lines" "$pct" | tee -a "$SUMMARY"
done

# Our own header annotations stay (an inline helper there is ours to cover); miniaudio and system headers go.
mv -f ./*.gcov "$OUT"/ 2>/dev/null || true
rm -f "$OUT"/miniaudio.* "$OUT"/*'#'*.gcov 2>/dev/null || true

if [ "$total_lines" -gt 0 ]; then
    total_pct=$(awk -v e="$total_exec" -v l="$total_lines" 'BEGIN { printf "%.2f", (e/l)*100 }')
else
    total_pct="0.00"
fi
printf '%-14s %10s %8s %7s%%\n' "TOTAL" "$total_exec" "$total_lines" "$total_pct" | tee -a "$SUMMARY"
echo ""
echo "Annotated sources: coverage/c/*.c.gcov (uncovered lines marked #####)"
echo "Go bridge coverage: coverage/go-native.out (go tool cover -func=...)"

# --- 6. Optional HTML report when lcov/genhtml are installed ------------------
if command -v lcov >/dev/null 2>&1 && command -v genhtml >/dev/null 2>&1; then
    if lcov --capture --directory build --no-external \
        --output-file "$OUT/c.info" >/dev/null 2>&1 &&
        lcov --remove "$OUT/c.info" '*/miniaudio.*' \
            -o "$OUT/c.info" >/dev/null 2>&1 &&
        genhtml "$OUT/c.info" --output-directory "$OUT/html" >/dev/null 2>&1; then
        echo "C HTML report: coverage/c/html/index.html"
    fi
fi

# --- 7. Optional floor check ---------------------------------------------------
if [ -n "${COVERAGE_C_FLOOR:-}" ]; then
    if awk -v t="$total_pct" -v f="$COVERAGE_C_FLOOR" 'BEGIN { exit (t+0 < f+0) }'; then
        printf 'coverage-c: OK — total %s%% >= floor %s%%\n' "$total_pct" "$COVERAGE_C_FLOOR"
    else
        printf 'coverage-c: FAIL — total %s%% < floor %s%%\n' "$total_pct" "$COVERAGE_C_FLOOR"
        exit 1
    fi
fi

if [ $test_rc -ne 0 ]; then
    exit $test_rc
fi
if [ $missing -ne 0 ]; then
    echo "WARNING: some files produced no coverage data (never linked/executed?)"
fi
