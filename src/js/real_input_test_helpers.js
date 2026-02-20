/**
 * Real Input Test Helpers
 *
 * These helpers are specifically designed for testing REAL canvas interactions
 * in the full WASM build (main.wasm). Unlike the playtest harness (play_ui.wasm),
 * the full build runs ebiten.RunGame() which properly registers input handlers.
 *
 * Uses Playwright's native mouse methods which properly integrate with the browser's
 * event system and Ebiten's input handling.
 *
 * Browser E2E Gotchas:
 *   - JS export names matter: startPlay()/stopPlay() — NOT start()/stop().
 *     Optional chaining (?.) silently returns undefined for missing functions,
 *     so typos fail silently.
 *   - After importJSON(), UI needs settling: call forceDraw() +
 *     page.waitForTimeout(300-500) before interacting. Button rects may not
 *     be valid until layout recalculates.
 *   - Use API calls (startPlay/stopPlay) when testing non-input features.
 *     Real mouse clicks are fragile after mid-test imports.
 *   - Canvas pixel reading: use canvasPixelAt() from real_input_actions.js
 *     (screenshot → PNG → offscreen canvas decode, bypasses WebGL buffer swap).
 *   - Cross-scenario state leaks: always stop playback before starting next
 *     scenario. A silent no-op (calling stop() instead of stopPlay()) means
 *     the next startPlay() toggles playback OFF.
 *   - Mobile audio unlock: audio.js registers listeners on touchstart,
 *     touchend, pointerdown, mousedown, keydown. For mobile emulation tests,
 *     use CDP trusted touch events via cdpTap() from touch_cdp_helpers.js.
 */

import { chromium } from "playwright";
import http from "http";
import { spawnSync } from "child_process";
import fs from "fs";
import path from "path";
import { fileURLToPath } from "url";
import { resolveGoBinary, shouldSkipWasmBuild } from "./browser_test_helpers.js";
import { flushCoverage, isCoverageEnabled } from "./coverage_helpers.js";

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
  // Hold long enough for at least one rAF tick so the WASM game loop sees
  // pressed=true.  Under CPU contention (parallel tests), rAF can be delayed,
  // so callers should use generous hold times (≥200ms) and poll for the
  // expected state change rather than relying on fixed waits.
  await page.waitForTimeout(holdMs);
  await page.mouse.up();
}

/**
 * Wait for the game loop to process a mouse-up so that internal input flags
 * (suppressClicksUntilRelease, held counters, etc.) are fully cleared.
 *
 * Polls `debugGridInputState()` — an existing JS export that exposes the
 * game's internal input state — until it reports a clean release.  Falls back
 * to a fixed wait when the export is unavailable (e.g. older WASM builds).
 *
 * @param {Page}   page       - Playwright page object
 * @param {number} [timeout=5000] - Max ms to wait for clean state
 */
export async function waitForGameLoopRelease(page, timeout = 5000) {
  // Ensure mouse is up before polling.
  await page.mouse.up();

  const hasExport = await page.evaluate(() => typeof debugGridInputState === "function");
  if (!hasExport) {
    await page.waitForTimeout(500);
    return;
  }

  await page.waitForFunction(() => {
    const s = debugGridInputState();
    return (
      s.suppressClicks === false &&
      s.drumMouseDownInBounds === false &&
      s.drumAnyDragActive === false &&
      s.drumAnyDropdownOpen === false
    );
  }, { timeout });
}

/**
 * Hold mouse down at (x, y) and poll until evalExpr returns a value different
 * from prevState.  Adapts to any rAF rate — under heavy CPU contention the
 * poll simply keeps waiting while the button is held.  Retries with a fresh
 * press cycle if the hold times out (clears suppressClicksUntilRelease).
 *
 * @param {Page}     page       - Playwright page object
 * @param {number}   x          - X coordinate
 * @param {number}   y          - Y coordinate
 * @param {Function} evalExpr   - Function evaluated in page context; should return the state to watch
 * @param {*}        prevState  - The "before" value; we wait until evalExpr !== prevState
 * @param {Object}   [options]
 * @param {number}   [options.maxAttempts=5]  - Press-release retry cycles
 * @param {number}   [options.holdTimeout=3000] - Max ms to keep the button held per attempt
 * @param {number}   [options.pollMs=100]     - Polling interval inside the hold
 * @returns {Promise<*>} The new state value (first value !== prevState)
 */
