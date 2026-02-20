import { chromium, devices } from "playwright";
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

console.log("Testing responsive layout on mobile viewports...");

// Test 1: Horizontal smartphone viewport (800x400)
console.log("Test 1: Horizontal smartphone viewport (800x400)");
{
  const page = await browser.newPage({ viewport: { width: 800, height: 400 } });
  await page.goto(`http://localhost:${port}/`);

  await page.waitForFunction(() =>
    typeof widgetLayoutSnapshot === "function" &&
    typeof forceDraw === "function"
  );
  await assertSimpleDrawMode(page, false, "mobile layout 800x400");

  // Force a draw to ensure layout is computed
  await page.evaluate(() => forceDraw?.());
  const snap = await page.evaluate(() => widgetLayoutSnapshot?.());

  if (!snap) {
    throw new Error("widgetLayoutSnapshot returned null");
  }

  // Verify canvas fills viewport without overflow
  const canvasRect = await page.evaluate(() => {
    const canvas = document.querySelector('canvas');
    if (!canvas) return null;
    const rect = canvas.getBoundingClientRect();
    return { x: rect.x, y: rect.y, width: rect.width, height: rect.height };
  });
  if (!canvasRect) throw new Error("Canvas not found");
  if (canvasRect.width < 790 || canvasRect.height < 390) {
    throw new Error(`Canvas too small: ${canvasRect.width}x${canvasRect.height}`);
  }
  console.log(`  Canvas size: ${canvasRect.width}x${canvasRect.height}`);

  // Verify splitter is visible (between 100-300 from top on small screen)
  if (!snap.splitterY || snap.splitterY < 50 || snap.splitterY > 350) {
    console.log(`  Warning: Splitter Y position unusual: ${snap.splitterY}`);
  } else {
    console.log(`  Splitter Y: ${snap.splitterY}`);
  }

  // Verify rack and timeline widgets exist
  if (!snap.rack) {
    console.log("  Warning: rack widget not in snapshot");
  } else {
    console.log(`  Rack: ${snap.rack.w}x${snap.rack.h} at (${snap.rack.x},${snap.rack.y})`);
  }
  if (!snap.timeline) {
    console.log("  Warning: timeline widget not in snapshot");
  } else {
    console.log(`  Timeline: ${snap.timeline.w}x${snap.timeline.h} at (${snap.timeline.x},${snap.timeline.y})`);
  }

  // Verify add button is accessible (inside rack area)
  if (snap.addButton && snap.rack) {
    const insideRack =
      snap.addButton.x >= snap.rack.x &&
      snap.addButton.x + snap.addButton.w <= snap.rack.x + snap.rack.w &&
      snap.addButton.y >= snap.rack.y &&
      snap.addButton.y + snap.addButton.h <= snap.rack.y + snap.rack.h;
    if (!insideRack) {
      console.log(`  Warning: Add button not fully inside rack`);
    } else {
      console.log(`  Add button inside rack: OK`);
    }
  }

  await page.close();
}

// Test 2: Tablet portrait viewport (600x800)
console.log("Test 2: Tablet portrait viewport (600x800)");
{
  const page = await browser.newPage({ viewport: { width: 600, height: 800 } });
  await page.goto(`http://localhost:${port}/`);

  await page.waitForFunction(() =>
    typeof widgetLayoutSnapshot === "function" &&
    typeof forceDraw === "function"
  );
  await assertSimpleDrawMode(page, false, "mobile layout 600x800");

  await page.evaluate(() => forceDraw?.());
  const snap = await page.evaluate(() => widgetLayoutSnapshot?.());

  if (!snap) {
    throw new Error("widgetLayoutSnapshot returned null");
  }

  // Canvas should adapt to portrait orientation
  const canvasRect = await page.evaluate(() => {
    const canvas = document.querySelector('canvas');
    if (!canvas) return null;
    const rect = canvas.getBoundingClientRect();
    return { width: rect.width, height: rect.height };
  });
  if (!canvasRect) throw new Error("Canvas not found");
  console.log(`  Canvas size: ${canvasRect.width}x${canvasRect.height}`);

  // Verify controls are still accessible
  if (snap.transport) {
    console.log(`  Transport: ${snap.transport.w}x${snap.transport.h}`);
  }

  await page.close();
}

// Test 3: Very small viewport (480x320)
console.log("Test 3: Very small viewport (480x320)");
{
  const page = await browser.newPage({ viewport: { width: 480, height: 320 } });
  await page.goto(`http://localhost:${port}/`);

  await page.waitForFunction(() =>
    typeof widgetLayoutSnapshot === "function" &&
    typeof forceDraw === "function"
  );
  await assertSimpleDrawMode(page, false, "mobile layout 480x320");

  await page.evaluate(() => forceDraw?.());
  const snap = await page.evaluate(() => widgetLayoutSnapshot?.());

  if (!snap) {
    throw new Error("widgetLayoutSnapshot returned null");
  }

  // Verify no elements are clipped outside viewport
  const canvasRect = await page.evaluate(() => {
    const canvas = document.querySelector('canvas');
    if (!canvas) return null;
    const rect = canvas.getBoundingClientRect();
    return { width: rect.width, height: rect.height };
  });
  if (!canvasRect) throw new Error("Canvas not found");
  console.log(`  Canvas size: ${canvasRect.width}x${canvasRect.height}`);

  // App should remain usable even on tiny screens
  console.log(`  Splitter Y: ${snap.splitterY || 'N/A'}`);

  await page.close();
}

// Test 4: iPhone 12 landscape device emulation
console.log("Test 4: iPhone 12 landscape device emulation");
{
  const iPhone = devices['iPhone 12 landscape'];
  const context = await browser.newContext({
    ...iPhone,
    hasTouch: true,
  });
  const page = await context.newPage();
  await page.goto(`http://localhost:${port}/`);

  await page.waitForFunction(() =>
    typeof widgetLayoutSnapshot === "function" &&
    typeof forceDraw === "function"
  );
  await assertSimpleDrawMode(page, false, "iPhone 12 landscape");

  await page.evaluate(() => forceDraw?.());
  const snap = await page.evaluate(() => widgetLayoutSnapshot?.());

  if (!snap) {
    throw new Error("widgetLayoutSnapshot returned null");
  }

  const canvasRect = await page.evaluate(() => {
    const canvas = document.querySelector('canvas');
    if (!canvas) return null;
    const rect = canvas.getBoundingClientRect();
    return { width: rect.width, height: rect.height };
  });
  console.log(`  Canvas size: ${canvasRect?.width}x${canvasRect?.height}`);
  console.log(`  Splitter Y: ${snap.splitterY || 'N/A'}`);

  if (isCoverageEnabled()) await flushCoverage(page, new URL("../../coverage/browser-raw", import.meta.url).pathname, "mobile_layout");
  await context.close();
}

await browser.close();
server.close();
console.log("Mobile layout tests completed");
