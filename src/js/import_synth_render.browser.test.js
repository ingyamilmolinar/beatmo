// import_synth_render.browser.test.js
//
// Render-level guards for synth instrument state across the WASM bridge —
// closing the gap where param/recipe state reached the Go manager but the
// JS-side render (the sound the browser actually plays) stayed wrong.
//
// 1) SEEDED-DEFAULTS MERGE (GAP B): instrumentParamsFor must MERGE the user
//    overlay over the bootstrap-seeded preset defaults (modular-pad), not OR
//    them. With the OR, one knob edit drops the whole seed and the elided
//    keys render as the base modular voice (fast 5 ms attack instead of the
//    pad's 120 ms attack).
// 2) IMPORT ROUNDTRIP GUARD: importing a project with a synth_params delta on
//    a seeded recipe must reproduce the pre-export render (the Go import pins
//    the full shipped⊕delta tone — import.go; this guards the JS half).
//
// COVERED-BY-GO: import/export param logic + pin-on-import semantics
// (internal/ui/synth_render_roundtrip_test.go). JS-owned here: the
// instrumentParamsFor merge + the rendered output of the C `_p` variant.

import { setupFullWasm } from "./real_input_test_helpers.js";

const PAD_ID = "modular-pad";

// Mean absolute head amplitude relative to peak: the attack discriminator.
// Pad seed amp_attack=0.12 s → first 64 samples ≈ silent (ratio < ~0.02).
// Modular identity amp_attack=0.005 s → ratio ≈ 0.1+.
function headRatio(cap) {
  if (!cap || !cap.head || !cap.peak) return Infinity;
  let sum = 0;
  for (const v of cap.head) sum += Math.abs(v);
  return sum / cap.head.length / cap.peak;
}

