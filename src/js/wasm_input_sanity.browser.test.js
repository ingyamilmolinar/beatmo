/**
 * WASM JS Bridge Sanity Test
 *
 * This test verifies that the WASM JS bridge functions work correctly.
 *
 * NOTE: The playtest harness (play_ui.wasm) doesn't run Ebiten's full game loop.
 * Instead, it calls g.Update() directly in a goroutine, which means Ebiten's
 * mouse/keyboard event handlers are NOT active. Mouse clicks dispatched to the
 * canvas don't reach the Go code.
 *
 * For actual input handling tests, see the Go unit tests in wasm_input_sanity_test.go
 * which use SetInputForTest to simulate input correctly.
 *
 * This test verifies:
 * 1. The JS bridge functions are exported and callable
 * 2. The functions correctly modify game state
 * 3. The UI state can be queried via JS
 */

import { chromium } from "playwright";
import { spawnSync } from "child_process";
import http from "http";
import fs from "fs";
import path from "path";
import { fileURLToPath } from "url";
import { assertSimpleDrawMode, resolveGoBinary, shouldSkipWasmBuild, flushCoverage, isCoverageEnabled } from "./browser_test_helpers.js";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const jsDir = __dirname;

// Ensure Chromium is installed only if missing.
const chromiumPath = path.join(jsDir, "node_modules", ".cache", "ms-playwright", "chromium");
if (!fs.existsSync(chromiumPath)) {
  spawnSync("npx", ["playwright", "install", "chromium"], { cwd: jsDir, stdio: "inherit" });
}

const goDir = path.resolve(jsDir, "../go");
const GO = resolveGoBinary();

// Build WASM
if (!shouldSkipWasmBuild("play_ui.wasm")) {
const build = spawnSync(GO, ["build", "-o", path.join(jsDir, "play_ui.wasm"), "./internal/ui/playtest"], {
  cwd: goDir,
  env: { ...process.env, GOOS: "js", GOARCH: "wasm" },
  stdio: "inherit"
});
if (build.status !== 0) throw new Error("go build play_ui failed");
}

// Start server
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
page.on('console', (msg) => { try { console.log('[PAGE]', msg.type(), msg.text()); } catch(_) {} });

await page.goto(`http://localhost:${port}/`);
await page.waitForFunction(() => typeof ensureDefaultPath === 'function');
await assertSimpleDrawMode(page, true, "wasm js bridge sanity");
await page.evaluate(() => ensureDefaultPath());

// Wait for the UI to be ready
await page.waitForFunction(() => typeof rowMuted === 'function' && typeof toggleMute === 'function');
await page.waitForTimeout(100); // Let UI settle

console.log('[TEST] Testing JS bridge functions...');

// Test 1: Toggle mute via JS bridge
console.log('[TEST] Testing toggleMute...');
const muteBefore = await page.evaluate(() => rowMuted(0));
await page.evaluate(() => toggleMute(0));
await page.waitForTimeout(100);
const muteAfter = await page.evaluate(() => rowMuted(0));
if (muteAfter === muteBefore) {
  throw new Error('toggleMute did not toggle mute state');
}
console.log('[TEST] toggleMute works! (' + muteBefore + ' -> ' + muteAfter + ')');

// Test 2: BPM functions
console.log('[TEST] Testing BPM functions...');
await page.waitForFunction(() => typeof getBPM === 'function' && typeof incrementBPM === 'function');
const bpmBefore = await page.evaluate(() => getBPM());
await page.evaluate(() => incrementBPM());
await page.waitForTimeout(100);
const bpmAfter = await page.evaluate(() => getBPM());
if (bpmAfter <= bpmBefore) {
  throw new Error('incrementBPM did not increment BPM: ' + bpmBefore + ' -> ' + bpmAfter);
}
console.log('[TEST] incrementBPM works! (' + bpmBefore + ' -> ' + bpmAfter + ')');

// Test 3: Start/stop playback
console.log('[TEST] Testing playback functions...');
await page.waitForFunction(() => typeof startPlay === 'function' && typeof stopPlay === 'function' && typeof transportSnapshot === 'function');

await page.evaluate(() => transportSnapshot().playing);
await page.evaluate(() => startPlay());
await page.waitForTimeout(200);
const playAfter = await page.evaluate(() => transportSnapshot().playing);
if (!playAfter) {
  throw new Error('startPlay() did not start playback');
}
console.log('[TEST] startPlay works! (playing=' + playAfter + ')');

await page.evaluate(() => stopPlay());
await page.waitForTimeout(100);
const playAfterStop = await page.evaluate(() => transportSnapshot().playing);
if (playAfterStop) {
  throw new Error('stopPlay() did not stop playback');
}
console.log('[TEST] stopPlay works! (playing=' + playAfterStop + ')');

// Test 4: Camera functions
console.log('[TEST] Testing camera functions...');
await page.waitForFunction(() => typeof camOffset === 'function' && typeof panBy === 'function');
const camBefore = await page.evaluate(() => camOffset());
await page.evaluate(() => panBy(50, 50));
await page.waitForTimeout(100);
const camAfter = await page.evaluate(() => camOffset());
if (camAfter.x === camBefore.x && camAfter.y === camBefore.y) {
  throw new Error('panBy did not move camera');
}
console.log('[TEST] panBy works! (x: ' + camBefore.x + ' -> ' + camAfter.x + ')');

console.log('[TEST] All JS bridge sanity checks passed!');

if (isCoverageEnabled()) await flushCoverage(page, new URL("../../coverage/browser-raw", import.meta.url).pathname, "wasm_input_sanity");
await browser.close();
server.close();
console.log('WASM JS bridge sanity test PASSED');
