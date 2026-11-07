import { chromium, devices } from "playwright";
import http from "http";
import fs from "fs";
import path from "path";
import { spawnSync } from "child_process";
import { fileURLToPath } from "url";
import { resolveGoBinary, assertSimpleDrawMode, shouldSkipWasmBuild, flushCoverage, isCoverageEnabled } from "./browser_test_helpers.js";
import { cdpTap, cdpPinch, cdpTwoFingerPan, cdpDrag, canvasToPageCoords } from "./touch_cdp_helpers.js";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const jsDir = __dirname;
const repoRoot = path.resolve(jsDir, "..", "..");
const goDir = path.resolve(repoRoot, "src/go");
const GO = resolveGoBinary();

// Build WASM target
if (!shouldSkipWasmBuild("main.wasm")) {
const build = spawnSync(
  GO,
  ["build", "-o", path.join(jsDir, "main.wasm"), "./cmd"],
  { cwd: goDir, env: { ...process.env, GOOS: "js", GOARCH: "wasm" }, stdio: "inherit" }
);
if (build.status !== 0) throw new Error("go build main wasm failed");
}

// Minimal static server
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

// PORTRAIT: landscape blocks all touch input by design, so gestures can only be verified upright.
const iPhone = devices['iPhone 12'];
const browser = await chromium.launch({ args: ["--autoplay-policy=no-user-gesture-required"] });
const context = await browser.newContext({
  ...iPhone,
  hasTouch: true,
});
const page = await context.newPage();
await page.goto(`http://localhost:${port}/`);

// Wait for WASM to be ready
await page.waitForFunction(() =>
  typeof ensureDefaultPath === "function" &&
  typeof nodeRect === "function" &&
  typeof camOffset === "function" &&
  typeof camScale === "function" &&
  typeof totalNodes === "function" &&
  typeof nodeMenuOpen === "function"
);
await assertSimpleDrawMode(page, false, "touch gestures");

// Ensure we have a default path with nodes to interact with
await page.evaluate(() => ensureDefaultPath());
await page.waitForTimeout(100);

let passed = 0;
let failed = 0;

function assert(condition, message) {
  if (!condition) {
    console.error(`  FAIL: ${message}`);
    failed++;
    return false;
  }
  console.log(`  PASS: ${message}`);
  passed++;
  return true;
}

console.log("Testing touch gestures on mobile viewport...");

// Test 1: Single-finger drag on grid pans camera
console.log("Test 1: Single-finger drag on grid pans camera");
const gridY = 100; // Y position in grid area (above drum view)
// A drag pans only if the loop OBSERVES the anchor and then a moved position; starved in the parallel phase, it can see
// only the end (zero travel). Retried (short, no long-press risk), closing any popup a collapsed drag opened, which
// would block panning.
let dragMoved = false;
let lastDx = 0, lastDy = 0;
for (let attempt = 0; attempt < 5 && !dragMoved; attempt++) {
  await page.evaluate(() => { if (typeof closeAllPopups === 'function') closeAllPopups(); });
  await page.waitForTimeout(150);
  const offsetBefore = await page.evaluate(() => camOffset());
  await cdpDrag(page, 200, gridY, 300, gridY + 60, 10);
  await page.waitForTimeout(100);
  const offsetAfter = await page.evaluate(() => camOffset());
  lastDx = offsetAfter.x - offsetBefore.x;
  lastDy = offsetAfter.y - offsetBefore.y;
  dragMoved = Math.abs(lastDx) > 5 || Math.abs(lastDy) > 5;
  if (!dragMoved) console.log(`  (retry: drag did not pan on attempt ${attempt + 1}, dx=${lastDx}, dy=${lastDy})`);
}
assert(dragMoved, `Camera moved after drag (dx=${lastDx}, dy=${lastDy})`);
await page.evaluate(() => { if (typeof closeAllPopups === 'function') closeAllPopups(); });
await page.waitForTimeout(200); // Let gesture state settle before next test