async function main() {
  const { page, cleanup } = await setupFullWasm({ logLevel: "INFO" });
  const failures = [];

  try {
    await page.waitForFunction(
      () =>
        typeof window.__testCaptureSynthRender === "function" &&
        typeof window.setInstrumentParam === "function" &&
        typeof window.importJSON === "function"
    );

    // --- Case 1: seeded-defaults merge under a live one-knob edit ---
    const baseline = await page.evaluate((id) => window.__testCaptureSynthRender(id), PAD_ID);
    if (!baseline || !(baseline.peak > 0)) {
      throw new Error(`baseline render of ${PAD_ID} is empty (peak=${baseline && baseline.peak})`);
    }
    const baseRatio = headRatio(baseline);
    console.log(`[test] baseline ${PAD_ID}: peak=${baseline.peak.toFixed(4)} tailRMS=${baseline.tailRMS.toFixed(4)} headRatio=${baseRatio.toFixed(4)}`);
    if (baseRatio > 0.05) {
      throw new Error(
        `precondition: baseline ${PAD_ID} does not show the seeded slow attack (headRatio=${baseRatio}); ` +
          `seedInstrumentDefaults bootstrap push missing?`
      );
    }

    await page.evaluate((id) => window.setInstrumentParam(id, "gain", 0.8), PAD_ID);
    const edited = await page.evaluate((id) => window.__testCaptureSynthRender(id), PAD_ID);
    const editedRatio = headRatio(edited);
    console.log(`[test] after gain edit: peak=${edited.peak.toFixed(4)} tailRMS=${edited.tailRMS.toFixed(4)} headRatio=${editedRatio.toFixed(4)}`);
    if (editedRatio > 0.05) {
      failures.push(
        `one knob edit dropped the seeded pad defaults: headRatio=${editedRatio.toFixed(4)} ` +
          `(want < 0.05 — the pad's 120 ms attack; got the base modular 5 ms attack instead)`
      );
    }
    // Gain 0.9→0.8 must scale the render, not re-voice it: tail energy stays
    // within a generous band of baseline×(0.8/0.9).
    const wantTail = baseline.tailRMS * (0.8 / 0.9);
    if (baseline.tailRMS > 1e-4 && Math.abs(edited.tailRMS - wantTail) > 0.35 * baseline.tailRMS) {
      failures.push(
        `gain edit re-voiced the pad: tailRMS=${edited.tailRMS.toFixed(4)} want ≈${wantTail.toFixed(4)} (±35%)`
      );
    }
    await page.evaluate((id) => window.resetInstrumentParams(id), PAD_ID);

    // --- Case 2: import roundtrip on the seeded recipe reproduces the render ---
    // modular-pad is not a default row, so build the project explicitly: the
    // current project plus a modular-pad instrument carrying a synth_params
    // delta (exactly what export emits for an edited pad row).
    await page.evaluate((id) => window.setInstrumentParam(id, "gain", 0.8), PAD_ID);
    const preExport = await page.evaluate((id) => window.__testCaptureSynthRender(id), PAD_ID);
    const exported = await page.evaluate((id) => {
      const proj = JSON.parse(window.exportJSON());
      proj.instruments.push({
        name: "Pad",
        id: id,
        kind: "builtin",
        volume: 1,
        origin: proj.instruments.length,
        color: "#44A8FFFF",
        recipe: "synth-modular-pad",
        synth_params: { gain: 0.8 },
      });
      return JSON.stringify(proj);
    }, PAD_ID);
    // Diverge live state, then import the captured project back.
    await page.evaluate((id) => window.setInstrumentParam(id, "gain", 0.3), PAD_ID);
    const errStr = await page.evaluate((txt) => window.importJSON(txt) || "", exported);
    if (errStr) failures.push(`importJSON returned error: ${errStr}`);
    const postImport = await page.evaluate((id) => window.__testCaptureSynthRender(id), PAD_ID);
    const preRatio = headRatio(preExport);
    const postRatio = headRatio(postImport);
    console.log(
      `[test] roundtrip: pre(peak=${preExport.peak.toFixed(4)} headRatio=${preRatio.toFixed(4)}) ` +
        `post(peak=${postImport.peak.toFixed(4)} headRatio=${postRatio.toFixed(4)})`
    );
    if (Math.abs(postImport.peak - preExport.peak) > 0.15 * Math.max(preExport.peak, 1e-6)) {
      failures.push(
        `post-import peak diverged from pre-export: ${postImport.peak.toFixed(4)} vs ${preExport.peak.toFixed(4)}`
      );
    }
    if (postRatio > 0.05) {
      failures.push(`post-import render lost the pad attack: headRatio=${postRatio.toFixed(4)} (want < 0.05)`);
    }

    // --- Case 2b: plain builtin delta round-trips at the render level ---
    // The everyday scenario: a drum row with edited knobs exported to
    // beatmo.json must sound identical after importing that file back.
    const builtinTarget = await page.evaluate(() => {
      const proj = JSON.parse(window.exportJSON());
      for (const inst of proj.instruments || []) {
        const r = window.recipeForInstrument(inst.id);
        if (r && r.startsWith("drum-")) return inst.id;
      }
      return "";
    });
    if (!builtinTarget) throw new Error("no drum-recipe instrument in default project");
    await page.evaluate((id) => window.setInstrumentParam(id, "decay", 3), builtinTarget);
    const builtinPre = await page.evaluate((id) => window.__testCaptureSynthRender(id), builtinTarget);
    const builtinExported = await page.evaluate(() => window.exportJSON());
    await page.evaluate((id) => window.setInstrumentParam(id, "decay", 0.2), builtinTarget);
    const builtinErr = await page.evaluate((txt) => window.importJSON(txt) || "", builtinExported);
    if (builtinErr) failures.push(`importJSON (builtin delta) returned error: ${builtinErr}`);
    const builtinPost = await page.evaluate((id) => window.__testCaptureSynthRender(id), builtinTarget);
    console.log(
      `[test] builtin roundtrip ${builtinTarget}: pre tailRMS=${builtinPre.tailRMS.toFixed(4)} post tailRMS=${builtinPost.tailRMS.toFixed(4)}`
    );
    if (
      builtinPre.tailRMS > 1e-4 &&
      Math.abs(builtinPost.tailRMS - builtinPre.tailRMS) > 0.15 * builtinPre.tailRMS
    ) {
      failures.push(
        `builtin delta did not round-trip at the render level: post tailRMS=${builtinPost.tailRMS.toFixed(4)} vs pre ${builtinPre.tailRMS.toFixed(4)}`
      );
    }
    await page.evaluate((id) => window.resetInstrumentParams(id), builtinTarget);

    // --- Case 3 (GAP C): a recipe rebind at import must reach the JS renderer ---
    // Legacy gen_type>=1 projects migrate to synth-modular at import
    // (audio.MigrateGenType). The Go binding flips, so the JS render must flip
    // too: a migrated drum renders the sustained modular voice, not its old
    // percussive renderer from the static RENDER table.
    const drumTarget = await page.evaluate(() => {
      const proj = JSON.parse(window.exportJSON());
      for (const inst of proj.instruments || []) {
        const r = window.recipeForInstrument(inst.id);
        if (r && r.startsWith("drum-")) return inst.id;
      }
      return "";
    });
    if (!drumTarget) throw new Error("no drum-recipe instrument in default project");
    const drumBaseline = await page.evaluate((id) => window.__testCaptureSynthRender(id), drumTarget);
    console.log(
      `[test] migration target=${drumTarget}: baseline peak=${drumBaseline.peak.toFixed(4)} tailRMS=${drumBaseline.tailRMS.toFixed(4)}`
    );
    const migErr = await page.evaluate((id) => {
      const proj = JSON.parse(window.exportJSON());
      const inst = (proj.instruments || []).find((i) => i.id === id);
      inst.synth_params = Object.assign({}, inst.synth_params, { gen_type: 2 }); // re-voiced: modular osc_type=1
      return window.importJSON(JSON.stringify(proj)) || "";
    }, drumTarget);
    if (migErr) failures.push(`importJSON (gen_type migration) returned error: ${migErr}`);
    const migRecipe = await page.evaluate((id) => window.recipeForInstrument(id), drumTarget);
    if (migRecipe !== "synth-modular") {
      failures.push(`migrated recipe binding: got ${migRecipe}, want synth-modular`);
    }
    const migRender = await page.evaluate((id) => window.__testCaptureSynthRender(id), drumTarget);
    console.log(
      `[test] migrated render: peak=${migRender.peak.toFixed(4)} tailRMS=${migRender.tailRMS.toFixed(4)} (drum baseline tailRMS=${drumBaseline.tailRMS.toFixed(4)})`
    );
    // The modular voice sustains (identity amp_sustain=0.6) where the drum
    // decays to ~nothing — tail energy is the renderer discriminator.
    if (!(migRender.tailRMS > Math.max(0.05, 5 * drumBaseline.tailRMS))) {
      failures.push(
        `migrated instrument still renders via its old drum renderer: tailRMS=${migRender.tailRMS.toFixed(4)} ` +
          `(want > ${Math.max(0.05, 5 * drumBaseline.tailRMS).toFixed(4)} — the sustained modular voice)`
      );
    }

    // --- Case 4 (GAP D): Save-customized recipe defaults must reach the JS render ---
    // Synth-tab Save bakes the tone into the recipe's registered defaults and
    // keeps the overlay. Clearing the overlay afterwards (Reset of the
    // per-instrument knobs) must still render the SAVED tone — the desktop
    // dispatch renders the customized registered defaults; pre-fix the browser
    // reverted to shipped because the Save never pushed defaults to JS.
    // Runs LAST: it customizes a recipe's registered defaults for the session.
    const saveTarget = await page.evaluate((skip) => {
      const proj = JSON.parse(window.exportJSON());
      for (const inst of proj.instruments || []) {
        if (inst.id === skip) continue;
        const r = window.recipeForInstrument(inst.id);
        if (r && r.startsWith("drum-")) return inst.id;
      }
      return "";
    }, drumTarget);
    if (!saveTarget) throw new Error("no second drum-recipe instrument for the Save case");
    const saveBaseline = await page.evaluate((id) => window.__testCaptureSynthRender(id), saveTarget);
    await page.evaluate((id) => window.setInstrumentParam(id, "decay", 4), saveTarget);
    const savedTone = await page.evaluate((id) => window.__testCaptureSynthRender(id), saveTarget);
    console.log(
      `[test] save target=${saveTarget}: baseline tailRMS=${saveBaseline.tailRMS.toFixed(4)} decay=4 tailRMS=${savedTone.tailRMS.toFixed(4)}`
    );
    if (!(savedTone.tailRMS > 2 * saveBaseline.tailRMS)) {
      throw new Error(
        `precondition: decay=4 did not lengthen ${saveTarget}'s tail (${savedTone.tailRMS} vs ${saveBaseline.tailRMS})`
      );
    }
    const savedRecipe = await page.evaluate((id) => window.saveActiveRecipe(id), saveTarget);
    if (!savedRecipe) failures.push(`saveActiveRecipe(${saveTarget}) returned empty recipe id`);
    await page.evaluate((id) => window.resetInstrumentParams(id), saveTarget);
    const afterReset = await page.evaluate((id) => window.__testCaptureSynthRender(id), saveTarget);
    console.log(`[test] after Save + overlay clear: tailRMS=${afterReset.tailRMS.toFixed(4)} (saved=${savedTone.tailRMS.toFixed(4)})`);
    if (Math.abs(afterReset.tailRMS - savedTone.tailRMS) > 0.3 * savedTone.tailRMS) {
      failures.push(
        `Save-customized defaults did not reach the JS render: after overlay clear tailRMS=${afterReset.tailRMS.toFixed(4)}, ` +
          `want ≈${savedTone.tailRMS.toFixed(4)} (the saved tone, like the desktop dispatch) — got the shipped tone instead`
      );
    }

    if (failures.length) throw new Error("IMPORT SYNTH RENDER FAILED:\n  - " + failures.join("\n  - "));
    console.log("[test] PASS: seeded defaults survive live edits and import round-trips at the render level");
  } finally {
    await cleanup();
  }
}

main().catch((e) => {
  console.error(e);
  process.exit(1);
});
