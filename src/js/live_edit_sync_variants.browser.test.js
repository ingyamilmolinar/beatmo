import { chromium } from "playwright";
import http from "http";
import { spawnSync } from "child_process";
import fs from "fs";
import path from "path";
import { fileURLToPath } from "url";
import { assertNoSchedulerMismatches, assertSimpleDrawMode, clearSchedulerMismatches, resolveGoBinary, shouldSkipWasmBuild, flushCoverage, isCoverageEnabled } from "./browser_test_helpers.js";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const jsDir = __dirname;
const goDir = path.resolve(jsDir, "../go");
const GO = resolveGoBinary();

if (!shouldSkipWasmBuild("main.wasm")) {
const build = spawnSync(
  GO,
  ["build", "-ldflags", "-X main.defaultLog=INFO", "-o", path.join(jsDir, "main.wasm"), "./cmd/..."],
  { cwd: goDir, env: { ...process.env, GOOS: "js", GOARCH: "wasm" }, stdio: "inherit" }
);
if (build.status !== 0) throw new Error("go build main wasm failed");
}

const server = http.createServer((req, res) => { const file = req.url === "/" ? "/index.html" : req.url;
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

const browser = await chromium.launch({ args: ["--autoplay-policy=no-user-gesture-required"] });

// Shared helper: build 1x1 rect, start playback, wait for advancement
async function setupAndPlay(page, bpm, label) {
  await page.waitForFunction(() => typeof startPlay === 'function');
  await assertSimpleDrawMode(page, false, label);

  // Clear default demo circuit — only 1 row needed for this test.
  await page.evaluate(() => {
    importJSON(JSON.stringify({
      version: 1, subdiv: 8, bpm: 120,
      instruments: [{name: "Kick", id: "kick", kind: "builtin", volume: 1.0, origin: -1, color: "#C87850FF"}],
      nodes: [],
      eq: {gains_db: [0,0,0,0,0,0,0,0,0,0], bands_hz: []}
    }));
    forceDraw();
  });

  await page.evaluate((bpm) => { setFollow?.(true);
    addNode?.(0,0,'regular');
    addNode?.(1,0,'regular');
    addNode?.(1,1,'regular');
    addNode?.(0,1,'regular');
    addEdgeGrid?.(0,0,1,0);
    addEdgeGrid?.(1,0,1,1);
    addEdgeGrid?.(1,1,0,1);
    addEdgeGrid?.(0,1,0,0);
    setOrigin?.(0,0,0);
    setBPM?.(bpm);
    forceDraw?.();
  }, bpm);

  await clearSchedulerMismatches(page);
  await page.evaluate(() => {
    document.dispatchEvent(new Event('pointerdown'));
    resumeAudio?.();
  });
  await page.evaluate(() => startPlay?.());

  // Poll until playback has started
  await page.waitForFunction(() => {
    const idxs = nextBeatIdxs?.();
    return idxs && idxs[0] > 0;
  }, null, { timeout: 10000 });
}

// Shared helper: verify post-edit state
function verifyPostEdit(before, after, label) {
  if (!after || !before) throw new Error(`${label}: missing snapshots`);
  if (!(after.nba && before.nba) || after.nba[0] <= before.nba[0]) {
    throw new Error(`${label}: nextBeatIdx did not advance: ${before.nba} -> ${after.nba}`);
  }
  if ((after.rowsDrawn ?? 0) < Math.min(after.vis ?? 0, 1) && !after.uiOk) {
    throw new Error(`${label}: UI not fully drawn after edit (rowsDrawn=${after.rowsDrawn} vis=${after.vis} uiOk=${after.uiOk})`);
  }
  if (after.metrics && before.metrics && (after.metrics.count > 0 || before.metrics.count > 0)) {
    if (after.metrics.count <= before.metrics.count) {
      throw new Error(`${label}: audio schedule metrics did not advance after edit`);
    }
  }
}

// Scenario 1: Before-playhead edit — add detour during playback
console.log("Scenario 1: Live edit before playhead (add detour)");
{
  const page = await browser.newPage();
  await page.goto(`http://localhost:${port}/`);
  await setupAndPlay(page, 200, "live edit sync (before)");

  const before = await page.evaluate(() => ({ off: drumOffset?.(),
    nba: nextBeatIdxs?.(),
    metrics: getAudioScheduleMetrics?.(),
  }));

  // Live edit in the future: add detour b(1,0)->(2,0)->(2,1)->c(1,1)
  await page.evaluate(() => { addNode?.(2,0,'regular');
    addNode?.(2,1,'regular');
    deleteEdgeGrid?.(1,0,1,1);
    addEdgeGrid?.(1,0,2,0);
    addEdgeGrid?.(2,0,2,1);
    addEdgeGrid?.(2,1,1,1);
    updateBeatInfosJS?.();
  });

  // Poll until nextBeatIdx advances past the 'before' snapshot
  await page.waitForFunction((prevIdx) => {
    const idxs = nextBeatIdxs?.();
    return idxs && idxs[0] > prevIdx;
  }, before.nba[0], { timeout: 10000 });

  const after = await page.evaluate(() => ({ off: drumOffset?.(),
    nba: nextBeatIdxs?.(),
    rowsDrawn: rowsContentVisibleCount?.(),
    vis: visibleRows?.(),
    uiOk: uiLayoutOk?.(),
    metrics: getAudioScheduleMetrics?.(),
  }));

  await assertNoSchedulerMismatches(page, "live edit sync (before): scheduler mismatches");
  verifyPostEdit(before, after, "Scenario 1");

  await page.close();
}

// Scenario 2: Mid-beat noop edit — delete+re-add same edge during playback
console.log("Scenario 2: Live edit mid-beat (delete+re-add same edge)");
{
  const page = await browser.newPage();
  await page.goto(`http://localhost:${port}/`);
  await setupAndPlay(page, 180, "live edit sync (mid)");

  const before = await page.evaluate(() => ({ off: drumOffset?.(), nba: nextBeatIdxs?.(), metrics: getAudioScheduleMetrics?.() }));

  // Mid-beat edit: remove and immediately re-add the same edge
  await page.evaluate(() => {
    deleteEdgeGrid?.(1,0,1,1);
    addEdgeGrid?.(1,0,1,1);
    updateBeatInfosJS?.();
  });

  // Poll until nextBeatIdx advances past the 'before' snapshot
  await page.waitForFunction((prevIdx) => {
    const idxs = nextBeatIdxs?.();
    return idxs && idxs[0] > prevIdx;
  }, before.nba[0], { timeout: 10000 });

  const after = await page.evaluate(() => ({ off: drumOffset?.(), nba: nextBeatIdxs?.(), rowsDrawn: rowsContentVisibleCount?.(), vis: visibleRows?.(), uiOk: uiLayoutOk?.(), metrics: getAudioScheduleMetrics?.() }));

  await assertNoSchedulerMismatches(page, "live edit sync (mid): scheduler mismatches");
  verifyPostEdit(before, after, "Scenario 2");

  if (isCoverageEnabled()) await flushCoverage(page, new URL("../../coverage/browser-raw", import.meta.url).pathname, "live_edit_sync_variants");
  await page.close();
}

await browser.close();
server.close();
console.log("live edit sync variant tests completed");
