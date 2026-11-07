/** For REAL canvas interaction in the full build, which runs ebiten.RunGame() and so registers input handlers. Gotchas:
 * the exports are startPlay()/stopPlay() (optional chaining hides a typo); settle with forceDraw() + 300-500 ms after
 * importJSON(); read pixels with canvasPixelAt(); stop playback between scenarios; unlock mobile audio with cdpTap(). */

import http from "http";
import { spawnSync } from "child_process";
import fs from "fs";
import path from "path";
import { fileURLToPath } from "url";
import { resolveGoBinary, shouldSkipWasmBuild } from "./browser_test_helpers.js";
import { flushCoverage, isCoverageEnabled } from "./coverage_helpers.js";
import { launchBrowser } from "./platform/launch.js";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const jsDir = __dirname;
const goDir = path.resolve(jsDir, "../go");

/** The full browser event chain Ebiten expects. */
export async function clickAt(page, x, y) {
  await page.mouse.click(x, y);
}

export async function clickAndHold(page, x, y, holdMs = 100) {
  await page.mouse.move(x, y);
  await page.mouse.down();
  // At least one rAF tick so the loop sees pressed=true; under contention use ≥ 200 ms and poll for the outcome.
  await page.waitForTimeout(holdMs);
  await page.mouse.up();
}

/** Polls debugGridInputState() until the release is clean (suppression and held counters cleared); a fixed wait where
 * the export is missing. */
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

/** Holds while polling until evalExpr() !== prevState, so any rAF rate works; retries with a fresh press when a hold
 * times out. options: maxAttempts (5), holdTimeout (3000), pollMs (100). Returns the new state. */
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
    // So held counters and the click suppression are cleared.
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

/** options.steps: intermediate steps (default 10). */
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

/** deltaY > 0 scrolls down. */
export async function wheelAt(page, x, y, deltaY) {
  await page.mouse.move(x, y);
  await page.mouse.wheel(0, deltaY);
}

/** The full build with Ebiten input handlers. Returns true on success. */
export function buildMainWasm(options = {}) {
  const GO = resolveGoBinary();

  // The ABI file is regenerated first, as `make wasm` does: served verbatim, a stale one would mis-size a re-voiced block
  // and silence it while a current wasm passes. Unconditional (even under WASM_PREBUILT); a host tool, so GOOS/GOARCH
  // unset.
  {
    const hostEnv = { ...process.env };
    delete hostEnv.GOOS;
    delete hostEnv.GOARCH;
    const abi = spawnSync(GO, ["run", "./cmd/gen-synth-abi"], {
      cwd: goDir,
      env: hostEnv,
      encoding: "utf8",
    });
    if (abi.status !== 0) {
      console.error("[buildMainWasm] gen-synth-abi failed:", abi.stderr || abi.error || "");
      return false;
    }
    fs.writeFileSync(path.join(jsDir, "synth_param_abi.gen.js"), abi.stdout);
  }

  if (shouldSkipWasmBuild("main.wasm")) {
    return true;
  }

  const logLevel = options.logLevel ?? "INFO";

  // Only ./cmd: ./cmd/... would match tool binaries and fail the single -o output.
  const build = spawnSync(
    GO,
    [
      "build",
      "-ldflags",
      `-X main.defaultLog=${logLevel}`,
      "-o",
      path.join(jsDir, "main.wasm"),
      "./cmd",
    ],
    {
      cwd: goDir,
      env: { ...process.env, GOOS: "js", GOARCH: "wasm" },
      stdio: "inherit",
    }
  );

  return build.status === 0;
}

/** Port 0 avoids EADDRINUSE. BEATMO_LAN_SERVE=1 binds 0.0.0.0 for a phone on the LAN; "?probe=audio" serves the in-page
 * audio check. */
