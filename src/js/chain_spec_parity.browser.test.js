// chain_spec_parity.browser.test.js
//
// Layer 2 of the cross-platform audio parity suite: asserts the BROWSER's live
// WebAudio master chain (compressor + soft-clip limiter) is configured to the
// exact same values as the desktop Go/C chain. The shared source of truth is
// src/js/chain_spec.gen.js (generated from Go's chain_spec.go); the desktop
// side is locked by internal/audio/chain_spec_test.go.
//
// This test FAILS before the audio.js fix (compressor was -6dB/4:1 with no
// soft-clip) and PASSES after — it is the proof that the audible desktop↔browser
// divergence in the master chain is closed.
//
// Lightweight: only imports audio.js (no main.wasm build). The lazy WebAudio
// nodes are forced into existence via window.getChainConfigForTest().
//
// Usage: node src/js/chain_spec_parity.browser.test.js

import { chromium } from "playwright";
import http from "http";
import fs from "fs";
import path from "path";
import { fileURLToPath } from "url";
import { CHAIN_SPEC } from "./chain_spec.gen.js";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const jsDir = __dirname;

const chromiumPath = path.join(jsDir, "node_modules", ".cache", "ms-playwright", "chromium");
if (!fs.existsSync(chromiumPath)) {
  const { spawnSync } = await import("child_process");
  spawnSync("npx", ["playwright", "install", "chromium"], { cwd: jsDir, stdio: "inherit" });
}

const server = http.createServer((req, res) => {
  if (req.url === "/" || req.url === "/test.html") {
    const html = `<!DOCTYPE html><html><body>
<script type="module">
  import './audio.js';
  window.__ready = true;
</script>
</body></html>`;
    res.writeHead(200, { "Content-Type": "text/html" });
    res.end(html);
    return;
  }
  const filePath = path.join(jsDir, req.url.replace(/^\//, ""));
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
const page = await browser.newPage();
page.on("console", (msg) => { if (process.env.TEST_LOG) console.log("[PAGE]", msg.type(), msg.text()); });

let failed = false;
const fail = (m) => { console.log(`  FAIL: ${m}`); failed = true; };
const ok = (m) => console.log(`  OK: ${m}`);
const approx = (a, b, eps = 1e-6) => Math.abs(a - b) <= eps;

try {
  await page.goto(`http://localhost:${port}/test.html`);
  await page.waitForFunction(() => window.__ready === true, { timeout: 10000 });
  await page.waitForFunction(() => typeof window.getChainConfigForTest === "function", { timeout: 5000 });

  const cfg = await page.evaluate(() => window.getChainConfigForTest());
  if (!cfg) {
    console.log("SKIP: no AudioContext available in this headless environment");
    await browser.close();
    server.close();
    process.exit(0);
  }

  console.log("--- Master compressor parity (browser live nodes vs CHAIN_SPEC) ---");
  const c = cfg.compressor;
  if (!approx(c.thresholdDb, CHAIN_SPEC.compressorThresholdDb)) {
    fail(`compressor threshold: live=${c.thresholdDb} spec=${CHAIN_SPEC.compressorThresholdDb}`);
  } else ok(`threshold ${c.thresholdDb} dB`);
  if (!approx(c.ratio, CHAIN_SPEC.compressorRatio)) {
    fail(`compressor ratio: live=${c.ratio} spec=${CHAIN_SPEC.compressorRatio}`);
  } else ok(`ratio ${c.ratio}:1`);
  if (!approx(c.attackSec, CHAIN_SPEC.compressorAttackMs / 1000)) {
    fail(`compressor attack: live=${c.attackSec}s spec=${CHAIN_SPEC.compressorAttackMs / 1000}s`);
  } else ok(`attack ${c.attackSec * 1000} ms`);
  if (!approx(c.releaseSec, CHAIN_SPEC.compressorReleaseMs / 1000)) {
    fail(`compressor release: live=${c.releaseSec}s spec=${CHAIN_SPEC.compressorReleaseMs / 1000}s`);
  } else ok(`release ${c.releaseSec * 1000} ms`);
  if (!approx(c.kneeDb, CHAIN_SPEC.compressorKneeDb)) {
    fail(`compressor knee: live=${c.kneeDb} spec=${CHAIN_SPEC.compressorKneeDb}`);
  } else ok(`knee ${c.kneeDb} dB`);

  console.log("\n--- Soft-clip parity (limiter curve applies tanh knee) ---");
  const t = CHAIN_SPEC.softClipThreshold;
  if (!approx(cfg.softClip.threshold, t)) {
    fail(`soft-clip threshold: live=${cfg.softClip.threshold} spec=${t}`);
  } else ok(`threshold ${t}`);
  // A pure hard clamp would map probeIn=1.5 -> 1.0. The soft-clip maps it to
  // t*tanh(1.5/t) < 1.0 (and < the hard-clamp value), proving the knee exists.
  const expected = t * Math.tanh(cfg.softClip.probeIn / t);
  if (!approx(cfg.softClip.probeOut, expected)) {
    fail(`soft-clip output: live=${cfg.softClip.probeOut} expected=${expected}`);
  } else if (!(cfg.softClip.probeOut < 1.0 - 1e-6)) {
    fail(`soft-clip not applied: probeOut=${cfg.softClip.probeOut} should be < 1.0 (hard-clamp regression)`);
  } else ok(`tanh soft-clip active (1.5 -> ${cfg.softClip.probeOut.toFixed(4)})`);
} catch (err) {
  console.error("ERROR:", err);
  failed = true;
} finally {
  await browser.close();
  server.close();
}

if (failed) {
  console.log("\nCHAIN SPEC PARITY: FAIL");
  process.exit(1);
}
console.log("\nCHAIN SPEC PARITY: PASS");
process.exit(0);
