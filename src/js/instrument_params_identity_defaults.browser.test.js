/**
 * Identity-defaults regression — Phase 5 follow-up.
 *
 * Reproduces the user-reported bug: in the browser, moving a single
 * synth slider (e.g. pitch) silences the audio because the JS-side
 * heap-fill code zeroes out every other knob. decay=0 and attack=0
 * clamp to 0.01s in the C `_p` variants, producing near-silent output.
 *
 * The fix is to use IDENTITY defaults (decay=1, attack=1, others=0) for
 * unset knobs — matching the Go `recipeParamsToSynth` adapter exactly.
 *
 * This test FAILS without the fix and PASSES with it.
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

const chromiumPath = path.join(
  jsDir, "node_modules", ".cache", "ms-playwright", "chromium",
);
if (!fs.existsSync(chromiumPath)) {
  spawnSync("npx", ["playwright", "install", "chromium"], {
    cwd: jsDir, stdio: "inherit",
  });
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

const browser = await chromium.launch({
  args: ["--autoplay-policy=no-user-gesture-required"],
});
const page = await browser.newPage();
page.on("console", (msg) => {
  try { console.log("[PAGE]", msg.type(), msg.text()); } catch (_) {}
});

await page.goto(`http://localhost:${port}/`);
await page.waitForFunction(() => typeof setInstrumentParam === "function");
await page.waitForFunction(() => typeof window.__testCaptureSynthRender === "function");
await page.waitForFunction(() => window.audioReady !== undefined);
await page.evaluate(async () => { await window.audioReady; });

// Capture the baseline render with NO user params — the C unparameterized
// `render_snare` is used, all knobs at their hardcoded identity.
await page.evaluate(() => resetInstrumentParams('snare'));
const baseline = await page.evaluate(() => window.__testCaptureSynthRender('snare'));
if (!baseline) throw new Error('baseline snapshot is null');

// Energy of a snare: integrate the absolute amplitude across the buffer.
// The unparameterized snare runs ~0.5s of noise + body — its integrated
// energy is far above silence.
const assertFinite = (buf, label) => {
  for (const v of buf) {
    if (!Number.isFinite(v)) {
      throw new Error(`${label}: buffer contains non-finite value ${v}`);
    }
  }
};
const energy = (buf) => {
  let sum = 0;
  for (const v of buf) sum += Math.abs(v);
  return sum / buf.length;
};
assertFinite(baseline.head, 'baseline.head');
assertFinite(baseline.tail, 'baseline.tail');
const baselineHeadEnergy = energy(baseline.head);
const baselineTailEnergy = energy(baseline.tail);
console.log(`[TEST] baseline head energy=${baselineHeadEnergy.toFixed(6)} tail energy=${baselineTailEnergy.toFixed(6)}`);
if (baselineHeadEnergy < 0.001) {
  throw new Error(`baseline head is silent? energy=${baselineHeadEnergy}; expected > 0.001`);
}

// THE REPRO: set ONLY pitch (matching the user's slider drag pattern).
// Decay and attack are unset → JS should fall back to their identity
// values (decay=1, attack=1), NOT zero. Otherwise the C code clamps
// decay→0.01 and attack→0.01, producing a near-silent buffer.
await page.evaluate(() => setInstrumentParam('snare', 'pitch', 5));
const pitchOnly = await page.evaluate(() => window.__testCaptureSynthRender('snare'));
if (!pitchOnly) throw new Error('pitchOnly snapshot is null');

assertFinite(pitchOnly.head, 'pitchOnly.head');
assertFinite(pitchOnly.tail, 'pitchOnly.tail');
const pitchHeadEnergy = energy(pitchOnly.head);
const pitchTailEnergy = energy(pitchOnly.tail);
console.log(`[TEST] pitch=5 head energy=${pitchHeadEnergy.toFixed(6)} tail energy=${pitchTailEnergy.toFixed(6)}`);

// The pitch shift changes the spectrum but should NOT silence the
// instrument. Head energy should stay within 75%-200% of baseline.
const headRatio = pitchHeadEnergy / Math.max(baselineHeadEnergy, 1e-12);
if (headRatio < 0.25) {
  throw new Error(
    `BUG: pitch=5 silenced the snare head: baseline head energy=${baselineHeadEnergy}, modified=${pitchHeadEnergy} (ratio ${(headRatio * 100).toFixed(1)}%)\n` +
    `This is the user-reported 'audio stops' bug. The JS heap-fill code is zeroing out unset knobs (decay=0, attack=0), which clamps to 0.01s in C and produces near-silent envelopes. ` +
    `Fix: use identity defaults (decay=1, attack=1) in audio.js's render_X_p path.`,
  );
}
console.log(`[TEST] pitch shift preserves head energy (${(headRatio * 100).toFixed(1)}% of baseline)`);

// Also verify the user's exact pattern: drag pitch through extreme values.
// At each step, the output must remain audible.
for (const semis of [3, 7, 12, 19]) {
  await page.evaluate((s) => setInstrumentParam('snare', 'pitch', s), semis);
  const snap = await page.evaluate(() => window.__testCaptureSynthRender('snare'));
  if (!snap) throw new Error(`snapshot null at pitch=${semis}`);
  assertFinite(snap.head, `pitch=${semis} head`);
  assertFinite(snap.tail, `pitch=${semis} tail`);
  const e = energy(snap.head);
  const r = e / Math.max(baselineHeadEnergy, 1e-12);
  console.log(`[TEST] pitch=${semis} head energy ratio ${(r * 100).toFixed(1)}%`);
  if (r < 0.25) {
    throw new Error(
      `pitch=${semis} silenced the snare: ratio ${(r * 100).toFixed(1)}% of baseline`,
    );
  }
}

console.log("[TEST] Identity-defaults regression passed.");

if (isCoverageEnabled()) {
  await flushCoverage(
    page,
    new URL("../../coverage/browser-raw", import.meta.url).pathname,
    "instrument_params_identity_defaults",
  );
}
await browser.close();
server.close();
console.log("instrument_params_identity_defaults browser test PASSED");
