// dev_server_boot.browser.test.js
//
// The page must BOOT when served by `make serve` -- the server a developer
// actually runs (scripts/serve_nocache.py), not a Node replica of it.
//
// Every other browser test serves src/js/ from Node (real_input_test_helpers'
// createServer, landing_server.js, wasm_load_startup's gzip server), and Node
// answers requests concurrently. `make serve` was a ONE-connection-at-a-time
// Python server, and index.html's first <head> script (landing page spec,
// "boot order is game-first") starts fetch("main.wasm") before audio.js and its
// static imports have loaded. Chromium stops reading an unconsumed body after a
// few MB, the single-threaded server blocks writing the wasm, audio.js's
// imports (synth_param_abi.gen.js, chain_spec.gen.js, dsp_host.js) are
// never answered, and the module that would consume the wasm never runs: the
// page sits on "Loading..." forever with NO console error. Firefox and WebKit
// buffer the whole body, so only Chromium-family browsers hung.
//
// Checked, per engine (Chromium, Firefox, WebKit):
//   - the plain dev page (no landing) reaches its first frame (TTI);
//   - ?landing=1 shows the landing and its Play button becomes ready.
// Fast, browser-free half of the same guard (the server answers while another
// response is unread): src/go/internal/deploy/dev_server_concurrency_test.go.
//
// Run: GO=$(pwd)/.tools/go/bin/go node src/js/dev_server_boot.browser.test.js
//   WASM_PREBUILT=1 serves the existing src/js/main.wasm;
//   DEV_BOOT_ENGINES=chromium,firefox,webkit picks engines;
//   DEV_BOOT_TIMEOUT_MS (default 60000) bounds each boot.

import { chromium, firefox, webkit } from "playwright";
import { spawn, spawnSync } from "child_process";
import net from "net";
import path from "path";
import { fileURLToPath } from "url";
import { resolveGoBinary, shouldSkipWasmBuild } from "./browser_test_helpers.js";
import { FIREFOX_WEBGL_PREFS } from "./platform/launch.js";

const jsDir = path.dirname(fileURLToPath(import.meta.url));
const repoRoot = path.resolve(jsDir, "../..");
const goDir = path.resolve(jsDir, "../go");
const BOOT_TIMEOUT_MS = Number(process.env.DEV_BOOT_TIMEOUT_MS || 60000);
const ENGINES = { chromium, firefox, webkit };
const engineNames = (process.env.DEV_BOOT_ENGINES || "chromium,firefox,webkit").split(",").map((s) => s.trim()).filter(Boolean);

if (!shouldSkipWasmBuild("main.wasm")) {
  const build = spawnSync(
    resolveGoBinary(),
    ["build", "-ldflags", "-s -w -X main.defaultLog=INFO", "-o", path.join(jsDir, "main.wasm"), "./cmd/"],
    { cwd: goDir, env: { ...process.env, GOOS: "js", GOARCH: "wasm" }, stdio: "inherit" },
  );
  if (build.status !== 0) throw new Error("go build main wasm failed");
}

async function freePort() {
  return new Promise((resolve, reject) => {
    const s = net.createServer();
    s.once("error", reject);
    s.listen(0, "127.0.0.1", () => { const { port } = s.address(); s.close(() => resolve(port)); });
  });
}

async function waitListening(port, ms = 10000) {
  const deadline = Date.now() + ms;
  while (Date.now() < deadline) {
    const ok = await new Promise((resolve) => {
      const c = net.connect(port, "127.0.0.1", () => { c.end(); resolve(true); });
      c.once("error", () => resolve(false));
    });
    if (ok) return;
    await new Promise((r) => setTimeout(r, 100));
  }
  throw new Error(`serve_nocache.py never listened on ${port}`);
}

