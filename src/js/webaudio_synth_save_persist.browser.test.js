/**
 * Synth-tab Save / Reset persistence — WASM↔bridge boundary.
 *
 * Charter (CLAUDE.md § JS Test Charter, tier 4 "WASM↔JS bridge plumbing"):
 * this verifies that the real Go Save / Reset logic, running in WASM, leaves
 * the per-instrument param state the WebAudio voice cache reads in the correct
 * shape. Logic correctness (the rendered sound is bit-identical before/after
 * Save, and returns to the original after Reset) is owned by the Go native
 * tests:
 *   - internal/audio/synth_recipe_save_render_test.go
 *       TestSynthSavePersists_RenderUnchangedAfterSave
 *       TestSynthReset_RenderReturnsToOriginal
 *   - internal/ui/synth_panel_save_persist_test.go
 *       TestSynthSave_KeepsLikedParamsForBridge
 *       TestSynthReset_RestoresOriginalAfterSave
 *
 * The bug this guards: clicking Save used to push an empty param map to JS
 * (updateInstrumentParams("kick","{}")), dropping the liked tone from the
 * browser's voice cache; and Reset could not undo a Save's overwrite of the
 * recipe defaults. Here we drive saveActiveRecipe / resetActiveRecipe (the
 * same Go core the buttons use) and assert the bridge-readable state.
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
    GO,
    ["build", "-o", path.join(jsDir, "play_ui.wasm"), "./internal/ui/playtest"],
    { cwd: goDir, env: { ...process.env, GOOS: "js", GOARCH: "wasm" }, stdio: "inherit" },
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

const browser = await chromium.launch({ args: ["--autoplay-policy=no-user-gesture-required"] });
const page = await browser.newPage();
page.on("console", (msg) => {
  try {
    if (msg.type() === "error") console.log("[PAGE]", msg.type(), msg.text());
  } catch (_) {}
});

const failures = [];
function check(cond, msg) {
  if (!cond) failures.push(msg);
}
const approx = (a, b, eps = 1e-6) => Math.abs(a - b) <= eps;

try {
  await page.goto(`http://localhost:${port}/`);
  await page.waitForFunction(() => typeof ensureDefaultPath === "function");
  await page.evaluate(() => ensureDefaultPath());
  await page.waitForFunction(() => typeof saveActiveRecipe === "function");

  const INST = "kick";
  // Drive the whole scenario inside one page.evaluate so the actual numeric
  // values cross the bridge (callExport only returns a shape descriptor).
  const r = await page.evaluate((inst) => {
    const recipeID = recipeForInstrument(inst);
    resetActiveRecipe(inst); // clean baseline (undo any prior customization)
    const origDecay = recipeDefaultParams(recipeID).decay;
    const likedDecay = origDecay === 2 ? 3 : 2; // ensure a real change

    setInstrumentParam(inst, "decay", likedDecay); // the knob drag
    const overlayBeforeSave = getInstrumentParams(inst);

    const savedRecipe = saveActiveRecipe(inst); // the Save button
    const overlayAfterSave = getInstrumentParams(inst);
    const defaultDecayAfterSave = recipeDefaultParams(recipeID).decay;

    const resetRecipe = resetActiveRecipe(inst); // the Reset button
    const defaultDecayAfterReset = recipeDefaultParams(recipeID).decay;
    const overlayAfterReset = getInstrumentParams(inst);

    return {
      recipeID, origDecay, likedDecay, savedRecipe, resetRecipe,
      overlayBeforeDecay: overlayBeforeSave.decay,
      overlayAfterSaveKeys: Object.keys(overlayAfterSave).length,
      overlayAfterSaveDecay: overlayAfterSave.decay,
      defaultDecayAfterSave,
      defaultDecayAfterReset,
      overlayAfterResetKeys: Object.keys(overlayAfterReset).length,
    };
  }, INST);

  check(typeof r.recipeID === "string" && r.recipeID.length > 0, `recipeForInstrument(${INST}) empty`);
  check(typeof r.origDecay === "number", `recipe ${r.recipeID} has no decay default`);
  check(approx(r.overlayBeforeDecay, r.likedDecay), `pre-save overlay decay=${r.overlayBeforeDecay}, want ${r.likedDecay}`);
  check(r.savedRecipe === r.recipeID, `saveActiveRecipe returned ${r.savedRecipe}, want ${r.recipeID}`);

  // BUG GUARD #1 (Save must not change the sound): the params the WebAudio
  // voice cache reads must still carry the liked tone — the overlay is NOT
  // emptied, and the persisted recipe default now equals the liked value.
  check(r.overlayAfterSaveKeys > 0, "after Save the per-instrument overlay was emptied — browser would drop the liked sound");
  check(approx(r.overlayAfterSaveDecay, r.likedDecay), `post-save overlay decay=${r.overlayAfterSaveDecay}, want ${r.likedDecay}`);
  check(approx(r.defaultDecayAfterSave, r.likedDecay), `post-save recipe default decay=${r.defaultDecayAfterSave}, want ${r.likedDecay}`);

  // BUG GUARD #2 (Reset restores original even after Save).
  check(r.resetRecipe === r.recipeID, `resetActiveRecipe returned ${r.resetRecipe}, want ${r.recipeID}`);
  check(approx(r.defaultDecayAfterReset, r.origDecay), `after Reset, default decay=${r.defaultDecayAfterReset}, want original ${r.origDecay}`);
  check(r.overlayAfterResetKeys === 0, `after Reset, overlay still has ${r.overlayAfterResetKeys} keys`);

  if (failures.length) {
    console.error("[synth-save-persist] FAILURES:\n  " + failures.join("\n  "));
    process.exitCode = 1;
  } else {
    console.log("[synth-save-persist] OK — Save preserves the tone and Reset restores the original through the WASM bridge");
  }
} finally {
  await browser.close();
  server.close();
}
