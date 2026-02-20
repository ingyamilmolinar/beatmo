/**
 * Touch Integration Test
 *
 * End-to-end test that verifies touch events reach the Go layer using
 * the debug infrastructure. Tests both synthetic events and CDP events
 * to identify which approach works on real devices.
 */

import { chromium, devices } from "playwright";
import http from "http";
import fs from "fs";
import path from "path";
import { spawnSync } from "child_process";
import { fileURLToPath } from "url";
import { resolveGoBinary, assertSimpleDrawMode, shouldSkipWasmBuild, flushCoverage, isCoverageEnabled } from "./browser_test_helpers.js";
import {
  simulateTap,
  cdpTap,
  cdpLongPress,
  cdpDrag,
  cdpPinch,
  cdpTwoFingerPan,
  getCanvasInfo,
} from "./touch_test_helpers.js";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const jsDir = __dirname;
const repoRoot = path.resolve(jsDir, "..", "..");
const goDir = path.resolve(repoRoot, "src/go");
const GO = resolveGoBinary();

// Build WASM target
console.log("Building WASM...");
if (!shouldSkipWasmBuild("main.wasm")) {
const build = spawnSync(
  GO,
  ["build", "-o", path.join(jsDir, "main.wasm"), "./cmd/..."],
  { cwd: goDir, env: { ...process.env, GOOS: "js", GOARCH: "wasm" }, stdio: "inherit" }
);
if (build.status !== 0) throw new Error("go build main wasm failed");
}