// Exactly what `make serve` runs (Makefile: cd src/js && python3 ../../scripts/serve_nocache.py 8080).
const port = await freePort();
const server = spawn("python3", [path.join(repoRoot, "scripts/serve_nocache.py"), String(port)], { cwd: jsDir, stdio: "ignore" });
const failures = [];
let checks = 0;
function check(ok, msg) {
  checks++;
  if (!ok) { failures.push(msg); console.error(`  FAIL ${msg}`); } else console.log(`  ok   ${msg}`);
}

// What the page reached, for the failure message: which boot milestone it
// stopped at and which requests never completed.
async function bootState(page) {
  return page.evaluate(() => ({
    boot: window.__beatmoBoot || null,
    audioJsRan: typeof window.audioReady !== "undefined",
    readyState: document.readyState,
  })).catch((e) => ({ error: String(e) }));
}

try {
  await waitListening(port);
  for (const name of engineNames) {
    const engine = ENGINES[name];
    if (!engine) throw new Error(`unknown engine ${name}`);
    console.log(`\n== ${name}`);
    const browser = await engine.launch(name === "firefox" ? { firefoxUserPrefs: FIREFOX_WEBGL_PREFS } : {});
    try {
      for (const variant of [
        { label: "dev page", query: "", landing: false },
        { label: "?landing=1", query: "?landing=1", landing: true },
      ]) {
        const page = await browser.newPage();
        const pending = new Set();
        page.on("request", (r) => pending.add(r));
        page.on("requestfinished", (r) => pending.delete(r));
        page.on("requestfailed", (r) => pending.delete(r));
        const errors = [];
        page.on("pageerror", (e) => errors.push(e.message));
        // A Go panic or exit reaches the console (Go's ERROR lines as plain
        // logs), never pageerror: the last lines name the cause.
        const consoleTail = [];
        page.on("console", (m) => { consoleTail.push(`${m.type()}: ${m.text()}`.slice(0, 300)); if (consoleTail.length > 15) consoleTail.shift(); });
        // "commit", not "load": a hung boot never fires load, and the
        // diagnosis below is the point of the test.
        await page.goto(`http://127.0.0.1:${port}/${variant.query}`, { waitUntil: "commit" });
        const ready = variant.landing
          ? () => document.documentElement.hasAttribute("data-landing") &&
              !!(window.__beatmoBoot && window.__beatmoBoot.firstFrame) &&
              document.getElementById("lc-play")?.classList.contains("ready")
          : () => !!(window.__beatmoBoot && window.__beatmoBoot.firstFrame);
        let booted = true;
        try {
          await page.waitForFunction(ready, null, { timeout: BOOT_TIMEOUT_MS, polling: 250 });
        } catch (_) {
          booted = false;
        }
        const st = await bootState(page);
        const stuck = [...pending].map((r) => new URL(r.url()).pathname);
        check(booted, `${name} ${variant.label}: boots under \`make serve\` within ${BOOT_TIMEOUT_MS} ms` +
          (booted ? ` (firstFrame ${Math.round(st.boot?.firstFrame ?? -1)} ms)` :
            ` -- stuck at ${JSON.stringify(st)}; requests never answered: ${JSON.stringify(stuck)}`));
        const landing = await page.evaluate(() => document.documentElement.hasAttribute("data-landing")).catch(() => null);
        check(landing === variant.landing, variant.landing
          ? `${name} ${variant.label}: the landing is shown`
          : `${name} ${variant.label}: an unstamped dev page shows no landing`);
        check(errors.length === 0, `${name} ${variant.label}: no page errors ${errors.length ? JSON.stringify(errors) +
          "; console: " + JSON.stringify(consoleTail) : ""}`);
        await page.close();
      }
    } finally {
      await browser.close();
    }
  }
} finally {
  server.kill();
}

if (failures.length) {
  console.error(`\ndev_server_boot: ${failures.length}/${checks} checks FAILED`);
  for (const f of failures) console.error(`  - ${f}`);
  process.exit(1);
}
console.log(`\ndev_server_boot: all ${checks} checks passed`);
