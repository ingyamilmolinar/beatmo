/** Under 60 Hz slider abuse during playback the audio chain must stay operational: no silence, no NaN. */

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
  waitForAudioReady,
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
await waitForAudioReady(page);

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

// audio.js gates plays on a user gesture, so the context is woken by hand.
await page.evaluate(async () => {
  // The autoplay flag skips the gesture, but the context still needs resuming.
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

// 60 Hz for 1 s while plays fly: the user's pattern.
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

// The last drag value must still leave an audible snare.
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
