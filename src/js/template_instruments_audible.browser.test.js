/**
 * Masterpiece-template instruments — WASM audible regression test.
 *
 * The genre templates ship instruments that are Go builtins (config.go +
 * instrument_ids_gen.go, bound to synth-modular-* recipes). On WASM every synth
 * instrument is rendered by the JS bridge, which keys on the static RENDER /
 * RENDER_INFO tables in audio.js. `organ` and `sax` were added as Go builtins
 * but NOT to those tables, so playSoundParams('organ'|'sax') found no render
 * function and threw "Unknown sound" — SILENT on beatmo.io while playing fine on
 * desktop (the native build renders them directly via render_modular, never
 * consulting the JS table). Templates such as sax-blues / bach-toccata were
 * therefore mute in the browser.
 *
 * This test renders each instrument through the REAL WASM↔WebAudio bridge
 * (window.__testCaptureSynthRender → ensureRenderedSample → render_modular_p)
 * and asserts a non-null, non-silent buffer. Without the RENDER/RENDER_INFO
 * entries the helper returns null (the "Unknown sound" path) and the test fails.
 *
 * COVERED-BY-GO: internal/audio/render_table_jssrc_test.go pins the static-table
 * drift; THIS test owns the end-to-end WASM render path the user actually hears.
 */

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
} from "./browser_test_helpers.js";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const jsDir = __dirname;

const chromiumPath = path.join(jsDir, "node_modules", ".cache", "ms-playwright", "chromium");
if (!fs.existsSync(chromiumPath)) {
  spawnSync("npx", ["playwright", "install", "chromium"], { cwd: jsDir, stdio: "inherit" });
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

// Hard backstop: never let a stuck headless WASM boot wedge the test runner.
const hardTimer = setTimeout(() => {
  console.error("template_instruments_audible: hard timeout (180s) — aborting");
  process.exit(1);
}, 180_000);

const browser = await chromium.launch({ args: ["--autoplay-policy=no-user-gesture-required"] });
const page = await browser.newPage();
page.on("console", (msg) => { try { console.log("[PAGE]", msg.type(), msg.text()); } catch (_) {} });
let pageDied = null;
page.on("crash", () => { pageDied = "page crashed"; });
page.on("pageerror", (e) => { console.log("[PAGEERROR]", String(e)); });

// withTimeout bounds a single render so a hang becomes a clear failure rather
// than a wall-clock runner timeout.
const withTimeout = (p, ms, label) =>
  Promise.race([p, new Promise((_, rej) => setTimeout(() => rej(new Error(`${label} timed out after ${ms}ms`)), ms))]);

await page.goto(`http://localhost:${port}/`);
await page.waitForFunction(() => typeof window.__testCaptureSynthRender === "function", null, { timeout: 60_000 });
await page.waitForFunction(() => window.audioReady !== undefined, null, { timeout: 60_000 });
await page.evaluate(async () => { await window.audioReady; });

// The instruments the masterpiece templates depend on. `bass-guitar` is the
// control: it was always in the RENDER table and proves the harness renders.
// `organ` and `sax` are the regression subjects.
const SUBJECTS = ["bass-guitar", "organ", "sax"];

const failures = [];
for (const id of SUBJECTS) {
  if (pageDied) throw new Error(`page died before rendering ${id}: ${pageDied}`);
  let cap;
  try {
    cap = await withTimeout(page.evaluate((x) => window.__testCaptureSynthRender(x), id), 30_000, `${id} render`);
  } catch (e) {
    failures.push(`${id}: ${String(e.message || e)}`);
    continue;
  }
  if (!cap) {
    failures.push(`${id}: render returned null — RENDER['${id}'] missing in audio.js, so playSoundParams('${id}') throws "Unknown sound" and is SILENT on WASM`);
    continue;
  }
  const headRMS = Math.sqrt(cap.head.reduce((s, v) => s + v * v, 0) / cap.head.length);
  const audible = cap.tailRMS > 1e-6 || headRMS > 1e-6 || cap.peak > 1e-4;
  if (!audible) {
    failures.push(`${id}: render is silent (headRMS=${headRMS}, tailRMS=${cap.tailRMS}, peak=${cap.peak})`);
    continue;
  }
  console.log(`[TEST] ${id} renders audibly on WASM (tailRMS=${cap.tailRMS.toFixed(6)}, peak=${cap.peak.toFixed(6)})`);
}

if (isCoverageEnabled()) {
  await flushCoverage(page, new URL("../../coverage/browser-raw", import.meta.url).pathname, "template_instruments_audible");
}
await browser.close();
server.close();
clearTimeout(hardTimer);

if (failures.length > 0) {
  console.error("template_instruments_audible browser test FAILED:\n  - " + failures.join("\n  - "));
  process.exit(1);
}
console.log("template_instruments_audible browser test PASSED");
