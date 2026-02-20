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
    typeof splitY === "function" &&
    typeof addNode === "function" &&
    typeof gridToScreen === "function" &&
    typeof centerCamera === "function"
  );
  await assertSimpleDrawMode(page, false, `portrait_fullcanvas ${width}x${height}`);
  await page.evaluate(() => forceDraw?.());
  return page;
}

// Test 1: Drum pane extends to the bottom of the canvas
console.log("Test 1: Drum pane bottom edge equals canvas height");
{
  const page = await setupPage(390, 844);
  await page.evaluate(() => forceDraw?.());
  await page.waitForTimeout(200);

  const bounds = await page.evaluate(() => drumBounds?.());
  const canvasH = 844;

  console.log(`  drumBounds: y=${bounds.y} h=${bounds.h} bottom=${bounds.y + bounds.h}`);

  const drumBottom = bounds.y + bounds.h;
  if (drumBottom !== canvasH) {
    throw new Error(
      `Drum pane bottom edge at y=${drumBottom}, expected y=${canvasH}. ` +
      `The UI does not cover the full canvas height.`
    );
  }

  await page.close();
}

// Test 2: Grid and drum panes tile the full canvas with no gap
console.log("Test 2: Grid + drum panes tile full canvas (no gap at top or between panes)");
{
  const page = await setupPage(390, 844);
  await page.evaluate(() => forceDraw?.());
  await page.waitForTimeout(200);

  const bounds = await page.evaluate(() => drumBounds?.());
  const sY = await page.evaluate(() => splitY?.());

  console.log(`  splitY=${sY}, drumBounds: y=${bounds.y} h=${bounds.h}`);

  // Grid pane: [0, 0, 390, splitY]
  // Drum pane: [0, splitY, 390, 844]
  // The drum should start exactly at splitY.
  if (bounds.y !== sY) {
    throw new Error(
      `Drum pane top y=${bounds.y} != splitY=${sY}. ` +
      `There is a gap between the grid and drum panes.`
    );
  }

  // Grid pane top is at y=0 (implicit from splitY usage).
  // Drum pane bottom should be at 844.
  if (bounds.y + bounds.h !== 844) {
    throw new Error(
      `Drum pane does not reach canvas bottom: bottom=${bounds.y + bounds.h}, expected 844`
    );
  }

  await page.close();
}

// Test 3: Node placed at world origin appears near the TOP of the canvas
// (no topOffset gap pushing content down)
console.log("Test 3: Node at world origin appears near top of canvas (no dead zone)");
{
  const page = await setupPage(390, 844);

  // Add a node at grid (0,0) and get its screen position
  await page.evaluate(() => addNode?.(0, 0, "regular"));
  await page.evaluate(() => forceDraw?.());
  await page.waitForTimeout(200);
  await page.evaluate(() => centerCamera?.());
  await page.evaluate(() => forceDraw?.());

  const screenPos = await page.evaluate(() => gridToScreen?.(0, 0));
  console.log(`  Node at grid(0,0) → screen(${screenPos?.x}, ${screenPos?.y})`);

  // The node screen position depends on camera offset, but the camera
  // should be centered to use the full grid pane height.
  // With the camera centered in the grid pane, world (0,0) should appear
  // near the center of the grid pane. If topOffset pushes it down,
  // it will appear at (splitY + topOffset) / 2 instead of splitY / 2.
  const sY = await page.evaluate(() => splitY?.());
  const expectedCenter = sY / 2;
  // topOffset = 40 would push the center to approximately (sY - 40) / 2 + 40
  // = sY/2 + 20. Allow some tolerance but catch the 40px offset.
  const topOffsetValue = 40; // the known constant in the Go code
  const tolerance = 15;

  if (screenPos && screenPos.y > expectedCenter + tolerance) {
    throw new Error(
      `Node at world origin has screen y=${screenPos.y}, expected near ` +
      `${expectedCenter} (splitY/2). The node is shifted down by ~${Math.round(screenPos.y - expectedCenter)}px, ` +
      `suggesting topOffset=${topOffsetValue} is creating a gap at the top of the grid pane.`
    );
  }

  await page.close();
}

