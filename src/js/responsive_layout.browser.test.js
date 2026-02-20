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
    typeof widgetLayoutSnapshot === "function" &&
    typeof forceDraw === "function" &&
    typeof splitY === "function" &&
    typeof splitX === "function" &&
    typeof layoutHorizontal === "function" &&
    typeof drumBounds === "function"
  );
  await assertSimpleDrawMode(page, false, `responsive ${width}x${height}`);
  await page.evaluate(() => forceDraw?.());
  return page;
}

// Helper: get the relevant split ratio for the current orientation.
// Layout is always stacked on mobile (layoutHorizontal=true), so we always check splitY / height.
async function getSplitRatio(page, w, h) {
  const horiz = await page.evaluate(() => layoutHorizontal?.());
  if (horiz) {
    // Stacked: split on Y
    const sY = await page.evaluate(() => splitY?.());
    return { dim: "Y", value: sY, total: h, ratio: sY / h };
  } else {
    // Side-by-side: split on X (desktop only)
    const sX = await page.evaluate(() => splitX?.());
    return { dim: "X", value: sX, total: w, ratio: sX / w };
  }
}

console.log("Testing responsive 50/50 layout on rotation...");

// Test 1: Portrait → Landscape rotation
console.log("Test 1: Portrait → Landscape rotation");
{
  const page = await setupPage(390, 844);
  const p = await getSplitRatio(page, 390, 844);

  console.log(`  Portrait (390x844): split${p.dim}=${p.value}, ratio=${p.ratio.toFixed(2)}`);
  if (p.ratio < 0.33 || p.ratio > 0.65) {
    throw new Error(`Portrait split ratio ${p.ratio.toFixed(2)} not near 50% (expected 0.33-0.65)`);
  }

  // Verify orientation layout: portrait = stacked (layoutHorizontal=true, drumBounds.x=0, y>0)
  const pBounds = await page.evaluate(() => drumBounds?.());
  const pHoriz = await page.evaluate(() => layoutHorizontal?.());
  if (pHoriz !== true) throw new Error(`Expected layoutHorizontal=true for portrait, got ${pHoriz}`);
  if (pBounds.x !== 0) throw new Error(`Expected drumBounds.x=0 for portrait, got ${pBounds.x}`);
  if (pBounds.y === 0) throw new Error(`Expected drumBounds.y>0 for portrait, got ${pBounds.y}`);

  // Rotate to landscape
  await page.setViewportSize({ width: 844, height: 390 });
  await page.evaluate(() => forceDraw?.());
  await page.waitForTimeout(200);

  const l = await getSplitRatio(page, 844, 390);

  console.log(`  Landscape (844x390): split${l.dim}=${l.value}, ratio=${l.ratio.toFixed(2)}`);
  if (l.ratio < 0.33 || l.ratio > 0.65) {
    throw new Error(`Landscape split ratio ${l.ratio.toFixed(2)} not near 50% (expected 0.33-0.65)`);
  }

  // Verify orientation layout: landscape = stacked (always stacked on mobile)
  const lBounds = await page.evaluate(() => drumBounds?.());
  const lHoriz = await page.evaluate(() => layoutHorizontal?.());
  if (lHoriz !== true) throw new Error(`Expected layoutHorizontal=true for landscape, got ${lHoriz}`);
  if (lBounds.x !== 0) throw new Error(`Expected drumBounds.x=0 for landscape, got ${lBounds.x}`);
  if (lBounds.y === 0) throw new Error(`Expected drumBounds.y>0 for landscape, got ${lBounds.y}`);

  await page.close();
}

// Test 2: Landscape → Portrait rotation
console.log("Test 2: Landscape → Portrait rotation");
{
  const page = await setupPage(844, 390);
  const l = await getSplitRatio(page, 844, 390);

  console.log(`  Landscape (844x390): split${l.dim}=${l.value}, ratio=${l.ratio.toFixed(2)}`);
  if (l.ratio < 0.33 || l.ratio > 0.65) {
    throw new Error(`Landscape split ratio ${l.ratio.toFixed(2)} not near 50% (expected 0.33-0.65)`);
  }

  // Rotate to portrait
  await page.setViewportSize({ width: 390, height: 844 });
  await page.evaluate(() => forceDraw?.());
  await page.waitForTimeout(200);

  const p = await getSplitRatio(page, 390, 844);

  console.log(`  Portrait (390x844): split${p.dim}=${p.value}, ratio=${p.ratio.toFixed(2)}`);
  if (p.ratio < 0.33 || p.ratio > 0.65) {
    throw new Error(`Portrait split ratio ${p.ratio.toFixed(2)} not near 50% (expected 0.33-0.65)`);
  }

  await page.close();
}