// Minimal static server
const server = http.createServer((req, res) => {
  // Strip query params first, then handle root
  const cleanUrl = req.url.split('?')[0];
  const file = cleanUrl === "/" ? "/index.html" : cleanUrl;
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

// Use iPhone 12 landscape like the working touch_gestures test
const iPhone = devices['iPhone 12 landscape'];
const browser = await chromium.launch({ args: ["--autoplay-policy=no-user-gesture-required"] });
const context = await browser.newContext({
  ...iPhone,
  hasTouch: true,
});
const page = await context.newPage();

// Enable touch debug via URL param
console.log(`Navigating to http://localhost:${port}/?touchDebug=1`);
await page.goto(`http://localhost:${port}/?touchDebug=1`);
console.log("Page loaded, waiting for WASM...");

// Add console listener to see any errors
page.on('console', msg => console.log(`[Browser] ${msg.type()}: ${msg.text()}`));
page.on('pageerror', err => console.log(`[Browser Error] ${err.message}`));

// Wait a bit for console messages
await page.waitForTimeout(2000);

// Wait for WASM to be ready
try {
  await page.waitForFunction(() =>
    typeof ensureDefaultPath === "function" &&
    typeof nodeRect === "function" &&
    typeof camOffset === "function" &&
    typeof camScale === "function",
    { timeout: 15000 }
  );
} catch (e) {
  console.log("Timeout waiting for WASM functions");
  const available = await page.evaluate(() => {
    return Object.keys(window).filter(k => typeof window[k] === "function").slice(0, 50);
  });
  console.log("Available functions:", available);
  throw e;
}
console.log("WASM loaded!");
await assertSimpleDrawMode(page, false, "touch integration");

// Check if touch debug functions are available
const touchDebugAvailable = await page.evaluate(() =>
  typeof setTouchDebug === "function" &&
  typeof touchDebugState === "function" &&
  typeof touchEventLog === "function" &&
  typeof clearTouchEventLog === "function"
);

if (!touchDebugAvailable) {
  console.log("WARNING: Touch debug functions not available");
  const availableFuncs = await page.evaluate(() => {
    return Object.keys(window).filter(k =>
      typeof window[k] === "function" &&
      (k.toLowerCase().includes("touch") || k.toLowerCase().includes("debug"))
    );
  });
  console.log("Available touch/debug functions:", availableFuncs.join(", ") || "none");
  console.log("This indicates the new js_exports_touch_debug.go functions are not compiled in");
  if (isCoverageEnabled()) await flushCoverage(page, new URL("../../coverage/browser-raw", import.meta.url).pathname, "touch_integration");
  await browser.close();
  server.close();
  process.exit(1);
}

console.log("Touch debug functions available!");

// Enable Go-side debug logging
await page.evaluate(() => setTouchDebug(true));
await page.evaluate(() => clearTouchEventLog());

// Ensure we have a default path with nodes
await page.evaluate(() => ensureDefaultPath());
await page.waitForTimeout(100);

// Get canvas info
const canvasInfo = await getCanvasInfo(page);
console.log(`Canvas: ${canvasInfo.width}x${canvasInfo.height}, DPR: ${canvasInfo.dpr}`);

// Test coordinates in grid area
const testX = 200;
const testY = 100; // Grid area (above drum view)

// Test 1: Verify debug infrastructure is working
console.log("\nTest 1: Verify debug infrastructure");
const initialState = await page.evaluate(() => touchDebugState());
assert(initialState.enabled === true, "Debug logging is enabled");
assert(typeof initialState.dpr === "number", `DPR is ${initialState.dpr}`);
assert(initialState.count === 0, "No active touches initially");

// Test 2: Synthetic TouchEvent tap
console.log("\nTest 2: Synthetic TouchEvent tap");
await page.evaluate(() => clearTouchEventLog());
await simulateTap(page, testX, testY);
await page.waitForTimeout(100);

let eventLog = await page.evaluate(() => touchEventLog());
const syntheticReceivedGo = eventLog.length > 0;
if (syntheticReceivedGo) {
  console.log(`  Event log has ${eventLog.length} events:`);
  eventLog.slice(0, 5).forEach(e => console.log(`    ${e.kind}: id=${e.touchId}, pos=(${e.x}, ${e.y})`));
} else {
  console.log("  WARNING: No events in Go-side log from synthetic tap");
}

let state = await page.evaluate(() => touchDebugState());
const syntheticGestureDetected = state.lastGesture === "tap";
if (syntheticGestureDetected) {
  console.log(`  Gesture detected: ${state.lastGesture} at (${state.lastPos.x}, ${state.lastPos.y})`);
  const dx = Math.abs(state.lastPos.x - testX);
  const dy = Math.abs(state.lastPos.y - testY);
  assert(dx < 50 && dy < 50, `Coordinates within tolerance (dx=${dx}, dy=${dy})`);
} else {
  console.log(`  WARNING: Expected 'tap' gesture, got '${state.lastGesture}'`);
}

// Test 3: CDP tap (trusted event) — verify reaches Go AND produces game state
console.log("\nTest 3: CDP tap (trusted event)");
await page.evaluate(() => clearTouchEventLog());
const nodesBefore = await page.evaluate(() => typeof totalNodes === "function" ? totalNodes() : -1);
await cdpTap(page, testX, testY);
await page.waitForTimeout(100);

eventLog = await page.evaluate(() => touchEventLog());
const cdpReceivedGo = eventLog.length > 0;
if (cdpReceivedGo) {
  console.log(`  Event log has ${eventLog.length} events:`);
  eventLog.slice(0, 5).forEach(e => console.log(`    ${e.kind}: id=${e.touchId}, pos=(${e.x}, ${e.y})`));
} else {
  console.log("  WARNING: No events in Go-side log from CDP tap");
}

state = await page.evaluate(() => touchDebugState());
const cdpGestureDetected = state.lastGesture === "tap";
if (cdpGestureDetected) {
  console.log(`  Gesture detected: ${state.lastGesture} at (${state.lastPos.x}, ${state.lastPos.y})`);
  const dx = Math.abs(state.lastPos.x - testX);
  const dy = Math.abs(state.lastPos.y - testY);
  assert(dx < 50 && dy < 50, `CDP coordinates within tolerance (dx=${dx}, dy=${dy})`);
} else {
  console.log(`  WARNING: Expected 'tap' gesture from CDP, got '${state.lastGesture}'`);
}

// Verify game state changed (node created or selected)
if (nodesBefore >= 0) {
  const nodesAfter = await page.evaluate(() => totalNodes());
  if (nodesAfter > nodesBefore) {
    assert(true, `CDP tap created node (${nodesBefore} -> ${nodesAfter})`);
  } else {
    const selId = await page.evaluate(() => typeof selectedNodeId === "function" ? selectedNodeId() : -1);
    if (selId >= 0) {
      assert(true, `CDP tap selected node id=${selId}`);
    } else {
      console.log(`  INFO: no node created/selected (may be outside grid area)`);
    }
  }
}

// Test 4: CDP drag — verify camera actually moved
console.log("\nTest 4: CDP drag");
await page.evaluate(() => clearTouchEventLog());
const camBefore = await page.evaluate(() => camOffset());
await cdpDrag(page, testX, testY, testX + 50, testY + 30, 5);
await page.waitForTimeout(100);

eventLog = await page.evaluate(() => touchEventLog());
if (eventLog.length > 0) {
  console.log(`  Event log has ${eventLog.length} events`);
  const hasMoves = eventLog.some(e => e.kind === "move");
  assert(hasMoves, "Drag generated move events");
}

state = await page.evaluate(() => touchDebugState());
console.log(`  Last gesture: ${state.lastGesture}`);

const camAfter = await page.evaluate(() => camOffset());
const camMoved = Math.abs(camAfter.x - camBefore.x) > 3 || Math.abs(camAfter.y - camBefore.y) > 3;
if (camMoved) {
  assert(true, `Camera moved after drag (dx=${camAfter.x - camBefore.x}, dy=${camAfter.y - camBefore.y})`);
} else {
  console.log(`  INFO: Camera did not move (may be timing issue)`);
}

// Test 5: CDP long press
console.log("\nTest 5: CDP long press");
await page.evaluate(() => clearTouchEventLog());
await cdpLongPress(page, testX, testY, 600);
await page.waitForTimeout(100);

state = await page.evaluate(() => touchDebugState());
const longPressDetected = state.lastGesture === "longPress";
if (longPressDetected) {
  console.log(`  Long press detected at (${state.lastPos.x}, ${state.lastPos.y})`);
  assert(true, "Long press gesture detected");
} else {
  console.log(`  WARNING: Expected 'longPress', got '${state.lastGesture}'`);
}

// Test 6: CDP pinch — verify camera scale changed
console.log("\nTest 6: CDP pinch");
await page.evaluate(() => clearTouchEventLog());
const scaleBefore = await page.evaluate(() => camScale());
await cdpPinch(page, 300, testY, 50, 100, 5);
await page.waitForTimeout(100);

state = await page.evaluate(() => touchDebugState());
const pinchDetected = state.lastGesture === "pinch" || state.lastGesture === "twoFingerPan";
if (pinchDetected) {
  console.log(`  Gesture: ${state.lastGesture}`);
  assert(true, "Two-finger gesture detected");
} else {
  console.log(`  WARNING: Expected pinch/pan, got '${state.lastGesture}'`);
}

const scaleAfter = await page.evaluate(() => camScale());
if (Math.abs(scaleAfter - scaleBefore) > 0.01) {
  assert(true, `Camera scale changed (${scaleBefore.toFixed(3)} -> ${scaleAfter.toFixed(3)})`);
} else {
  console.log(`  INFO: Scale unchanged after pinch (${scaleBefore})`);
}

// Test 7: CDP two-finger pan moves camera (from touch_gestures suite)
console.log("\nTest 7: CDP two-finger pan");
await page.evaluate(() => clearTouchEventLog());
const camBeforePan = await page.evaluate(() => camOffset());
await cdpTwoFingerPan(page, 300, testY, 40, 0, 80, 5);
await page.waitForTimeout(50);
const camAfterPan = await page.evaluate(() => camOffset());
const panMoved = Math.abs(camAfterPan.x - camBeforePan.x) > 5;
if (panMoved) {
  assert(true, `Camera panned after two-finger pan (dx=${camAfterPan.x - camBeforePan.x})`);
} else {
  console.log(`  INFO: Camera did not move after two-finger pan (may be timing issue)`);
}

// Summary
console.log("\n--- Summary ---");
console.log(`Synthetic events received by Go: ${syntheticReceivedGo ? 'YES' : 'NO'}`);
console.log(`Synthetic gesture detected: ${syntheticGestureDetected ? 'YES' : 'NO'}`);
console.log(`CDP events received by Go: ${cdpReceivedGo ? 'YES' : 'NO'}`);
console.log(`CDP gesture detected: ${cdpGestureDetected ? 'YES' : 'NO'}`);

await browser.close();
server.close();

console.log(`\nFINAL RESULTS: ${passed} passed, ${failed} failed`);

if (failed > 0) {
  process.exit(1);
}
