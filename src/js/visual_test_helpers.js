/**
 * Visual Test Helpers
 *
 * Shared utilities for visual regression and cross-viewport parity tests.
 * Provides screenshot capture, PNG region extraction, pixel analysis,
 * pixelmatch wrapper, and golden image I/O.
 */

import http from "http";
import fs from "fs";
import path from "path";
import { spawnSync } from "child_process";
import { fileURLToPath } from "url";
import { PNG } from "pngjs";
import pixelmatch from "pixelmatch";
import { resolveGoBinary } from "./browser_test_helpers.js";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const jsDir = __dirname;
const repoRoot = path.resolve(jsDir, "..", "..");
const goDir = path.resolve(repoRoot, "src/go");
const goldenDir = path.join(jsDir, "testdata", "golden");

// ─── WASM Build & Server ─────────────────────────────────────────────

/**
 * Build the WASM binary.
 * @returns {void} Throws on failure.
 */
export function buildWasm() {
  const GO = resolveGoBinary();
  const build = spawnSync(
    GO,
    ["build", "-o", path.join(jsDir, "main.wasm"), "./cmd/..."],
    {
      cwd: goDir,
      env: { ...process.env, GOOS: "js", GOARCH: "wasm" },
      stdio: "inherit",
    }
  );
  if (build.status !== 0) throw new Error("go build main.wasm failed");
}

/**
 * Start a minimal static server for WASM.
 * Uses OS-assigned port (port 0) to avoid EADDRINUSE collisions.
 * @returns {Promise<{server: http.Server, port: number}>}
 */
