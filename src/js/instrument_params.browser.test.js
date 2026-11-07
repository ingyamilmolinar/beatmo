/**
 * Instrument Params Browser Test — Phase 5
 *
 * Exercises the Go-WASM ↔ JS bridge for the SynthRecipe per-instrument
 * parameter manager. Specifically:
 *
 *   1. setInstrumentParam(id, name, value) — Go-side mutation flows
 *      through the instrument document push → window.setInstrumentBlock,
 *      which mirrors the JS-side params map and invalidates renderCache.
 *
 *   2. getInstrumentParams(id) — returns the merged param map as a
 *      plain JS object (no JSON.parse needed).
 *
 *   3. getInstrumentDoc(id) — every instrument crosses as its document JSON
 *      (id + origin), see Test 1.
 *
 *   4. instrumentParamDefs(id) — the knob schema of the instrument's
 *      document (the Synth tab's controls), see Test 2.
 *
 *   5. resetInstrumentParams(id) — returns the document to its library entry.
 *
 * What this test does NOT do (intentionally):
 *   - Capture WebAudio output and assert audible param change. That
 *     covers the C-side _p() variants, which Go's native suite already
 *     verifies bit-identically in TestPhase2Native_RecipeRenderHonorsParamMutations.
 *     The WASM-equivalent parity check lives in xplat_synth_recipe_parity.
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
let snareDefs;

// Ensure Chromium is installed.
const chromiumPath = path.join(
  jsDir,
  "node_modules",
  ".cache",
  "ms-playwright",
  "chromium",
);
if (!fs.existsSync(chromiumPath)) {
  spawnSync("npx", ["playwright", "install", "chromium"], {
    cwd: jsDir,
    stdio: "inherit",
  });
}

const goDir = path.resolve(jsDir, "../go");
const GO = resolveGoBinary();

// Build WASM (skip when WASM_PREBUILT=1 and the binary already exists).
if (!shouldSkipWasmBuild("play_ui.wasm")) {
  const build = spawnSync(
    GO,
    ["build", "-o", path.join(jsDir, "play_ui.wasm"), "./internal/ui/playtest"],
    {
      cwd: goDir,
      env: { ...process.env, GOOS: "js", GOARCH: "wasm" },
      stdio: "inherit",
    },
  );
  if (build.status !== 0) throw new Error("go build play_ui failed");
}

const server = http.createServer((req, res) => {
  const file = req.url === "/" ? "/play_ui.html" : req.url;
  const fp = path.join(jsDir, file.replace(/^\//, ""));
  fs.readFile(fp, (err, data) => {
    if (err) {
      res.writeHead(404);
      res.end();
      return;
    }
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
  try {
    console.log("[PAGE]", msg.type(), msg.text());
  } catch (_) {}
});

await page.goto(`http://localhost:${port}/`);
await page.waitForFunction(() => typeof setInstrumentParam === "function");
await page.waitForFunction(() => typeof getInstrumentParams === "function");
await page.waitForFunction(() => typeof getInstrumentDoc === "function");
await page.waitForFunction(() => typeof instrumentParamDefs === "function");
await page.waitForFunction(() => typeof window.setInstrumentBlock === "function");

console.log("[TEST] All synth-recipe exports present.");

// Test 1: getInstrumentDoc crosses each instrument's document as JSON — its
// id, the library entry it came from, and its modules. Which values a document
// holds is Go-owned (internal/audio instrument_docs tests); the BRIDGE owes a
// parseable document for every instrument the app plays.
{
  const got = await page.evaluate(() => Object.fromEntries(
    ["snare", "kick", "fm-bass", "fm-pluck-tight"].map((id) => [id, getInstrumentDoc(id)]),
  ));
  for (const [id, raw] of Object.entries(got)) {
    if (typeof raw !== "string" || raw === "") {
      throw new Error(`getInstrumentDoc(${id}) returned ${JSON.stringify(raw)}; want document JSON`);
    }
    const d = JSON.parse(raw);
    if (d.id !== id || !d.origin || !d.stages || !d.stages.core || !d.stages.finish) {
      throw new Error(`getInstrumentDoc(${id}): id=${d.id} origin=${d.origin} stages=${Object.keys(d.stages || {})}`);
    }
  }
  const none = await page.evaluate(() => getInstrumentDoc("no-such-instrument"));
  if (none !== "") throw new Error(`getInstrumentDoc(unknown) = ${JSON.stringify(none)}, want ""`);
  console.log("[TEST] getInstrumentDoc OK.");
}

// Test 2: instrumentParamDefs returns the snare's knob schema with the expected
// shape. The exact set is Go-owned (COVERED-BY-GO: instrument_param_defs_test.go).
{
  snareDefs = await page.evaluate(() => instrumentParamDefs("snare"));
  if (!Array.isArray(snareDefs) || snareDefs.length < 10) {
    throw new Error(`instrumentParamDefs(snare) returned ${snareDefs && snareDefs.length} defs`);
  }
  const names = new Set(snareDefs.map((p) => p.name));
  for (const name of ["gen1_snare_tune", "post_decay", "gain"]) {
    if (!names.has(name)) throw new Error(`instrumentParamDefs(snare) missing ${name}`);
  }
  // Spot-check a ParamDef shape. post_decay is the FINISH fade, range [0,1]
  // (1 = no fade).
  const decayDef = snareDefs.find((p) => p.name === "post_decay");
  if (decayDef.min !== 0 || decayDef.max !== 1) {
    throw new Error(`post_decay range=[${decayDef.min},${decayDef.max}], want [0,1]`);
  }
  const none = await page.evaluate(() => instrumentParamDefs("no-such-instrument"));
  if (!Array.isArray(none) || none.length !== 0) throw new Error("instrumentParamDefs(unknown) must be []");
  console.log(`[TEST] instrumentParamDefs OK (${snareDefs.length} defs).`);
}

// Test 3: setInstrumentParam → getInstrumentParams round-trip + every edit
// pushes the instrument document's block through window.setInstrumentBlock.
let snareLibrary;
{
  await page.evaluate(() => resetInstrumentParams("snare"));
  snareLibrary = await page.evaluate(() => getInstrumentParams("snare"));

  // Install a probe on window.setInstrumentBlock so we can see the platform
  // push fire. The original handler still runs (we wrap, not replace).
  await page.evaluate(() => {
    window.__synthParamUpdates = [];
    const orig = window.setInstrumentBlock;
    window.setInstrumentBlock = (id, ...rest) => {
      window.__synthParamUpdates.push({ id });
      return orig(id, ...rest);
    };
  });

  await page.evaluate(() => setInstrumentParam("snare", "amp_decay", 0.5));
  await page.evaluate(() => setInstrumentParam("snare", "drive", 0.7));

  const got = await page.evaluate(() => getInstrumentParams("snare"));
  if (got.amp_decay !== 0.5) {
    throw new Error(`getInstrumentParams.amp_decay=${got.amp_decay}, want 0.5`);
  }
  if (got.drive !== 0.7) {
    throw new Error(`getInstrumentParams.drive=${got.drive}, want 0.7`);
  }

  const updates = await page.evaluate(() => window.__synthParamUpdates);
  if (updates.length !== 2) {
    throw new Error(
      `expected 2 setInstrumentBlock pushes; got ${updates.length}`,
    );
  }
  console.log(
    `[TEST] setInstrumentParam round-trip OK (${updates.length} updates fired).`,
  );
}

// Test 4: resetInstrumentParams returns the document to its library entry +
// pushes it once.
{
  await page.evaluate(() => (window.__synthParamUpdates = []));
  await page.evaluate(() => resetInstrumentParams("snare"));
  const got = await page.evaluate(() => getInstrumentParams("snare"));
  if (got.amp_decay !== snareLibrary.amp_decay || got.drive !== snareLibrary.drive) {
    throw new Error(`after reset, got amp_decay=${got.amp_decay} drive=${got.drive}, want the library ${snareLibrary.amp_decay} / ${snareLibrary.drive}`);
  }
  const updates = await page.evaluate(() => window.__synthParamUpdates);
  if (updates.length !== 1) {
    throw new Error(
      `expected 1 setInstrumentBlock push on reset; got ${updates.length}`,
    );
  }
  console.log("[TEST] resetInstrumentParams OK.");
}

// Test 5: instrumentParamDefs and getInstrumentParams read ONE document: each
// def's default is the document's value (the Synth tab shows the instrument's
// own values as its knobs' defaults).
{
  const vals = await page.evaluate(() => getInstrumentParams("snare"));
  for (const d of snareDefs) {
    if (d.name in vals && vals[d.name] !== d.default) {
      throw new Error(`snare ${d.name}: def default ${d.default}, document value ${vals[d.name]}`);
    }
  }
  console.log("[TEST] instrumentParamDefs defaults == document values OK.");
}

console.log("[TEST] All instrument-params bridge checks passed.");

if (isCoverageEnabled())
  await flushCoverage(
    page,
    new URL("../../coverage/browser-raw", import.meta.url).pathname,
    "instrument_params",
  );
await browser.close();
server.close();
console.log("instrument_params browser test PASSED");
