#!/usr/bin/env bash
# Capture screenshots of every UI surface in the beatmo scene catalog.
#
# Iterates ui.SceneNames() and runs each scene through both:
#   - Desktop (native Ebiten via xvfb-run + -scene flag)
#   - Browser (Playwright + window.runScene)
#
# Scenes whose name starts with "crop_" declare a Subject in the catalog
# (see internal/ui/scene_catalog.go); the desktop binary and Playwright
# runner both crop the resulting PNG to that surface's bounds. Other
# scenes capture full-screen.
#
# Set MOBILE=1 to additionally capture mobile-viewport browser scenes.
# Set SCENES=name1,name2 to filter to specific scenes. To capture only
# the cropped subjects, pass a comma-separated list of crop_* names —
# e.g. SCENES=$(./tmp/beatmo_screenshot -list-scenes | grep '^crop_' | paste -sd ,).
#
# Output: screenshots/all/{desktop,mobile}/<scene>.png + manifest.json
# Usage:  make screenshots-all [MOBILE=1] [SCENES=transport_idle,context_menu_open]

set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
GO="${GO:-$ROOT/.tools/go/bin/go}"
OUTDIR="${OUTDIR:-$ROOT/screenshots/all}"
SCENES="${SCENES:-}"
MOBILE="${MOBILE:-0}"

