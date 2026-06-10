import { chromium } from "playwright";
import { spawnSync } from "child_process";
import http from "http";
import fs from "fs";
import path from "path";
import { fileURLToPath } from "url";
import { assertSimpleDrawMode, resolveGoBinary, shouldSkipWasmBuild, flushCoverage, isCoverageEnabled } from "./browser_test_helpers.js";

// Per-instrument volume-bus pool: getBus(id, vol) quantizes vol to one of
// VOL_Q+1 buckets (q = round(vol*VOL_Q), q in 0..VOL_Q) and reuses one GainNode
// per (id, q). maybeEvictBuses caps the live bus count per instrument at
// MAX_BUSES_PER_INST, evicting least-recently-used buckets. This test pins the
// EXACT bus count for a known set of distinct quantized volumes, proves the
// eviction path caps growth, and proves a bus actually carries audio.
//
// Source constants (src/js/audio.js): VOL_Q = 16, MAX_BUSES_PER_INST = 16.
// The test reads these off the page (audio.js does not export them, so they are
// recomputed by probing getBus' quantizer through observable bus counts) and
// asserts against the values exactly — no loose [8,32] window.

if (!shouldSkipWasmBuild("play_ui.wasm")) {
const build = spawnSync(
  resolveGoBinary(),
  ["build", "-o", path.join(path.dirname(fileURLToPath(import.meta.url)), "play_ui.wasm"), "./internal/ui/playtest"],
  { cwd: path.resolve(path.dirname(fileURLToPath(import.meta.url)), "../go"), env: { ...process.env, GOOS: "js", GOARCH: "wasm" }, stdio: "inherit" }
);
if (build.status !== 0) throw new Error("go build play_ui failed");
}

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const jsDir = __dirname;