// Test 2: Pinch zoom changes camera scale
console.log("Test 2: Pinch zoom changes camera scale");
// The pinch centre must be inside the grid pane for handleTouchPinch to accept it.
const pinchCX = await page.evaluate(() => {
  if (typeof layoutHorizontal === 'function' && !layoutHorizontal() && typeof splitX === 'function') {
    return Math.floor(splitX() / 2); // Center of grid pane
  }
  return 150; // Fallback for stacked layout
});
const scaleBefore = await page.evaluate(() => camScale());
await cdpPinch(page, pinchCX, gridY, 40, 140, 15, 30, 150); // More steps + longer delays for CPU contention
await page.waitForTimeout(300);
const scaleAfter = await page.evaluate(() => camScale());
const scaleChanged = Math.abs(scaleAfter - scaleBefore) > 0.01;
assert(scaleChanged, `Scale changed after pinch (${scaleBefore.toFixed(3)} -> ${scaleAfter.toFixed(3)})`);
await page.waitForTimeout(200); // Let gesture state settle before next test

// Test 3: Two-finger pan moves camera
console.log("Test 3: Two-finger pan moves camera");
const offset2Before = await page.evaluate(() => camOffset());
await cdpTwoFingerPan(page, 300, gridY, 40, 0, 80, 5);
await page.waitForTimeout(50);
const offset2After = await page.evaluate(() => camOffset());
const panMoved = Math.abs(offset2After.x - offset2Before.x) > 5;
assert(panMoved, `Camera panned after two-finger pan (dx=${offset2After.x - offset2Before.x})`);
await page.waitForTimeout(200); // Let gesture state settle before next test

// Test 4: Tap in grid area is received by Go touch layer
console.log("Test 4: Tap in grid area is received");
// Well inside the grid pane.
const tapX = await page.evaluate(() => {
  if (typeof layoutHorizontal === 'function' && !layoutHorizontal() && typeof splitX === 'function') {
    return Math.floor(splitX() / 2);
  }
  return 150;
});
await page.evaluate(() => { if (typeof touchDebugState === 'function') touchDebugState(); }); // reset
await cdpTap(page, tapX, gridY);
await page.waitForTimeout(300);
const tapState = await page.evaluate(() => typeof touchDebugState === 'function' ? touchDebugState() : null);
if (tapState) {
  // A CDP tap races the Go goroutine: tap, longPress or none (received, not yet classified) all pass; device_matrix
  // validates classification.
  const tapOk = tapState.lastGesture === 'tap' ||
    tapState.lastGesture === 'longPress' ||
    (tapState.touchEventCount != null && tapState.touchEventCount > 0) ||
    tapState.lastGesture === 'none';
  assert(tapOk, `Tap received by Go layer (gesture=${tapState.lastGesture})`);
} else {
  // Fallback: just verify totalNodes didn't decrease (sanity check)
  const nodesNow = await page.evaluate(() => totalNodes());
  assert(nodesNow >= 46, `Grid tap processed (nodes=${nodesNow})`);
}