export async function clickUntilStateChanges(page, x, y, evalExpr, prevState, options = {}) {
  const { maxAttempts = 5, holdTimeout = 3000, pollMs = 100 } = options;
  for (let attempt = 0; attempt < maxAttempts; attempt++) {
    await page.mouse.move(x, y);
    await page.mouse.down();
    const deadline = Date.now() + holdTimeout;
    while (Date.now() < deadline) {
      await page.waitForTimeout(pollMs);
      const current = await page.evaluate(evalExpr);
      if (current !== prevState) {
        await page.mouse.up();
        return current;
      }
    }
    // Release and wait for the game loop to fully process the mouse-up
    // so that held counters and suppressClicksUntilRelease are cleared.
    await waitForGameLoopRelease(page, 5000);
  }
  // Gather diagnostics before throwing
  let diag = "";
  try {
    const state = await page.evaluate(() =>
      typeof debugGridInputState === "function" ? JSON.stringify(debugGridInputState()) : "unavailable"
    );
    diag = ` | inputState: ${state}`;
  } catch (_) {}
  throw new Error(
    `clickUntilStateChanges: state did not change after ${maxAttempts} attempts at (${x}, ${y})${diag}`
  );
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
  if (shouldSkipWasmBuild("main.wasm")) {
    return true;
  }

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
 * Uses OS-assigned port (port 0) to avoid EADDRINUSE collisions.
 *
 * @returns {Promise<http.Server>} - HTTP server instance (call server.address().port for assigned port)
 */
export function createServer() {
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
    server.listen(0, () => resolve(server));
  });
}

/**
 * Set up a full WASM test environment.
 *
 * When LLM_RECORD=1 is set, automatically wraps the page with a recorder
 * that captures events + periodic screenshots. On cleanup, the recording is
 * saved to src/js/recordings/<name>/. Set LLM_RECORD_NAME to control the
 * session name (defaults to the test filename or a timestamp).
 *
 * @param {Object} options - Setup options
 * @param {string} options.logLevel - Log level (default: INFO)
 * @param {boolean} options.headless - Run headless (default: true)
 * @returns {Promise<{page: Page, browser: Browser, server: http.Server, cleanup: Function, recorder?: Object}>}
 */
export async function setupFullWasm(options = {}) {
  const logLevel = options.logLevel ?? "INFO";
  const shouldRecord = process.env.LLM_RECORD === "1";
  const headless = shouldRecord ? false : (options.headless ?? true);

  // Build main.wasm
  if (!buildMainWasm({ logLevel })) {
    throw new Error("go build main.wasm failed");
  }

  // Start server on OS-assigned port
  const server = await createServer();
  const port = server.address().port;

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

  // --- LLM_RECORD integration ---
  if (shouldRecord) {
    const { createRecorder } = await import("./llm_test/recorder.js");

    const name =
      process.env.LLM_RECORD_NAME ??
      `session_${Date.now()}`;
    const frameIntervalMs = parseInt(process.env.LLM_RECORD_INTERVAL ?? "500", 10);
    const viewport = page.viewportSize() ?? { width: 1280, height: 720 };
    const recordingsDir = path.resolve(jsDir, "recordings");
    const savePath = path.join(recordingsDir, name);

    const recorder = createRecorder(page, {
      mode: "test",
      frameIntervalMs,
      viewport,
    });

    await recorder.start();
    const wrappedPage = recorder.getWrappedPage();

    const cleanup = async () => {
      try {
        if (isCoverageEnabled()) {
          const testName = path.basename(process.argv[1], ".js");
          const covDir = path.resolve(jsDir, "..", "..", "coverage", "browser-raw");
          await flushCoverage(wrappedPage, covDir, testName);
        }
      } catch (e) {
        console.warn(`[coverage] flush error: ${e.message}`);
      }
      try {
        if (recorder.isRecording()) {
          await recorder.stop();
          const recording = await recorder.save(savePath);
          console.log(
            `[llm_record] Recording saved to ${savePath}/ ` +
            `(${recording.events.length} events, ${recording.frames.length} frames)`
          );
        }
      } catch (e) {
        console.warn(`[llm_record] Failed to save recording: ${e.message}`);
      }
      await browser.close();
      server.close();
    };

    return { page: wrappedPage, browser, server, cleanup, port, recorder };
  }

  const cleanup = async () => {
    try {
      if (isCoverageEnabled()) {
        const testName = path.basename(process.argv[1], ".js");
        const covDir = path.resolve(jsDir, "..", "..", "coverage", "browser-raw");
        await flushCoverage(page, covDir, testName);
      }
    } catch (e) {
      console.warn(`[coverage] flush error: ${e.message}`);
    }
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