const server = http.createServer((req, res) => { const file = req.url === "/" ? "/ui.html" : req.url;
  if (req.url === "/" || req.url === "/ui.html") { const html = `<!doctype html><body>
<script type="module" src="audio.js"></script>
<script src="wasm_exec.js"></script>
<script>
  const go = new Go();
  WebAssembly.instantiateStreaming(fetch('play_ui.wasm'), go.importObject)
    .then(r => go.run(r.instance))
    .catch(err => console.error(err));
  window.__ready = new Promise(r => { const iv = setInterval(() => { if (typeof startPlay === 'function') { clearInterval(iv); r(true); } }, 10);
  });
</script>`;
    res.writeHead(200, { "Content-Type": "text/html" });
    res.end(html);
    return;
  }
  const filePath = path.join(jsDir, file.replace(/^\//, ""));
  fs.readFile(filePath, (err, data) => { if (err) { res.writeHead(404); res.end(); return; }
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

let browser;
let exitCode = 0;
try { browser = await chromium.launch({ args: ["--autoplay-policy=no-user-gesture-required"] });
  const page = await browser.newPage();
  page.on("console", (m) => { if (process.env.TEST_LOG) console.log("[page]", m.text()); });
  await page.goto(`http://localhost:${port}/`);
  await page.waitForFunction(() => window.__ready);
  await assertSimpleDrawMode(page, true, "wav bus");
  await page.evaluate(() => { if (typeof resumeAudio === 'function') resumeAudio(); });

  // Unlock the AudioContext (gesture listener) and wait until it is running so
  // getBus() / the output-capture ScriptProcessor have a live context.
  await page.evaluate(() => document.dispatchEvent(new Event('mousedown')));
  await page.evaluate(() => { try { if (window.audioReady) return window.audioReady; } catch (_) {} });
  await page.waitForFunction(
    () => window.__audioCtx && window.__audioCtx.state === "running",
    {},
    { timeout: 10000 }
  ).catch(() => {});
  await page.waitForTimeout(200);

  // Sanity: the metrics + render-path play helpers we depend on exist.
  const exportsOk = await page.evaluate(() => (
    typeof window.playSound === "function" &&
    typeof window.__audioBusCount === "function" &&
    typeof window.__audioBusCountById === "function" &&
    typeof window.audioNow === "function"
  ));
  if (!exportsOk) throw new Error("required audio exports missing");

  // ---- Probe VOL_Q empirically (do NOT hardcode 16). ----
  // q = round(vol*VOL_Q) over vol in (0,1] yields exactly VOL_Q distinct
  // non-zero buckets (q = 1..VOL_Q). We sweep a fine grid of volumes on a fresh
  // instrument id and count distinct live buses; that count == VOL_Q provided
  // VOL_Q <= MAX_BUSES_PER_INST (true in source: both are 16). Use a high-res
  // sweep so every bucket boundary is crossed.
  const VOL_Q = await page.evaluate(async () => {
    const id = 'snare'; // render instrument; bus pool keyed per id
    const N = 256;
    for (let i = 1; i <= N; i++) {
      const v = i / N; // (0, 1]
      await window.playSound(id, v, window.audioNow());
    }
    return window.__audioBusCountById(id); // == number of distinct non-zero q buckets
  });
  console.log('wav_bus: probed VOL_Q (distinct non-zero buckets) =', VOL_Q);
  if (!Number.isInteger(VOL_Q) || VOL_Q < 2) {
    throw new Error('failed to probe VOL_Q: got ' + VOL_Q);
  }

  // ---- (1) EXACT bus count for VOL_Q distinct quantized volumes. ----
  // Fresh id 'kick'. Feed exactly the VOL_Q distinct non-zero buckets
  // (v = q/VOL_Q for q = 1..VOL_Q => round(v*VOL_Q) = q). No eviction expected:
  // VOL_Q buckets <= MAX_BUSES_PER_INST. Assert the count is EXACTLY VOL_Q.
  const exactId = 'kick';
  const exactCount = await page.evaluate(async ({ exactId, VOL_Q }) => {
    for (let q = 1; q <= VOL_Q; q++) {
      const v = q / VOL_Q;
      await window.playSound(exactId, v, window.audioNow());
    }
    return window.__audioBusCountById(exactId);
  }, { exactId, VOL_Q });
  console.log('wav_bus: exact bus count for', VOL_Q, 'distinct volumes =', exactCount);
  if (exactCount !== VOL_Q) {
    throw new Error(`exact bus count mismatch: expected ${VOL_Q}, got ${exactCount}`);
  }

  // ---- (2) EXERCISE EVICTION: feed MORE distinct buckets than the cap. ----
  // The bucket space is q in 0..VOL_Q => VOL_Q+1 distinct buckets, which is
  // strictly greater than MAX_BUSES_PER_INST (= VOL_Q in source). Feeding all
  // VOL_Q+1 distinct buckets forces maybeEvictBuses to run and cap the count.
  // Use a fresh id so the count is isolated. Assert the result is CAPPED (does
  // not grow to the number of distinct buckets fed).
  const evictId = 'snare-1';
  // 'snare-1' is a modular-variant render instrument that is NOT in the
  // warmupAudio pre-render set (unlike 'kick'/'snare' used above). Its first
  // play therefore hits the async render-defer path in processAudioEvent
  // (ensureRenderReady().then(re-enqueue)), which returns BEFORE getBus runs —
  // getBus (and thus bus creation) only happens once the WASM render resolves.
  // If we fed volumes and read the count immediately, the synchronous read would
  // race that async render and return a partial count (observed as 4 under the
  // 4-job parallel test load, 16 when run standalone — a flaky failure). Warm
  // the render first with a condition-based wait (no arbitrary sleep) so the feed
  // loop below takes the synchronous cached-render getBus path and the post-
  // eviction count is deterministic.
  await page.evaluate(async (id) => { await window.playSound(id, 1.0, window.audioNow()); }, evictId);
  await page.waitForFunction((id) => window.__audioBusCountById(id) > 0, evictId, { timeout: 10000 });
  const distinctFed = VOL_Q + 1; // q = 0..VOL_Q
  const cappedCount = await page.evaluate(async ({ evictId, VOL_Q }) => {
    // q = 0 (silent bucket) through q = VOL_Q, all distinct.
    for (let q = 0; q <= VOL_Q; q++) {
      const v = q / VOL_Q;
      await window.playSound(evictId, v, window.audioNow());
    }
    return window.__audioBusCountById(evictId);
  }, { evictId, VOL_Q });
  console.log('wav_bus: fed', distinctFed, 'distinct buckets, capped bus count =', cappedCount);
  if (cappedCount >= distinctFed) {
    throw new Error(`eviction did not run: count ${cappedCount} reached/exceeded distinct fed ${distinctFed}`);
  }
  // MAX_BUSES_PER_INST is VOL_Q in source; the cap must equal VOL_Q exactly.
  const MAX_BUSES_PER_INST = VOL_Q;
  if (cappedCount !== MAX_BUSES_PER_INST) {
    throw new Error(`eviction cap mismatch: expected ${MAX_BUSES_PER_INST}, got ${cappedCount}`);
  }

  // ---- (3) OUTPUT CHECK: a bus actually carries audible signal. ----
  // Capture master output across a play that routes through a freshly-created
  // bus and assert a non-zero peak. Without this, "a bus exists" would not imply
  // "the bus carries audio".
  const peak = await page.evaluate(async () => {
    if (typeof window.startOutputCapture !== "function" ||
        typeof window.getOutputCapture !== "function") return -1;
    window.startOutputCapture();
    // Full-volume play (q = VOL_Q bucket) through a bus.
    await window.playSound('snare', 1.0, window.audioNow());
    // Poll until the capture buffer shows signal (ScriptProcessor warmup), up
    // to ~2s, mirroring scenarios/webaudio_output_capture.js.
    let p = 0;
    for (let attempt = 0; attempt < 20; attempt++) {
      await new Promise((r) => setTimeout(r, 100));
      const snap = window.getOutputCapture();
      for (let i = 0; i < snap.length; i++) {
        const a = Math.abs(snap[i]);
        if (a > p) p = a;
      }
      if (p > 0.01) break;
    }
    return p;
  });
  console.log('wav_bus: captured output peak =', peak);
  if (!(peak > 0.01)) {
    throw new Error(`bus carried no audible signal: peak ${peak}`);
  }

  console.log('wav_bus: PASS', JSON.stringify({ VOL_Q, exactCount, distinctFed, cappedCount, peak }));
  if (isCoverageEnabled()) await flushCoverage(page, new URL("../../coverage/browser-raw", import.meta.url).pathname, "wav_bus");
} catch (err) {
  console.error("FAIL:", err && err.message ? err.message : err);
  exitCode = 1;
} finally {
  if (browser) await browser.close();
  server.close();
}
process.exit(exitCode);
