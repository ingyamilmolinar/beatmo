// mobile_import_gesture.browser.test.js
//
// REAL-GESTURE reproduction + regression guard for "tapping Import does nothing
// on iOS Safari" (the phone's file system never opens).
//
// WHY the existing mobile_file_picker.browser.test.js does not catch this: it
// stubs window._fpConsumePending and calls window.openJSONFile() directly, so it
// never exercises the actual tap-to-open-picker gesture.
//
// WHY headless Chromium alone is not enough: Chromium happily opens a file
// chooser from a programmatic input.click() fired in a canvas `touchend`
// handler. iOS Safari does NOT — it only reliably opens the native file picker
// when the user physically taps a real <input type=file> element. So the
// iOS-faithful invariant this test pins is:
//
//   After the mobile overflow menu opens, a REAL <input type=file> element must
//   exist in the DOM, positioned over the Import (and Upload) row — so the user's
//   tap lands on a real input, which every browser (including iOS Safari) honors.
//
// The test also drives the full end-to-end: tap the Import row, pick a file, and
// assert the circuit actually loads.
//
// Usage: GO=/path/to/.tools/go/bin/go node src/js/mobile_import_gesture.browser.test.js

import { chromium, devices } from "playwright";
import http from "http";
import fs from "fs";
import os from "os";
import path from "path";
import { spawnSync } from "child_process";
import { fileURLToPath } from "url";
import { resolveGoBinary, shouldSkipWasmBuild } from "./browser_test_helpers.js";
import { cdpTap } from "./touch_cdp_helpers.js";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const jsDir = __dirname;
const repoRoot = path.resolve(jsDir, "..", "..");
const goDir = path.resolve(repoRoot, "src/go");
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

// Temp circuit file with a distinctive BPM so we can prove the import applied.
const importFile = path.join(os.tmpdir(), "beatmo_import_gesture_probe.json");
fs.writeFileSync(importFile, JSON.stringify({
  version: 1, subdiv: 8, bpm: 123,
  instruments: [{ name: "Kick", id: "kick", kind: "builtin", volume: 1.0, origin: 0, color: "#C87850FF" }],
  nodes: [
    { id: 0, i: 0, j: 0, type: "regular", inputs: [], outputs: [1], volume: 1.0, pitch: 0, duration: 1.0 },
    { id: 1, i: 4, j: 0, type: "regular", inputs: [0], outputs: [0], volume: 1.0, pitch: 0, duration: 1.0 },
  ],
}));

let exitCode = 0;
const rectCenter = (r) => ({ x: r.x + r.w / 2, y: r.y + r.h / 2 });
const overlaps = (a, b) =>
  a.x < b.x + b.w && a.x + a.w > b.x && a.y < b.y + b.h && a.y + a.h > b.y;

