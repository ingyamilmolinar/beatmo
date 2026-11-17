// COVERED-BY-GO: none — WebAudio emission timing (BufferSource.start(when)
// vs the true AudioContext clock) is a JS-only boundary; Go has no visibility
// into when WebAudio actually starts a scheduled source. This test verifies
// the audio.js timingTraces ring (window.getTimingTraces/resetTimingTraces)
// records real sequencer playback. See CLAUDE.md "Parity Timing-Sync" plan
// Task 4.1/4.2 — Task 4.2 pulls these traces into the Go-WASM Stage-D drift
// checker.
import { chromium } from "playwright";
import { spawnSync } from "child_process";
import http from "http";
import fs from "fs";
import path from "path";
import { fileURLToPath } from "url";
import { resolveGoBinary, shouldSkipWasmBuild, flushCoverage, isCoverageEnabled } from "./browser_test_helpers.js";
import { startPlaybackWhenReady } from "./audio_probe_bench_helpers.js";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const jsDir = __dirname;
const goDir = path.resolve(jsDir, "../go");
const GO = resolveGoBinary();

// Build the full main.wasm (real sequencer + audio bridge, same as
// webaudio_timing_stress.browser.test.js) — a minimal playtest harness has no
// sequencer to drive BatchParam.Row/Abs through.
if (!shouldSkipWasmBuild("main.wasm")) {
  const build = spawnSync(
    GO,
    ["build", "-ldflags", "-X main.defaultLog=INFO", "-o", path.join(jsDir, "main.wasm"), "./cmd/"],
    { cwd: goDir, env: { ...process.env, GOOS: "js", GOARCH: "wasm" }, stdio: "inherit" },
  );
  if (build.status !== 0) throw new Error("go build main wasm failed");
}

const chromiumPath = path.join(jsDir, "node_modules", ".cache", "ms-playwright", "chromium");
if (!fs.existsSync(chromiumPath)) {
  spawnSync("npx", ["playwright", "install", "chromium"], { cwd: jsDir, stdio: "inherit" });
}

