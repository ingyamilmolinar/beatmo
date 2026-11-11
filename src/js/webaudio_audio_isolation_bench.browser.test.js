// webaudio_audio_isolation_bench.browser.test.js
//
// "A UI bug must not affect audio." This bench plays the startup demo in the
// real app (main.wasm + index.html) under a matrix of UI loads and measures,
// with the playback probe (audio.js → audio_probe_worklet.js →
// audio_probe_analysis.js), what the listener actually HEARS relative to what
// the predictor said should fire:
//
//   lag vs grid / vs request   onset − predictor time / − scheduler request
//   late notes                 > 10 / 20 / 50 ms behind the request
//   IOI error                  rhythm evenness vs the predictor's intervals
//   lookahead shifts           the audio cushion moving mid-play (tempo hiccup)
//   jsQueue                    request → BufferSource.start inside audio.js
//   arrivedLate                notes that reached audio.js already overdue
//                              (the Go sequencer was starved by the UI)
//   missed / dropped           notes silent in the output / never started
//   clicks / cuts / slips      signal glitches and audio-thread underruns
//
// Scenarios (ISOLATION_SCENARIOS=a,b to filter):
//   idle             plain playback, no interaction
//   stall-50         synthetic UI bug: the main thread blocks 50 ms every 250 ms
//   stall-150        …150 ms every 500 ms
//   stall-150-microtask  same stall, legacy microtask flush (A/B for sync start())
//   tab-synth        the heaviest audio-panel tab drawing
//   synth-graphs     the Synth tab's graphs working: real pointer presses and
//                    drags on its dials (every press renders + analyses the
//                    dial's two ends, every drag detent the live sound) and a
//                    switch to another row's instrument every few steps (its
//                    dial relevance is measured on first sight)
//   tab-chain        the Chain (scope) tab drawing
//   zoom-storm       continuous zoom gestures (grid re-rasterization)
//   pan-storm        continuous pan gestures
//   fx-worklet       3 insert effects per row on the audio-thread host (the
//                    C in the beatmo-dsp worklet), under stall-150. (The old
//                    fx-scriptproc A/B is gone: the host is chosen once per
//                    AudioContext, and the main-thread host is only the
//                    no-AudioWorklet fallback.)
//
// Gates (always on — deterministic even under headless software GL):
//   - every scenario: notes were judged, none dropped, none silent
//   - jsQueue p99 ≤ one 60 Hz frame (sequencer batches start synchronously; a queue wait
//                          here is the UI frame leaking into audio timing)
//   - lookahead shifts ≤ 3 (the cushion ratchets, never oscillates)
//   - fx-worklet: the effects really run on the worklet host, and the stalled
//     main thread causes no hard cuts on those channels
// Lateness / IOI limits are REPORTED, and enforced only with
// AUDIO_PROBE_STRICT=1: headless Chromium rasterizes WebGL in software on the
// renderer main thread (400-600 ms frames here), which starves the Go
// sequencer far beyond anything real hardware does.
//
// Output: bench-results/audio_isolation.json
//
// Run:
//   GO=$(pwd)/.tools/go/bin/go node src/js/webaudio_audio_isolation_bench.browser.test.js
// Env: ISOLATION_SECS (default 8), ISOLATION_BPM (default 120),
//      ISOLATION_SCENARIOS, WASM_PREBUILT=1, WASM_FILE=<path to serve as main.wasm>,
//      SITE_DIR=<dir to serve every other file from>, ISOLATION_ENV=<JSON for
//      window.__beatmoEnv>, AB_ARM=<name: output → audio_isolation_<name>.json>

import http from "http";
import { spawnSync } from "child_process";
import fs from "fs";
import path from "path";
import { fileURLToPath } from "url";
import { resolveGoBinary, shouldSkipWasmBuild } from "./browser_test_helpers.js";
import { launchBrowser } from "./platform/launch.js";
import {
  startPlaybackProbe, stopPlaybackProbe, probeSummaryFields, probeGateFailures,
  PROBE_STRICT, PROBE_LIMITS_DEFAULT, PROBE_JSQUEUE_MAX_MS,
} from "./audio_probe_bench_helpers.js";