// Test 3: Multiple rotations
console.log("Test 3: Multiple rotations stay near 50%");
{
  const page = await setupPage(390, 844);
  const sizes = [
    { w: 844, h: 390 },
    { w: 390, h: 844 },
    { w: 844, h: 390 },
    { w: 390, h: 844 },
  ];

  for (const { w, h } of sizes) {
    await page.setViewportSize({ width: w, height: h });
    await page.evaluate(() => forceDraw?.());
    await page.waitForTimeout(150);

    const s = await getSplitRatio(page, w, h);

    console.log(`  ${w}x${h}: split${s.dim}=${s.value}, ratio=${s.ratio.toFixed(2)}`);
    if (s.ratio < 0.33 || s.ratio > 0.65) {
      throw new Error(`After rotation to ${w}x${h}: split ratio ${s.ratio.toFixed(2)} not near 50%`);
    }

    // Verify always stacked on mobile (both portrait and landscape)
    const horiz = await page.evaluate(() => layoutHorizontal?.());
    const bounds = await page.evaluate(() => drumBounds?.());
    if (horiz !== true) throw new Error(`Expected layoutHorizontal=true at ${w}x${h}, got ${horiz}`);
    if (bounds.x !== 0) throw new Error(`Expected drumBounds.x=0 at ${w}x${h}, got ${bounds.x}`);
    if (bounds.y === 0) throw new Error(`Expected drumBounds.y>0 at ${w}x${h}, got ${bounds.y}`);
  }

  await page.close();
}

// Test 4: Both panes functional after rotation
console.log("Test 4: Both panes functional after rotation");
{
  const page = await setupPage(390, 844);

  // Rotate to landscape (always stacked on mobile)
  await page.setViewportSize({ width: 844, height: 390 });
  await page.evaluate(() => forceDraw?.());
  await page.waitForTimeout(200);

  const sY = await page.evaluate(() => splitY?.());
  if (sY < 50) {
    throw new Error(`Grid pane too small after rotation: splitY=${sY}`);
  }
  if (390 - sY < 50) {
    throw new Error(`Drum pane too small after rotation: drumH=${390 - sY}`);
  }
  console.log(`  Stacked: splitY=${sY}, gridH=${sY}, drumH=${390 - sY}`);

  // Verify widget layout snapshot is valid
  const snap = await page.evaluate(() => widgetLayoutSnapshot?.());
  if (!snap) {
    throw new Error("widgetLayoutSnapshot returned null after rotation");
  }
  if (snap.rack) {
    console.log(`  Rack: ${snap.rack.w}x${snap.rack.h} at (${snap.rack.x},${snap.rack.y})`);
  }
  if (snap.timeline) {
    console.log(`  Timeline: ${snap.timeline.w}x${snap.timeline.h}`);
  }

  await page.close();
}

// Test 5: Small viewport stress (320x480 → 480x320)
console.log("Test 5: Small viewport stress");
{
  const page = await setupPage(320, 480);
  // Portrait: stacked, check splitY
  const p = await getSplitRatio(page, 320, 480);
  console.log(`  320x480: split${p.dim}=${p.value}, ratio=${p.ratio.toFixed(2)}`);
  if (p.value < 50 || p.value > 430) {
    throw new Error(`Small portrait split out of range: ${p.value}`);
  }

  await page.setViewportSize({ width: 480, height: 320 });
  await page.evaluate(() => forceDraw?.());
  await page.waitForTimeout(200);

  // Landscape: also stacked on mobile
  const l = await getSplitRatio(page, 480, 320);
  console.log(`  480x320: split${l.dim}=${l.value}, ratio=${l.ratio.toFixed(2)}`);
  if (l.value < 50 || l.value > 430) {
    throw new Error(`Small landscape split out of range: ${l.value}`);
  }

  // Both panes must have at least some space
  if (l.value < 30) throw new Error(`Grid pane too small: ${l.value}px`);
  if (l.total - l.value < 30) throw new Error(`Drum pane too small: ${l.total - l.value}px`);

  if (isCoverageEnabled()) await flushCoverage(page, new URL("../../coverage/browser-raw", import.meta.url).pathname, "responsive_layout");
  await page.close();
}

await browser.close();
server.close();
console.log("Responsive layout tests completed");