# Resolve OUTDIR to an absolute path: the desktop build below cds into
# $ROOT/src/go and stays there for the desktop capture loop. A relative
# OUTDIR (e.g. the Makefile default "screenshots/all") would then write
# under src/go/, where the pre-mkdir-ed directory doesn't exist — and
# xvfb-run silently fails without writing the PNG (every desktop scene
# reports "no PNG written"). Make OUTDIR absolute up front so neither
# the mkdir nor the xvfb-run -screenshot path depends on cwd.
if [[ "$OUTDIR" != /* ]]; then
    OUTDIR="$(pwd)/$OUTDIR"
fi

mkdir -p "$OUTDIR/desktop"
[[ "$MOBILE" == "1" ]] && mkdir -p "$OUTDIR/mobile"

echo "=== Beatmo Capture-All UI ==="
echo "Output: $OUTDIR"
echo "Mobile: $MOBILE"
[[ -n "$SCENES" ]] && echo "Scenes filter: $SCENES"

# ── Build the desktop binary once ──────────────────────────────────
cd "$ROOT/src/go"
CGO_ENABLED=1 "$GO" build -o /tmp/beatmo_screenshot ./cmd

# ── Discover scene names ───────────────────────────────────────────
ALL_NAMES=$(/tmp/beatmo_screenshot -list-scenes 2>/dev/null)
if [[ -z "$ALL_NAMES" ]]; then
    echo "ERROR: -list-scenes returned no output" >&2
    exit 1
fi
MOBILE_NAMES=""
if [[ "$MOBILE" == "1" ]]; then
    MOBILE_NAMES=$(/tmp/beatmo_screenshot -list-mobile-scenes 2>/dev/null)
    if [[ -z "$MOBILE_NAMES" ]]; then
        echo "WARN: -list-mobile-scenes returned no output" >&2
    fi
fi

# Apply SCENES filter if set.
filter_names() {
    local pool="$1"
    if [[ -z "$SCENES" ]]; then
        echo "$pool"
        return
    fi
    local wanted_csv="$SCENES"
    local IFS_BAK="$IFS"
    IFS=',' read -ra wanted <<< "$wanted_csv"
    IFS="$IFS_BAK"
    while IFS= read -r n; do
        [[ -z "$n" ]] && continue
        for w in "${wanted[@]}"; do
            if [[ "$n" == "$w" ]]; then
                echo "$n"
                break
            fi
        done
    done <<< "$pool"
}

SCENE_LIST=$(filter_names "$ALL_NAMES")
MOBILE_SCENE_LIST=$(filter_names "$MOBILE_NAMES")
if [[ -z "$SCENE_LIST" ]]; then
    echo "WARN: no scenes match filter '$SCENES'"
    exit 0
fi

DESKTOP_COUNT=0
DESKTOP_FAILED=0

# ── Desktop pass ───────────────────────────────────────────────────
echo ""
echo "[desktop] capturing $(echo "$SCENE_LIST" | wc -l) scenes..."
for name in $SCENE_LIST; do
    # Skip mobile-only scenes for desktop pass.
    if [[ "$name" == mobile_* ]]; then
        continue
    fi
    out="$OUTDIR/desktop/$name.png"
    if xvfb-run -a -s "-screen 0 1280x720x24" /tmp/beatmo_screenshot \
        -log ERROR -scene "$name" -screenshot "$out" 2>/dev/null; then
        if [[ -f "$out" ]]; then
            echo "  ✓ $name"
            DESKTOP_COUNT=$((DESKTOP_COUNT + 1))
        else
            echo "  ✗ $name (no PNG written)"
            DESKTOP_FAILED=$((DESKTOP_FAILED + 1))
        fi
    else
        echo "  ✗ $name (binary error)"
        DESKTOP_FAILED=$((DESKTOP_FAILED + 1))
    fi
done

# ── Browser pass ──────────────────────────────────────────────────
BROWSER_COUNT=0
BROWSER_FAILED=0
if [[ -d "$ROOT/src/js/node_modules/playwright" ]]; then
    cd "$ROOT"
    # Build WASM if not present.
    if [[ ! -f "$ROOT/src/js/main.wasm" ]]; then
        echo ""
        echo "[browser] building WASM..."
        make -C "$ROOT" wasm >/dev/null
    fi
    echo ""
    echo "[browser] capturing scenes via Playwright..."

    SCENE_LIST_CSV=$(echo "$SCENE_LIST" | tr '\n' ',' | sed 's/,$//')
    MOBILE_SCENE_LIST_CSV=$(echo "$MOBILE_SCENE_LIST" | tr '\n' ',' | sed 's/,$//')

    OUTDIR="$OUTDIR" SCENES="$SCENE_LIST_CSV" MOBILE_SCENES="$MOBILE_SCENE_LIST_CSV" \
        MOBILE="$MOBILE" \
        node "$ROOT/scripts/capture_browser_runner.mjs" || true

    BROWSER_COUNT=$(ls "$OUTDIR/desktop"/*.browser.png 2>/dev/null | wc -l)
else
    echo "[browser] skipped (playwright not installed)"
fi

# ── Manifest ──────────────────────────────────────────────────────
GIT_SHA=$(git -C "$ROOT" rev-parse HEAD 2>/dev/null || echo "unknown")
TS=$(date -u +%FT%TZ)

# Build a "name<TAB>subject" lookup from the binary so each manifest entry
# carries its declared crop subject (empty for full-screen scenes).
SUBJECTS_TSV=$(/tmp/beatmo_screenshot -list-scene-subjects 2>/dev/null || true)
subject_for() {
    local name="$1"
    awk -F'\t' -v n="$name" '$1 == n { print $2; found=1; exit } END { if (!found) print "" }' <<< "$SUBJECTS_TSV"
}

MANIFEST="$OUTDIR/manifest.json"
{
    echo "{"
    echo "  \"git_sha\": \"$GIT_SHA\","
    echo "  \"timestamp\": \"$TS\","
    echo "  \"mobile\": $([[ "$MOBILE" == "1" ]] && echo "true" || echo "false"),"
    echo "  \"scenes\": ["
    first=1
    for f in "$OUTDIR/desktop"/*.png; do
        [[ -f "$f" ]] || continue
        n=$(basename "$f" .png)
        subj=$(subject_for "$n")
        [[ $first -eq 1 ]] && first=0 || echo ","
        printf "    {\"name\": \"%s\", \"profile\": \"desktop\", \"subject\": \"%s\", \"path\": \"desktop/%s.png\"}" "$n" "$subj" "$n"
    done
    if [[ "$MOBILE" == "1" ]]; then
        for f in "$OUTDIR/mobile"/*.png; do
            [[ -f "$f" ]] || continue
            n=$(basename "$f" .png)
            subj=$(subject_for "$n")
            [[ $first -eq 1 ]] && first=0 || echo ","
            printf "    {\"name\": \"%s\", \"profile\": \"mobile\", \"subject\": \"%s\", \"path\": \"mobile/%s.png\"}" "$n" "$subj" "$n"
        done
    fi
    echo ""
    echo "  ]"
    echo "}"
} > "$MANIFEST"

echo ""
echo "Done."
echo "  Desktop: $DESKTOP_COUNT captured ($DESKTOP_FAILED failed)"
[[ "$MOBILE" == "1" ]] && echo "  Mobile: see $OUTDIR/mobile/"
echo "  Manifest: $MANIFEST"