// Open the overflow menu on `deviceName` and assert a real <input type=file>
// overlay exists over the Import row. When `endToEnd` is set, also tap the row,
// pick a file, and assert the circuit loads (bpm -> 123).
async function runMobileImport(deviceName, { endToEnd } = {}) {
  console.log(`--- ${deviceName}: real Import file-input overlay${endToEnd ? " + end-to-end" : ""} ---`);
  const browser = await chromium.launch({ args: ["--autoplay-policy=no-user-gesture-required"] });
  try {
    const context = await browser.newContext({ ...devices[deviceName], hasTouch: true });
    const page = await context.newPage();
    await page.goto(`http://localhost:${port}/`);

    await page.waitForFunction(
      () => typeof fullLayoutSnapshot === "function" && typeof getBPM === "function",
      { timeout: 30000 }
    );
    await page.waitForTimeout(500);

    const isMobile = await page.evaluate(() => fullLayoutSnapshot()?.drumLayout?.isSmallScreen);
    if (!isMobile) throw new Error(`${deviceName}: expected mobile profile; threshold/runtime drift`);

    // Open the overflow menu via a real tap so OnOverflowOpen runs (this is what
    // registers the file-picker rects and, after the fix, creates the overlays).
    const overflow = await page.evaluate(() => fullLayoutSnapshot()?.buttons?.overflow || null);
    if (!overflow || overflow.w <= 0) throw new Error(`${deviceName}: no overflow button rect`);
    const oc = rectCenter(overflow);
    await cdpTap(page, oc.x, oc.y);
    await page.waitForTimeout(400);

    const importRect = await page.evaluate(() => window._fpDebugRects?.().import || null);
    if (!importRect || importRect.w <= 0) {
      throw new Error(`${deviceName}: Import file-picker rect not registered on overflow open`);
    }

    // Canvas page offset so we can compare DOM client rects against the canvas-
    // relative import rect.
    const canvasBox = await page.evaluate(() => {
      const c = document.querySelector("canvas");
      const r = c.getBoundingClientRect();
      return { left: r.left, top: r.top };
    });
    const importPageRect = {
      x: canvasBox.left + importRect.x, y: canvasBox.top + importRect.y,
      w: importRect.w, h: importRect.h,
    };

    // THE iOS-FAITHFUL ASSERTION: a real <input type=file> for JSON exists over
    // the Import row. Before the fix no such element exists (the input is only
    // created on touchend), so this fails — matching the iOS symptom.
    const fileInputs = await page.evaluate(() =>
      Array.from(document.querySelectorAll('input[type="file"]')).map((el) => {
        const r = el.getBoundingClientRect();
        return { accept: el.accept || "", x: r.left, y: r.top, w: r.width, h: r.height };
      })
    );
    const jsonOverlay = fileInputs.find(
      (el) => /json/i.test(el.accept) && el.w > 0 && el.h > 0 && overlaps(el, importPageRect)
    );
    if (!jsonOverlay) {
      throw new Error(
        `${deviceName} FAIL: no real <input type=file> overlay over the Import row ` +
        `(import row=${JSON.stringify(importPageRect)}, file inputs=${JSON.stringify(fileInputs)}). ` +
        `iOS Safari will not open the file picker without a real tappable input.`
      );
    }
    console.log(`  OK: JSON file-input overlay present at ${JSON.stringify(jsonOverlay)}`);

    // The same iOS-safe overlay must exist for Upload (WAV), since it shares the
    // exact gesture mechanism that fails on iOS.
    const uploadRect = await page.evaluate(() => window._fpDebugRects?.().upload || null);
    if (uploadRect && uploadRect.w > 0) {
      const uploadPageRect = {
        x: canvasBox.left + uploadRect.x, y: canvasBox.top + uploadRect.y,
        w: uploadRect.w, h: uploadRect.h,
      };
      const wavOverlay = fileInputs.find(
        (el) => /wav/i.test(el.accept) && el.w > 0 && el.h > 0 && overlaps(el, uploadPageRect)
      );
      if (!wavOverlay) {
        throw new Error(`${deviceName} FAIL: no real <input type=file accept=.wav> overlay over the Upload row`);
      }
      console.log(`  OK: WAV file-input overlay present`);
    }

    if (endToEnd) {
      page.on("filechooser", async (fc) => { try { await fc.setFiles(importFile); } catch (_) {} });
      const ic = rectCenter(importRect);
      await cdpTap(page, ic.x, ic.y);
      // The import chain crosses several Update ticks (overlay change ->
      // _fpStartImport -> QueueAction -> importBtn.OnClick -> openJSONFile ->
      // consume pending -> importCh -> apply). Under the headless ~7fps loop
      // that is well over a second, so poll instead of using a fixed sleep.
      let bpm = 0;
      const deadline = Date.now() + 12000;
      while (Date.now() < deadline) {
        await page.evaluate(() => forceDraw && forceDraw());
        bpm = await page.evaluate(() => getBPM());
        if (bpm === 123) break;
        await page.waitForTimeout(200);
      }
      if (bpm !== 123) {
        throw new Error(`${deviceName} FAIL: import did not apply (bpm=${bpm}, expected 123)`);
      }
      console.log(`  OK: tapping Import opened the picker and the circuit loaded (bpm=123)`);
    }

    console.log(`  PASS: ${deviceName}`);
  } finally {
    await browser.close();
  }
}

