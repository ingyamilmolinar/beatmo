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
    typeof fullLayoutSnapshot === "function" &&
    typeof forceDraw === "function" &&
    typeof splitY === "function" &&
    typeof splitX === "function" &&
    typeof layoutHorizontal === "function" &&
    typeof drumBounds === "function" &&
    typeof playBtnRect === "function" &&
    typeof stopBtnRect === "function"
  );
  await assertSimpleDrawMode(page, false, `orientation_resize ${width}x${height}`);
  await page.evaluate(() => forceDraw?.());
  return page;
}

let passed = 0;
let failed = 0;

function assert(condition, msg) {
  if (!condition) {
    failed++;
    console.error("  FAIL:", msg);
    throw new Error(msg);
  }
}

console.log("Testing orientation resize layout integrity...\n");

// Scenario 1: Layout integrity after rotation
console.log("Scenario 1: Layout integrity after portrait→landscape rotation");
{
  const page = await setupPage(390, 844);

  // Rotate to landscape
  await page.setViewportSize({ width: 844, height: 390 });
  await page.evaluate(() => forceDraw?.());
  await page.waitForTimeout(300);

  const bounds = await page.evaluate(() => drumBounds?.());
  const horiz = await page.evaluate(() => layoutHorizontal?.());
  const snap = await page.evaluate(() => fullLayoutSnapshot?.());

  assert(bounds && bounds.w > 0 && bounds.h > 0,
    `drum bounds should be non-empty after rotation, got w=${bounds?.w} h=${bounds?.h}`);

  // Grid + drum should cover the canvas
  if (horiz) {
    const sY = await page.evaluate(() => splitY?.());
    assert(sY > 0, `splitY should be positive, got ${sY}`);
    assert(bounds.y === sY, `drum should start at splitY=${sY}, got y=${bounds.y}`);
  } else {
    const sX = await page.evaluate(() => splitX?.());
    assert(sX > 0, `splitX should be positive, got ${sX}`);
    assert(bounds.x === sX, `drum should start at splitX=${sX}, got x=${bounds.x}`);
  }

  // Buttons should be non-empty and within drum bounds
  const playR = await page.evaluate(() => playBtnRect?.());
  const stopR = await page.evaluate(() => stopBtnRect?.());
  assert(playR && playR.w > 0 && playR.h > 0,
    `play button rect should be non-empty, got w=${playR?.w} h=${playR?.h}`);
  assert(stopR && stopR.w > 0 && stopR.h > 0,
    `stop button rect should be non-empty, got w=${stopR?.w} h=${stopR?.h}`);

  // Buttons within drum bounds
  assert(playR.x >= bounds.x && playR.y >= bounds.y,
    `play button outside drum bounds: btn=(${playR.x},${playR.y}) bounds=(${bounds.x},${bounds.y})`);
  assert(stopR.x >= bounds.x && stopR.y >= bounds.y,
    `stop button outside drum bounds: btn=(${stopR.x},${stopR.y}) bounds=(${bounds.x},${bounds.y})`);

  // Timeline from snapshot should have positive dimensions
  if (snap && snap.timelineRect) {
    assert(snap.timelineRect.w > 0 && snap.timelineRect.h > 0,
      `timeline rect non-positive after rotation: w=${snap.timelineRect.w} h=${snap.timelineRect.h}`);
  }

  await page.close();
  passed++;
  console.log("  PASS\n");
}

// Scenario 2: Round-trip preserves drum state
console.log("Scenario 2: Round-trip portrait→landscape→portrait preserves state");
{
  const page = await setupPage(390, 844);

  // Record initial state
  const initBounds = await page.evaluate(() => drumBounds?.());

  // Rotate to landscape
  await page.setViewportSize({ width: 844, height: 390 });
  await page.evaluate(() => forceDraw?.());
  await page.waitForTimeout(200);

  // Rotate back to portrait
  await page.setViewportSize({ width: 390, height: 844 });
  await page.evaluate(() => forceDraw?.());
  await page.waitForTimeout(200);

  const finalBounds = await page.evaluate(() => drumBounds?.());

  // Dimensions should be reasonable — close to initial
  assert(finalBounds && finalBounds.w > 0 && finalBounds.h > 0,
    `drum bounds should be non-empty after round-trip`);
  // Width should be similar (portrait both times)
  const widthDiff = Math.abs(finalBounds.w - initBounds.w);
  assert(widthDiff < 50,
    `drum width changed too much after round-trip: initial=${initBounds.w} final=${finalBounds.w}`);

  // Split ratio should still be near 50%
  const sY = await page.evaluate(() => splitY?.());
  const ratio = sY / 844;
  assert(ratio >= 0.35 && ratio <= 0.65,
    `split ratio should be near 50%% after round-trip, got ${ratio.toFixed(2)}`);

  await page.close();
  passed++;
  console.log("  PASS\n");
}