export function createServer() {
  const server = http.createServer((req, res) => {
    // "?run=tests" serves production index.html VERBATIM with one <script> injected, so every inline script runs exactly
    // as on beatmo.io; anything else is a strict production reproduction.
    const url = new URL(req.url, "http://placeholder");
    const noCache = {
      "Cache-Control": "no-store, no-cache, must-revalidate",
      "Pragma": "no-cache",
      "Expires": "0",
    };

    if (url.pathname === "/" && url.searchParams.get("run") === "tests") {
      fs.readFile(path.join(jsDir, "index.html"), "utf8", (err, html) => {
        if (err) { res.writeHead(404); res.end(); return; }
        const inject = '<script type="module" src="test_runner_overlay.js"></script>\n';
        const out = html.includes("</body>")
          ? html.replace("</body>", inject + "</body>")
          : html + inject;
        res.writeHead(200, { "Content-Type": "text/html", ...noCache });
        res.end(out);
      });
      return;
    }

    let file = url.pathname === "/" ? "/index.html" : url.pathname;
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

      res.writeHead(200, { "Content-Type": ct, ...noCache });
      res.end(data);
    });
  });

  const host = process.env.BEATMO_LAN_SERVE === "1" ? "0.0.0.0" : "127.0.0.1";
  const port = process.env.BEATMO_LAN_PORT
    ? parseInt(process.env.BEATMO_LAN_PORT, 10)
    : 0;

  return new Promise((resolve) => {
    server.listen(port, host, () => resolve(server));
  });
}

/** With LLM_RECORD=1 the page is wrapped by a recorder saved to src/js/recordings/<name>/ (LLM_RECORD_NAME).
 * options: logLevel (INFO), headless (true). Returns {page, browser, server, cleanup, recorder?}. */
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

  // Honours TEST_PLATFORM with no test changes.
  const launch = await launchBrowser({ headless });
  const browser = launch.browser;
  const browserCleanup = launch.cleanup;

  const page = await browser.newPage();
  page.on("console", (msg) => {
    try {
      console.log("[PAGE]", msg.type(), msg.text());
    } catch (_) {}
  });

  // Navigate and wait for WASM to be ready
  await page.goto(`http://localhost:${port}/`);
  await page.waitForFunction(() => typeof ensureDefaultPath === "function", null, {
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
      await browserCleanup();
      server.close();
    };

    return { page: wrappedPage, browser, server, cleanup, port, recorder, platform: launch.platform };
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
    await browserCleanup();
    server.close();
  };

  return { page, browser, server, cleanup, port, platform: launch.platform };
}

/** A bare audio.js page has no instruments, so boot the app once, copy the blocks out, and install them with
 * setInstrumentBlock(id, Float64Array.from(b.block), b.seconds, b.amp, b.melodic, b.gateTail, b.fx). */
export async function readAppInstrumentBlocks(ids) {
  const app = await setupFullWasm({ logLevel: "ERROR" });
  let blocks;
  try {
    blocks = await app.page.evaluate((ids) => Object.fromEntries(ids.map((id) => {
      const s = window.__instrumentBlockForTest(id);
      return [id, s && { block: Array.from(s.block), seconds: s.seconds, amp: s.amp, melodic: s.melodic, gateTail: s.gateTail, fx: s.fx }];
    })), ids);
  } finally {
    await app.cleanup();
  }
  const missing = ids.filter((id) => !blocks[id]);
  if (missing.length) throw new Error(`the app pushed no block for ${missing.join(", ")}`);
  return blocks;
}

export function rectCenter(rect) {
  return {
    x: Math.floor(rect.x + rect.w / 2),
    y: Math.floor(rect.y + rect.h / 2),
  };
}

/** name is for the error message. */
export function assertValidRect(rect, name) {
  if (!rect) {
    throw new Error(`${name} rect is null/undefined`);
  }
  if (rect.w <= 0 || rect.h <= 0) {
    throw new Error(`${name} rect has invalid dimensions: ${JSON.stringify(rect)}`);
  }
}
