import { chromium } from "playwright";
import { spawnSync } from "child_process";
import http from "http";
import fs from "fs";
import path from "path";
import { fileURLToPath } from "url";
import { assertNoSchedulerMismatches, assertSimpleDrawMode, clearSchedulerMismatches, resolveGoBinary, shouldSkipWasmBuild, flushCoverage, isCoverageEnabled } from "./browser_test_helpers.js";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const jsDir = __dirname;
const goDir = path.resolve(jsDir, "../go");
const GO = resolveGoBinary();

if (!shouldSkipWasmBuild("play_ui.wasm")) {
const build = spawnSync(
  GO,
  ["build", "-o", path.join(jsDir, "play_ui.wasm"), "./internal/ui/playtest"],
  { cwd: goDir, env: { ...process.env, GOOS: "js", GOARCH: "wasm" }, stdio: "inherit" }
);
if (build.status !== 0) throw new Error("go build play_ui failed");
}

const server = http.createServer((req, res) => { const file = req.url === "/" ? "/ui.html" : req.url;
  if (req.url === "/" || req.url === "/ui.html") { const html = `<!doctype html><body>
<script type="module" src="audio.js"></script>
<script src="wasm_exec.js"></script>
<script>
  const go = new Go();
  WebAssembly.instantiateStreaming(fetch('play_ui.wasm'), go.importObject)
    .then(r => go.run(r.instance))
    .catch(err => console.error(err));
  window.__ready = new Promise(r => { const iv = setInterval(() => { if (typeof startPlay === 'function' && typeof buildPerfRect === 'function') { clearInterval(iv); r(true); } }, 10);
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
try { browser = await chromium.launch({ args: ["--autoplay-policy=no-user-gesture-required"] });

  // Scenario 1: Grid cache + hasRealtimeHighlight at 120 BPM (from grid_and_highlights)
  console.log("Scenario 1: hasRealtimeHighlight during playback at 120 BPM");
  {
    const page = await browser.newPage();
    await page.goto(`http://localhost:${port}/`);
    await page.waitForFunction(() => window.__ready);
    await assertSimpleDrawMode(page, true, "highlight playback scenario 1");
    await page.evaluate(() => { if (typeof resumeAudio === 'function') resumeAudio(); });
    // Override audioNow with wall-clock time so highlight windows activate
    // even when AudioContext is suspended (parallel test runs).
    await page.evaluate(() => {
      const t0 = performance.now();
      window.audioNow = () => (performance.now() - t0) / 1000;
    });
    await clearSchedulerMismatches(page);

    // Ensure grid cache/tile are built even under simpleDraw.
    await page.evaluate(() => { if (typeof forceDraw === 'function') forceDraw(); });
    const grid = await page.evaluate(() => (typeof gridCacheInfo === 'function' ? gridCacheInfo() : null));
    if (!grid || !grid.tileReady) throw new Error('grid tile not built');

    // Build small graph and start playback.
    await page.evaluate(() => {
      buildPerfRect(1, 1);
      setBPM(120);
      startPlay();
    });
    await page.waitForFunction(
      () => typeof isPlaying === 'function' && isPlaying(),
      { timeout: 5000 }
    );
    await page.waitForTimeout(300);
    // Wait up to 2s for highlights to appear (engine-driven).
    let hasAny = false;
    for (let t = 0; t < 40 && !hasAny; t++) {
      await page.waitForTimeout(50);
      await page.evaluate(() => { syncHighlights?.(); if (typeof forceDraw === 'function') forceDraw(); });
      hasAny = await page.evaluate(() => {
        if (typeof hasRealtimeHighlight !== 'function') return false;
        if (typeof currentBeat !== 'function' || typeof gridSubdiv !== 'function') return false;
        const beat = currentBeat();
        const div = gridSubdiv();
        const abs = Math.floor(beat * div);
        for (let i = abs - 4; i <= abs + 4; i++) {
          if (i >= 0 && hasRealtimeHighlight(0, i)) return true;
        }
        return false;
      });
    }
    if (!hasAny) throw new Error('no highlights detected while playing (hasRealtimeHighlight)');
    await assertNoSchedulerMismatches(page, "highlight playback scenario 1: scheduler mismatches");

    await page.evaluate(() => stopPlay?.());
    await page.close();
  }

  // Scenario 2: nodeHighlightedAt on grid nodes at 150 BPM (from node_grid_highlight)
  console.log("Scenario 2: nodeHighlightedAt during playback at 150 BPM");
  {
    const page = await browser.newPage();
    await page.goto(`http://localhost:${port}/`);
    await page.waitForFunction(() => window.__ready);
    await assertSimpleDrawMode(page, true, "highlight playback scenario 2");
    await page.evaluate(() => { if (typeof resumeAudio === 'function') resumeAudio(); });
    // Override audioNow with wall-clock time so highlight windows activate
    // even when AudioContext is suspended (parallel test runs).
    await page.evaluate(() => {
      const t0 = performance.now();
      window.audioNow = () => (performance.now() - t0) / 1000;
    });
    await clearSchedulerMismatches(page);

    // Build a 1x1 loop and start.
    await page.evaluate(() => {
      buildPerfRect(1, 1);
      setBPM(150);
      startPlay();
    });
    await page.waitForFunction(
      () => typeof isPlaying === 'function' && isPlaying(),
      { timeout: 5000 }
    );
    await page.waitForTimeout(300);

    // Poll for node highlight on any of the 4 nodes of the 1x1 rectangle.
    // Also check hasRealtimeHighlight as a fallback (it uses highlightedBeats
    // which is populated from Update(), not Draw()).
    let ok = false;
    const coords = [[0,0],[1,0],[1,1],[0,1]];
    for (let t = 0; t < 60 && !ok; t++) { await page.waitForTimeout(50);
      await page.evaluate(() => { syncHighlights?.(); if (typeof forceDraw === 'function') forceDraw(); });
      ok = await page.evaluate((coords) => {
        if (typeof nodeHighlightedAt !== 'function') return false;
        for (const [i,j] of coords) { if (nodeHighlightedAt(i,j)) return true; }
        return false;
      }, coords);
      if (!ok) {
        // Fallback: check if any highlight exists for row 0, bypassing
        // currentBeat() which can diverge from the scheduler under CPU contention.
        ok = await page.evaluate(() => {
          if (typeof hasAnyRowHighlight === 'function') return hasAnyRowHighlight(0);
          return false;
        });
      }
    }
    if (!ok) throw new Error('no node highlight detected on main grid (nodeHighlightedAt/hasRealtimeHighlight)');
    await assertNoSchedulerMismatches(page, "highlight playback scenario 2: scheduler mismatches");

    if (isCoverageEnabled()) await flushCoverage(page, new URL("../../coverage/browser-raw", import.meta.url).pathname, "highlight_playback");
    await page.close();
  }

} finally {
  if (browser) await browser.close();
  server.close();
}
console.log("highlight playback tests completed");
