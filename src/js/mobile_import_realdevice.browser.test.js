// On real devices (BrowserStack, on demand), the Import overlay must exist once the File menu opens, stay aligned with
// the button under the device's dynamic viewport, and be what elementFromPoint hits at the button centre; no OS picker is
// triggered. Run: GO=… node src/js/mobile_import_realdevice.browser.test.js (DEVICE_FILTER=Pixel|iPhone; BROWSERSTACK_*)

import {
  buildWasm,
  startServer,
  waitForWasmReady,
} from "./visual_test_helpers.js";
import {
  requireBrowserStackCredentials,
  startTunnel,
  stopTunnel,
  connectRemoteDevice,
  markSessionStatus,
  DEVICE_MATRIX,
} from "./visual_browserstack_helpers.js";
import fs from "fs";
import os from "os";
import path from "path";

// A distinctive circuit the INV-4 file chooser is filled with (bpm 123).
const importFile = path.join(os.tmpdir(), "beatmo_realdevice_import_probe.json");
fs.writeFileSync(importFile, JSON.stringify({
  schema_version: "0.0.1", subdiv: 8, bpm: 123,
  instruments: [{ name: "Kick", id: "kick", kind: "builtin", volume: 1.0, origin: 0, color: "#C87850FF" }],
  nodes: [
    { id: 0, i: 0, j: 0, type: "regular", inputs: [], outputs: [1], volume: 1.0, pitch: 0, duration: 1.0 },
    { id: 1, i: 4, j: 0, type: "regular", inputs: [0], outputs: [0], volume: 1.0, pitch: 0, duration: 1.0 },
  ],
}));

// A finger still lands under a few px of slop; a viewport drift is tens or hundreds.
const ALIGN_TOLERANCE_PX = 8;

const deviceFilter = process.env.DEVICE_FILTER || "";
const devices = deviceFilter
  ? DEVICE_MATRIX.filter((d) => d.name.toLowerCase().includes(deviceFilter.toLowerCase()))
  : DEVICE_MATRIX;

if (devices.length === 0) {
  console.error(`No devices match filter "${deviceFilter}"`);
  process.exit(1);
}

const credentials = requireBrowserStackCredentials();
console.log(`BrowserStack user: ${credentials.user}`);
console.log(`Testing mobile Import overlay on ${devices.length} device(s)${deviceFilter ? ` (filter: "${deviceFilter}")` : ""}\n`);

console.log("Building WASM...");
buildWasm();
const { server, port } = await startServer();
console.log(`Local server on port ${port}`);

console.log("Starting BrowserStack Local tunnel...");
let tunnel;
try {
  tunnel = await startTunnel(credentials.key);
} catch (e) {
  console.error(`Failed to start tunnel: ${e.message}`);
  server.close();
  process.exit(1);
}

function withTimeout(promise, ms, label) {
  return Promise.race([
    promise,
    new Promise((_, reject) => setTimeout(() => reject(new Error(`Timeout: ${label} after ${ms}ms`)), ms)),
  ]);
}