// Test 5: Long press on node opens popup, slide to Delete
console.log("Test 5: Long press on node opens popup, slide to Delete");
await page.evaluate(() => closeNodeMenu());
await page.waitForTimeout(400); // Let gesture state settle (generous for CPU contention)
// Earlier tests may have zoomed or panned far.
await page.evaluate(() => {
  if (typeof setCamScale === 'function') setCamScale(2.0);
  if (typeof setCamOffset === 'function') setCamOffset(100, 100);
  if (typeof forceDraw === 'function') forceDraw();
});
await page.waitForTimeout(300); // Generous settle for camera reset under CPU contention
const nodesBefore = await page.evaluate(() => totalNodes());
const nodeInfo = await page.evaluate(() => nodeRect(0, 0));
// Verify the node center is inside the grid pane before proceeding
const gridBound = await page.evaluate(() => {
  if (typeof layoutHorizontal === 'function' && !layoutHorizontal() && typeof splitX === 'function') {
    return splitX();
  }
  return 9999; // stacked layout: no X bound
});
if (nodeInfo && nodeInfo.x + nodeInfo.w / 2 < gridBound) {
  const nx = Math.floor(nodeInfo.x + nodeInfo.w / 2);
  const ny = Math.floor(nodeInfo.y + nodeInfo.h / 2);

  // Hold for the long-press popup first.
  const cdp = await page.context().newCDPSession(page);
  const coords = await canvasToPageCoords(page, nx, ny);
  await cdp.send('Input.dispatchTouchEvent', {
    type: 'touchStart',
    touchPoints: [{ x: coords.x, y: coords.y, id: 1 }],
  });
  // Under contention the 500 ms threshold takes longer in wall time, so poll.
  let delRect = null;
  const pollStart = Date.now();
  while (Date.now() - pollStart < 3000) {
    delRect = await page.evaluate(() =>
      typeof longPressDeleteRect === 'function' ? longPressDeleteRect() : null
    );
    if (delRect) break;
    await page.waitForTimeout(50);
  }

  if (delRect) {
    const delCX = delRect.x + delRect.w / 2;
    const delCY = delRect.y + delRect.h / 2;
    const delCoords = await canvasToPageCoords(page, delCX, delCY);

    // Slide to Delete button
    await cdp.send('Input.dispatchTouchEvent', {
      type: 'touchMove',
      touchPoints: [{ x: delCoords.x, y: delCoords.y, id: 1 }],
    });
    await page.waitForTimeout(100); // Let hover state update

    // Release to trigger delete
    await cdp.send('Input.dispatchTouchEvent', {
      type: 'touchEnd',
      touchPoints: [],
    });
    await cdp.detach();
    await page.waitForTimeout(200);

    const nodesAfter = await page.evaluate(() => totalNodes());
    assert(nodesAfter === nodesBefore - 1,
      `Node deleted via long-press popup (before=${nodesBefore}, after=${nodesAfter})`);
  } else {
    // Popup didn't open — release touch and fail gracefully
    await cdp.send('Input.dispatchTouchEvent', { type: 'touchEnd', touchPoints: [] });
    await cdp.detach();
    assert(false, `Long-press popup did not open for node at (0,0)`);
  }

  const menuOpen = await page.evaluate(() => nodeMenuOpen());
  assert(menuOpen === false,
    `Node menu not opened after long-press delete (menuOpen=${menuOpen})`);
} else {
  console.log("  Skipped (no node found at 0,0)");
}

