// wasm_env.browser.test.js
//
// Proves window.__beatmoEnv reaches Go's os.Getenv in the browser.
//
// Before the fix, src/js/wasm_exec.js constructed the Go bridge with
// `this.env = {}` and nothing in the codebase ever assigned to it, so every
// os.Getenv call in the WASM build silently returned "". That disabled ~95
// environment-variable knobs, including PARITY_WASM_FATAL and
// PARITY_TIMING_FATAL, which exist for no other purpose than being set on
// WASM. The fix populates `go.env` from `window.__beatmoEnv` in index.html
// before `go.run()`, since wasm_exec.js serializes `go.env` into WASM linear
// memory as KEY=value pairs at run(), which is exactly what Go's
// goenvs_unix reads at startup.
//
// This test boots the REAL production index.html (not a synthetic harness)
// so it exercises the actual bootstrap fix, and calls the debug-only
// `window.__envProbe(name)` export (internal/ui/js_exports_diag.go), which
// wraps os.Getenv, to observe what Go actually sees.

import { chromium } from "playwright";
import { spawnSync } from "child_process";
import http from "http";
import fs from "fs";
import path from "path";
import { fileURLToPath } from "url";
import { resolveGoBinary, shouldSkipWasmBuild } from "./browser_test_helpers.js";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const jsDir = __dirname;
const goDir = path.resolve(jsDir, "../go");
const GO = resolveGoBinary();

// Build the real production main.wasm (same target as wasm_load_startup) so
// this test exercises the actual index.html bootstrap, not a stand-in.
if (!shouldSkipWasmBuild("main.wasm")) {
  const build = spawnSync(
    GO,
    ["build", "-o", path.join(jsDir, "main.wasm"), "./cmd/"],
    { cwd: goDir, env: { ...process.env, GOOS: "js", GOARCH: "wasm" }, stdio: "inherit" },
  );
  if (build.status !== 0) throw new Error("go build main wasm failed");
}

function contentType(file) {
  if (file.endsWith(".html")) return "text/html; charset=utf-8";
  if (file.endsWith(".js")) return "application/javascript; charset=utf-8";
  if (file.endsWith(".wasm")) return "application/wasm";
  return "application/octet-stream";
}

const server = http.createServer((req, res) => {
  const file = req.url === "/" ? "/index.html" : req.url.split("?")[0];
  const filePath = path.join(jsDir, file.replace(/^\//, ""));
  fs.readFile(filePath, (err, data) => {
    if (err) { res.writeHead(404); res.end(); return; }
    res.writeHead(200, { "Content-Type": contentType(filePath) });
    res.end(data);
  });
});
await new Promise((r) => server.listen(0, r));
const port = server.address().port;

const browser = await chromium.launch({ args: ["--autoplay-policy=no-user-gesture-required"] });

try {
  const page = await browser.newPage();

  // Must run BEFORE any page script (including index.html's bootstrap), so
  // window.__beatmoEnv exists by the time `go.env = { ...window.__beatmoEnv }`
  // executes.
  await page.addInitScript(() => {
    window.__beatmoEnv = { BEATMO_WASM_ENV_PROBE: "hello-from-js" };
  });

  await page.goto(`http://localhost:${port}/`);

  // __envProbe is registered synchronously during Game construction
  // (initJSDiag, called from initJS in game_new.go), well before the app's
  // first frame — poll for it directly rather than depending on the
  // unrelated boot-waterfall milestones.
  await page.waitForFunction(() => typeof window.__envProbe === "function", {}, { timeout: 30000 });

  const got = await page.evaluate(() => window.__envProbe("BEATMO_WASM_ENV_PROBE"));
  if (got !== "hello-from-js") {
    throw new Error(`envProbe returned ${JSON.stringify(got)}, want "hello-from-js"`);
  }

  const missing = await page.evaluate(() => window.__envProbe("BEATMO_WASM_ENV_ABSENT"));
  if (missing !== "") {
    throw new Error(`absent var returned ${JSON.stringify(missing)}, want ""`);
  }

  await page.close();
} finally {
  await browser.close();
  server.close();
}

console.log("wasm_env: __beatmoEnv reaches Go's os.Getenv via go.env — PASS");