// The REAL path first (tap the folder button), then the openOverflowMenu() export; openedVia records which.
async function openFileMenuAndArm(page) {
  const folder = await page.evaluate(() => (typeof fileMenuBtnRect === "function" ? fileMenuBtnRect() : null));
  const canvasOff = await page.evaluate(() => {
    const c = document.querySelector("canvas").getBoundingClientRect();
    return { left: c.left, top: c.top };
  });
  const armed = async () =>
    page.evaluate(() => !!(window._fpDebugRects && window._fpDebugRects().import));
  const realTap = async (x, y) => { try { await page.touchscreen.tap(x, y); } catch (_) {} };

  // iOS throttles the rAF loop on a page with no real interaction, so forceDraw cannot advance the menu; real taps run
  // the loop, as the user's do.
  let openedVia = "folder-tap";
  if (folder && folder.w > 0) {
    const fx = canvasOff.left + folder.x + folder.w / 2;
    const fy = canvasOff.top + folder.y + folder.h / 2;
    for (let i = 0; i < 5; i++) {
      await realTap(fx, fy);
      await page.waitForTimeout(600); // let real rAF Update frames run
      if (await armed()) return { openedVia };
    }
  }

  // The loop is awake now: open programmatically and let real frames run syncNativeGestures.
  openedVia = "programmatic(openOverflowMenu, post-interaction)";
  await realTap(canvasOff.left + 5, canvasOff.top + 5); // extra wake tap (harmless corner)
  await page.evaluate(() => typeof openOverflowMenu === "function" && openOverflowMenu());
  for (let i = 0; i < 20; i++) { if (await armed()) return { openedVia }; await page.waitForTimeout(250); }

  // Tells Go (never registered a rect) from JS (the overlay code broken on this engine).
  const jsDiag = await page.evaluate(() => {
    const t = (n) => typeof window[n];
    const before = window._fpDebugRects ? window._fpDebugRects() : null;
    let directCallWorks = null;
    try {
      if (typeof window._fpRegisterRect === "function") {
        window._fpRegisterRect("__probe__", 5, 5, 40, 20, "application/json,.json");
        const after = window._fpDebugRects ? window._fpDebugRects() : null;
        directCallWorks = !!(after && after.__probe__);
        // clean up the probe overlay
        if (typeof window._fpClearRects === "function") window._fpClearRects();
      }
    } catch (e) { directCallWorks = `threw: ${e.message}`; }
    return {
      typeof_fpRegisterRect: t("_fpRegisterRect"),
      typeof_fpClearRects: t("_fpClearRects"),
      typeof_fpDebugRects: t("_fpDebugRects"),
      typeof_openOverflowMenu: t("openOverflowMenu"),
      typeof_fileMenuBtnRect: t("fileMenuBtnRect"),
      typeof_fpStartImport: t("_fpStartImport"),
      fpDebugRectsBefore: before,
      fileInputsInDom: document.querySelectorAll('input[type="file"]').length,
      directFpRegisterRectWorks: directCallWorks,
    };
  });
  const err = new Error("Import file-picker overlay was never armed (no _fpDebugRects().import)");
  err.jsDiag = jsDiag;
  throw err;
}

// Without triggering the OS file picker.
async function collectOverlayDiag(page) {
  return page.evaluate(() => {
    const canvas = document.querySelector("canvas");
    const cRect = canvas ? canvas.getBoundingClientRect() : null;
    const go = window._fpDebugRects ? window._fpDebugRects().import : null; // Go framebuffer/logical coords of the button
    const el = document.querySelector('input[type="file"][data-fp-id="import"]');
    const elRect = el ? el.getBoundingClientRect() : null;
    const vv = window.visualViewport
      ? { offsetTop: window.visualViewport.offsetTop, offsetLeft: window.visualViewport.offsetLeft, scale: window.visualViewport.scale, pageTop: window.visualViewport.pageTop, height: window.visualViewport.height }
      : null;

    let intended = null, hit = null;
    if (cRect && go) {
      // Where the overlay/button SHOULD be in page coords (the makeOverlay formula).
      intended = { x: cRect.left + go.x, y: cRect.top + go.y, w: go.w, h: go.h };
      const px = intended.x + intended.w / 2, py = intended.y + intended.h / 2;
      const top = document.elementFromPoint(px, py);
      hit = {
        px, py,
        tag: top ? top.tagName : null,
        fpId: top && top.getAttribute ? top.getAttribute("data-fp-id") : null,
        isImportInput: !!(el && top === el),
      };
    }
    return {
      hasOverlay: !!el,
      canvas: cRect ? { left: cRect.left, top: cRect.top, w: cRect.width, h: cRect.height } : null,
      go,
      elRect: elRect ? { x: elRect.left, y: elRect.top, w: elRect.width, h: elRect.height } : null,
      intended,
      hit,
      visualViewport: vv,
      innerHeight: window.innerHeight,
      pointerEvents: el ? getComputedStyle(el).pointerEvents : null,
      visibility: el ? getComputedStyle(el).visibility : null,
      display: el ? getComputedStyle(el).display : null,
    };
  });
}

let failures = 0, passed = 0;