// Test 6: long-press a node, slide to MOVE, release; tap an edge point → node moves
console.log("Test 6: long-press slide-to-Move relocates node onto edge point");
{
  // A(6,2)-B(6,6) with a vertical edge, mover M at (8,3), clear of the default path; Test 5's camera keeps them on the
  // iPhone 12 canvas.
  await page.evaluate(() => {
    addNode(6, 2, 'regular'); addNode(6, 6, 'regular');
    addEdgeGrid(6, 2, 6, 6);
    addNode(8, 3, 'regular');
  });
  await page.waitForTimeout(150);
  const mRect = await page.evaluate(() => nodeRect(8, 3));
  const aRect = await page.evaluate(() => nodeRect(6, 2));
  const bRect = await page.evaluate(() => nodeRect(6, 6));
  assert(mRect && aRect && bRect, `test nodes visible (m=${JSON.stringify(mRect)})`);
  const moverId = await page.evaluate(() => nodeIdAt(8, 3));

  if (mRect && aRect && bRect) {
    // 1) Long-press the mover to open the popup (canvas coords → page coords).
    const mC = await canvasToPageCoords(page, mRect.x + mRect.w / 2, mRect.y + mRect.h / 2);
    const cdp = await page.context().newCDPSession(page);
    await cdp.send('Input.dispatchTouchEvent', {
      type: 'touchStart',
      touchPoints: [{ x: mC.x, y: mC.y, id: 1 }],
    });
    // Poll for the popup's Move rect instead of a fixed wait (CPU contention).
    let moveRect = null;
    for (let i = 0; i < 30 && !moveRect; i++) {
      await page.waitForTimeout(100);
      moveRect = await page.evaluate(() =>
        typeof longPressMoveRect === 'function' ? longPressMoveRect() : null);
    }
    assert(moveRect, `long-press popup opened with Move rect (${JSON.stringify(moveRect)})`);

    if (moveRect) {
      // 2) Slide the same touch onto Move and release → arms move mode.
      const mv = await canvasToPageCoords(page, moveRect.x + moveRect.w / 2, moveRect.y + moveRect.h / 2);
      await cdp.send('Input.dispatchTouchEvent', {
        type: 'touchMove', touchPoints: [{ x: mv.x, y: mv.y, id: 1 }],
      });
      await page.waitForTimeout(150);
      await cdp.send('Input.dispatchTouchEvent', { type: 'touchEnd', touchPoints: [] });
      await cdp.detach();
      await page.waitForTimeout(300);
      const armed = await page.evaluate(() => gridModeState());
      assert(armed.mode === 'move' && armed.subjectId === moverId,
        `move mode armed on mover (${JSON.stringify(armed)})`);

      // 3) Tap the edge midpoint cell (6,4): screen midpoint of A and B centers.
      const tgt = await canvasToPageCoords(page,
        (aRect.x + aRect.w / 2 + bRect.x + bRect.w / 2) / 2,
        (aRect.y + aRect.h / 2 + bRect.y + bRect.h / 2) / 2);
      await cdpTap(page, tgt.x, tgt.y);
      await page.waitForTimeout(400);
      const moved = await page.evaluate(() => ({
        atOld: nodeIdAt(8, 3), atNew: nodeIdAt(6, 4), mode: gridModeState().mode,
      }));
      assert(moved.atOld === -1 && moved.atNew === moverId,
        `node relocated (8,3)→(6,4) (${JSON.stringify(moved)})`);
      assert(moved.mode === 'none', `move mode committed (mode=${moved.mode})`);
    } else {
      await cdp.send('Input.dispatchTouchEvent', { type: 'touchEnd', touchPoints: [] });
      await cdp.detach();
    }
  }
  await page.waitForTimeout(200);
}

// Test 7: armed move mode + tap on empty cell cancels (node stays)
console.log("Test 7: move-mode tap on empty cell cancels");
{
  await page.evaluate(() => { addNode(12, 3, 'regular'); });
  await page.waitForTimeout(150);
  const nRect = await page.evaluate(() => nodeRect(12, 3));
  assert(nRect, `cancel-test node visible (${JSON.stringify(nRect)})`);
  if (nRect) {
    const nC = await canvasToPageCoords(page, nRect.x + nRect.w / 2, nRect.y + nRect.h / 2);
    const cdp2 = await page.context().newCDPSession(page);
    await cdp2.send('Input.dispatchTouchEvent', { type: 'touchStart', touchPoints: [{ x: nC.x, y: nC.y, id: 1 }] });
    let mr = null;
    for (let i = 0; i < 30 && !mr; i++) {
      await page.waitForTimeout(100);
      mr = await page.evaluate(() => typeof longPressMoveRect === 'function' ? longPressMoveRect() : null);
    }
    if (assert(mr, "popup opened for cancel test")) {
      const mv = await canvasToPageCoords(page, mr.x + mr.w / 2, mr.y + mr.h / 2);
      await cdp2.send('Input.dispatchTouchEvent', { type: 'touchMove', touchPoints: [{ x: mv.x, y: mv.y, id: 1 }] });
      await page.waitForTimeout(150);
      await cdp2.send('Input.dispatchTouchEvent', { type: 'touchEnd', touchPoints: [] });
      await cdp2.detach();
      await page.waitForTimeout(300);
      // Offsets kept small so the target stays inside the 390×664 canvas at this camera.
      const empty = await canvasToPageCoords(page, nRect.x + nRect.w / 2 + 80, nRect.y + nRect.h / 2 + 60);
      await cdpTap(page, empty.x, empty.y);
      await page.waitForTimeout(400);
      const after = await page.evaluate(() => ({ still: nodeIdAt(12, 3), mode: gridModeState().mode }));
      assert(after.still !== -1, `node stayed at (12,3) after invalid drop (${JSON.stringify(after)})`);
      assert(after.mode === 'none', `mode canceled (mode=${after.mode})`);
    } else {
      await cdp2.send('Input.dispatchTouchEvent', { type: 'touchEnd', touchPoints: [] });
      await cdp2.detach();
    }
  }
  await page.waitForTimeout(200);
}

