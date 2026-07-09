#!/bin/bash
# C coverage for our own DSP sources (src/c/, miniaudio excluded).
#
# Builds a gcov-instrumented build/libdrums.a (own files at -O0 -g --coverage;
# miniaudio.c compiled WITHOUT coverage so it never produces counters), runs
# the native (no -tags test) Go audio tests through the CGo bridge, and emits:
#   coverage/c/<file>.c.gcov   annotated sources (uncovered lines marked #####)
#   coverage/c-summary.txt     per-file table + TOTAL line coverage
#   coverage/go-native.out     Go coverage of the native build of internal/audio
#                              (the CGo bridge files that -tags test excludes)
#
# Key mechanics (do not "simplify" these away):
#  - .gcno/.gcda paths are baked from the -o path at compile time, so compiling
#    `-o build/<f>.o` from the repo root means counters land in <repo>/build/
#    when the test process exits, regardless of the test cwd.
#  - CGO_LDFLAGS="--coverage" links the gcov runtime AND changes the Go build
#    ID, forcing a relink — Go's build cache does NOT hash static-lib content,
#    so without this the freshly instrumented libdrums.a would be ignored.
#  - .gcda accumulates across runs; we clear it up front so every run reports
#    exactly what this run executed.
#  - A trap restores build/ to a pristine state so the instrumented lib can
#    never poison subsequent normal builds.
#
# Usage: [CC=cc] [GO=...] [COVERAGE_C_FLOOR=NN] [COVERAGE_C_XVFB=1] \
#          ./scripts/coverage-c.sh

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
cd "$REPO_ROOT"

GO="${GO:-$REPO_ROOT/.tools/go/bin/go}"
CC="${CC:-cc}"

# Own sources = C_SRC minus miniaudio.c (third-party, excluded from coverage).
# modular_stages.c (the unified gen-bank / family-voice DSP) joined C_SRC in
# the modular-unification migration and is very much "DSP under test".
OWN_SRC=(drums fmsynth effects insert_fx wavetable adsr pan noise modular modular_stages)

OUT="$REPO_ROOT/coverage/c"
SUMMARY="$REPO_ROOT/coverage/c-summary.txt"
mkdir -p "$OUT" "$REPO_ROOT/coverage"

# Always leave build/ pristine — even on failure — so the next `make` rebuilds
# a normal -O2 lib instead of linking against instrumented objects.
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
# The flush shim makes a STRONG __gcov_dump reference (-DBEATMO_GCOV) so the
# defining member is pulled out of libgcov.a — a weak reference would not pull
# it and the dump would silently no-op. Not itself instrumented (no .gcno):
# it is tooling, not DSP under test.
$CC -O2 -DBEATMO_GCOV -c src/c/gcov_flush.c -o build/gcov_flush.o
# noise_ma_oracle.c is the REAL-ma_noise test oracle the native bridge links
# (oracle_ma_noise_white_fill in modular_c.go) — tooling, not DSP under test,
# so compiled without instrumentation, mirroring the Makefile's C_SRC_TEST.
$CC -O2 -c src/c/noise_ma_oracle.c -o build/noise_ma_oracle.o

# --- 3. Archive at the path hardcoded in the #cgo LDFLAGS --------------------
ar rcs build/libdrums.a build/miniaudio.o build/gcov_flush.o build/noise_ma_oracle.o \
    $(printf 'build/%s.o ' "${OWN_SRC[@]}")

# --- 4. Run the native (no -tags test) audio tests through the bridge --------
export CGO_LDFLAGS="--coverage"
export CGO_ENABLED=1

# -linkmode=external is REQUIRED: gcov registers each object's counters via
# C constructors (.init_array), which only run when the binary boots through
# the libc startup path. Go's internal linker entry skips them, leaving
# nothing registered for __gcov_dump to write.
test_rc=0
(
    cd src/go
    "$GO" test -count=1 -ldflags '-linkmode=external' \
        -coverpkg=./internal/audio \
        -coverprofile="$REPO_ROOT/coverage/go-native.out" \
        -covermode=atomic -timeout 360s ./internal/audio
) || test_rc=$?

# Optional xvfb pass picks up mixer tests that need a display; their counters
# accumulate additively into the same build/*.gcda (more coverage, desired).
if [ "${COVERAGE_C_XVFB:-0}" = 1 ] && command -v xvfb-run >/dev/null 2>&1; then
    (
        cd src/go
        xvfb-run -a "$GO" test -count=1 -ldflags '-linkmode=external' \
            -timeout 360s -run 'Mixer|Signal' ./internal/audio
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
    # Run gcov from the repo root: the .gcno records the compile-relative
    # source path (src/c/<f>.c), so annotation only resolves from here. The
    # generated *.gcov land in the cwd and are moved to $OUT afterwards.
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

# Move the annotated files into $OUT, then drop annotations we don't own
# (miniaudio + system headers picked up transitively). Our own header
# annotations stay (synth_params.h / synth_post.h carry inline accessors
# with real branches).
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
