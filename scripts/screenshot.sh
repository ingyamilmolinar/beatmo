#!/bin/bash
# Capture screenshots of beatmo in three modes:
#   1. Desktop (native Ebiten app via xvfb-run)
#   2. Browser desktop layout (WASM, 1280x720)
#   3. Browser mobile layout (WASM, 390x844)
#
# Output: screenshots/ directory with timestamped PNGs
# Usage: make screenshot
#        make screenshot OUTDIR=/tmp/shots

set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
GO="${GO:-$ROOT/.tools/go/bin/go}"
OUTDIR="${OUTDIR:-$ROOT/screenshots}"
mkdir -p "$OUTDIR"

echo "=== Beatmo Screenshots ==="
echo "Output: $OUTDIR"

# ── 1. Desktop (native Ebiten) ──────────────────────────────────────────────
echo ""
echo "[1/3] Desktop (native Ebiten)..."
DESKTOP_OUT="$OUTDIR/desktop.png"

cd "$ROOT/src/go"
CGO_ENABLED=1 "$GO" build -o /tmp/beatmo_screenshot ./cmd

xvfb-run -a -s "-screen 0 1280x720x24" /tmp/beatmo_screenshot \
    -log ERROR -screenshot "$DESKTOP_OUT" 2>/dev/null

if [ -f "$DESKTOP_OUT" ]; then
    echo "  Saved: $DESKTOP_OUT"
else
    echo "  ERROR: Desktop screenshot failed"
    exit 1
fi

# ── 2 & 3. Browser screenshots (WASM via Playwright) ────────────────────────
echo ""
echo "[2/3] Browser desktop layout (1280x720)..."
echo "[3/3] Browser mobile layout (390x844)..."

cd "$ROOT"
node --no-warnings - "$OUTDIR" <<'SCRIPT'
const { chromium } = require('./src/js/node_modules/playwright');
const path = require('path');
const http = require('http');
const fs = require('fs');

(async () => {
    const outdir = process.argv[2];
    const root = path.join(__dirname, 'src/js');

    // Simple static file server
    const server = http.createServer((req, res) => {
        let fp = path.join(root, req.url === '/' ? 'index.html' : req.url);
        if (!fs.existsSync(fp)) { res.writeHead(404); res.end(); return; }
        const ct = {
            '.html': 'text/html',
            '.js': 'text/javascript',
            '.wasm': 'application/wasm',
            '.css': 'text/css',
            '.json': 'application/json',
        }[path.extname(fp)] || 'application/octet-stream';
        res.writeHead(200, { 'Content-Type': ct });
        fs.createReadStream(fp).pipe(res);
    });
    await new Promise(r => server.listen(0, r));
    const port = server.address().port;
    const browser = await chromium.launch({ headless: true });

    // Desktop browser layout
    const dc = await browser.newContext({ viewport: { width: 1280, height: 720 } });
    const dp = await dc.newPage();
    await dp.goto(`http://localhost:${port}/`);
    await dp.waitForTimeout(4000);
    const desktopPath = path.join(outdir, 'browser_desktop.png');
    await dp.screenshot({ path: desktopPath });
    console.log(`  Saved: ${desktopPath}`);
    await dc.close();

    // Mobile browser layout
    const mc = await browser.newContext({
        viewport: { width: 390, height: 844 },
        isMobile: true,
        hasTouch: true,
    });
    const mp = await mc.newPage();
    await mp.goto(`http://localhost:${port}/`);
    await mp.waitForTimeout(4000);
    const mobilePath = path.join(outdir, 'browser_mobile.png');
    await mp.screenshot({ path: mobilePath });
    console.log(`  Saved: ${mobilePath}`);
    await mc.close();

    await browser.close();
    server.close();
})();
SCRIPT

echo ""
echo "Done! Screenshots in $OUTDIR/"
ls -lh "$OUTDIR"/*.png
