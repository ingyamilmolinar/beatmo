// Browser capture runner for the screenshots-all target.
//
// Iterates the comma-separated SCENES env var. For each scene:
//   1. Loads the WASM page via a tiny static-file server.
//   2. Awaits runScene(name) (registered by initJSScenes).
//   3. Runs forceDraw to flush the cache.
//   4. page.screenshot to OUTDIR/{desktop,mobile}/<name>.browser.png
//
// MOBILE=1 enables a second pass with Playwright's mobile viewport flags.

import { chromium } from "../src/js/node_modules/playwright/index.mjs";
import path from "node:path";
import http from "node:http";
import fs from "node:fs";
import { fileURLToPath } from "node:url";

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);
const ROOT = path.join(__dirname, "..");

const outdir = process.env.OUTDIR;
const scenesCsv = process.env.SCENES || "";
const mobileFlag = process.env.MOBILE === "1";

if (!outdir) {
    console.error("OUTDIR not set");
    process.exit(1);
}

const scenes = scenesCsv.split(",").map((s) => s.trim()).filter(Boolean);
if (scenes.length === 0) {
    console.error("no scenes to run");
    process.exit(0);
}

// ─── static server ──────────────────────────────────────────────
const jsroot = path.join(ROOT, "src/js");
const server = http.createServer((req, res) => {
    let fp = path.join(jsroot, req.url === "/" ? "index.html" : req.url);
    if (!fs.existsSync(fp)) {
        res.writeHead(404);
        res.end();
        return;
    }
    const ct =
        {
            ".html": "text/html",
            ".js": "text/javascript",
            ".wasm": "application/wasm",
            ".css": "text/css",
            ".json": "application/json",
        }[path.extname(fp)] || "application/octet-stream";
    res.writeHead(200, { "Content-Type": ct });
    fs.createReadStream(fp).pipe(res);
});
await new Promise((r) => server.listen(0, r));
const port = server.address().port;

const browser = await chromium.launch({ headless: true });

// Wait for runScene + forceDraw to be registered.
async function awaitWasmReady(page) {
    await page.waitForFunction(
        () => typeof window.runScene === "function" && typeof window.forceDraw === "function",
        { timeout: 15000 }
    );
    // Settle.
    await page.waitForTimeout(800);
}

async function captureScene(ctx, scene, subdir) {
    const page = await ctx.newPage();
    try {
        await page.goto(`http://localhost:${port}/`);
        await awaitWasmReady(page);
        await page.evaluate((name) => window.runScene(name), scene);
        await page.waitForTimeout(600);
        await page.evaluate(() => window.forceDraw && window.forceDraw());
        await page.waitForTimeout(400);
        const out = path.join(outdir, subdir, `${scene}.browser.png`);
        fs.mkdirSync(path.dirname(out), { recursive: true });
        await page.screenshot({ path: out });
        console.log(`  ✓ ${scene} (${subdir})`);
    } catch (e) {
        console.log(`  ✗ ${scene} (${subdir}): ${e.message}`);
    } finally {
        await page.close();
    }
}

// ─── desktop browser pass ───────────────────────────────────────
const desktopCtx = await browser.newContext({
    viewport: { width: 1280, height: 720 },
});
for (const s of scenes) {
    if (s.startsWith("mobile_")) continue;
    await captureScene(desktopCtx, s, "desktop");
}
await desktopCtx.close();

// ─── mobile browser pass ────────────────────────────────────────
if (mobileFlag) {
    const mobileCtx = await browser.newContext({
        viewport: { width: 390, height: 844 },
        isMobile: true,
        hasTouch: true,
    });
    for (const s of scenes) {
        // Mobile pass captures all scenes; mobile-only scenes belong here.
        await captureScene(mobileCtx, s, "mobile");
    }
    await mobileCtx.close();
}

await browser.close();
server.close();
