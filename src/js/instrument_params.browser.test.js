/**
 * Instrument Params Browser Test — Phase 5
 *
 * Exercises the Go-WASM ↔ JS bridge for the SynthRecipe per-instrument
 * parameter manager. Specifically:
 *
 *   1. setInstrumentParam(id, name, value) — Go-side mutation flows
 *      through platformInstrumentParamsChanged → window.updateInstrumentParams,
 *      which mirrors the JS-side params map and invalidates renderCache.
 *
 *   2. getInstrumentParams(id) — returns the merged param map as a
 *      plain JS object (no JSON.parse needed).
 *
 *   3. recipeForInstrument(id) — every shipped instrument is bound to
 *      its drum-* or fm-* recipe id (Phase 2.4 binding table).
 *
 *   4. synthRecipeCatalog() — surfaces all 25 recipes with their
 *      ParamDef schemas, so the UI (or a plugin browser) can render
 *      knob layouts without crossing the Go↔JS boundary at every read.
 *
 *   5. resetInstrumentParams(id) — clears the per-instrument map.
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
await page.waitForFunction(() => typeof recipeForInstrument === "function");
await page.waitForFunction(() => typeof synthRecipeCatalog === "function");
await page.waitForFunction(() => typeof window.updateInstrumentParams === "function");

console.log("[TEST] All synth-recipe exports present.");

// Test 1: recipeForInstrument returns the canonical id for shipped instruments.
{
  const got = await page.evaluate(() => ({
    snare: recipeForInstrument("snare"),
    kick: recipeForInstrument("kick"),
    fmBass: recipeForInstrument("fm-bass"),
    fmPluckVariant: recipeForInstrument("fm-pluck-1"),
  }));
  const want = {
    snare: "drum-snare",
    kick: "drum-kick",
    fmBass: "fm-bass",
    fmPluckVariant: "fm-pluck",
  };
  for (const k of Object.keys(want)) {
    if (got[k] !== want[k]) {
      throw new Error(
        `recipeForInstrument mismatch for ${k}: got ${got[k]}, want ${want[k]}`,
      );
    }
  }
  console.log("[TEST] recipeForInstrument bindings OK.");
}

// Test 2: synthRecipeCatalog returns at least 25 entries with the expected shape.
{
  const cat = await page.evaluate(() => synthRecipeCatalog());
  const recipeIDs = Object.keys(cat);
  if (recipeIDs.length < 25) {
    throw new Error(
      `synthRecipeCatalog returned ${recipeIDs.length} entries; want >= 25`,
    );
  }
  const ds = cat["drum-snare"];
  if (!ds) throw new Error("synthRecipeCatalog missing drum-snare");
  if (ds.category !== "drum") {
    throw new Error(`drum-snare category=${ds.category}, want drum`);
  }
  // ParamSchema is filtered to the recipe's wired params (the knobs whose
  // values the C renderer actually reads). drum-snare's wired set is
  // {pitch, decay, tone, drive} — see src/go/internal/audio/synth_recipe_wired.go.
  // The generic 8-knob set is no longer surfaced verbatim because rendering a
  // slider that has no audible effect was the most-cited Synth-tab UX bug
  // (see project_synth_pipeline_architecture memory).
  // The native-deprecation migration adds the snare family knobs (the
  // engine's own DSP constants exposed as knobs — body freqs, noise bands,
  // envelope rates, mixes; see snareFamilyParamDefs in
  // src/go/internal/audio/synth_recipe_wired.go). The gen_type Generator
  // selector is gone — re-voicing is the Modular recipe's job.
  //
  // The modular unification (Phase-8A/8C) additionally appends the shared
  // pipeline stage params (osc/env/filter/drive + PITCH/LFO/BURST enables and
  // knobs) to EVERY family recipe, so the exact entry count is Go-owned and
  // intentionally NOT pinned here. COVERED-BY-GO:
  // TestStageParams_SchemaDisciplineNoCollisions (stage_params_test.go),
  // TestBuiltinDrumRecipes_DeclareOnlyWiredGenericKnobs
  // (synth_registry_entries_test.go). This test asserts the bridge shape:
  // every snare-family knob must be surfaced through the catalog.
  const wantSnareParams = [
    "pitch", "decay", "tone", "drive",
    "snare_wave", "fundamental", "snare_tone2_freq", "snare_noise_tune",
    "snare_tone_decay", "snare_noise_decay", "snare_tail_decay",
    "snare_tone_mix", "snare_noise_mix", "snare_wire_mix", "snare_attack",
  ];
  if (!Array.isArray(ds.params) || ds.params.length < wantSnareParams.length) {
    throw new Error(
      `drum-snare ParamSchema has ${ds.params?.length} entries; want >= ${wantSnareParams.length}`,
    );
  }
  const gotSnareParamNames = new Set(ds.params.map((p) => p.name));
  for (const name of wantSnareParams) {
    if (!gotSnareParamNames.has(name)) {
      throw new Error(
        `drum-snare ParamSchema missing ${name}; got [${[...gotSnareParamNames].join(",")}]`,
      );
    }
  }
  // Spot-check a generic ParamDef shape.
  const decayDef = ds.params.find((p) => p.name === "decay");
  if (!decayDef) throw new Error("drum-snare missing 'decay' ParamDef");
  if (decayDef.default !== 1) {
    throw new Error(`decay default=${decayDef.default}, want 1`);
  }
  if (decayDef.min !== 0 || decayDef.max !== 4) {
    throw new Error(
      `decay range=[${decayDef.min},${decayDef.max}], want [0,4]`,
    );
  }
  console.log(`[TEST] synthRecipeCatalog OK (${recipeIDs.length} recipes).`);
}

// Test 3: setInstrumentParam → getInstrumentParams round-trip + the
// window.updateInstrumentParams hook fires with the JS-side mirror.
{
  await page.evaluate(() => resetInstrumentParams("snare"));

  // Install a probe on window.updateInstrumentParams so we can see the
  // platform callback fire. The original handler still runs (we wrap, not replace).
  await page.evaluate(() => {
    window.__synthParamUpdates = [];
    const orig = window.updateInstrumentParams;
    window.updateInstrumentParams = (id, paramsJSON) => {
      window.__synthParamUpdates.push({ id, paramsJSON });
      return orig(id, paramsJSON);
    };
  });

  await page.evaluate(() => setInstrumentParam("snare", "decay", 0.5));
  await page.evaluate(() => setInstrumentParam("snare", "drive", 0.7));

  const got = await page.evaluate(() => getInstrumentParams("snare"));
  if (got.decay !== 0.5) {
    throw new Error(`getInstrumentParams.decay=${got.decay}, want 0.5`);
  }
  if (got.drive !== 0.7) {
    throw new Error(`getInstrumentParams.drive=${got.drive}, want 0.7`);
  }

  const updates = await page.evaluate(() => window.__synthParamUpdates);
  if (updates.length !== 2) {
    throw new Error(
      `expected 2 updateInstrumentParams calls; got ${updates.length}`,
    );
  }
  console.log(
    `[TEST] setInstrumentParam round-trip OK (${updates.length} updates fired).`,
  );
}

// Test 4: resetInstrumentParams clears the manager state + fires the callback.
{
  await page.evaluate(() => (window.__synthParamUpdates = []));
  await page.evaluate(() => resetInstrumentParams("snare"));
  const got = await page.evaluate(() => getInstrumentParams("snare"));
  if (Object.keys(got).length !== 0) {
    throw new Error(`after reset, got params: ${JSON.stringify(got)}`);
  }
  const updates = await page.evaluate(() => window.__synthParamUpdates);
  if (updates.length !== 1) {
    throw new Error(
      `expected 1 updateInstrumentParams call on reset; got ${updates.length}`,
    );
  }
  console.log("[TEST] resetInstrumentParams OK.");
}

// Test 5: recipeDefaultParams returns the recipe's declared defaults. drum-snare's
// wired params are {pitch, decay, tone, drive} — `attack` is filtered out
// because render_snare_p() never reads it.
{
  const got = await page.evaluate(() => recipeDefaultParams("drum-snare"));
  if (got.decay !== 1) {
    throw new Error(`drum-snare default.decay=${got.decay}, want 1`);
  }
  if (got.pitch !== 0) {
    throw new Error(`drum-snare default.pitch=${got.pitch}, want 0`);
  }
  if (got.tone !== 0) {
    throw new Error(`drum-snare default.tone=${got.tone}, want 0`);
  }
  if (got.drive !== 0) {
    throw new Error(`drum-snare default.drive=${got.drive}, want 0`);
  }
  if ("attack" in got) {
    throw new Error(
      `drum-snare defaults must not include 'attack' (wired params are {pitch,decay,tone,drive}); got ${JSON.stringify(got)}`,
    );
  }
  console.log("[TEST] recipeDefaultParams OK.");
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
