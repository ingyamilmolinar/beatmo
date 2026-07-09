// Browser capture runner for the screenshots-all target.
//
// Iterates the comma-separated SCENES env var. For each scene:
//   1. Loads the WASM page via a tiny static-file server.
//   2. Awaits runScene(name) (registered by initJSScenes).
//   3. Runs forceDraw to flush the cache.
//   4. Queries subjectRectJS for the scene's declared subject. If the
//      scene declares a non-empty subject, page.screenshot is called with
//      a clip rectangle so the resulting PNG is cropped to that surface.
//      Scenes with no subject (the legacy default) capture full-screen.
//   5. page.screenshot to OUTDIR/{desktop,mobile}/<name>.browser.png
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
const mobileScenesCsv = process.env.MOBILE_SCENES || "";
const mobileFlag = process.env.MOBILE === "1";

if (!outdir) {
    console.error("OUTDIR not set");
    process.exit(1);
}

const scenes = scenesCsv.split(",").map((s) => s.trim()).filter(Boolean);
const mobileScenes = mobileScenesCsv.split(",").map((s) => s.trim()).filter(Boolean);
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

// --autoplay-policy=no-user-gesture-required lets WebAudio start without
// a real user gesture. Without it, playback scenes capture silence
// because AudioContext stays in the "suspended" state — the analyser's
// time-domain buffer never updates and the Spectrum / Levels / Chain
// renderers all read zeros (Phase 0a audio-panel redesign).
const browser = await chromium.launch({
    headless: true,
    args: ["--autoplay-policy=no-user-gesture-required"],
});

// Wait for runScene + forceDraw to be registered.
async function awaitWasmReady(page) {
    await page.waitForFunction(
        () => typeof window.runScene === "function" && typeof window.forceDraw === "function" && typeof window.subjectRectJS === "function",
        { timeout: 15000 }
    );
    // Settle.
    await page.waitForTimeout(800);
}

async function captureScene(ctx, scene, subdir, mobile) {
    const page = await ctx.newPage();
    try {
        await page.goto(`http://localhost:${port}/`);
        await awaitWasmReady(page);
        // Unlock the AudioContext before running the scene. Playback
        // scenes that don't do this capture silence: AudioContext stays
        // "suspended", the AnalyserNode never updates, and the Spectrum
        // / Levels / Chain renderers all read zeros (Phase 0a audio-
        // panel redesign root cause).
        await page.evaluate(() => {
            try { document.dispatchEvent(new Event("pointerdown")); } catch (_) {}
            try { window.resumeAudio && window.resumeAudio(); } catch (_) {}
        });
        try {
            await page.waitForFunction(
                () => window.__audioCtx && window.__audioCtx.state === "running",
                {},
                { timeout: 5000 },
            );
        } catch (_) {
            // Some non-playback scenes don't create the audio context;
            // proceed regardless — those scenes don't need the unlock.
        }
        await page.evaluate(() => window.audioReady).catch(() => {});
        await page.evaluate(
            ({ name, mobile }) => {
                if (mobile && typeof window.runSceneMobile === "function") {
                    window.runSceneMobile(name);
                } else {
                    window.runScene(name);
                }
            },
            { name: scene, mobile },
        );

        // Honor the scene's declared SettleFrames the same way the desktop
        // binary does (screenshotThreshold): wait frames/60 → seconds before
        // capturing so slow-settling scenes (menus, audio analyzers, caches)
        // are fully rendered. 0/undefined → the 90-frame default. A small floor
        // and post-forceDraw buffer keep fast scenes stable.
        const settleFrames = await page.evaluate((sceneName) => {
            const all = (window.listScenes && window.listScenes()) || [];
            const meta = all.find((s) => s.name === sceneName);
            return (meta && meta.settleFrames) || 0;
        }, scene);
        const settleMs = Math.max(1000, Math.ceil(((settleFrames || 90) / 60) * 1000));
        await page.waitForTimeout(settleMs);
        await page.evaluate(() => window.forceDraw && window.forceDraw());
        await page.waitForTimeout(200);

        // Resolve the scene's declared subject (if any) and crop to its
        // bounds. Scenes with no subject capture full-screen — same as
        // before.
        const subjectInfo = await page.evaluate((sceneName) => {
            const all = (window.listScenes && window.listScenes()) || [];
            const meta = all.find((s) => s.name === sceneName);
            if (!meta || !meta.subject) {
                return { subject: "", visible: true };
            }
            const r = window.subjectRectJS(meta.subject);
            return { subject: meta.subject, ...r };
        }, scene);

        const out = path.join(outdir, subdir, `${scene}.browser.png`);
        fs.mkdirSync(path.dirname(out), { recursive: true });
        const shotOpts = { path: out };
        if (subjectInfo.subject && subjectInfo.visible && subjectInfo.w > 0 && subjectInfo.h > 0) {
            shotOpts.clip = {
                x: subjectInfo.x,
                y: subjectInfo.y,
                width: subjectInfo.w,
                height: subjectInfo.h,
            };
        } else if (subjectInfo.subject) {
            // Scene declared a subject but it's not visible — emit a clear
            // failure rather than a misleading full-screen PNG.
            throw new Error(`subject ${subjectInfo.subject} not visible (Setup did not produce it)`);
        }
        await page.screenshot(shotOpts);
        const tag = subjectInfo.subject ? ` [${subjectInfo.subject}]` : "";
        console.log(`  ✓ ${scene} (${subdir})${tag}`);
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
    await captureScene(desktopCtx, s, "desktop", false);
}
await desktopCtx.close();

// ─── mobile browser pass ────────────────────────────────────────
//
// Only scenes the catalog marks Mobile=true (received via MOBILE_SCENES)
// run here. Scenes with no mobile route (e.g. desktop-only inspectors)
// would otherwise produce screenshots byte-identical to mobile_default,
// which is what this rewrite fixes.
if (mobileFlag) {
    const mobilePool = mobileScenes.length > 0 ? mobileScenes : scenes;
    const mobileCtx = await browser.newContext({
        viewport: { width: 390, height: 844 },
        isMobile: true,
        hasTouch: true,
    });
    for (const s of mobilePool) {
        await captureScene(mobileCtx, s, "mobile", true);
    }
    await mobileCtx.close();
}

await browser.close();
server.close();