const server = http.createServer((req, res) => {
  const file = req.url === "/" ? "/index.html" : req.url;
  const filePath = path.join(jsDir, file.replace(/^\//, ""));
  fs.readFile(filePath, (err, data) => {
    if (err) {
      res.writeHead(404);
      res.end();
      return;
    }
    let ct = "text/plain";
    if (filePath.endsWith(".html")) ct = "text/html";
    else if (filePath.endsWith(".js")) ct = "application/javascript";
    else if (filePath.endsWith(".wasm")) ct = "application/wasm";
    res.writeHead(200, { "Content-Type": ct });
    res.end(data);
  });
});
await new Promise((r) => server.listen(0, r));
const port = server.address().port;

const browser = await chromium.launch({ args: ["--autoplay-policy=no-user-gesture-required"] });
const page = await browser.newPage();
const pageErrors = [];
page.on("pageerror", (e) => pageErrors.push(String(e.message)));
page.on("console", (m) => {
  if (m.type() === "error") pageErrors.push("console.error: " + m.text());
});

// Gate readiness on REAL exports that only register after Game.New() finishes
// installing the full JS bridge (see webaudio_timing_stress.browser.test.js
// header comment for why startPlay alone races ahead of full init).
await page.goto(`http://localhost:${port}/`);
await page.waitForFunction(
  () =>
    typeof startPlay === "function" &&
    typeof stopPlay === "function" &&
    typeof getTimingTraces === "function" &&
    typeof resetTimingTraces === "function" &&
    typeof buildPerfRect === "function",
);

await page.evaluate(() => {
  document.dispatchEvent(new Event("pointerdown"));
  resumeAudio?.();
  resetTimingTraces();
  buildPerfRect(2, 1);
  forceDraw?.(); forceDraw?.();
  // Observe every trace the Go Stage-D drain consumes. Sequencer batches now
  // start their voices SYNCHRONOUSLY inside the Go→JS call (audio.js
  // enqueueAudioEvents(…, true)), and the drain runs right after PlayBatch in
  // the same audioLoop iteration — so the ring is refilled and emptied before
  // control ever returns to the page, and a poll between frames sees it empty.
  // Go resolves window.resetTimingTraces by name on every drain, so wrapping
  // it captures exactly what Stage D receives.
  window.__ttCaptured = [];
  const origReset = window.resetTimingTraces;
  window.resetTimingTraces = () => {
    const snap = window.getTimingTraces();
    if (snap.length) window.__ttCaptured.push(...snap);
    return origReset();
  };
});
// Press Play only once the readiness gate opens — a bare startPlay() right
// after buildPerfRect is ignored while the new rows' voices warm up.
await startPlaybackWhenReady(page, { bpm: 240 });

// Task 4.2 wires drainEmissionReports() into the js build: on every audio
// batch dispatch (game_audio_loop.go), Go now pulls window.getTimingTraces()
// AND resets the ring (window.resetTimingTraces()) to feed Stage-D drift.
// That means the ring no longer accumulates monotonically across the whole
// playback window — it is drained near-continuously from the Go side. So
// instead of waiting once and reading a final snapshot, poll repeatedly
// during playback and keep the largest non-empty snapshot seen. A non-empty
// snapshot at any poll instant proves both: (a) audio.js is still recording
// real BufferSource.start(when) calls (Task 4.1), and (b) polling itself
// (mirroring what the Go drain does every batch) never throws.
let best = [];
let pollThrew = false;
for (let i = 0; i < 30; i++) {
  await page.waitForTimeout(100);
  let snap;
  try {
    snap = await page.evaluate(() => window.getTimingTraces());
  } catch (e) {
    pollThrew = true;
    break;
  }
  if (Array.isArray(snap) && snap.length > best.length) best = snap;
}
const drained = await page.evaluate(() => window.__ttCaptured.slice());
if (drained.length > best.length) best = drained;

await page.evaluate(() => { stopPlay?.(); });

if (pollThrew) {
  throw new Error("getTimingTraces() threw while polling during playback");
}
if (best.length === 0) {
  throw new Error("no timing traces observed during playback (Go's Stage-D drain may be over-eager, or Task 4.1 recording broke)");
}
for (const tr of best) {
  if (typeof tr.startWhen !== "number" || !Number.isFinite(tr.startWhen)) {
    throw new Error("trace missing numeric startWhen: " + JSON.stringify(tr));
  }
  if (typeof tr.ctxAtStart !== "number" || !Number.isFinite(tr.ctxAtStart)) {
    throw new Error("trace missing numeric ctxAtStart: " + JSON.stringify(tr));
  }
  if (typeof tr.row !== "number" || tr.row < 0) {
    throw new Error("trace has non-sequencer row (should never be recorded): " + JSON.stringify(tr));
  }
}
console.log("PASS timing_traces:", best.length, "traces (best snapshot while Go Stage-D drain is concurrently active)");

// ── Task 4.2 extension: reset + replay cycle ───────────────────────────
// Browser Stage-D is log-only by default (RuntimeProf().Timing.FatalDefault
// == false for the browser profile) — it must never panic even while the
// Go drain is pulling + resetting this ring on every dispatch. Prove the
// drain+reset cycle doesn't permanently wedge recording: explicitly reset,
// confirm the ring is empty, replay, and confirm traces resume flowing.
//
// Reset and read must share ONE evaluate — traces keep landing after
// stopPlay() resolves, so two round-trips race. stopPlay() is not
// synchronous: it sets the one-frame `stopPressed` pulse
// (js_exports_playback_perf.go) that the next Game.Update() consumes, and
// audio.js's flushAudioQueue keeps draining already-queued events past that,
// each BufferSource.start(when) pushing a trace. In the gap between two
// round-trips that tail lands and the read sees a non-empty ring (~21 traces
// on a box running 4 browser jobs). One evaluate body runs to completion
// without yielding, so nothing can push mid-check — which makes the assertion
// stronger, not weaker: it holds even while recording is concurrently active.
const { before, after } = await page.evaluate(() => {
  const before = window.getTimingTraces().length;
  window.resetTimingTraces();
  return { before, after: window.getTimingTraces().length };
});
if (after !== 0) {
  throw new Error("resetTimingTraces() did not clear the ring: len=" + after + " (was " + before + " before reset)");
}

const capturedBeforeReplay = await page.evaluate(() => window.__ttCaptured.length);
await startPlaybackWhenReady(page, { bpm: 240 });

let bestAfterReplay = 0;
for (let i = 0; i < 20; i++) {
  await page.waitForTimeout(100);
  const len = await page.evaluate((b) => window.getTimingTraces().length + window.__ttCaptured.length - b, capturedBeforeReplay);
  if (len > bestAfterReplay) bestAfterReplay = len;
}

await page.evaluate(() => { stopPlay?.(); });

if (isCoverageEnabled()) await flushCoverage(page, new URL("../../coverage/browser-raw", import.meta.url).pathname, "timing_traces");
await browser.close();
server.close();

if (bestAfterReplay === 0) {
  throw new Error("no traces observed after resetTimingTraces() + replay — drain/reset cycle wedged recording");
}
const fatalErrors = pageErrors.filter((e) => /panic|already exited/i.test(e));
if (fatalErrors.length > 0) {
  throw new Error("WASM panic/crash detected during Stage-D drain: " + fatalErrors.join(" | "));
}

console.log("PASS timing_traces armed drain: reset cleared", before, "traces; reset+replay observed", bestAfterReplay, "traces; drain ran without throwing");
