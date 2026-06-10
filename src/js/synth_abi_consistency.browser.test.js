/**
 * Synth ABI consistency guard (WASM).
 *
 * The generated src/js/synth_param_abi.gen.js maps the C modular_params struct
 * field order to the JS heap-write indices audio.js uses. If it ships stale
 * (e.g. a `make wasm` that skipped gen-synth-abi before that was wired into the
 * build), the modular param block is undersized and a re-voiced instrument's
 * oscillator reads an uninitialised enable flag → SILENCE. That was the
 * "audio is gone when I change the oscillator off Native" bug.
 *
 * This is the fast guard: it asserts the loaded ABI carries every field the C
 * modular voice requires and that audio.js's own validateModularABI() did not
 * flag it stale. Pairs with the Go drift test (synth_param_schema_test.go) and
 * the behavioural test (synth_generator_audible.browser.test.js).
 */

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

const chromiumPath = path.join(jsDir, "node_modules", ".cache", "ms-playwright", "chromium");
if (!fs.existsSync(chromiumPath)) {
  spawnSync("npx", ["playwright", "install", "chromium"], { cwd: jsDir, stdio: "inherit" });
}
if (!shouldSkipWasmBuild("play_ui.wasm")) {
  const build = spawnSync(
    GO, ["build", "-o", path.join(jsDir, "play_ui.wasm"), "./internal/ui/playtest"],
    { cwd: goDir, env: { ...process.env, GOOS: "js", GOARCH: "wasm" }, stdio: "inherit" },
  );
  if (build.status !== 0) throw new Error("go build play_ui failed");
}

const server = http.createServer((req, res) => {
  const file = req.url === "/" ? "/play_ui.html" : req.url;
  const fp = path.join(jsDir, file.replace(/^\//, ""));
  fs.readFile(fp, (err, data) => {
    if (err) { res.writeHead(404); res.end(); return; }
    let ct = "text/plain";
    if (fp.endsWith(".html")) ct = "text/html";
    else if (fp.endsWith(".js")) ct = "application/javascript";
    else if (fp.endsWith(".wasm")) ct = "application/wasm";
    res.writeHead(200, { "Content-Type": ct });
    res.end(data);
  });
});
await new Promise((r) => server.listen(0, r));
const port = server.address().port;

const browser = await chromium.launch({ args: ["--autoplay-policy=no-user-gesture-required"] });
const page = await browser.newPage();
page.on("console", (msg) => { try { console.log("[PAGE]", msg.type(), msg.text()); } catch (_) {} });

let failed = false;
const fail = (m) => { failed = true; console.error("[FAIL] " + m); };

try {
  await page.goto(`http://localhost:${port}/`);
  await page.waitForFunction(() => typeof window.__synthABI === "object" && window.__synthABI !== null, { timeout: 10000 });
  const abi = await page.evaluate(() => window.__synthABI);
  console.log("[TEST] __synthABI =", JSON.stringify(abi));

  // The C modular_params struct currently has 33 fields. A stale gen.js (the
  // pre-generator-selector 27-field version) is the exact silence trigger.
  if (abi.stale) {
    fail("audio.js validateModularABI() flagged the ABI STALE — gen.js is out of sync with the C struct (run `make gen-synth-abi`).");
  }
  if (!abi.hasModularFields) {
    fail("loaded ABI is missing required modular fields (osc_enabled/.../noise_seed) — re-voiced instruments would be silent.");
  }
  if (!(abi.modularCount >= 33)) {
    fail(`MODULAR_PARAM_COUNT=${abi.modularCount}, want >= 33 (the modular_params struct width). A smaller count undersizes the param malloc → silent generator.`);
  }
} finally {
  await browser.close();
  server.close();
}

if (failed) { console.error("[TEST] synth ABI consistency: FAIL"); process.exit(1); }
console.log("[TEST] synth ABI consistency: PASS");