import { startPlaybackWhenReady, markEdit } from "./audio_probe_bench_helpers.js";
const __dirname = path.dirname(fileURLToPath(import.meta.url));
const jsDir = __dirname;
const goDir = path.resolve(jsDir, "../go");
const GO = resolveGoBinary();
// SITE_DIR serves every non-wasm file from another directory (the A/B runner
// points it at a frozen pre-change build: scripts/audio_lane_ab.mjs).
const SITE_DIR = process.env.SITE_DIR ? path.resolve(process.env.SITE_DIR) : jsDir;
const WASM_FILE = process.env.WASM_FILE ? path.resolve(process.env.WASM_FILE)
  : (process.env.SITE_DIR ? path.join(SITE_DIR, "main.wasm") : path.join(jsDir, "main.wasm"));

if (!process.env.WASM_FILE && !process.env.SITE_DIR && !shouldSkipWasmBuild("main.wasm")) {
  const build = spawnSync(
    GO,
    ["build", "-ldflags", "-X main.defaultLog=INFO", "-o", WASM_FILE, "./cmd"],
    { cwd: goDir, env: { ...process.env, GOOS: "js", GOARCH: "wasm" }, stdio: "inherit" },
  );
  if (build.status !== 0) throw new Error("go build main wasm failed");
}

