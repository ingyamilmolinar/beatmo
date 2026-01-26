/**
 * Real Input Test Helpers
 *
 * These helpers are specifically designed for testing REAL canvas interactions
 * in the full WASM build (main.wasm). Unlike the playtest harness (play_ui.wasm),
 * the full build runs ebiten.RunGame() which properly registers input handlers.
 *
 * Uses Playwright's native mouse methods which properly integrate with the browser's
 * event system and Ebiten's input handling.
 */

import { chromium } from "playwright";
import http from "http";
import { spawnSync } from "child_process";
import fs from "fs";
import path from "path";
import { fileURLToPath } from "url";
import { resolveGoBinary } from "./browser_test_helpers.js";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const jsDir = __dirname;
const goDir = path.resolve(jsDir, "../go");

/**
 * Click at specific coordinates using Playwright's native mouse methods.
 * This properly triggers the full browser event chain that Ebiten expects.
 *
 * @param {Page} page - Playwright page object
 * @param {number} x - X coordinate (screen pixels)
 * @param {number} y - Y coordinate (screen pixels)
 */
export async function clickAt(page, x, y) {
  await page.mouse.click(x, y);
}

/**
 * Click and hold, then release after a delay.
 *
 * @param {Page} page - Playwright page object
 * @param {number} x - X coordinate
 * @param {number} y - Y coordinate
 * @param {number} holdMs - Time to hold in milliseconds
 */
export async function clickAndHold(page, x, y, holdMs = 100) {
  await page.mouse.move(x, y);
  await page.mouse.down();
  await page.waitForTimeout(holdMs);
  await page.mouse.up();
}

/**
 * Perform a drag operation.
 *
 * @param {Page} page - Playwright page object
 * @param {number} x1 - Start X coordinate
 * @param {number} y1 - Start Y coordinate
 * @param {number} x2 - End X coordinate
 * @param {number} y2 - End Y coordinate
 * @param {Object} options - Optional settings
 * @param {number} options.steps - Number of intermediate steps (default: 10)
 */
export async function dragMouse(page, x1, y1, x2, y2, options = {}) {
  const steps = options.steps ?? 10;
  await page.mouse.move(x1, y1);
  await page.mouse.down();

  // Move through intermediate points
  for (let i = 1; i <= steps; i++) {
    const t = i / steps;
    const x = Math.round(x1 + (x2 - x1) * t);
    const y = Math.round(y1 + (y2 - y1) * t);
    await page.mouse.move(x, y);
    await page.waitForTimeout(10);
  }

  await page.mouse.up();
}

/**
 * Scroll the mouse wheel.
 *
 * @param {Page} page - Playwright page object
 * @param {number} x - X coordinate
 * @param {number} y - Y coordinate
 * @param {number} deltaY - Scroll delta (positive = scroll down)
 */
export async function wheelAt(page, x, y, deltaY) {
  await page.mouse.move(x, y);
  await page.mouse.wheel(0, deltaY);
}

/**
 * Build main.wasm (full WASM build with Ebiten input handlers).
 *
 * @param {Object} options - Build options
 * @param {string} options.logLevel - Log level (default: INFO)
 * @returns {boolean} - true if build succeeded
 */
export function buildMainWasm(options = {}) {
  const logLevel = options.logLevel ?? "INFO";
  const GO = resolveGoBinary();

  const build = spawnSync(
    GO,
    [
      "build",
      "-ldflags",
      `-X main.defaultLog=${logLevel}`,
      "-o",
      path.join(jsDir, "main.wasm"),
      "./cmd/...",
    ],
    {
      cwd: goDir,
      env: { ...process.env, GOOS: "js", GOARCH: "wasm" },
      stdio: "inherit",
    }
  );

  return build.status === 0;
}

/**
 * Create an HTTP server that serves the WASM files with correct MIME types.
 *
 * @param {number} port - Port to listen on
 * @returns {Promise<http.Server>} - HTTP server instance
 */
export function createServer(port) {
  const server = http.createServer((req, res) => {
    const file = req.url === "/" ? "/index.html" : req.url;
    const filePath = path.join(jsDir, file.replace(/^\//, ""));

    fs.readFile(filePath, (err, data) => {
      if (err) {
        res.writeHead(404);
        res.end();
        return;
      }

      let ct = "text/plain";
      if (filePath.endsWith(".html")) ct = "text/html";
      else if (filePath.endsWith(".js")) ct = "application/javascript";
      else if (filePath.endsWith(".wasm")) ct = "application/wasm";

      res.writeHead(200, { "Content-Type": ct });
      res.end(data);
    });
  });

  return new Promise((resolve) => {
    server.listen(port, () => resolve(server));
  });
}

/**
 * Set up a full WASM test environment.
 *
 * @param {Object} options - Setup options
 * @param {number} options.port - Port to use (default: random 8500-9500)
 * @param {string} options.logLevel - Log level (default: INFO)
 * @param {boolean} options.headless - Run headless (default: true)
 * @returns {Promise<{page: Page, browser: Browser, server: http.Server, cleanup: Function}>}
 */
export async function setupFullWasm(options = {}) {
  const port = options.port ?? 8500 + Math.floor(Math.random() * 1000);
  const logLevel = options.logLevel ?? "INFO";
  const headless = options.headless ?? true;

  // Build main.wasm
  if (!buildMainWasm({ logLevel })) {
    throw new Error("go build main.wasm failed");
  }

  // Start server
  const server = await createServer(port);

  // Launch browser
  const browser = await chromium.launch({
    headless,
    args: ["--autoplay-policy=no-user-gesture-required"],
  });

  const page = await browser.newPage();
  page.on("console", (msg) => {
    try {
      console.log("[PAGE]", msg.type(), msg.text());
    } catch (_) {}
  });

  // Navigate and wait for WASM to be ready
  await page.goto(`http://localhost:${port}/`);
  await page.waitForFunction(() => typeof ensureDefaultPath === "function", {
    timeout: 30000,
  });

  // Wait for initial layout and first draw
  await page.waitForTimeout(200);

  const cleanup = async () => {
    await browser.close();
    server.close();
  };

  return { page, browser, server, cleanup, port };
}

/**
 * Get the center coordinates of a rect.
 *
 * @param {{x: number, y: number, w: number, h: number}} rect
 * @returns {{x: number, y: number}}
 */
export function rectCenter(rect) {
  return {
    x: Math.floor(rect.x + rect.w / 2),
    y: Math.floor(rect.y + rect.h / 2),
  };
}

/**
 * Assert that a rect is valid and visible.
 *
 * @param {Object} rect - Rect object {x, y, w, h}
 * @param {string} name - Name for error messages
 */
export function assertValidRect(rect, name) {
  if (!rect) {
    throw new Error(`${name} rect is null/undefined`);
  }
  if (rect.w <= 0 || rect.h <= 0) {
    throw new Error(`${name} rect has invalid dimensions: ${JSON.stringify(rect)}`);
  }
}
