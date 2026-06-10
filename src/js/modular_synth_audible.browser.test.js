/**
 * Modular synth voice — end-to-end audible test.
 *
 * Proves the unified modular instrument ("modular") renders sound through the
 * real WASM↔WebAudio bridge and that editing its pipeline params (oscillator
 * type, filter cutoff) actually changes the rendered bytes via the
 * render_modular_p path + the modular param block in synth_param_abi.gen.js.
 *
 * COVERED-BY-GO: internal/audio/modular_render_test.go (C render correctness),
 * internal/audio/modular_param_schema_test.go (ABI schema). This test owns ONLY
 * the WASM↔WebAudio boundary for the modular param block.
 */

import { chromium } from "playwright";
import { spawnSync } from "child_process";
import http from "http";
import fs from "fs";
import path from "path";
import { fileURLToPath } from "url";
import {
  resolveGoBinary,
  shouldSkipWasmBuild,
  flushCoverage,
  isCoverageEnabled,
} from "./browser_test_helpers.js";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const jsDir = __dirname;

const chromiumPath = path.join(jsDir, "node_modules", ".cache", "ms-playwright", "chromium");
if (!fs.existsSync(chromiumPath)) {
  spawnSync("npx", ["playwright", "install", "chromium"], { cwd: jsDir, stdio: "inherit" });
}

const goDir = path.resolve(jsDir, "../go");
const GO = resolveGoBinary();

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

const browser = await chromium.launch({ args: ["--autoplay-policy=no-user-gesture-required"] });
const page = await browser.newPage();
page.on("console", (msg) => { try { console.log("[PAGE]", msg.type(), msg.text()); } catch (_) {} });

await page.goto(`http://localhost:${port}/`);
await page.waitForFunction(() => typeof setInstrumentParam === "function");
await page.waitForFunction(() => typeof window.__testCaptureSynthRender === "function");
await page.waitForFunction(() => window.audioReady !== undefined);
await page.evaluate(async () => { await window.audioReady; });

// 1) Default modular voice renders non-silent.
await page.evaluate(() => resetInstrumentParams("modular"));
const base = await page.evaluate(() => window.__testCaptureSynthRender("modular"));
if (!base) throw new Error("modular default render returned null");
const headRMS = Math.sqrt(base.head.reduce((s, v) => s + v * v, 0) / base.head.length);
if (!(base.tailRMS > 0) && !(headRMS > 0)) {
  throw new Error(`modular default render is silent (headRMS=${headRMS}, tailRMS=${base.tailRMS})`);
}
console.log(`[TEST] modular default render non-silent (headRMS=${headRMS.toFixed(6)})`);

// 2) Switching the oscillator/generator (sine -> saw) changes the bytes.
await page.evaluate(() => setInstrumentParam("modular", "osc_type", 1)); // saw
const saw = await page.evaluate(() => window.__testCaptureSynthRender("modular"));
if (!saw) throw new Error("modular saw render returned null");
let oscDiffers = false;
for (let i = 0; i < base.head.length; i++) {
  if (Math.abs(base.head[i] - saw.head[i]) > 1e-6) { oscDiffers = true; break; }
}
if (!oscDiffers) {
  throw new Error("changing osc_type sine->saw produced an identical buffer — the generator selector did not reach render_modular_p");
}
console.log("[TEST] osc_type sine->saw audibly changed the waveform");

// 3) Lowering the filter cutoff changes the tone (tail RMS shifts).
await page.evaluate(() => { setInstrumentParam("modular", "osc_type", 1); setInstrumentParam("modular", "filter_cutoff", 300); });
const dark = await page.evaluate(() => window.__testCaptureSynthRender("modular"));
if (!dark) throw new Error("modular filtered render returned null");
let filterDiffers = false;
for (let i = 0; i < saw.tail.length; i++) {
  if (Math.abs(saw.tail[i] - dark.tail[i]) > 1e-6) { filterDiffers = true; break; }
}
if (!filterDiffers) {
  throw new Error("lowering filter_cutoff did not change the rendered tail — the filter stage did not reach render_modular_p");
}
console.log("[TEST] filter_cutoff change audibly altered the tone");

// 4) Reset restores the default sine voice.
await page.evaluate(() => resetInstrumentParams("modular"));
const restored = await page.evaluate(() => window.__testCaptureSynthRender("modular"));
if (!restored) throw new Error("modular restored render returned null");
let restoredMatches = true;
for (let i = 0; i < base.head.length; i++) {
  if (Math.abs(base.head[i] - restored.head[i]) > 1e-6) { restoredMatches = false; break; }
}
if (!restoredMatches) {
  throw new Error("resetInstrumentParams did not restore the default modular voice");
}
console.log("[TEST] reset restored the default modular voice");

// 5) The second shipped preset (modular-pad) must render DISTINCTLY from the
// base modular voice WITHOUT any user edit — proving the bootstrap defaults
// push (Go SeedInstrumentDefaultsToPlatform → window.seedInstrumentDefaults →
// instrumentDefaultParams) reaches render_modular_p in the browser. Without it,
// modular-pad would sound identical to base modular (no param block).
await page.evaluate(() => resetInstrumentParams("modular"));
const padBase = await page.evaluate(() => window.__testCaptureSynthRender("modular"));
const pad = await page.evaluate(() => window.__testCaptureSynthRender("modular-pad"));
if (!pad) throw new Error("modular-pad render returned null");
const padHeadRMS = Math.sqrt(pad.head.reduce((a, v) => a + v * v, 0) / pad.head.length);
if (!(padHeadRMS > 1e-6 || pad.tailRMS > 1e-6)) {
  throw new Error(`modular-pad render is silent (headRMS=${padHeadRMS}, tailRMS=${pad.tailRMS})`);
}
let padDiffers = false;
for (let i = 0; i < padBase.head.length; i++) {
  if (Math.abs(padBase.head[i] - pad.head[i]) > 1e-6) { padDiffers = true; break; }
}
if (!padDiffers) {
  throw new Error("modular-pad rendered identically to base modular — bootstrap defaults push did not reach the browser render path");
}
console.log("[TEST] modular-pad preset renders distinctly via bootstrap defaults push");

if (isCoverageEnabled()) {
  await flushCoverage(page, new URL("../../coverage/browser-raw", import.meta.url).pathname, "modular_synth_audible");
}
await browser.close();
server.close();
console.log("modular_synth_audible browser test PASSED");
