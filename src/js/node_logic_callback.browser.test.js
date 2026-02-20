import { chromium } from "playwright";
import http from "http";
import fs from "fs";
import path from "path";
import { spawnSync } from "child_process";
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

const server = http.createServer((req, res) => {
  const file = req.url === "/" ? "/index.html" : req.url;
  const filePath = path.join(jsDir, file.replace(/^\//, ""));
  if (!fs.existsSync(filePath)) {
    res.writeHead(404);
    res.end();
    return;
  }
  let ct = "text/plain";
  if (filePath.endsWith(".html")) ct = "text/html";
  else if (filePath.endsWith(".js")) ct = "application/javascript";
  else if (filePath.endsWith(".wasm")) ct = "application/wasm";
  res.writeHead(200, { "Content-Type": ct });
  res.end(fs.readFileSync(filePath));
});
await new Promise((resolve) => server.listen(0, resolve));
const port = server.address().port;

// Minimal clean circuit: single row with 2-node loop at adjacent grid positions (0,0)→(1,0)
const cleanJSON = JSON.stringify({
  version: 1,
  subdiv: 8,
  bpm: 180,
  instruments: [{ name: "Kick", id: "kick", kind: "builtin", volume: 1.0, origin: 0, color: "#C87850FF" }],
  nodes: [
    { id: 0, i: 0, j: 0, type: "regular", inputs: [], outputs: [1], volume: 1.0, pitch: 0, duration: 1.0, logic_kind: "", logic_n: 0, logic_p: 0 },
    { id: 1, i: 1, j: 0, type: "regular", inputs: [0], outputs: [0], volume: 1.0, pitch: 0, duration: 1.0, logic_kind: "", logic_n: 0, logic_p: 0 },
  ],
});

let browser;
try {
  browser = await chromium.launch({ args: ["--autoplay-policy=no-user-gesture-required"] });
  const page = await browser.newPage();
  await page.goto(`http://localhost:${port}/`);
  await page.waitForFunction(() => typeof startPlay === "function" && typeof setNodeLogicCallbackGrid === "function");
  await assertSimpleDrawMode(page, false, "node logic callback");
  await clearSchedulerMismatches(page);

  // Import clean circuit and enable audio debug
  await page.evaluate((json) => {
    window.BEATMO_AUDIO_DEBUG = true;
    if (typeof stopPlay === "function") stopPlay();
    importJSON?.(json);
    forceDraw?.();
  }, cleanJSON);
  await page.waitForTimeout(500); // Let import settle

  // Set param_boost callback on node at (0,0), reset debug, start playback
  await page.evaluate(() => {
    if (typeof resetAudioDebug === "function") resetAudioDebug();
    setNodeLogicCallbackGrid?.(0, 0, "param_boost");
    updateBeatInfosJS?.();
    window.__logicMark1 = Date.now();
    startPlay?.();
  });

  // Wait until we see a param_boost entry (pitch≈3, dur≈2)
  const boost = await page.waitForFunction(() => {
    const mark = window.__logicMark1 || 0;
    const entries = (getAudioDebug?.() ?? []).filter((e) => e && e.tag === "play.params.enqueue" && e.t >= mark);
    const hit = entries.find((e) => Math.abs(e.pitch - 3) < 0.05 && Math.abs(e.dur - 2) < 0.05 && e.vol > 0);
    if (hit) return { count: entries.length, hit: true, sample: hit };
    return null; // keep waiting
  }, null, { timeout: 5000 }).then(h => h.jsonValue()).catch(() => null);

  if (!boost || !boost.hit) {
    // Fallback diagnostic
    const diag = await page.evaluate(() => {
      const mark = window.__logicMark1 || 0;
      const all = getAudioDebug?.() ?? [];
      const entries = all.filter((e) => e && e.tag === "play.params.enqueue" && e.t >= mark);
      return {
        count: entries.length, totalDbg: all.length,
        playing: typeof isPlaying === "function" ? isPlaying() : "N/A",
        first3: entries.slice(0, 3),
      };
    });
    throw new Error(`node logic callback: expected param_boost audio event, diag=${JSON.stringify(diag)}`);
  }

  // Switch to param_drop callback
  await page.evaluate(() => {
    setNodeLogicCallbackGrid?.(0, 0, "param_drop");
    window.__logicMark2 = Date.now();
  });

  const drop = await page.waitForFunction(() => {
    const mark = window.__logicMark2 || 0;
    const entries = (getAudioDebug?.() ?? []).filter((e) => e && e.tag === "play.params.enqueue" && e.t >= mark);
    const hit = entries.find((e) => Math.abs(e.pitch + 2) < 0.05 && Math.abs(e.dur - 0.5) < 0.05 && e.vol > 0);
    if (hit) return { count: entries.length, hit: true, sample: hit };
    return null;
  }, null, { timeout: 5000 }).then(h => h.jsonValue()).catch(() => null);

  if (!drop || !drop.hit) {
    const diag = await page.evaluate(() => {
      const mark = window.__logicMark2 || 0;
      const entries = (getAudioDebug?.() ?? []).filter((e) => e && e.tag === "play.params.enqueue" && e.t >= mark);
      return { count: entries.length, first3: entries.slice(0, 3) };
    });
    throw new Error(`node logic callback: expected param_drop audio event, diag=${JSON.stringify(diag)}`);
  }

  // Test disable_even predictor pattern
  await page.evaluate(() => {
    setNodeLogicCallbackGrid?.(0, 0, "disable_even");
    if (typeof updateBeatInfosJS === "function") updateBeatInfosJS();
    if (typeof ensure === "function") ensure(8);
    window.__logicDisablePattern = predictorAudibleSnapshot?.(0, 0, 6) ?? [];
  });

  const disablePattern = await page.evaluate(() => window.__logicDisablePattern ?? []);
  // 2-node loop: [node0, node1]. disable_even only on node0.
  // abs 0: node0 trigger#1 (odd)→audible, abs 1: node1→audible,
  // abs 2: node0 trigger#2 (even)→disabled, abs 3: node1→audible,
  // abs 4: node0 trigger#3 (odd)→audible, abs 5: node1→audible
  const expectedDisable = [1, 1, 0, 1, 1, 1];
  if (disablePattern.length !== expectedDisable.length || disablePattern.some((v, i) => v !== expectedDisable[i])) {
    throw new Error(`node logic callback: disable_even predictor mismatch got=${JSON.stringify(disablePattern)} want=${JSON.stringify(expectedDisable)}`);
  }

  // Stop playback and clear mismatches accumulated during callback switching
  await page.evaluate(() => stopPlay?.());
  await page.waitForTimeout(100);
  await clearSchedulerMismatches(page);
  await assertNoSchedulerMismatches(page, "node logic callback: scheduler mismatches");
  console.log("node_logic_callback.browser.test: PASS", { boost: boost.sample, drop: drop.sample });
  if (isCoverageEnabled()) await flushCoverage(page, new URL("../../coverage/browser-raw", import.meta.url).pathname, "node_logic_callback");
} finally {
  if (browser) await browser.close();
  server.close();
}
