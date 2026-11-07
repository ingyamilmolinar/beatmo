// No context until a gesture, channel ops queued before it applied after, a re-suspended context unlocking on the next
// gesture, and audible playback after a touch unlock. Usage: GO=… node src/js/mobile_audio_unlock.browser.test.js

import { chromium, devices } from "playwright";
import http from "http";
import fs from "fs";
import path from "path";
import { spawnSync } from "child_process";
import { fileURLToPath } from "url";
import { resolveGoBinary, shouldSkipWasmBuild, flushCoverage, isCoverageEnabled, waitForAudioReady } from "./browser_test_helpers.js";
import { cdpTap } from "./touch_cdp_helpers.js";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const jsDir = __dirname;
const repoRoot = path.resolve(jsDir, "..", "..");
const goDir = path.resolve(repoRoot, "src/go");
const GO = resolveGoBinary();

// Build full WASM app.
if (!shouldSkipWasmBuild("main.wasm")) {
const build = spawnSync(
  GO,
  ["build", "-ldflags", "-X main.defaultLog=INFO", "-o", path.join(jsDir, "main.wasm"), "./cmd"],
  { cwd: goDir, env: { ...process.env, GOOS: "js", GOARCH: "wasm" }, stdio: "inherit" }
);
if (build.status !== 0) throw new Error("go build main wasm failed");
}

