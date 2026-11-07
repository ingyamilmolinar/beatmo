/** setInstrumentParam must change the rendered BYTES, not only a params map: the instrument rendered twice, the second
 * after post_decay 0.3, and the buffers must differ. */

import { chromium } from "playwright";
import { spawnSync } from "child_process";
import http from "http";
import fs from "fs";
import path from "path";
import { fileURLToPath } from "url";
import {
  resolveGoBinary,
  shouldSkipWasmBuild,
  flushCoverage,
  isCoverageEnabled,
  waitForAudioReady,
} from "./browser_test_helpers.js";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const jsDir = __dirname;

const chromiumPath = path.join(
  jsDir, "node_modules", ".cache", "ms-playwright", "chromium",
);
if (!fs.existsSync(chromiumPath)) {
  spawnSync("npx", ["playwright", "install", "chromium"], {
    cwd: jsDir, stdio: "inherit",
  });
}

const goDir = path.resolve(jsDir, "../go");
const GO = resolveGoBinary();

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

const browser = await chromium.launch({
  args: ["--autoplay-policy=no-user-gesture-required"],
});
const page = await browser.newPage();
page.on("console", (msg) => {
  try { console.log("[PAGE]", msg.type(), msg.text()); } catch (_) {}
});

await page.goto(`http://localhost:${port}/`);
await page.waitForFunction(() => typeof setInstrumentParam === "function");
await page.waitForFunction(() => typeof window.__testCaptureSynthRender === "function");
await page.waitForFunction(() => window.audioReady !== undefined);
await waitForAudioReady(page);

await page.evaluate(() => resetInstrumentParams('snare'));
const baseline = await page.evaluate(() => window.__testCaptureSynthRender('snare'));
if (!baseline) throw new Error('baseline render snapshot returned null');
console.log(`[TEST] baseline tailRMS=${baseline.tailRMS.toFixed(6)} length=${baseline.length}`);

// post_decay, not decay: an unknown key is dropped by the renderer, which made a bit-identical buffer look like a cache
// bug.
await page.evaluate(() => setInstrumentParam('snare', 'post_decay', 0.3));
const modified = await page.evaluate(() => window.__testCaptureSynthRender('snare'));
if (!modified) throw new Error('modified render snapshot returned null');
console.log(`[TEST] modified tailRMS=${modified.tailRMS.toFixed(6)} length=${modified.length}`);

if (baseline.length !== modified.length) {
  throw new Error(`buffer length changed unexpectedly: ${baseline.length} → ${modified.length}`);
}

// Identical would mean the edit never reached the C render.
let allEqual = true;
for (let i = 0; i < baseline.head.length; i++) {
  if (Math.abs(baseline.head[i] - modified.head[i]) > 1e-9) { allEqual = false; break; }
}
if (allEqual) {
  // Heads may match for the attack transient; check the tail explicitly.
  for (let i = 0; i < baseline.tail.length; i++) {
    if (Math.abs(baseline.tail[i] - modified.tail[i]) > 1e-9) { allEqual = false; break; }
  }
}
if (allEqual) {
  throw new Error(
    `BUG: setInstrumentParam('snare','post_decay',0.3) produced bit-identical buffer.\n` +
    `  baseline tailRMS=${baseline.tailRMS}\n  modified tailRMS=${modified.tailRMS}\n` +
    `This means the renderCache was either not invalidated, or the next render ` +
    `did not pick up snare's pushed block (setInstrumentBlock).`,
  );
}

// A conservative 25 % against C-side tweaks.
const drop = (baseline.tailRMS - modified.tailRMS) / Math.max(baseline.tailRMS, 1e-12);
if (drop < 0.25) {
  throw new Error(
    `post_decay=0.3 should reduce tail RMS by >25%; got ${(drop * 100).toFixed(1)}% drop.\n` +
    `  baseline=${baseline.tailRMS}\n  modified=${modified.tailRMS}`,
  );
}
console.log(`[TEST] setInstrumentParam audibly shortened decay envelope (tail RMS drop: ${(drop * 100).toFixed(1)}%)`);

// ─── Test 2: ResetInstrumentParams restores the baseline render ───
await page.evaluate(() => resetInstrumentParams('snare'));
const restored = await page.evaluate(() => window.__testCaptureSynthRender('snare'));
if (!restored) throw new Error('restored render snapshot returned null');
const restoredDrop = Math.abs(baseline.tailRMS - restored.tailRMS) / Math.max(baseline.tailRMS, 1e-12);
if (restoredDrop > 0.05) {
  throw new Error(
    `Reset did not restore baseline tail RMS: baseline=${baseline.tailRMS}, restored=${restored.tailRMS} (drift ${(restoredDrop * 100).toFixed(1)}%)`,
  );
}
console.log(`[TEST] resetInstrumentParams restored baseline (drift: ${(restoredDrop * 100).toFixed(2)}%)`);

console.log("[TEST] All audible-change regression checks passed.");

if (isCoverageEnabled()) {
  await flushCoverage(
    page,
    new URL("../../coverage/browser-raw", import.meta.url).pathname,
    "instrument_params_audible",
  );
}
await browser.close();
server.close();
console.log("instrument_params_audible browser test PASSED");
