/**
 * Playback stability regression — Phase 5 follow-up.
 *
 * The user reports that audio stops after interacting with the synth
 * sliders in the browser. The identity-defaults fix eliminated the
 * NaN-produced silence, but there may be another race / crash mode
 * triggered by 60Hz slider drag during active playback.
 *
 * This test reproduces the scenario:
 *   1. Start active playback (loop the snare via playSound).
 *   2. Spam setInstrumentParam at 60Hz for 1 second.
 *   3. After the spam, verify playSound still produces non-silent audio.
 *
 * The contract: even under pathological slider abuse, the audio chain
 * must remain operational. If audio is silenced or NaN'd, this test
 * detects it via tail-RMS dropping to 0 OR Number.isFinite() failing.
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
const pageErrors = [];
page.on("pageerror", (err) => {
  pageErrors.push(String(err));
  console.log("[PAGE-ERROR]", String(err));
});
page.on("console", (msg) => {
  try {
    const text = msg.text();
    // Capture warnings + errors aggressively; suppress INFO log spam.
    if (msg.type() === "error" || msg.type() === "warning" || text.startsWith("[TEST]") || text.startsWith("[PAGE-ERROR]")) {
      console.log("[PAGE]", msg.type(), text);
    }
  } catch (_) {}
});

await page.goto(`http://localhost:${port}/`);
await page.waitForFunction(() => typeof setInstrumentParam === "function");
await page.waitForFunction(() => typeof window.__testCaptureSynthRender === "function");
await page.waitForFunction(() => typeof window.playSound === "function");
await page.waitForFunction(() => window.audioReady !== undefined);
await page.evaluate(async () => { await window.audioReady; });

const assertFinite = (buf, label) => {
  for (const v of buf) {
    if (!Number.isFinite(v)) {
      throw new Error(`${label}: non-finite value ${v} in buffer`);
    }
  }
};
const energy = (buf) => {
  let sum = 0;
  for (const v of buf) sum += Math.abs(v);
  return sum / buf.length;
};

// ─── Step 1: baseline reference ───
await page.evaluate(() => resetInstrumentParams('snare'));
const baseline = await page.evaluate(() => window.__testCaptureSynthRender('snare'));
assertFinite(baseline.head, 'baseline.head');
const baselineEnergy = energy(baseline.head);
console.log(`[TEST] baseline head energy=${baselineEnergy.toFixed(6)}`);

// ─── Step 2: trigger active playback (simulate sequencer) ───
//
// Force the AudioContext awake (audio.js gates plays on user gesture).
await page.evaluate(async () => {
  // Some Chromium versions require an explicit user gesture event.
  // chromium.launch's --autoplay-policy=no-user-gesture-required avoids
  // it, but we still need the context to resume.
  if (window.__audioCtx && window.__audioCtx.state === 'suspended') {
    await window.__audioCtx.resume();
  }
});

// Schedule 50 plays over the next 1s to simulate playback.
await page.evaluate(async () => {
  const ctx = window.__audioCtx;
  const start = ctx ? ctx.currentTime : 0;
  const promises = [];
  for (let i = 0; i < 50; i++) {
    promises.push(window.playSound('snare', 0.8, start + i * 0.02));
  }
  await Promise.allSettled(promises);
});

// ─── Step 3: rapid slider drag mid-playback (the user's exact pattern) ───
//
// Set pitch at 60Hz for 1s while plays are flying. The bug pattern:
// after this spam, subsequent plays may be silent / NaN-propagated.
await page.evaluate(async () => {
  const stamps = [];
  const startMS = performance.now();
  for (let frame = 0; frame < 60; frame++) {
    // Sweep pitch from -24 to +24 over the 60 frames.
    const pitch = -24 + (48 * frame) / 60;
    setInstrumentParam('snare', 'pitch', pitch);
    stamps.push({ frame, pitch });
    // Yield to the event loop for 16ms (~60Hz).
    await new Promise((r) => setTimeout(r, 16));
  }
  window.__synthSpamStamps = stamps;
  const dur = performance.now() - startMS;
  console.log(`[TEST] slider spam done in ${dur.toFixed(0)} ms`);
});

// Allow the audio context to flush queued events.
await page.waitForTimeout(500);

// ─── Step 4: post-spam render must still be valid + audible ───
const post = await page.evaluate(() => window.__testCaptureSynthRender('snare'));
if (!post) throw new Error('post-spam snapshot is null');
assertFinite(post.head, 'post-spam head');
assertFinite(post.tail, 'post-spam tail');
const postEnergy = energy(post.head);
console.log(`[TEST] post-spam head energy=${postEnergy.toFixed(6)}`);

// Without identity-defaults fix, pitch=24 (the last drag value) leaves
// the cache with a render that has all knobs in a strange state. With
// the fix, the snare should still produce audible head energy.
const ratio = postEnergy / Math.max(baselineEnergy, 1e-12);
console.log(`[TEST] post-spam head ratio = ${(ratio * 100).toFixed(1)}%`);
if (ratio < 0.25) {
  throw new Error(
    `BUG: after 60Hz slider drag during playback, the snare's head energy dropped to ${(ratio * 100).toFixed(1)}% of baseline (post=${postEnergy} baseline=${baselineEnergy}).\n` +
    `This is the user-reported "audio stops" symptom.`,
  );
}

// ─── Step 5: a fresh play after the spam must complete cleanly ───
const playResult = await page.evaluate(async () => {
  try {
    await window.playSound('snare', 0.8);
    return { ok: true };
  } catch (err) {
    return { ok: false, err: String(err) };
  }
});
if (!playResult.ok) {
  throw new Error(`post-spam playSound threw: ${playResult.err}`);
}
console.log("[TEST] post-spam playSound succeeded");

// ─── Step 6: surface any page errors / warnings collected during the spam ───
if (pageErrors.length > 0) {
  throw new Error(`page emitted ${pageErrors.length} errors during slider spam:\n  ${pageErrors.join("\n  ")}`);
}

console.log("[TEST] Playback stability regression passed.");

if (isCoverageEnabled()) {
  await flushCoverage(
    page,
    new URL("../../coverage/browser-raw", import.meta.url).pathname,
    "instrument_params_playback_stability",
  );
}
await browser.close();
server.close();
console.log("instrument_params_playback_stability browser test PASSED");