// Mobile landscape is intentionally unsupported: the mobile profile detects
// landscape (IsMobile && winW > winH) and renders a full-screen
// rotate-to-portrait notice while BLOCKING all touch input (see
// internal/ui/landscape_unsupported.go + game_input_touch.go). The disable
// itself — the DOM #landscape-block overlay — is owned and fully tested by
// landscape_block.browser.test.js. This function pins the matching invariant
// for THIS test's concern: because input is blocked, the overflow menu cannot
// open in landscape, so no Import file-picker rect registers and no real
// <input type=file> overlay leaks into the blocked state. Importing in
// landscape is impossible by design — asserting it works (as a prior version
// of this test did) contradicts the shipped feature.
async function assertLandscapeImportBlocked(deviceName) {
  console.log(`--- ${deviceName}: import flow blocked (landscape unsupported) ---`);
  const browser = await chromium.launch({ args: ["--autoplay-policy=no-user-gesture-required"] });
  try {
    const context = await browser.newContext({ ...devices[deviceName], hasTouch: true });
    const page = await context.newPage();
    await page.goto(`http://localhost:${port}/`);

    await page.waitForFunction(
      () => typeof fullLayoutSnapshot === "function" && typeof getBPM === "function",
      { timeout: 30000 }
    );
    await page.waitForTimeout(500);

    const snap = await page.evaluate(() => fullLayoutSnapshot());
    if (!snap?.drumLayout?.isSmallScreen) {
      throw new Error(`${deviceName}: expected mobile profile; threshold/runtime drift`);
    }
    // Sanity: this device really is landscape (width > height) so the
    // landscapeUnsupported() gate is actually exercised.
    if (!(snap.canvasWidth > snap.canvasHeight)) {
      throw new Error(`${deviceName}: expected landscape (w>h), got ${snap.canvasWidth}x${snap.canvasHeight}`);
    }

    // Best-effort tap on the overflow button location. Input is blocked in
    // landscape, so the menu must NOT open and no picker rect may register.
    const overflow = snap?.buttons?.overflow || null;
    if (overflow && overflow.w > 0) {
      const oc = rectCenter(overflow);
      await cdpTap(page, oc.x, oc.y);
      await page.waitForTimeout(400);
    }

    const importRect = await page.evaluate(() => window._fpDebugRects?.().import || null);
    if (importRect && importRect.w > 0) {
      throw new Error(
        `${deviceName} FAIL: Import file-picker rect registered in landscape — ` +
        `input must be blocked by the rotate-to-portrait notice (got ${JSON.stringify(importRect)})`
      );
    }

    const jsonOverlays = await page.evaluate(() =>
      Array.from(document.querySelectorAll('input[type="file"]'))
        .filter((el) => /json/i.test(el.accept || ""))
        .map((el) => { const r = el.getBoundingClientRect(); return { w: r.width, h: r.height }; })
        .filter((r) => r.w > 0 && r.h > 0)
    );
    if (jsonOverlays.length > 0) {
      throw new Error(
        `${deviceName} FAIL: a real <input type=file> overlay leaked into the blocked ` +
        `landscape state (${JSON.stringify(jsonOverlays)})`
      );
    }

    console.log("  OK: no Import picker rect and no file-input overlay in landscape (input blocked)");
    console.log(`  PASS: ${deviceName}`);
  } finally {
    await browser.close();
  }
}

try {
  await runMobileImport("iPhone 12", { endToEnd: true });
  await runMobileImport("Pixel 5", {});
  await assertLandscapeImportBlocked("iPhone 12 landscape");
  console.log("\nAll scenarios passed!");
} catch (err) {
  console.error("FAIL:", err.message);
  exitCode = 1;
} finally {
  server.close();
  process.exit(exitCode);
}
