import { chromium } from "playwright";
import http from "http";
import fs from "fs";
import path from "path";
import { spawnSync } from "child_process";
import { fileURLToPath } from "url";
import { resolveGoBinary, assertSimpleDrawMode, shouldSkipWasmBuild, flushCoverage, isCoverageEnabled } from "./browser_test_helpers.js";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const jsDir = __dirname;
const repoRoot = path.resolve(jsDir, "..", "..");
const goDir = path.resolve(repoRoot, "src/go");
const GO = resolveGoBinary();

// Build WASM target
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

const browser = await chromium.launch({ args: ["--autoplay-policy=no-user-gesture-required"] });

async function setupPage(width, height) {
  const page = await browser.newPage({ viewport: { width, height } });
  await page.goto(`http://localhost:${port}/`);
  await page.waitForFunction(() =>
    typeof drumBounds === "function" &&
    typeof forceDraw === "function" &&
    typeof layoutHorizontal === "function" &&
    typeof splitY === "function"
  );
  await assertSimpleDrawMode(page, false, `orientation ${width}x${height}`);
  await page.evaluate(() => forceDraw?.());
  return page;
}

// Test 1: Landscape -> stacked (mobile always uses stacked layout)
console.log("Test 1: Landscape -> stacked");
{
  const page = await setupPage(844, 390);
  await page.evaluate(() => forceDraw?.());
  await page.waitForTimeout(200);

  const bounds = await page.evaluate(() => drumBounds?.());
  const horiz = await page.evaluate(() => layoutHorizontal?.());

  console.log(`  drumBounds: x=${bounds.x} y=${bounds.y} w=${bounds.w} h=${bounds.h}, horizontal=${horiz}`);

  if (horiz !== true) {
    throw new Error(`Expected layoutHorizontal=true for landscape (always stacked), got ${horiz}`);
  }
  if (bounds.x !== 0) {
    throw new Error(`Expected drum bounds x == 0 for stacked layout, got x=${bounds.x}`);
  }
  if (bounds.y === 0) {
    throw new Error(`Expected drum bounds y > 0 for stacked layout, got y=${bounds.y}`);
  }

  await page.close();
}

// Test 2: Portrait -> stacked
console.log("Test 2: Portrait -> stacked");
{
  const page = await setupPage(390, 844);
  await page.evaluate(() => forceDraw?.());
  await page.waitForTimeout(200);

  const bounds = await page.evaluate(() => drumBounds?.());
  const horiz = await page.evaluate(() => layoutHorizontal?.());

  console.log(`  drumBounds: x=${bounds.x} y=${bounds.y} w=${bounds.w} h=${bounds.h}, horizontal=${horiz}`);

  if (horiz !== true) {
    throw new Error(`Expected layoutHorizontal=true for portrait, got ${horiz}`);
  }
  if (bounds.x !== 0) {
    throw new Error(`Expected drum bounds x == 0 for portrait stacked, got x=${bounds.x}`);
  }
  if (bounds.y === 0) {
    throw new Error(`Expected drum bounds y > 0 for portrait stacked, got y=${bounds.y}`);
  }

  await page.close();
}

// Test 3: Rotation switches layout
console.log("Test 3: Rotation switches layout");
{
  // Start portrait
  const page = await setupPage(390, 844);
  await page.evaluate(() => forceDraw?.());
  await page.waitForTimeout(200);

  let bounds = await page.evaluate(() => drumBounds?.());
  let horiz = await page.evaluate(() => layoutHorizontal?.());
  console.log(`  Portrait: x=${bounds.x} y=${bounds.y} horizontal=${horiz}`);
  if (horiz !== true || bounds.x !== 0 || bounds.y === 0) {
    throw new Error(`Portrait should be stacked: horiz=${horiz} x=${bounds.x} y=${bounds.y}`);
  }

  // Rotate to landscape
  await page.setViewportSize({ width: 844, height: 390 });
  await page.evaluate(() => forceDraw?.());
  await page.waitForTimeout(200);

  bounds = await page.evaluate(() => drumBounds?.());
  horiz = await page.evaluate(() => layoutHorizontal?.());
  console.log(`  Landscape: x=${bounds.x} y=${bounds.y} horizontal=${horiz}`);
  if (horiz !== true || bounds.x !== 0 || bounds.y === 0) {
    throw new Error(`Landscape should be stacked: horiz=${horiz} x=${bounds.x} y=${bounds.y}`);
  }

  // Rotate back to portrait
  await page.setViewportSize({ width: 390, height: 844 });
  await page.evaluate(() => forceDraw?.());
  await page.waitForTimeout(200);

  bounds = await page.evaluate(() => drumBounds?.());
  horiz = await page.evaluate(() => layoutHorizontal?.());
  console.log(`  Portrait again: x=${bounds.x} y=${bounds.y} horizontal=${horiz}`);
  if (horiz !== true || bounds.x !== 0 || bounds.y === 0) {
    throw new Error(`Portrait (2nd) should be stacked: horiz=${horiz} x=${bounds.x} y=${bounds.y}`);
  }

  await page.close();
}

// Test 4: Both panes functional in landscape (stacked layout)
console.log("Test 4: Both panes functional in landscape");
{
  const page = await setupPage(844, 390);
  await page.evaluate(() => forceDraw?.());
  await page.waitForTimeout(200);

  const bounds = await page.evaluate(() => drumBounds?.());
  const sy = await page.evaluate(() => splitY?.());

  // Grid pane has usable height (stacked layout)
  if (sy < 50) {
    throw new Error(`Grid pane height too small in landscape: splitY=${sy}`);
  }
  // Drum pane has usable width (full width in stacked layout)
  const drumW = bounds.w;
  if (drumW < 100) {
    throw new Error(`Drum pane width too small in landscape: drumW=${drumW}`);
  }

  // Verify widget layout snapshot is valid
  const snap = await page.evaluate(() => widgetLayoutSnapshot?.());
  if (!snap) {
    throw new Error("widgetLayoutSnapshot returned null in landscape");
  }
  console.log(`  splitY=${sy}, gridH=${sy}, drumW=${drumW}, drumH=${bounds.h}`);

  if (isCoverageEnabled()) await flushCoverage(page, new URL("../../coverage/browser-raw", import.meta.url).pathname, "orientation_layout");
  await page.close();
}

await browser.close();
server.close();
console.log("Orientation layout tests completed");