// Test 4: Take a screenshot and verify no black strip at the top
console.log("Test 4: No black strip at top of canvas in portrait");
{
  const page = await setupPage(390, 844);

  // Build a simple circuit so the grid has visible content
  await page.evaluate(() => {
    addNode?.(0, 0, "regular");
    addNode?.(4, 0, "regular");
    addEdgeGrid?.(0, 0, 4, 0);
    forceDraw?.();
  });
  await page.waitForTimeout(300);

  // Take a screenshot and read the top row of pixels
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

  console.log("  Top 10 pixel rows at center column:");
  let allBlack = true;
  for (const p of topPixels) {
    console.log(`    y=${p.y}: rgb(${p.r},${p.g},${p.b})`);
    // Consider "black" as any very dark pixel (r+g+b < 30)
    if (p.r + p.g + p.b >= 30) {
      allBlack = false;
    }
  }

  // The grid background might be dark, but we should see SOME non-black
  // content (grid lines, background pattern) in the first few rows.
  // A fully black top strip indicates a dead zone from topOffset.
  // NOTE: The grid background color (colBGTop) may itself be very dark.
  // If ALL of the first 10 rows are pure black (0,0,0), that indicates
  // unused canvas space, not just a dark theme.
  if (allBlack && topPixels.every(p => p.r === 0 && p.g === 0 && p.b === 0)) {
    throw new Error(
      "Top 10 pixel rows are pure black (0,0,0) — this indicates an unused " +
      "dead zone at the top of the canvas. The grid background should fill " +
      "this area even if it's a dark color."
    );
  }

  await page.close();
}

// Test 5: In portrait mobile, the grid pane's effective content height
// should equal its full allocated height (no topOffset reduction)
console.log("Test 5: Grid pane effective height equals allocated height");
{
  const page = await setupPage(390, 844);
  await page.evaluate(() => forceDraw?.());
  await page.waitForTimeout(200);

  const sY = await page.evaluate(() => splitY?.());
  const bounds = await page.evaluate(() => drumBounds?.());

  // Grid pane allocated height = splitY
  // Drum pane starts at splitY
  // The grid should use ALL of its allocated height for content.
  // If topOffset = 40 creates a dead zone, the effective content height
  // is only (splitY - 40), wasting 40px.
  const gridAllocatedH = sY;
  const drumAllocatedH = bounds.h;
  const totalAllocated = gridAllocatedH + drumAllocatedH;

  console.log(`  Grid allocated: ${gridAllocatedH}px, Drum allocated: ${drumAllocatedH}px, Total: ${totalAllocated}px`);

  if (totalAllocated !== 844) {
    throw new Error(
      `Grid (${gridAllocatedH}) + Drum (${drumAllocatedH}) = ${totalAllocated}px, ` +
      `expected 844px. Canvas space is not fully utilized.`
    );
  }

  // Verify that the grid's usable content area is the full gridAllocatedH.
  // Place nodes at extreme top and bottom of grid pane to verify coverage.
  // Node at bottom of grid: should appear near y = splitY
  // Node at top of grid: should appear near y = 0
  // If topOffset exists, the "top" node appears at y = topOffset, not y = 0.
  await page.evaluate(() => {
    addNode?.(0, 0, "regular");
    forceDraw?.();
  });
  await page.evaluate(() => centerCamera?.());
  await page.evaluate(() => forceDraw?.());

  const pos = await page.evaluate(() => gridToScreen?.(0, 0));
  if (pos) {
    // Camera is centered, so node at (0,0) should be near the center of
    // the grid pane. With topOffset, it's pushed below center.
    const gridCenter = sY / 2;
    const offset = Math.abs(pos.y - gridCenter);
    console.log(`  Node(0,0) screen y=${pos.y}, grid center=${gridCenter}, offset=${offset}px`);

    // If offset > 25px, topOffset is likely pushing content down
    if (offset > 25) {
      throw new Error(
        `Node at grid center has ${offset}px offset from the grid pane center. ` +
        `Expected node near y=${gridCenter}, got y=${pos.y}. ` +
        `This suggests a dead zone at the top of the grid pane.`
      );
    }
  }

  if (isCoverageEnabled()) await flushCoverage(page, new URL("../../coverage/browser-raw", import.meta.url).pathname, "portrait_fullcanvas");
  await page.close();
}

await browser.close();
server.close();
console.log("Portrait fullcanvas tests completed");