// Scenario 3: Rapid rotation stress
console.log("Scenario 3: Rapid rotation stress (6 viewport changes)");
{
  const page = await setupPage(390, 844);

  const sizes = [
    { w: 844, h: 390 },
    { w: 390, h: 844 },
    { w: 844, h: 390 },
    { w: 390, h: 844 },
    { w: 844, h: 390 },
    { w: 390, h: 844 },
  ];

  for (const { w, h } of sizes) {
    await page.setViewportSize({ width: w, height: h });
    await page.evaluate(() => forceDraw?.());
    await page.waitForTimeout(100);

    const bounds = await page.evaluate(() => drumBounds?.());
    assert(bounds && bounds.w > 0 && bounds.h > 0,
      `drum bounds should be non-empty at ${w}x${h}, got w=${bounds?.w} h=${bounds?.h}`);

    const playR = await page.evaluate(() => playBtnRect?.());
    assert(playR && playR.w > 0 && playR.h > 0,
      `play button should be non-empty at ${w}x${h}`);

    console.log(`  ${w}x${h}: bounds=${bounds.w}x${bounds.h} play=${playR.w}x${playR.h}`);
  }

  await page.close();
  passed++;
  console.log("  PASS\n");
}

// Scenario 4: No black bar after portrait→landscape→portrait
console.log("Scenario 4: No black bar at top of canvas after orientation round-trip");
{
  const page = await setupPage(390, 844);

  // Build a simple circuit so the grid has visible content
  await page.evaluate(() => {
    addNode?.(0, 0, "regular");
    addNode?.(4, 0, "regular");
    addEdgeGrid?.(0, 0, 4, 0);
    forceDraw?.();
  });
  await page.waitForTimeout(200);

  // Rotate to landscape
  await page.setViewportSize({ width: 844, height: 390 });
  await page.evaluate(() => forceDraw?.());
  await page.waitForTimeout(300);

  // Rotate back to portrait
  await page.setViewportSize({ width: 390, height: 844 });
  await page.evaluate(() => forceDraw?.());
  await page.waitForTimeout(300);

  // Take a screenshot and check top rows for pure-black pixels
  const screenshot = await page.screenshot();
  const topPixels = await page.evaluate(async (pngBase64) => {
    const img = new Image();
    const blob = new Blob(
      [Uint8Array.from(atob(pngBase64), c => c.charCodeAt(0))],
      { type: "image/png" }
    );
    img.src = URL.createObjectURL(blob);
    await new Promise(r => { img.onload = r; });
    const canvas = new OffscreenCanvas(img.width, img.height);
    const ctx = canvas.getContext("2d");
    ctx.drawImage(img, 0, 0);

    // Sample pixels in the first 10 rows at the center column
    const results = [];
    const cx = Math.floor(img.width / 2);
    for (let y = 0; y < 10; y++) {
      const px = ctx.getImageData(cx, y, 1, 1).data;
      results.push({ y, r: px[0], g: px[1], b: px[2], a: px[3] });
    }
    return results;
  }, screenshot.toString("base64"));

  console.log("  Top 10 pixel rows at center column after round-trip:");
  for (const p of topPixels) {
    console.log(`    y=${p.y}: rgb(${p.r},${p.g},${p.b})`);
  }

  // If ALL of the first 10 rows are pure black (0,0,0), that indicates
  // a black bar from the draw throttle producing blank frames during resize.
  if (topPixels.every(p => p.r === 0 && p.g === 0 && p.b === 0)) {
    throw new Error(
      "Top 10 pixel rows are pure black (0,0,0) after orientation round-trip — " +
      "this indicates a black bar from blank frames during resize."
    );
  }

  // Also verify drum bounds cover the full canvas
  const bounds = await page.evaluate(() => drumBounds?.());
  const sY = await page.evaluate(() => splitY?.());
  assert(bounds && bounds.y + bounds.h === 844,
    `drum pane should reach canvas bottom after round-trip: bottom=${bounds?.y + bounds?.h}`);
  assert(sY > 0 && sY < 844,
    `splitY should be in valid range after round-trip: got ${sY}`);

  if (isCoverageEnabled()) await flushCoverage(page, new URL("../../coverage/browser-raw", import.meta.url).pathname, "orientation_resize");
  await page.close();
  passed++;
  console.log("  PASS\n");
}

await browser.close();
server.close();

console.log(`Orientation resize tests: ${passed} passed, ${failed} failed`);
if (failed > 0) process.exit(1);
