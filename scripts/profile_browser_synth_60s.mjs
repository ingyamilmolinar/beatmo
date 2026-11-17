// Browser-side memory profile for the synth-tab playback session.
//
// Mirrors scripts/capture_browser_runner.mjs's static-server + Playwright
// scaffolding, but instead of capturing screenshots, it:
//   1. Loads the WASM page, awaits runScene/startPlay/setInstrumentParam
//      registration.
//   2. Applies the crop_synth_tab_drum scene (opens Synth tab on a known recipe).
//   3. Starts playback at the requested BPM.
//   4. Drives audio.setInstrumentParam at 12 Hz across every available
//      instrument for 60 seconds (mirrors the desktop record-bench churn).
//   5. Captures, at t = 5/30/60 s:
//        - performance.memory.usedJSHeapSize (Chromium-only; logged "n/a"
//          otherwise)
//        - window.dumpHeapProbe() Go-side CSV (BEATMO_HEAP_PROBE=1 sets the
//          per-second runtime.MemStats ring)
//        - window.perfStats() snapshot
//        - Chromium heap snapshot via CDP HeapProfiler.takeHeapSnapshot
//   6. Writes everything under bench-results/synth-browser-<ts>/.
//
// Run as:
//   GO=$(pwd)/.tools/go/bin/go node scripts/profile_browser_synth_60s.mjs
//
// Environment overrides:
//   PROFILE_BROWSER_SECS         total duration in seconds (default 60)
//   PROFILE_BROWSER_SCENE        scene name (default crop_synth_tab_drum)
//   PROFILE_BROWSER_BPM          BPM (default 140)
//   PROFILE_BROWSER_CHURN_HZ     param churn rate (default 12)
//   PROFILE_BROWSER_SKIP_SNAPSHOT  if "1", skip CDP heap snapshots (faster)

import { chromium } from "../src/js/node_modules/playwright/index.mjs";
import path from "node:path";
import http from "node:http";
import fs from "node:fs";
import { fileURLToPath } from "node:url";

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);
const ROOT = path.join(__dirname, "..");

const DUR_SEC = Number(process.env.PROFILE_BROWSER_SECS ?? 60);
const SCENE = process.env.PROFILE_BROWSER_SCENE ?? "crop_synth_tab_drum";
const BPM = Number(process.env.PROFILE_BROWSER_BPM ?? 140);
const CHURN_HZ = Number(process.env.PROFILE_BROWSER_CHURN_HZ ?? 12);
const SKIP_SNAPSHOT = process.env.PROFILE_BROWSER_SKIP_SNAPSHOT === "1";

const ts = new Date()
    .toISOString()
    .replace(/[:.]/g, "-")
    .replace("T", "_")
    .slice(0, 19);
const outdir = path.join(ROOT, "bench-results", `synth-browser-${ts}`);
fs.mkdirSync(outdir, { recursive: true });
console.log(`[profile] outdir=${outdir}`);

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

const browser = await chromium.launch({
    headless: true,
    args: ["--enable-precise-memory-info"], // unlocks performance.memory in Chromium
});

const ctx = await browser.newContext({
    viewport: { width: 1280, height: 800 },
});
const page = await ctx.newPage();
const cdp = await ctx.newCDPSession(page);
await cdp.send("HeapProfiler.enable");

const consoleLogPath = path.join(outdir, "browser_console.log");
const consoleStream = fs.createWriteStream(consoleLogPath);
page.on("console", (msg) => {
    consoleStream.write(`[${msg.type()}] ${msg.text()}\n`);
});
page.on("pageerror", (err) => {
    consoleStream.write(`[pageerror] ${err.message}\n${err.stack}\n`);
});

const fail = (msg, err) => {
    console.error(`[profile] ${msg}`, err ?? "");
    consoleStream.end();
    server.close();
    browser.close();
    process.exit(1);
};

