/**
 * Generator-selector LIVE-CIRCUIT regression test (WASM).
 *
 * Reproduces the user report: with the default circuit PLAYING, changing the
 * Snare's generator off Native makes the audio stop. The earlier tests rendered
 * a single hit in isolation; this drives the real Go engine/sequencer (startPlay
 * → scheduled playSoundsBatchFlat → enqueueAudioEvents → processAudioEvent) and
 * captures the live master output before and after the generator change.
 *
 * Debug logging (window.__beatmoDebugSynthDispatch + BEATMO_AUDIO_DEBUG) is
 * enabled so a failure surfaces WHICH render path each scheduled hit took.
 */

import { chromium } from "playwright";
import { spawnSync } from "child_process";
import http from "http";
import fs from "fs";
import path from "path";
import { fileURLToPath } from "url";
import { resolveGoBinary, shouldSkipWasmBuild } from "./browser_test_helpers.js";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const jsDir = __dirname;
const goDir = path.resolve(jsDir, "../go");
const GO = resolveGoBinary();

const chromiumPath = path.join(jsDir, "node_modules", ".cache", "ms-playwright", "chromium");
if (!fs.existsSync(chromiumPath)) {
  spawnSync("npx", ["playwright", "install", "chromium"], { cwd: jsDir, stdio: "inherit" });
}
if (!shouldSkipWasmBuild("play_ui.wasm")) {
  const build = spawnSync(
    GO, ["build", "-o", path.join(jsDir, "play_ui.wasm"), "./internal/ui/playtest"],
    { cwd: goDir, env: { ...process.env, GOOS: "js", GOARCH: "wasm" }, stdio: "inherit" },
  );
  if (build.status !== 0) throw new Error("go build play_ui failed");
}

const server = http.createServer((req, res) => {
  const file = req.url === "/" ? "/play_ui.html" : req.url;
  const fp = path.join(jsDir, file.replace(/^\//, ""));
  fs.readFile(fp, (err, data) => {
    if (err) { res.writeHead(404); res.end(); return; }
    let ct = "text/plain";
    if (fp.endsWith(".html")) ct = "text/html";
    else if (fp.endsWith(".js")) ct = "application/javascript";
    else if (fp.endsWith(".wasm")) ct = "application/wasm";
    res.writeHead(200, { "Content-Type": ct });
    res.end(data);
  });
});
await new Promise((r) => server.listen(0, r));
const port = server.address().port;

// A 4-node loop driving a single Snare row at 200 BPM so hits land frequently.
const CIRCUIT = JSON.stringify({
  version: 1, subdiv: 8, bpm: 200,
  instruments: [{ name: "Snare", id: "snare", kind: "builtin", volume: 1.0, origin: 1, color: "#C87850FF" }],
  nodes: [
    { id: 1, i: 0, j: 0, type: "regular", outputs: [2] },
    { id: 2, i: 4, j: 0, type: "regular", outputs: [3] },
    { id: 3, i: 8, j: 0, type: "regular", outputs: [4] },
    { id: 4, i: 12, j: 0, type: "regular", outputs: [1] },
  ],
});

const browser = await chromium.launch({ args: ["--autoplay-policy=no-user-gesture-required"] });
const page = await browser.newPage();
const synthLogs = [];
page.on("console", (msg) => {
  const t = msg.text();
  if (/SYNTH-DISPATCH|play\.render|audio\.render|nobuf|defer/.test(t)) synthLogs.push(t);
  try { console.log("[PAGE]", msg.type(), t); } catch (_) {}
});

let failed = false;
const fail = (m) => { failed = true; console.error("[FAIL] " + m); };

const captureWindowRMS = async (ms) => {
  await page.evaluate(() => window.startOutputCapture());
  await page.waitForTimeout(ms);
  return page.evaluate(() => {
    window.stopOutputCapture();
    const b = window.getOutputCapture();
    let peak = 0, rms = 0;
    for (let i = 0; i < b.length; i++) { const a = Math.abs(b[i]); if (a > peak) peak = a; rms += b[i] * b[i]; }
    return { peak, rms: Math.sqrt(rms / Math.max(1, b.length)), n: b.length };
  });
};

try {
  await page.goto(`http://localhost:${port}/`);
  await page.waitForFunction(() => typeof importJSON === "function" && typeof startPlay === "function" && typeof setInstrumentParam === "function");
  await page.waitForFunction(() => window.audioReady !== undefined);
  await page.evaluate(async () => { await window.audioReady; });
  await page.evaluate(() => { window.__beatmoDebugSynthDispatch = true; window.BEATMO_AUDIO_DEBUG = true; });

  const abi = await page.evaluate(() => window.__synthABI || null);
  console.log("[TEST] __synthABI =", JSON.stringify(abi));
  if (abi && abi.stale) fail("synth_param_abi.gen.js is STALE — re-voiced instruments will be silent.");

  await page.evaluate((j) => { importJSON(j); }, CIRCUIT);
  await page.evaluate(() => { if (typeof forceDraw === "function") forceDraw(); });
  await page.evaluate(() => startPlay());

  // Open the Synth tab so its live re-layout (the section-schema swap on
  // re-voicing) runs every frame against the playing engine — the exact UI
  // state the user is in. A panic/hang here would stop the engine → all audio.
  const opened = await page.evaluate(() => (typeof setActiveEQTab === "function" ? setActiveEQTab("synth") : "no-export"));
  console.log(`[TEST] setActiveEQTab('synth') => ${opened}`);

  // Baseline: the circuit must be producing audio at the master output.
  const base = await captureWindowRMS(1200);
  console.log(`[TEST] LIVE baseline (Native snare) peak=${base.peak.toFixed(5)} rms=${base.rms.toFixed(6)} n=${base.n}`);
  if (!(base.peak > 0.0001)) {
    fail(`baseline circuit produced no audio (peak=${base.peak}) — the loop isn't firing the snare; test setup issue.`);
  }

  // Change the Snare generator off Native (Saw) WHILE the circuit plays.
  synthLogs.length = 0;
  await page.evaluate(() => setInstrumentParam("snare", "gen_type", 2));
  const after = await captureWindowRMS(1500);
  console.log(`[TEST] LIVE after gen_type=Saw peak=${after.peak.toFixed(5)} rms=${after.rms.toFixed(6)} n=${after.n}`);
  console.log("[TEST] dispatch logs after change:\n  " + synthLogs.slice(0, 12).join("\n  "));
  if (!(after.peak > 0.0001)) {
    fail(`AUDIO STOPPED after changing the Snare generator off Native (peak=${after.peak}). Reproduces the user report.`);
  }

  // And a few more waveforms, still live.
  for (const [gt, label] of [[6, "Noise White"], [0, "Native (back)"]]) {
    await page.evaluate(([v]) => setInstrumentParam("snare", "gen_type", v), [gt]);
    const w = await captureWindowRMS(1200);
    console.log(`[TEST] LIVE gen_type=${gt} (${label}) peak=${w.peak.toFixed(5)} rms=${w.rms.toFixed(6)}`);
    if (!(w.peak > 0.0001)) fail(`live audio stopped at gen_type=${gt} (${label})`);
  }

  await page.evaluate(() => stopPlay());
} finally {
  await browser.close();
  server.close();
}

if (failed) { console.error("[TEST] synth generator live circuit: FAIL"); process.exit(1); }
console.log("[TEST] synth generator live circuit: PASS");
