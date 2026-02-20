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

let failures = 0;

const viewports = [
  { name: "iPhone14_portrait", width: 390, height: 844 },
  { name: "iPhone14_landscape", width: 844, height: 390 },
  { name: "iPhoneSE_portrait", width: 320, height: 568 },
  { name: "small_landscape", width: 480, height: 320 },
];

// Background color: RGB(20,20,20). Cell borders: RGB(70,70,70).
// Off-cells: RGB(35,35,35). Any pixel with R,G,B > 30 indicates
// rendered content (borders, on-cells, or transport controls).
const BG_THRESHOLD = 30;

/**
 * Read multiple pixels from a screenshot PNG buffer.
 * Returns an array of {r,g,b,a} objects for each (x,y) pair.
 */
async function readPixels(page, pngBuf, points) {
  return page.evaluate(async ([pngB64, pts]) => {
    const blob = new Blob(
      [Uint8Array.from(atob(pngB64), c => c.charCodeAt(0))],
      { type: "image/png" }
    );
    const bmp = await createImageBitmap(blob);
    const cvs = new OffscreenCanvas(bmp.width, bmp.height);
    const ctx = cvs.getContext("2d");
    ctx.drawImage(bmp, 0, 0);
    return pts.map(([px, py]) => {
      const d = ctx.getImageData(px, py, 1, 1).data;
      return { r: d[0], g: d[1], b: d[2], a: d[3] };
    });
  }, [pngBuf.toString("base64"), points]);
}

