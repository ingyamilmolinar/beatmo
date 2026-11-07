/** Every instrument a shipped template plays must render audibly through the real bridge: a built-in missing from the
 * browser's render path once played silently on beatmo.io while desktop was fine. organ and sax stay as the original
 * anchors. */

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

// A hang becomes a clear failure rather than a runner timeout.
const withTimeout = (p, ms, label) =>
  Promise.race([p, new Promise((_, rej) => setTimeout(() => rej(new Error(`${label} timed out after ${ms}ms`)), ms))]);

await page.goto(`http://localhost:${port}/`);
await page.waitForFunction(() => typeof window.__testCaptureSynthRender === "function", null, { timeout: 60_000 });
await page.waitForFunction(() => window.audioReady !== undefined, null, { timeout: 60_000 });
await waitForAudioReady(page);

// Read from the template files, since a hand-kept list drifted with every new template; a "<id>--2" row plays its
// base id.
const templatesDir = path.join(jsDir, "..", "go", "internal", "assets", "templates");
const SUBJECTS = [...new Set([
  "organ", "sax",
  ...fs.readdirSync(templatesDir)
    .filter((f) => f.endsWith(".json"))
    .flatMap((f) => JSON.parse(fs.readFileSync(path.join(templatesDir, f), "utf8")).instruments)
    .map((inst) => inst.id.split("--")[0]),
])].sort();
if (SUBJECTS.length < 40) throw new Error(`only ${SUBJECTS.length} template instruments found in ${templatesDir}`);

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
