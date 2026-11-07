// iOS Safari opens a picker only for a physical tap on a real <input type=file>, never a programmatic click (which
// Chromium honours), so once the File menu opens a real input must sit over the Import and Upload rows; then the whole
// tap → pick → load path. Usage: GO=… node src/js/mobile_import_gesture.browser.test.js

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
    // Only ./cmd: under GOOS=js "./cmd/..." also matches the generator mains, and -o refuses several packages.
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

// Temp circuit file with a distinctive BPM so we can prove the import applied.
const importFile = path.join(os.tmpdir(), "beatmo_import_gesture_probe.json");
fs.writeFileSync(importFile, JSON.stringify({
  schema_version: "0.0.1", subdiv: 8, bpm: 123,
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

// With endToEnd, also tap the row, pick a file and assert the load (bpm → 123).
async function runMobileImport(deviceName, { endToEnd } = {}) {
  console.log(`--- ${deviceName}: real Import file-input overlay${endToEnd ? " + end-to-end" : ""} ---`);
  const browser = await chromium.launch({ args: ["--autoplay-policy=no-user-gesture-required"] });
  try {
    const context = await browser.newContext({ ...devices[deviceName], hasTouch: true });
    const page = await context.newPage();
    await page.goto(`http://localhost:${port}/`);

    await page.waitForFunction(
      () => typeof fullLayoutSnapshot === "function" && typeof getBPM === "function", null,
      { timeout: 30000 }
    );
    await page.waitForTimeout(500);

    const isMobile = await page.evaluate(() => fullLayoutSnapshot()?.drumLayout?.isSmallScreen);
    if (!isMobile) throw new Error(`${deviceName}: expected mobile profile; threshold/runtime drift`);

    // A real tap registers the picker rects and creates the overlays; the File menu opens from the grid folder button.
    const overflow = await page.evaluate(() => (typeof fileMenuBtnRect === "function" ? fileMenuBtnRect() : null));
    if (!overflow || overflow.w <= 0) throw new Error(`${deviceName}: no file-menu (folder) button rect`);
    const oc = rectCenter(overflow);
    await cdpTap(page, oc.x, oc.y);
    await page.waitForTimeout(400);

    const importRect = await page.evaluate(() => window._fpDebugRects?.().import || null);
    if (!importRect || importRect.w <= 0) {
      throw new Error(`${deviceName}: Import file-picker rect not registered on overflow open`);
    }

    // To compare DOM client rects with the canvas-relative rect.
    const canvasBox = await page.evaluate(() => {
      const c = document.querySelector("canvas");
      const r = c.getBoundingClientRect();
      return { left: r.left, top: r.top };
    });
    const importPageRect = {
      x: canvasBox.left + importRect.x, y: canvasBox.top + importRect.y,
      w: importRect.w, h: importRect.h,
    };

    // THE iOS-faithful assertion: a real input exists over the Import row before any tap.
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

    // Upload shares the same gesture mechanism.
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
      // The import crosses several Update ticks on the rAF loop, which the browser throttles under load, and a CDP tap can
      // drop: re-tap a few times, each polling with a generous budget while pumping frames.
      let bpm = 0;
      const pollForApply = async (ms) => {
        const deadline = Date.now() + ms;
        while (Date.now() < deadline) {
          await page.evaluate(() => forceDraw && forceDraw());
          bpm = await page.evaluate(() => getBPM());
          if (bpm === 123) return true;
          await page.waitForTimeout(200);
        }
        return false;
      };
      let applied = false;
      for (let attempt = 0; attempt < 4 && !applied; attempt++) {
        await cdpTap(page, ic.x, ic.y);
        applied = await pollForApply(8000);
      }
      if (!applied) {
        throw new Error(`${deviceName} FAIL: import did not apply (bpm=${bpm}, expected 123)`);
      }
      console.log(`  OK: tapping Import opened the picker and the circuit loaded (bpm=123)`);
    }

    console.log(`  PASS: ${deviceName}`);
  } finally {
    await browser.close();
  }
}

// Landscape blocks all touch input by design, so the File menu cannot open and no picker rect or overlay may leak into
// the blocked state.
async function assertLandscapeImportBlocked(deviceName) {
  console.log(`--- ${deviceName}: import flow blocked (landscape unsupported) ---`);
  const browser = await chromium.launch({ args: ["--autoplay-policy=no-user-gesture-required"] });
  try {
    const context = await browser.newContext({ ...devices[deviceName], hasTouch: true });
    const page = await context.newPage();
    await page.goto(`http://localhost:${port}/`);

    await page.waitForFunction(
      () => typeof fullLayoutSnapshot === "function" && typeof getBPM === "function", null,
      { timeout: 30000 }
    );
    await page.waitForTimeout(500);

    const snap = await page.evaluate(() => fullLayoutSnapshot());
    if (!snap?.drumLayout?.isSmallScreen) {
      throw new Error(`${deviceName}: expected mobile profile; threshold/runtime drift`);
    }
    // So the landscape gate is actually exercised.
    if (!(snap.canvasWidth > snap.canvasHeight)) {
      throw new Error(`${deviceName}: expected landscape (w>h), got ${snap.canvasWidth}x${snap.canvasHeight}`);
    }

    // Input is blocked: the menu must NOT open.
    const overflow = await page.evaluate(() => (typeof fileMenuBtnRect === "function" ? fileMenuBtnRect() : null));
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