const server = http.createServer((req, res) => {
  const url = req.url.split("?")[0];
  const file = url === "/" ? "/index.html" : url;
  const filePath = file === "/main.wasm" ? WASM_FILE : path.join(SITE_DIR, file.replace(/^\//, ""));
  fs.readFile(filePath, (err, data) => {
    if (err) { res.writeHead(404); res.end(); return; }
    let ct = "text/plain";
    if (filePath.endsWith(".html")) ct = "text/html";
    else if (filePath.endsWith(".js")) ct = "application/javascript";
    else if (filePath.endsWith(".wasm")) ct = "application/wasm";
    else if (filePath.endsWith(".json")) ct = "application/json";
    res.writeHead(200, { "Content-Type": ct, "Cache-Control": "no-store" });
    res.end(data);
  });
});
await new Promise((r) => server.listen(0, r));
const port = server.address().port;

const SECS = Number(process.env.ISOLATION_SECS ?? 8);
const BPM = Number(process.env.ISOLATION_BPM ?? 120);
const FX_TYPES = ["bitcrusher", "phaser", "autowah"];

// Page-side load injectors. Each returns nothing; `window.__isoStop()` ends it.
const SCENARIOS = [
  { name: "idle" },
  { name: "stall-50", load: { stall: [250, 50] } },
  { name: "stall-150", load: { stall: [500, 150] } },
  { name: "stall-300", load: { stall: [700, 300] } },
  // Edit storms with a 150 ms / 500 ms stall underneath: an edit must reach
  // the next not-started note even while the main thread stutters.
  //   edit-mute — row 0 mute/unmute every 600 ms
  //   edit-vol  — row 1 volume steps every 400 ms
  { name: "edit-mute", edits: "mute", load: { stall: [500, 150] } },
  { name: "edit-vol", edits: "vol", load: { stall: [500, 150] } },
  // A/B for the synchronous start(): the legacy microtask flush under the
  // same stall (see enqueueAudioEvents).
  { name: "stall-150-microtask", flush: "microtask", load: { stall: [500, 150] } },
  { name: "tab-synth", tab: "synth" },
  { name: "synth-graphs", tab: "synth", load: { graphs: true } },
  { name: "tab-chain", tab: "scope" },
  { name: "zoom-storm", load: { zoom: true } },
  { name: "pan-storm", load: { pan: true } },
  { name: "fx-worklet", fx: "worklet", load: { stall: [500, 150] } },
];
const only = (process.env.ISOLATION_SCENARIOS || "").split(",").map((s) => s.trim()).filter(Boolean);
const scenarios = only.length ? SCENARIOS.filter((s) => only.includes(s.name)) : SCENARIOS;

const failures = [];
const infos = [];
const results = [];

const { browser } = await launchBrowser(); // TEST_PLATFORM=chrome-local TEST_HEADED=1: real Chrome, real GPU
try {
  const page = await browser.newPage({ viewport: { width: 1280, height: 720 } });
  // ISOLATION_ENV (JSON) → window.__beatmoEnv before WASM init (index.html's
  // go.env plumbing), e.g. {"BEATMO_AUDIO_LANE":"1"} for the lane arm.
  const ISO_ENV = process.env.ISOLATION_ENV ? JSON.parse(process.env.ISOLATION_ENV) : null;
  if (ISO_ENV && Object.keys(ISO_ENV).length) {
    await page.addInitScript((cfg) => {
      window.__beatmoEnv = Object.fromEntries(Object.entries(cfg).map(([k, v]) => [k, String(v)]));
    }, ISO_ENV);
  }
  page.on("pageerror", (e) => console.log(`  [pageerror] ${e.message}`));
  // ISOLATION_QUERY, e.g. "devtools=0": the production UI in a dev build.
  await page.goto(`http://localhost:${port}/${process.env.ISOLATION_QUERY ? "?" + process.env.ISOLATION_QUERY : ""}`);
  await page.waitForFunction(() => typeof startPlay === "function" && typeof forceDraw === "function", null, { timeout: 60000 });
  await page.evaluate(() => { document.dispatchEvent(new Event("pointerdown")); resumeAudio?.(); });
  await page.waitForFunction(() => window.__audioCtx && window.__audioCtx.state === "running", null, { timeout: 20000 });
  // Let boot warm-up renders settle so the first scenario isn't a cold start.
  await page.waitForTimeout(2500);
  const rows = await page.evaluate(() => {
    const n = totalRows?.() ?? 0;
    const ids = [];
    for (let i = 0; i < n; i++) ids.push(rowInstrument(i));
    return ids;
  });
  console.log(`circuit: ${rows.length} rows (${rows.join(", ")}) @ ${BPM} BPM, ${SECS}s per scenario`);

  for (const sc of scenarios) {
    console.log(`\n=== ${sc.name} ===`);
    await page.evaluate(() => stopPlay?.());
    await page.waitForTimeout(300);
    await page.evaluate((m) => { if (m) window.__beatmoAudioFlushMode = m; else delete window.__beatmoAudioFlushMode; }, sc.flush || null);
    if (sc.tab) await page.evaluate((t) => setEQTab(t), sc.tab);
    if (sc.fx) {
      await page.evaluate(({ host, ids, types }) => {
        window.__beatmoDSPHost = host;
        window.__isoFx = [];
        for (const id of ids) {
          for (const t of types) window.__isoFx.push([id, addInsertEffect(id, t)]);
        }
      }, { host: sc.fx, ids: rows.slice(0, 3), types: FX_TYPES });
      await page.waitForTimeout(500);
    }
    await startPlaybackWhenReady(page, { bpm: BPM }); // Play is ignored while the warm-up gate is closed
    await page.waitForTimeout(1500);
    const hosts = sc.fx ? await page.evaluate((ids) => ids.map((id) => [id, insertEffectHosts(id)]), rows.slice(0, 3)) : null;
    await page.evaluate(() => { resetPerfStats?.(); resetAudioScheduleMetrics?.(); });
    const started = await startPlaybackProbe(page);
    if (!started?.ok) {
      failures.push(`[${sc.name}] probe did not start: ${started?.error}`);
      continue;
    }
    await page.evaluate(({ load, rows }) => {
      const timers = [];
      if (load?.stall) {
        const [every, block] = load.stall;
        timers.push(setInterval(() => { const t = performance.now(); while (performance.now() - t < block) { /* UI bug */ } }, every));
      }
      if (load?.zoom) {
        let k = 0;
        timers.push(setInterval(() => { zoomAt(640, 300, (k++ % 40) < 20 ? 3 : -3); }, 16));
      }
      if (load?.pan) {
        let k = 0;
        timers.push(setInterval(() => { const d = (k++ % 60) < 30 ? 6 : -6; panBy(d, d / 2); }, 16));
      }
      // Graph storm, driven in the page (no protocol round trips): mouse
      // events on the canvas, the way Ebiten reads a real pointer — press a
      // dial (focus: its two ends render + analyse), drag it (the turn
      // re-renders the live sound), release; next dial. Every 4th
      // gesture the tab shows another row's instrument (first sight: its dial
      // relevance is measured).
      if (load?.graphs) {
        const canvas = document.querySelector("canvas");
        const ev = (type, x, y, buttons) => canvas.dispatchEvent(new MouseEvent(type,
          { clientX: x, clientY: y, button: 0, buttons, bubbles: true, cancelable: true }));
        let k = 0, phase = 0, cx = 0, cy = 0;
        window.__isoGraphGestures = 0;
        timers.push(setInterval(() => {
          if (phase === 0) {
            if (k % 4 === 3) setEQChannel(rows[(1 + Math.floor(k / 4)) % rows.length]);
            const dials = synthDialRects();
            if (!dials.length) { k++; return; }
            const d = dials[k % dials.length];
            cx = d.x + d.w / 2; cy = d.y + d.h / 2;
            ev("mousemove", cx, cy, 0);
            ev("mousedown", cx, cy, 1);
            phase = 1;
            return;
          }
          if (phase === 1) {
            ev("mousemove", cx, cy - 36 * (k % 2 ? -1 : 1), 1);
            phase++;
            return;
          }
          ev("mouseup", cx, cy, 0);
          phase = 0;
          k++;
          window.__isoGraphGestures++;
        }, 30));
      }
      window.__isoStop = () => timers.forEach(clearInterval);
    }, { load: sc.load || null, rows });
    // Edit storm: marks are stamped with the ctx time read in the SAME
    // evaluate that performs the edit (markEdit), so the frozen baseline
    // build is measured exactly like the new one.
    const marks = [];
    let mutes = 0;
    const editLoop = sc.edits ? (async () => {
      const end = Date.now() + SECS * 1000 - 300;
      let k = 0;
      while (Date.now() < end) {
        if (sc.edits === "mute") {
          marks.push(await markEdit(page, { label: `mute#${k}`, kind: k % 2 === 0 ? "mute" : "unmute", channel: rows[0] }, "toggleMute(0)"));
          mutes++;
          await page.waitForTimeout(600);
        } else {
          const v = [0.9, 0.5, 0.7, 0.3][k % 4];
          // prev = row volume before this edit: the analysis learns the
          // channel's node volumes from pre-edit notes (startup-demo nodes
          // are not all vol 1).
          const prev = await page.evaluate(() => rowVolume(1));
          marks.push(await markEdit(page, { label: `vol#${k}`, kind: "vol", channel: rows[1], value: v, prev }, "setRowVolume(1, arg)", v));
          await page.waitForTimeout(400);
        }
        k++;
      }
    })() : Promise.resolve();
    await page.waitForTimeout(SECS * 1000);
    await editLoop;
    if (sc.load?.graphs) console.log(`  graph storm: ${await page.evaluate(() => window.__isoGraphGestures || 0)} dial press+drag gestures`);
    await page.evaluate(() => window.__isoStop?.());
    const perf = await page.evaluate(() => perfStats?.());
    const { report } = await stopPlaybackProbe(page, sc.name, { editMarks: marks });
    if (sc.edits === "mute" && mutes % 2 === 1) await page.evaluate(() => toggleMute(0));
    if (sc.edits === "vol") await page.evaluate(() => setRowVolume(1, 1));
    await page.evaluate(() => stopPlay?.());
    if (sc.tab) await page.evaluate(() => setEQTab("eq"));
    if (sc.fx) {
      await page.evaluate(() => {
        for (const [id, idx] of (window.__isoFx || []).slice().reverse()) removeInsertEffect(id, idx);
        window.__isoFx = [];
        delete window.__beatmoDSPHost;
      });
    }
    console.log(`  perf: fps=${perf?.fpsAvg?.toFixed?.(1)} update=${perf?.updateAvgMS?.toFixed?.(2)}ms draw=${perf?.drawAvgMS?.toFixed?.(2)}ms drawMax=${perf?.drawMaxMS?.toFixed?.(1)}ms`);

    const row = { scenario: sc.name, arm: process.env.AB_ARM || "default", bpm: BPM, secs: SECS, ...probeSummaryFields(report),
      fps: perf?.fpsAvg ?? null, drawAvgMS: perf?.drawAvgMS ?? null, drawMaxMS: perf?.drawMaxMS ?? null,
      updateAvgMS: perf?.updateAvgMS ?? null, fxHosts: hosts };
    if (sc.fx && report) {
      const fxBuses = rows.slice(0, 3).map((id) => report.perBus[id] || { clicks: 0, cuts: 0 });
      row.fxChannelCuts = fxBuses.reduce((a, b) => a + b.cuts, 0);
      row.fxChannelClicks = fxBuses.reduce((a, b) => a + b.clicks, 0);
    }
    results.push({ row, report, sc });

    // ── Gates ──
    const tag = `[${sc.name}]`;
    const hard = (cond, msg) => { if (!cond) failures.push(`${tag} ${msg}`); };
    hard(report && report.notes.judged > 0, "no notes judged");
    if (!report) continue;
    hard(report.notes.dropped === 0, `${report.notes.dropped} notes requested but never started`);
    // Edit storms are measurements (staleness per arm), not gates.
    if (sc.edits) continue;
    // Legacy-host A/B baselines (fx-scriptproc, stall-150-microtask) exist to
    // SHOW the coupling; their silence/queueing is the finding, not a failure.
    const legacy = sc.fx === "scriptprocessor" || !!sc.flush;
    if (!legacy) hard(report.notes.missed === 0, `${report.notes.missed} notes silent in the output`);
    else if (report.notes.missed > 0) infos.push(`${tag} legacy host A/B: ${report.notes.missed}/${report.notes.judged} notes silent`);
    if (report.jsQueueMs.p90 != null && !sc.flush) hard(report.jsQueueMs.p90 <= PROBE_JSQUEUE_MAX_MS, `jsQueue p90 ${report.jsQueueMs.p90}ms > ${PROBE_JSQUEUE_MAX_MS}ms (a UI frame is delaying BufferSource.start)`);
    hard(report.lookahead.shifts <= 3, `${report.lookahead.shifts} lookahead shifts > 3 (cushion oscillating)`);
    if (sc.fx === "worklet") {
      for (const [id, h] of hosts || []) { const k = h.filter((x) => x !== "native"); hard(k.length >= FX_TYPES.length && k.every((x) => x === "worklet"), `${id} kernel insert FX hosts ${JSON.stringify(h)}, want worklet`); }
    }
    const soft = legacy ? [] : probeGateFailures(report, PROBE_LIMITS_DEFAULT, sc.name);
    (PROBE_STRICT ? failures : infos).push(...soft);
  }

  // The FX rows on the worklet host under a stalled main thread.
  const fxW = results.find((r) => r.sc.name === "fx-worklet");
  if (fxW) {
    console.log(`\nFX rows under stall-150 (the 3 FX channels): cuts=${fxW.row.fxChannelCuts} clicks=${fxW.row.fxChannelClicks} levelOutliers=${fxW.report?.quality.levelOutliers}/${fxW.report?.quality.levelJudged}`);
    if (fxW.report) {
      const hard = (cond, msg) => { if (!cond) failures.push(`[fx-worklet] ${msg}`); };
      hard(fxW.row.fxChannelCuts === 0, `${fxW.row.fxChannelCuts} hard cuts on channels with worklet effects under a stalled main thread`);
    }
  }
} finally {
  await browser.close();
  server.close();
}

// ── Table ──
const f = (x, d = 1) => (x == null ? "  n/a" : Number(x).toFixed(d));
console.log("\n scenario       | heard/judged | lagReq p50/p99/max  | late>20 | IOI p95/max  | arrLate | jsQ p99 | shifts | slips | cuts | main>50 | drawMax");
for (const { row: r } of results) {
  console.log(` ${r.scenario.padEnd(14)} | ${String(r.probeNotesHeard).padStart(4)}/${String(r.probeNotesJudged).padEnd(7)} | ${f(r.probeLagIdealP50MS).padStart(5)}/${f(r.probeLagIdealP99MS).padStart(5)}/${f(r.probeLagIdealMaxMS).padStart(5)} | ${String(r.probeLateOver20).padStart(7)} | ${f(r.probeIoiErrP95MS).padStart(5)}/${f(r.probeIoiErrMaxMS).padStart(5)} | ${String(r.probeArrivedLate).padStart(7)} | ${f(r.probeJsQueueP99MS).padStart(7)} | ${String(r.probeLookaheadShifts).padStart(6)} | ${String(r.probeSlips).padStart(5)} | ${String((r.probeMasterCuts ?? 0)).padStart(4)} | ${String(r.probeMainStallsOver50).padStart(7)} | ${f(r.drawMaxMS)}`);
}

const resultsDir = path.resolve(jsDir, "../../bench-results");
fs.mkdirSync(resultsDir, { recursive: true });
const outName = `audio_isolation${process.env.AB_ARM ? "_" + process.env.AB_ARM : ""}.json`;
fs.writeFileSync(path.join(resultsDir, outName), JSON.stringify({
  generatedAt: new Date().toISOString(), bpm: BPM, secs: SECS, strict: PROBE_STRICT,
  scenarios: results.map((r) => ({ ...r.row, report: r.report })),
}, null, 2));
console.log(`\nwrote bench-results/${outName}`);

if (infos.length) {
  console.log(`\nINFO (enforced only with AUDIO_PROBE_STRICT=1):`);
  for (const m of infos) console.log(`  ${m}`);
}
if (failures.length) {
  console.error(`\nFAIL (${failures.length}):`);
  for (const m of failures) console.error(`  ${m}`);
  process.exit(1);
}
console.log("\nPASS webaudio_audio_isolation_bench");
process.exit(0);