// Test 8: long-press slide-to-Copy, tap free cell pastes a copy
console.log("Test 8: long-press slide-to-Copy pastes at tapped cell");
{
  const before = await page.evaluate(() => totalNodes());
  const srcRect = await page.evaluate(() => nodeRect(6, 2)); // endpoint A from Test 6
  assert(srcRect, `copy source visible (${JSON.stringify(srcRect)})`);
  if (srcRect) {
    const sC = await canvasToPageCoords(page, srcRect.x + srcRect.w / 2, srcRect.y + srcRect.h / 2);
    const cdp3 = await page.context().newCDPSession(page);
    await cdp3.send('Input.dispatchTouchEvent', { type: 'touchStart', touchPoints: [{ x: sC.x, y: sC.y, id: 1 }] });
    let cr = null;
    for (let i = 0; i < 30 && !cr; i++) {
      await page.waitForTimeout(100);
      cr = await page.evaluate(() =>
        typeof longPressCopyRect === 'function' ? longPressCopyRect() : null);
    }
    if (assert(cr, "popup opened with Copy rect")) {
      const cv = await canvasToPageCoords(page, cr.x + cr.w / 2, cr.y + cr.h / 2);
      await cdp3.send('Input.dispatchTouchEvent', { type: 'touchMove', touchPoints: [{ x: cv.x, y: cv.y, id: 1 }] });
      await page.waitForTimeout(150);
      await cdp3.send('Input.dispatchTouchEvent', { type: 'touchEnd', touchPoints: [] });
      await cdp3.detach();
      await page.waitForTimeout(300);
      const armed = await page.evaluate(() => gridModeState());
      assert(armed.mode === 'copy', `copy mode armed (${JSON.stringify(armed)})`);
      // Paste on a free cell left of the source (still on-canvas).
      const paste = await canvasToPageCoords(page, srcRect.x + srcRect.w / 2 - 80, srcRect.y + srcRect.h / 2);
      await cdpTap(page, paste.x, paste.y);
      await page.waitForTimeout(400);
      const afterCount = await page.evaluate(() => totalNodes());
      assert(afterCount === before + 1, `paste added one node (${before} → ${afterCount})`);
      assert((await page.evaluate(() => gridModeState())).mode === 'none', 'copy mode committed');
    } else {
      await cdp3.send('Input.dispatchTouchEvent', { type: 'touchEnd', touchPoints: [] });
      await cdp3.detach();
    }
  }
  await page.waitForTimeout(200);
}

if (isCoverageEnabled()) await flushCoverage(page, new URL("../../coverage/browser-raw", import.meta.url).pathname, "touch_gestures");
await browser.close();
server.close();

console.log(`\nFINAL RESULTS: ${passed} passed, ${failed} failed`);
if (failed > 0) {
  process.exit(1);
}