for (const vp of viewports) {
  console.log(`\nTest: ${vp.name} (${vp.width}x${vp.height})`);
  const page = await browser.newPage({ viewport: { width: vp.width, height: vp.height } });

  try {
    await page.goto(`http://localhost:${port}/`);

    await page.waitForFunction(() =>
      typeof widgetLayoutSnapshot === "function" &&
      typeof visibleRows === "function" &&
      typeof forceDraw === "function" &&
      typeof drumBounds === "function" &&
      typeof debugDrumLayout === "function"
    );
    await assertSimpleDrawMode(page, false, `mobile drumrow ${vp.name}`);

    // Force a few draws to settle layout
    for (let i = 0; i < 3; i++) {
      await page.evaluate(() => forceDraw?.());
      await page.waitForTimeout(50);
    }

    // Dump full debug layout for diagnosis
    const dbgLayout = await page.evaluate(() => debugDrumLayout?.());
    if (dbgLayout) {
      console.log(`  debugDrumLayout: eqH=${dbgLayout.eqH} headerH=${dbgLayout.headerH} ` +
        `rowsArea=${dbgLayout.rowsAreaHeight} vis=${dbgLayout.visibleRows} ` +
        `rowH=${dbgLayout.rowHeight} striping=${dbgLayout.rowsStripingEnabled} ` +
        `layerExists=${dbgLayout.rowsLayerExists} layerDirty=${dbgLayout.rowsLayerDirty} ` +
        `isSmall=${dbgLayout.isSmallScreen} touchW=${dbgLayout.touchScreenWidth}`);
    }

    // Dump render trace
    const dbgRender = await page.evaluate(() => {
      forceDraw?.();
      return typeof debugDrumRender === "function" ? debugDrumRender() : null;
    });
    if (dbgRender) {
      console.log(`  debugDrumRender: frame=${dbgRender.frame} layerFrame=${dbgRender.rowsLayerFrame} ` +
        `drawnRows=${dbgRender.drawnRows} repaints=${dbgRender.rowsRepaints} ` +
        `layerExists=${dbgRender.rowsLayerExists} ` +
        `directDrawCount=${dbgRender.directDrawCount} directDrawCells=${dbgRender.directDrawCells}`);
    }

    // Verify rows are rendered on mobile viewports (via directDraw OR stripes).
    // Wider mobile screens (e.g. iPhone14 landscape, 844px) may have a timeline
    // wide enough (>=440px) for stripes to succeed instead of bailing to directDraw.
    if (dbgLayout && dbgLayout.isSmallScreen) {
      const ddc = dbgLayout.directDrawCount ?? 0;
      const stripesActive = dbgRender && dbgRender.numRowsStripes > 0;
      if (ddc === 0 && !stripesActive) {
        throw new Error(
          `No rendering path active on small screen — ` +
          `directDrawCount=0 and numRowsStripes=${dbgRender?.numRowsStripes ?? 0}`
        );
      }
      if (ddc > 0) {
        console.log(`  directDrawCount: ${ddc}, directDrawCells: ${dbgLayout.directDrawCells ?? 0}`);
      } else {
        console.log(`  stripesActive: numRowsStripes=${dbgRender.numRowsStripes}`);
      }
    }

    // Check visibleRows > 0
    const vis = await page.evaluate(() => visibleRows?.());
    if (vis == null || vis < 1) {
      throw new Error(`visibleRows()=${vis}, want >= 1`);
    }
    console.log(`  visibleRows: ${vis}`);

    // Check timeline width > 0
    const snap = await page.evaluate(() => widgetLayoutSnapshot?.());
    if (!snap) throw new Error("widgetLayoutSnapshot returned null");
    const tlW = snap.timeline?.w ?? 0;
    if (tlW <= 0) {
      throw new Error(`timeline width=${tlW}, want > 0`);
    }
    console.log(`  timeline width: ${tlW}`);

    // Check drumBounds dimensions
    const db = await page.evaluate(() => drumBounds?.());
    if (!db) throw new Error("drumBounds returned null");
    const drumW = db.w ?? 0;
    if (drumW <= 0) throw new Error(`drumBounds width=${drumW}, want > 0`);

    // Verify timeline gets at least 30% of drum pane width
    const pct = Math.round(tlW * 100 / drumW);
    console.log(`  timeline is ${pct}% of drum width (${tlW}/${drumW})`);
    if (pct < 30) {
      throw new Error(`timeline only ${pct}% of drum width, want >= 30%`);
    }

    // Sample pixels in the drum row area to verify rows are actually rendered.
    // The row area starts below the timeline bar (timelineRect).
    // Row 0 center is approximately at timelineRect.y + timelineRect.h + rowHeight/2.
    const tlX = snap.timeline?.x ?? 0;
    const tlY = snap.timeline?.y ?? 0;
    const tlH = snap.timeline?.h ?? 0;
    const rowAreaTop = tlY + tlH + 5; // just below timeline bar

    // Sample 8 evenly-spaced points across the row, plus a few at different Y offsets.
    const samplePoints = [];
    const sampleCount = 8;
    for (let i = 0; i < sampleCount; i++) {
      const x = tlX + Math.floor((i + 0.5) * tlW / sampleCount);
      samplePoints.push([x, rowAreaTop + 5]);  // near top of row
      samplePoints.push([x, rowAreaTop + 12]); // mid row
    }

    const buf = await page.screenshot({ type: "png" });
    const pixels = await readPixels(page, buf, samplePoints);

    // Log all sampled pixels for diagnostics.
    let hasBorderOrContent = false;
    let allBg = true;
    for (let i = 0; i < pixels.length; i++) {
      const p = pixels[i];
      const [sx, sy] = samplePoints[i];
      console.log(`    pixel(${sx},${sy}): rgba(${p.r},${p.g},${p.b},${p.a})`);
      if (p.a === 0) {
        // Transparent pixel — nothing drawn at all.
        continue;
      }
      // Check if this pixel differs from the flat background (RGB 20,20,20).
      if (p.r > BG_THRESHOLD || p.g > BG_THRESHOLD || p.b > BG_THRESHOLD) {
        hasBorderOrContent = true;
      }
      // Check if pixel differs from exact background color.
      if (p.r !== 20 || p.g !== 20 || p.b !== 20) {
        allBg = false;
      }
    }

    // At least some pixels must show content (cell borders at ~RGB(70,70,70),
    // off-cells at ~RGB(35,35,35), or on-cells with the row color).
    if (!hasBorderOrContent) {
      throw new Error(
        `No content pixels found in row area — all pixels at or below background threshold (${BG_THRESHOLD}). ` +
        `Rows are likely rendering as flat grey.`
      );
    }

    // Additionally, the row area should not be uniformly the background color.
    // Cell borders and off-cells create visible variation.
    if (allBg) {
      throw new Error(
        `All sampled pixels are exactly RGB(20,20,20) background — rows appear blank. ` +
        `Expected cell borders (70,70,70) or off-cells (35,35,35).`
      );
    }

    console.log(`  PASS (content detected in row area)`);
  } catch (e) {
    console.error(`  FAIL: ${e.message}`);
    failures++;
  } finally {
    if (isCoverageEnabled()) await flushCoverage(page, new URL("../../coverage/browser-raw", import.meta.url).pathname, "mobile_drumrow_render");
    await page.close();
  }
}

await browser.close();
server.close();

if (failures > 0) {
  console.error(`\n${failures} test(s) failed`);
  process.exit(1);
}
console.log("\nAll mobile drum row render tests passed.");