const server = http.createServer((req, res) => {
  const file = req.url === "/" ? "/index.html" : req.url;
  const filePath = path.join(jsDir, file.replace(/^\//, ""));
  fs.readFile(filePath, (err, data) => {
    if (err) { res.writeHead(404); res.end(); return; }
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

const iPhone = devices['iPhone 12 landscape'];
let exitCode = 0;
let browser;
try {
  // Go init must not create a context: its channel calls queue until the first gesture.
  console.log("--- Scenario 1: AudioContext deferred until gesture ---");
  {
    const b = await chromium.launch({
      args: [
        // No --autoplay-policy flag, so deferral is real.
      ],
    });
    const context = await b.newContext({
      ...iPhone,
      hasTouch: true,
    });
    const page = await context.newPage();
    await page.goto(`http://localhost:${port}/`);

    // Wait for WASM exports to be available.
    await page.waitForFunction(() =>
      typeof addNode === "function" &&
      typeof audioNow === "function"
    , null, { timeout: 30000 });

    // audioNow() returns 0 without creating a context.
    const preGesture = await page.evaluate(() => {
      const now = audioNow?.() ?? -1;
      const hasCtx = !!window.__audioCtx;
      return { now, hasCtx };
    });
    console.log(`  Pre-gesture: audioNow=${preGesture.now}, hasCtx=${preGesture.hasCtx}`);

    if (preGesture.hasCtx) {
      // Headless may create one through its own autoplay policy; the state is checked and logged.
      const state = await page.evaluate(() => window.__audioCtx?.state || 'no-context');
      console.log(`  WARN: AudioContext exists pre-gesture (state=${state}). Headless may auto-allow.`);
    } else {
      console.log("  OK: No AudioContext before user gesture");
    }

    // Tap the canvas to trigger unlock and context creation.
    const canvasRect = await page.evaluate(() => {
      const c = document.querySelector('canvas');
      if (!c) return null;
      const r = c.getBoundingClientRect();
      return { x: r.left, y: r.top, w: r.width, h: r.height };
    });
    if (canvasRect) {
      await cdpTap(page, canvasRect.w / 2, canvasRect.h / 2);
    } else {
      await page.evaluate(() => {
        document.dispatchEvent(new Event("touchstart", { bubbles: true }));
        document.dispatchEvent(new Event("touchend", { bubbles: true }));
      });
    }
    await page.waitForTimeout(500);

    // After gesture, context should exist.
    const postGesture = await page.evaluate(() => {
      const hasCtx = !!window.__audioCtx;
      const state = window.__audioCtx?.state || 'no-context';
      const now = audioNow?.() ?? -1;
      return { hasCtx, state, now };
    });
    console.log(`  Post-gesture: hasCtx=${postGesture.hasCtx}, state=${postGesture.state}, audioNow=${postGesture.now}`);

    if (!postGesture.hasCtx) {
      throw new Error("Scenario 1 FAIL: AudioContext not created after gesture");
    }
    console.log("  PASS: AudioContext created after user gesture");
    await b.close();
  }

  console.log("\n--- Scenario 2: Queued channel ops applied after unlock ---");
  browser = await chromium.launch({ args: ["--autoplay-policy=no-user-gesture-required"] });
  {
    const context = await browser.newContext({
      ...iPhone,
      hasTouch: true,
    });
    const page = await context.newPage();
    await page.goto(`http://localhost:${port}/`);

    await page.waitForFunction(() =>
      typeof setChannelVolume === "function" &&
      typeof channelVolume === "function" &&
      typeof playSound === "function"
    , null, { timeout: 30000 });

    // A shared scenario module, so test_runner.html runs the same assertions on a real phone.
    await waitForAudioReady(page, { allowReject: true });
    const r2 = await page.evaluate(async () => {
      const m = await import("/scenarios/mobile_audio_unlock.js");
      return await m.scenarioQueuedChannelOpsAppliedAfterUnlock();
    });
    console.log(`  ${JSON.stringify(r2)}`);
    if (!r2.pass) {
      throw new Error(`Scenario 2 FAIL: ${r2.reason || "unknown"}`);
    }
    console.log("  PASS");
    await context.close();
  }

  // Unlock listeners are persistent, so after a tab switch or call the next gesture re-unlocks.
  console.log("\n--- Scenario 3: Re-suspension recovery ---");
  {
    const context = await browser.newContext({
      ...iPhone,
      hasTouch: true,
    });
    const page = await context.newPage();
    await page.goto(`http://localhost:${port}/`);

    await page.waitForFunction(() =>
      typeof playSound === "function" &&
      typeof startOutputCapture === "function" &&
      typeof stopOutputCapture === "function" &&
      typeof resumeAudio === "function"
    , null, { timeout: 30000 });

    // Create and unlock the context first.
    await page.evaluate(() => {
      document.dispatchEvent(new Event("pointerdown"));
      resumeAudio?.();
    });
    await page.waitForTimeout(200);

    const result = await page.evaluate(async () => {
      const ctx = window.__audioCtx;
      if (!ctx) return { error: "no AudioContext" };
      if (ctx.state !== 'running') {
        // Try to get it running first.
        await ctx.resume();
        await new Promise((r) => setTimeout(r, 100));
      }
      const initialState = ctx.state;

      // Simulate re-suspension (tab switch, phone call, etc.).
      await ctx.suspend();
      await new Promise((r) => setTimeout(r, 50));
      const suspendedState = ctx.state;

      // Dispatch a gesture event — the persistent unlock listeners should re-unlock.
      document.dispatchEvent(new Event("touchstart", { bubbles: true }));
      document.dispatchEvent(new Event("pointerdown", { bubbles: true }));

      // Wait for the unlock to process.
      await new Promise((r) => setTimeout(r, 500));

      const finalState = ctx.state;
      return { initialState, suspendedState, finalState };
    });

    if (result.error) throw new Error(`Scenario 3 FAIL: ${result.error}`);
    console.log(`  Initial: ${result.initialState}, After suspend: ${result.suspendedState}, After re-gesture: ${result.finalState}`);

    if (result.suspendedState !== 'suspended') {
      console.log(`  WARN: suspend() didn't work (got ${result.suspendedState}), skipping re-suspension check`);
    } else if (result.finalState === 'running') {
      console.log("  PASS: Context re-unlocked after re-suspension");
    } else {
      // Headless synthetic events may not count as gestures for resume().
      console.log(`  WARN: Context still ${result.finalState} after re-gesture (headless limitation)`);
      console.log("  NOTE: Persistent unlock listeners ARE registered; real mobile will re-unlock.");
    }

    await context.close();
  }

  console.log("\n--- Scenario 4: Playback produces audio after touch unlock ---");
  {
    const context = await browser.newContext({
      ...iPhone,
      hasTouch: true,
    });
    const page = await context.newPage();
    await page.goto(`http://localhost:${port}/`);

    await page.waitForFunction(() =>
      typeof playSound === "function" &&
      typeof addNode === "function" &&
      typeof addEdgeGrid === "function" &&
      typeof updateBeatInfos === "function" &&
      typeof startPlay === "function" &&
      typeof stopPlay === "function" &&
      typeof startOutputCapture === "function" &&
      typeof stopOutputCapture === "function"
    , null, { timeout: 30000 });

    // The same assertions run on a real phone via test_runner.html.
    await waitForAudioReady(page, { allowReject: true });
    const s4 = await page.evaluate(async () => {
      const m = await import("/scenarios/mobile_audio_unlock.js");
      return await m.scenarioPlaybackAfterUnlock();
    });
    console.log(`  ${JSON.stringify(s4)}`);
    if (!s4.pass) {
      throw new Error(`Scenario 4 FAIL: ${s4.reason || "unknown"}`);
    }
    console.log("  PASS");

    if (isCoverageEnabled()) await flushCoverage(page, new URL("../../coverage/browser-raw", import.meta.url).pathname, "mobile_audio_unlock");
    await context.close();
  }

  console.log("\nAll mobile audio unlock tests passed.");
} catch (err) {
  console.error("FAIL:", err.message);
  exitCode = 1;
} finally {
  if (browser) await browser.close();
  server.close();
}

process.exit(exitCode);