export async function startServer() {
  const server = http.createServer((req, res) => {
    const file = req.url === "/" ? "/index.html" : req.url;
    const filePath = path.join(jsDir, file.replace(/^\//, ""));
    fs.readFile(filePath, (err, data) => {
      if (err) { res.writeHead(404); res.end(); return; }
      let ct = "text/plain";
      if (filePath.endsWith(".html")) ct = "text/html";
      else if (filePath.endsWith(".js")) ct = "application/javascript";
      else if (filePath.endsWith(".wasm")) ct = "application/wasm";
      else if (filePath.endsWith(".json")) ct = "application/json";
      res.writeHead(200, { "Content-Type": ct });
      res.end(data);
    });
  });
  await new Promise((r) => server.listen(0, r));
  const port = server.address().port;
  return { server, port };
}

/**
 * Create a page at the given viewport and wait for WASM to initialize.
 * @param {Browser} browser
 * @param {{width: number, height: number}} viewport
 * @param {number} port
 * @returns {Promise<Page>}
 */
export async function initPage(browser, viewport, port) {
  const page = await browser.newPage({ viewport });
  await page.goto(`http://localhost:${port}/`);
  await page.waitForFunction(
    () =>
      typeof widgetLayoutSnapshot === "function" &&
      typeof forceDraw === "function" &&
      typeof importJSON === "function" &&
      typeof fullLayoutSnapshot === "function",
    { timeout: 30000 }
  );
  // Let layout settle
  for (let i = 0; i < 3; i++) {
    await page.evaluate(() => forceDraw?.());
    await page.waitForTimeout(60);
  }
  return page;
}

/**
 * Wait for WASM JS exports to be available on an already-navigated page.
 * Used for remote BrowserStack pages where navigation happened separately
 * and the real device determines the viewport (not controllable at creation).
 * @param {Page} page
 * @param {number} [timeout=60000] Timeout in ms (remote devices are slower)
 */
export async function waitForWasmReady(page, timeout = 60000) {
  await page.waitForFunction(
    () =>
      typeof widgetLayoutSnapshot === "function" &&
      typeof forceDraw === "function" &&
      typeof importJSON === "function" &&
      typeof fullLayoutSnapshot === "function",
    { timeout }
  );
  // Let layout settle — extra iterations for remote device latency
  for (let i = 0; i < 5; i++) {
    await page.evaluate(() => forceDraw?.());
    await page.waitForTimeout(100);
  }
}

// ─── Screenshot & Region Extraction ──────────────────────────────────

/**
 * Take a full-page screenshot and return a PNG buffer.
 * @param {Page} page
 * @returns {Promise<Buffer>}
 */
export async function captureScreenshot(page) {
  return page.screenshot({ type: "png" });
}

/**
 * Decode a PNG buffer and extract a sub-rectangle.
 * @param {Buffer} pngBuf
 * @param {{x: number, y: number, w: number, h: number}} rect
 * @returns {{width: number, height: number, data: Uint8Array}} RGBA pixel data
 */
export function extractRegion(pngBuf, rect) {
  const png = PNG.sync.read(pngBuf);
  const x0 = Math.max(0, Math.floor(rect.x));
  const y0 = Math.max(0, Math.floor(rect.y));
  const x1 = Math.min(png.width, Math.floor(rect.x + rect.w));
  const y1 = Math.min(png.height, Math.floor(rect.y + rect.h));
  const w = x1 - x0;
  const h = y1 - y0;
  if (w <= 0 || h <= 0) return { width: 0, height: 0, data: new Uint8Array(0) };

  const data = new Uint8Array(w * h * 4);
  for (let row = 0; row < h; row++) {
    const srcOff = ((y0 + row) * png.width + x0) * 4;
    const dstOff = row * w * 4;
    data.set(png.data.subarray(srcOff, srcOff + w * 4), dstOff);
  }
  return { width: w, height: h, data };
}

/**
 * Get the drum row cell region from the WASM page.
 * This is the area where step sequencer cells appear — below the timeline
 * header bar, using the same x/width as the timeline.
 *
 * The layout is:
 *   timelineRect (header: transport controls)
 *   rows area    (cells: rowsAreaHeight tall)
 *   EQ panel     (optional, eqH tall)
 *
 * @param {Page} page
 * @returns {Promise<{x: number, y: number, w: number, h: number}>}
 */
export async function getDrumRowRegion(page) {
  const dbg = await page.evaluate(() => {
    if (typeof debugDrumLayout !== "function") return null;
    return debugDrumLayout();
  });

  if (dbg) {
    // debugDrumLayout gives us precise info
    const tl = dbg.timelineRect;
    if (!tl || tl.w <= 0) {
      throw new Error(`Invalid timelineRect from debugDrumLayout: ${JSON.stringify(tl)}`);
    }
    // Rows start below the timeline header
    const rowsY = tl.y + tl.h;
    const rowsH = dbg.rowsAreaHeight ?? 0;
    if (rowsH <= 0) {
      throw new Error(`rowsAreaHeight=${rowsH} from debugDrumLayout`);
    }
    return { x: tl.x, y: rowsY, w: tl.w, h: rowsH };
  }

  // Fallback: use widgetLayoutSnapshot
  const snap = await page.evaluate(() => widgetLayoutSnapshot?.());
  if (!snap) throw new Error("widgetLayoutSnapshot returned null");

  const tl = snap.timeline;
  if (!tl || tl.w <= 0 || tl.h <= 0) {
    throw new Error(`Invalid timeline rect: ${JSON.stringify(tl)}`);
  }

  // Use the full timeline widget rect as an approximation
  return { x: tl.x, y: tl.y, w: tl.w, h: tl.h };
}

// ─── Pixel Analysis (no external deps) ──────────────────────────────

/**
 * Compute color channel variance across all pixels.
 * Flat uniform areas have near-zero variance; rendered content has high variance.
 * @param {Uint8Array} data RGBA pixel data
 * @param {number} pixelCount
 * @returns {{r: number, g: number, b: number, total: number}}
 */
export function colorVariance(data, pixelCount) {
  if (pixelCount === 0) return { r: 0, g: 0, b: 0, total: 0 };

  let sumR = 0, sumG = 0, sumB = 0;
  for (let i = 0; i < pixelCount; i++) {
    sumR += data[i * 4];
    sumG += data[i * 4 + 1];
    sumB += data[i * 4 + 2];
  }
  const meanR = sumR / pixelCount;
  const meanG = sumG / pixelCount;
  const meanB = sumB / pixelCount;

  let varR = 0, varG = 0, varB = 0;
  for (let i = 0; i < pixelCount; i++) {
    varR += (data[i * 4] - meanR) ** 2;
    varG += (data[i * 4 + 1] - meanG) ** 2;
    varB += (data[i * 4 + 2] - meanB) ** 2;
  }
  varR /= pixelCount;
  varG /= pixelCount;
  varB /= pixelCount;

  return { r: varR, g: varG, b: varB, total: varR + varG + varB };
}

/**
 * Fraction of pixels that differ from a background color.
 * @param {Uint8Array} data RGBA pixel data
 * @param {number} pixelCount
 * @param {{r: number, g: number, b: number}} bgColor Background color
 * @param {number} [threshold=15] Distance threshold per channel
 * @returns {number} Ratio 0..1
 */
export function nonBackgroundRatio(data, pixelCount, bgColor, threshold = 15) {
  if (pixelCount === 0) return 0;
  let count = 0;
  for (let i = 0; i < pixelCount; i++) {
    const dr = Math.abs(data[i * 4] - bgColor.r);
    const dg = Math.abs(data[i * 4 + 1] - bgColor.g);
    const db = Math.abs(data[i * 4 + 2] - bgColor.b);
    if (dr > threshold || dg > threshold || db > threshold) count++;
  }
  return count / pixelCount;
}

/**
 * Count unique RGB colors in the region.
 * @param {Uint8Array} data RGBA pixel data
 * @param {number} pixelCount
 * @returns {number}
 */
export function uniqueColorCount(data, pixelCount) {
  const seen = new Set();
  for (let i = 0; i < pixelCount; i++) {
    const key = (data[i * 4] << 16) | (data[i * 4 + 1] << 8) | data[i * 4 + 2];
    seen.add(key);
  }
  return seen.size;
}

/**
 * Compute all analysis metrics for a region.
 * @param {{width: number, height: number, data: Uint8Array}} region
 * @returns {{variance: object, nonBgRatio: number, uniqueColors: number, pixelCount: number}}
 */
export function analyzeRegion(region) {
  const pixelCount = region.width * region.height;
  const bgColor = { r: 20, g: 20, b: 20 }; // Beatmo background
  return {
    variance: colorVariance(region.data, pixelCount),
    nonBgRatio: nonBackgroundRatio(region.data, pixelCount, bgColor),
    uniqueColors: uniqueColorCount(region.data, pixelCount),
    pixelCount,
  };
}

// ─── Golden Image Comparison ─────────────────────────────────────────

/**
 * Ensure the golden directory exists.
 */
function ensureGoldenDir() {
  fs.mkdirSync(goldenDir, { recursive: true });
}

/**
 * Compare a screenshot against a golden baseline using pixelmatch.
 * If no golden exists, saves the current image as the baseline (bootstrap mode).
 * @param {string} name Base name (e.g. "desktop_1280x720")
 * @param {Buffer} pngBuf Current screenshot PNG buffer
 * @param {number} [threshold=0.1] pixelmatch color threshold (0..1)
 * @returns {{match: boolean, bootstrapped: boolean, diffCount: number, diffPct: number, diffPng: Buffer|null}}
 */
export function compareWithGolden(name, pngBuf, threshold = 0.1) {
  ensureGoldenDir();
  const goldenPath = path.join(goldenDir, `${name}.png`);

  if (!fs.existsSync(goldenPath)) {
    // First run — bootstrap golden
    fs.writeFileSync(goldenPath, pngBuf);
    return { match: true, bootstrapped: true, diffCount: 0, diffPct: 0, diffPng: null };
  }

  const golden = PNG.sync.read(fs.readFileSync(goldenPath));
  const current = PNG.sync.read(pngBuf);

  // If dimensions differ, fail immediately
  if (golden.width !== current.width || golden.height !== current.height) {
    return {
      match: false,
      bootstrapped: false,
      diffCount: -1,
      diffPct: 100,
      diffPng: null,
      error: `Dimension mismatch: golden=${golden.width}x${golden.height} vs current=${current.width}x${current.height}`,
    };
  }

  const diff = new PNG({ width: golden.width, height: golden.height });
  const diffCount = pixelmatch(
    golden.data,
    current.data,
    diff.data,
    golden.width,
    golden.height,
    { threshold }
  );

  const totalPixels = golden.width * golden.height;
  const diffPct = (diffCount / totalPixels) * 100;
  const diffPng = PNG.sync.write(diff);

  return { match: diffPct < 0.5, bootstrapped: false, diffCount, diffPct, diffPng };
}

/**
 * Save a PNG buffer as a golden baseline (overwrite existing).
 * @param {string} name
 * @param {Buffer} pngBuf
 */
export function saveGolden(name, pngBuf) {
  ensureGoldenDir();
  fs.writeFileSync(path.join(goldenDir, `${name}.png`), pngBuf);
}

/**
 * Save a diff image for debugging.
 * @param {string} name
 * @param {Buffer} diffPng
 */
export function saveDiffImage(name, diffPng) {
  ensureGoldenDir();
  fs.writeFileSync(path.join(goldenDir, `${name}.diff.png`), diffPng);
}

/**
 * Save a screenshot for diagnostic purposes (not a golden).
 * @param {string} name
 * @param {Buffer} pngBuf
 */
export function saveDiagnostic(name, pngBuf) {
  ensureGoldenDir();
  fs.writeFileSync(path.join(goldenDir, `${name}.png`), pngBuf);
}

/**
 * Encode region data back to PNG buffer for saving.
 * @param {{width: number, height: number, data: Uint8Array}} region
 * @returns {Buffer}
 */
export function regionToPng(region) {
  const png = new PNG({ width: region.width, height: region.height });
  png.data = Buffer.from(region.data);
  return PNG.sync.write(png);
}

// ─── Layout Snapshot ─────────────────────────────────────────────────

/**
 * Capture the full layout snapshot from the WASM page.
 * Returns null if the export doesn't exist (backward compatibility before WASM rebuild).
 * @param {Page} page
 * @returns {Promise<object|null>}
 */
export async function captureLayoutSnapshot(page) {
  return page.evaluate(() => {
    if (typeof fullLayoutSnapshot !== "function") return null;
    return fullLayoutSnapshot();
  });
}

// ─── Fixture Loading ─────────────────────────────────────────────────

/**
 * Load the multi-row parity fixture into the page.
 * @param {Page} page
 */
export async function loadMultiRowFixture(page) {
  const fixturePath = path.join(
    goDir,
    "internal/assets/parity_fixture_multi_row.json"
  );
  const fixture = fs.readFileSync(fixturePath, "utf-8");
  await page.evaluate((json) => importJSON?.(json), fixture);
  // Settle layout
  for (let i = 0; i < 3; i++) {
    await page.evaluate(() => forceDraw?.());
    await page.waitForTimeout(100);
  }
  await page.evaluate(() => forceDraw?.());
}

// Re-export for convenience
export { goldenDir, jsDir, goDir };