try {
    await page.goto(`http://localhost:${port}/`);
    // Wait for WASM scene plumbing to register.
    await page.waitForFunction(
        () =>
            typeof window.runScene === "function" &&
            typeof window.forceDraw === "function" &&
            typeof window.startPlay === "function" &&
            typeof window.stopPlay === "function" &&
            typeof window.setInstrumentParam === "function" &&
            typeof window.dumpHeapProbe === "function" &&
            typeof window.perfStats === "function" &&
            typeof window.totalRows === "function" &&
            typeof window.rowInstrument === "function",
        { timeout: 30000 }
    );
    // Settle.
    await page.waitForTimeout(800);

    // Apply scene.
    const sceneApplied = await page.evaluate((scene) => {
        return window.runScene(scene);
    }, SCENE);
    if (!sceneApplied) fail(`runScene(${SCENE}) returned falsy`);
    console.log(`[profile] scene applied: ${SCENE}`);
    await page.evaluate(() => window.forceDraw());
    await page.waitForTimeout(500);

    // Resolve instrument set — enumerate every row's instrument so the churn
    // mirrors the desktop record-bench (which iterates audio.Instruments()).
    const instruments = await page.evaluate(() => {
        const out = new Set();
        const n = (window.totalRows && window.totalRows()) || 0;
        for (let r = 0; r < n; r++) {
            try {
                const id = window.rowInstrument(r);
                if (id) out.add(id);
            } catch (e) {
                // ignore per-row failures
            }
        }
        return [...out];
    });
    if (!Array.isArray(instruments) || instruments.length === 0) {
        fail("no instruments available — runScene may have failed silently");
    }
    console.log(
        `[profile] instruments=${instruments.length}: ${instruments.join(", ")}`
    );

    // Set BPM, start playback.
    await page.evaluate((bpm) => {
        if (typeof window.setBPM === "function") window.setBPM(bpm);
    }, BPM);
    await page.evaluate(() => window.startPlay());
    console.log(`[profile] playback started bpm=${BPM}`);

    // Bench start moment.
    const t0 = Date.now();
    const period = Math.max(1, Math.round(1000 / CHURN_HZ));

    // Param churn loop runs in the page so timing isn't bottlenecked by IPC.
    // It uses setInterval so the JS event loop also exercises GC/scheduling
    // the way a real user-driven session does.
    await page.evaluate(
        ({ insts, period, durMs }) => {
            window.__profileChurnStop = false;
            const start = performance.now();
            window.__profileChurnTicks = 0;
            window.__profileChurnInterval = setInterval(() => {
                if (window.__profileChurnStop) {
                    clearInterval(window.__profileChurnInterval);
                    return;
                }
                const t = (performance.now() - start) / 1000;
                const pitch = -12 + 24 * Math.sin(t * 2.0);
                const decay = 0.25 + 0.75 * Math.sin(t * 1.7);
                for (const id of insts) {
                    try {
                        window.setInstrumentParam(id, "pitch", pitch);
                        window.setInstrumentParam(id, "decay", decay);
                    } catch (e) {
                        // swallow per-tick — surface via console.warn so the
                        // browser_console.log captures it once if it recurs.
                        if (!window.__profileChurnWarned) {
                            console.warn("setInstrumentParam threw:", e);
                            window.__profileChurnWarned = true;
                        }
                    }
                }
                window.__profileChurnTicks++;
                if (performance.now() - start >= durMs) {
                    window.__profileChurnStop = true;
                    clearInterval(window.__profileChurnInterval);
                }
            }, period);
        },
        { insts: instruments, period, durMs: DUR_SEC * 1000 }
    );

    // Capture schedule (offsets in seconds from t0). PROFILE_BROWSER_SAMPLES
    // overrides the default 3-point capture with a denser schedule so we
    // can see growth shape over the run.
    let offsets = [5, 30, DUR_SEC];
    if (process.env.PROFILE_BROWSER_DENSE === "1") {
        offsets = [];
        for (let t = 5; t <= DUR_SEC; t += 5) offsets.push(t);
        if (offsets[offsets.length - 1] !== DUR_SEC) offsets.push(DUR_SEC);
    }
    const samples = [];
    for (const o of offsets) {
        const wait = Math.max(0, t0 + o * 1000 - Date.now());
        if (wait > 0) await page.waitForTimeout(wait);
        let label;
        if (o === 5) label = "warmup";
        else if (o === 30) label = "mid";
        else if (o === DUR_SEC) label = "end";
        else label = `t${o}`;
        console.log(`[profile] capturing snapshot ${label} (t=${o}s)`);

        const goCsv = await page.evaluate(() => window.dumpHeapProbe() ?? "");
        const perfStats = await page.evaluate(() => window.perfStats() ?? {});
        const bridgeStats = await page.evaluate(() => {
            try {
                return window.analyzerBridgeStats?.() ?? null;
            } catch (e) {
                return null;
            }
        });
        const perfMem = await page.evaluate(() => {
            const m = performance.memory;
            if (!m)
                return {
                    note: "performance.memory unavailable (non-Chromium browser?)",
                };
            return {
                usedJSHeapSize: m.usedJSHeapSize,
                totalJSHeapSize: m.totalJSHeapSize,
                jsHeapSizeLimit: m.jsHeapSizeLimit,
            };
        });
        const ticks = await page.evaluate(
            () => window.__profileChurnTicks ?? 0
        );

        fs.writeFileSync(
            path.join(outdir, `heap_probe_${label}.csv`),
            goCsv,
            "utf8"
        );
        fs.writeFileSync(
            path.join(outdir, `perf_stats_${label}.json`),
            JSON.stringify(perfStats, null, 2),
            "utf8"
        );
        const sample = {
            label,
            tSeconds: o,
            wallClock: new Date().toISOString(),
            churnTicks: ticks,
            performance_memory: perfMem,
            perfStats,
            bridgeStats,
        };
        samples.push(sample);

        if (!SKIP_SNAPSHOT) {
            const snapPath = path.join(outdir, `heap_${label}.heapsnapshot`);
            const chunks = [];
            const onChunk = (event) => chunks.push(event.chunk);
            cdp.on("HeapProfiler.addHeapSnapshotChunk", onChunk);
            await cdp.send("HeapProfiler.collectGarbage");
            await cdp.send("HeapProfiler.takeHeapSnapshot", {
                reportProgress: false,
                captureNumericValue: true,
            });
            cdp.off("HeapProfiler.addHeapSnapshotChunk", onChunk);
            fs.writeFileSync(snapPath, chunks.join(""), "utf8");
            console.log(
                `[profile] heap snapshot → ${snapPath} size=${
                    fs.statSync(snapPath).size
                }B`
            );
        }
    }

    // Stop churn + playback.
    await page.evaluate(() => {
        window.__profileChurnStop = true;
        if (typeof window.stopPlay === "function") window.stopPlay();
    });
    await page.waitForTimeout(200);

    fs.writeFileSync(
        path.join(outdir, "summary.json"),
        JSON.stringify(
            {
                scene: SCENE,
                bpm: BPM,
                durationSec: DUR_SEC,
                churnHz: CHURN_HZ,
                instruments,
                skipSnapshot: SKIP_SNAPSHOT,
                samples,
            },
            null,
            2
        ),
        "utf8"
    );
    console.log(`[profile] summary → ${path.join(outdir, "summary.json")}`);
} catch (e) {
    fail("run failed", e);
} finally {
    consoleStream.end();
    await browser.close();
    server.close();
}
console.log(`[profile] done.  outdir=${outdir}`);
