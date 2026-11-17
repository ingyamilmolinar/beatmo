// probe_drawdrum_starvation.mjs
//
// Minimal hypothesis test for the "choppy without updates" baseline: is the
// per-beat row-rack recomposite (drawDrum) the cause of sequencer starvation
// (Stage A seqFireLate)?
//
// A/B the SAME plain playback twice:
//   A. follow=ON  (default) — playhead scrolls the window every beat →
//      offsetChanged → markRowsShiftDirty() → rows-layer recomposite/frame.
//   B. follow=OFF (setSimpleDraw(true)) — window does not scroll → no per-beat
//      recomposite. Audio scheduling path is byte-identical.
//
// If Stage A + drawDrum collapse in B, the row-rack recomposite is the
// dominant main-thread cost starving the sequencer goroutine.
//
// Run: WASM_PREBUILT=1 GO=$(pwd)/.tools/go/bin/go node scripts/probe_drawdrum_starvation.mjs

import { chromium } from "../src/js/node_modules/playwright/index.mjs";
import path from "node:path";
import http from "node:http";
import fs from "node:fs";
import { fileURLToPath } from "node:url";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const ROOT = path.join(__dirname, "..");
const jsroot = path.join(ROOT, "src/js");
const SECS = Number(process.env.PROBE_SECS ?? 12);
const BPM = Number(process.env.PROBE_BPM ?? 200);

const server = http.createServer((req, res) => {
  const fp = path.join(jsroot, req.url === "/" ? "index.html" : req.url.replace(/^\//, ""));
  if (!fs.existsSync(fp)) { res.writeHead(404); res.end(); return; }
  const ct = { ".html": "text/html", ".js": "application/javascript", ".wasm": "application/wasm", ".json": "application/json" }[path.extname(fp)] || "application/octet-stream";
  res.writeHead(200, { "Content-Type": ct });
  fs.createReadStream(fp).pipe(res);
});
await new Promise((r) => server.listen(0, r));
const port = server.address().port;

const browser = await chromium.launch({ headless: true, args: ["--autoplay-policy=no-user-gesture-required"] });
const page = await browser.newPage();
await page.goto(`http://localhost:${port}/`);
await page.waitForFunction(() => typeof startPlay === "function" && typeof setSimpleDraw === "function");
await page.waitForTimeout(400);

async function run(label, simple) {
  await page.evaluate((s) => {
    document.dispatchEvent(new Event("pointerdown"));
    resumeAudio?.();
    stopPlay();
    setSimpleDraw(!!s);          // s=true → follow OFF, no per-beat scroll
    resetPerfStats(); resetAudioScheduleMetrics(); resetThreeStageLatency();
    setBPM(s ? 200 : 200); startPlay();
  }, simple);
  // sample drawDrum each second (instantaneous last-frame), keep the max
  let drawDrumMax = 0, drawGridMax = 0;
  for (let i = 0; i < SECS; i++) {
    await page.waitForTimeout(1000);
    const p = await page.evaluate(() => perfStats());
    drawDrumMax = Math.max(drawDrumMax, p.drawDrumMS || 0);
    drawGridMax = Math.max(drawGridMax, p.drawGridMS || 0);
  }
  const f = await page.evaluate(() => ({ perf: perfStats(), three: getThreeStageLatency(), sched: getAudioScheduleMetrics() }));
  await page.evaluate(() => stopPlay());
  const a = f.three?.seqFireLate || {};
  return {
    label, simple,
    stageA_p99_ms: a.p99 != null ? +(a.p99 * 1000).toFixed(0) : null,
    stageA_max_ms: a.max != null ? +(a.max * 1000).toFixed(0) : null,
    drawAvg_ms: +(f.perf.drawAvgMS || 0).toFixed(1),
    drawMax_ms: +(f.perf.drawMaxMS || 0).toFixed(1),
    drawDrumMax_ms: +drawDrumMax.toFixed(1),
    drawGridMax_ms: +drawGridMax.toFixed(1),
    qlatMax_ms: +(f.perf.audioQLatMax || 0).toFixed(1),
    overdue: f.sched?.overdue, schedCount: f.sched?.count,
  };
}

// Warm caches first (discard), then measure A then B.
await run("warmup", false);
const A = await run("A: follow ON (per-beat recomposite)", false);
const B = await run("B: follow OFF (no recomposite)", true);

console.log("\n=== drawDrum→starvation A/B (plain playback, no mutations) ===");
for (const r of [A, B]) console.log(JSON.stringify(r));
const drop = A.stageA_p99_ms && B.stageA_p99_ms ? (1 - B.stageA_p99_ms / A.stageA_p99_ms) * 100 : null;
const drumDrop = A.drawDrumMax_ms ? (1 - B.drawDrumMax_ms / A.drawDrumMax_ms) * 100 : null;
console.log(`\nStage A p99 change A→B: ${drop != null ? drop.toFixed(0) + "% lower with follow OFF" : "n/a"}`);
console.log(`drawDrum max change A→B: ${drumDrop != null ? drumDrop.toFixed(0) + "% lower with follow OFF" : "n/a"}`);

await browser.close();
server.close();