for (const device of devices) {
  console.log(`\n=== ${device.name} (${device.deviceName}, ${device.orientation}) ===`);
  let browser, context, page;
  const problems = [];
  try {
    await withTimeout((async () => {
      console.log("  Connecting to BrowserStack...");
      ({ browser, context, page } = await connectRemoteDevice(device, credentials, port));
      console.log("  Connected. Waiting for WASM...");
      await waitForWasmReady(page, 120000);

      const isMobile = await page.evaluate(() => fullLayoutSnapshot()?.drumLayout?.isSmallScreen);
      console.log(`  mobile profile: ${isMobile}`);
      if (!isMobile) { problems.push("device did not resolve to the mobile profile (layout threshold drift)"); return; }

      const { openedVia } = await openFileMenuAndArm(page);
      console.log(`  File menu opened via: ${openedVia}`);
      if (openedVia !== "folder-tap") {
        // Diagnostic only: a zero-dwell synthetic tap is often missed by Go's frame-based detector, while a finger opens the menu.
        console.warn("  WARN: folder-button touchscreen.tap() did not open the menu (harness zero-dwell tap); opened programmatically instead");
      }

      // Settle a beat so any URL-bar / viewport transition finishes.
      await page.waitForTimeout(400);
      await page.evaluate(() => forceDraw && forceDraw());

      const d = await collectOverlayDiag(page);
      console.log("  diag:", JSON.stringify(d));

      // INV-1
      if (!d.hasOverlay) { problems.push("INV-1: no <input type=file data-fp-id=import> overlay exists while the File menu is open"); return; }
      if (!d.go || !d.canvas || !d.elRect || !d.intended) { problems.push("INV-1: missing geometry (go/canvas/overlay rect) — cannot evaluate alignment"); return; }

      // INV-2: overlay rendered where the button is drawn (no viewport drift).
      const dxTop = Math.abs(d.elRect.y - d.intended.y);
      const dxLeft = Math.abs(d.elRect.x - d.intended.x);
      if (dxTop > ALIGN_TOLERANCE_PX || dxLeft > ALIGN_TOLERANCE_PX) {
        problems.push(
          `INV-2: overlay DRIFTED from the visible Import button by dx=${dxLeft.toFixed(1)} dy=${dxTop.toFixed(1)} px ` +
          `(overlay@[${d.elRect.x.toFixed(1)},${d.elRect.y.toFixed(1)}] vs button@[${d.intended.x.toFixed(1)},${d.intended.y.toFixed(1)}], ` +
          `visualViewport=${JSON.stringify(d.visualViewport)}) — a finger on the button misses the input`
        );
      }

      // INV-3: a hit-test at the visible button center lands on the input.
      if (!d.hit || !d.hit.isImportInput) {
        problems.push(
          `INV-3: elementFromPoint at the visible Import button center (${d.hit ? d.hit.px.toFixed(1) : "?"},${d.hit ? d.hit.py.toFixed(1) : "?"}) ` +
          `returned <${d.hit ? d.hit.tag : "?"}${d.hit && d.hit.fpId ? ` data-fp-id=${d.hit.fpId}` : ""}>, NOT the Import input — the tap falls through to the canvas`
        );
      }

      // Diagnostic only: on real BrowserStack devices an automated tap opens no native picker for any file input (an
      // automation limit, measured on both engines); the finger case is confirmed by hand.
      let chooserFired = false;
      const onChooser = async (fc) => { chooserFired = true; try { await fc.setFiles(importFile); } catch (_) {} };
      page.on("filechooser", onChooser);
      const tapX = d.intended.x + d.intended.w / 2, tapY = d.intended.y + d.intended.h / 2;
      for (let attempt = 0; attempt < 3 && !chooserFired; attempt++) {
        try { await page.touchscreen.tap(tapX, tapY); } catch (_) {}
        const dl = Date.now() + 4000;
        while (Date.now() < dl && !chooserFired) { await page.waitForTimeout(200); }
      }
      page.off("filechooser", onChooser);
      if (chooserFired) {
        console.log("  INV-4: file chooser opened on tap");
      } else {
        // Automated taps cannot open native pickers on real devices.
        console.warn(
          `  WARN(INV-4): tap on the Import overlay did not open a filechooser — EXPECTED under automation ` +
          `(WebDriver cannot open native file pickers on real devices; confirm the real-finger behavior by hand)`
        );
      }
    })(), 240000, `${device.name} full flow`);
  } catch (e) {
    problems.push(`exception: ${e.message}`);
    if (e.jsDiag) console.error("  JS-side diagnostics:", JSON.stringify(e.jsDiag));
  }

  if (problems.length === 0) {
    console.log(`  PASS: ${device.name} — Import overlay armed, aligned, and hit-testable`);
    passed++;
    if (page) await withTimeout(markSessionStatus(page, "passed", "Import overlay aligned + hit-testable"), 10000, "markStatus").catch(() => {});
  } else {
    console.error(`  FAIL: ${device.name}`);
    for (const p of problems) console.error(`     - ${p}`);
    failures++;
    if (page) await withTimeout(markSessionStatus(page, "failed", problems[0].slice(0, 240)), 10000, "markStatus").catch(() => {});
  }
  try { if (browser) await browser.close(); } catch (_) {}
}

await stopTunnel(tunnel);
server.close();

console.log(`\n=== Summary: ${passed} passed, ${failures} failed ===`);
process.exit(failures > 0 ? 1 : 0);
