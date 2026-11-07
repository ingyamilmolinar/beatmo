// The sound the browser PLAYS must follow synth state: one knob edit re-pushes the whole block (the pad keeps its slow
// attack), and an imported edited document reproduces its render. COVERED-BY-GO: synth_render_roundtrip_test.go,
// project_docs_test.go

import { setupFullWasm } from "./real_input_test_helpers.js";

const PAD_ID = "modular-pad";

// The pad's 0.12 s attack leaves the first 64 samples near silent (< ~0.02); a 5 ms attack reads ≈ 0.1+.
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
          `setInstrumentBlock bootstrap push missing?`
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
    // A gain change scales the render; it must not re-voice it.
    const wantTail = baseline.tailRMS * (0.8 / 0.9);
    if (baseline.tailRMS > 1e-4 && Math.abs(edited.tailRMS - wantTail) > 0.35 * baseline.tailRMS) {
      failures.push(
        `gain edit re-voiced the pad: tailRMS=${edited.tailRMS.toFixed(4)} want ≈${wantTail.toFixed(4)} (±35%)`
      );
    }
    await page.evaluate((id) => window.resetInstrumentParams(id), PAD_ID);

    // The pad is not a default row, so the project is built with its row and edited document, as export emits.
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
      });
      proj.instrument_docs = proj.instrument_docs || {};
      proj.instrument_docs[id] = JSON.parse(window.getInstrumentDoc(id));
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

    // The everyday case: a drum row with edited knobs exported and re-imported must sound the same.
    const builtinTarget = await page.evaluate(async () => {
      // Chosen by EFFECT: a tail comparison on an inert knob would pass vacuously.
      const proj = JSON.parse(window.exportJSON());
      for (const inst of proj.instruments || []) {
        if (!window.getInstrumentDoc(inst.id) || window.isMelodicInstrumentJS(inst.id)) continue;
        window.setInstrumentParam(inst.id, "post_decay", 1);
        const a = await window.__testCaptureSynthRender(inst.id);
        window.setInstrumentParam(inst.id, "post_decay", 0.2);
        const b = await window.__testCaptureSynthRender(inst.id);
        window.resetInstrumentParams(inst.id);
        if (a && b && a.rms !== b.rms) return inst.id;
      }
      return "";
    });
    if (!builtinTarget) throw new Error("no drum instrument with a post_decay knob in default project");
    await page.evaluate((id) => window.setInstrumentParam(id, "post_decay", 1), builtinTarget);
    const builtinPre = await page.evaluate((id) => window.__testCaptureSynthRender(id), builtinTarget);
    const builtinExported = await page.evaluate(() => window.exportJSON());
    await page.evaluate((id) => window.setInstrumentParam(id, "post_decay", 0.2), builtinTarget);
    // The file proves it carries the tone only if the other value sounds different.
    const builtinOther = await page.evaluate((id) => window.__testCaptureSynthRender(id), builtinTarget);
    const voiceFP = (r) => JSON.stringify({ rms: r.rms, peak: r.peak, head: r.head, tail: r.tail, len: r.length });
    if (voiceFP(builtinOther) === voiceFP(builtinPre)) {
      throw new Error(`precondition: post_decay 1 vs 0.2 render identically on ${builtinTarget}`);
    }
    const builtinErr = await page.evaluate((txt) => window.importJSON(txt) || "", builtinExported);
    if (builtinErr) failures.push(`importJSON (builtin delta) returned error: ${builtinErr}`);
    const builtinPost = await page.evaluate((id) => window.__testCaptureSynthRender(id), builtinTarget);
    console.log(
      `[test] builtin roundtrip ${builtinTarget}: pre tailRMS=${builtinPre.tailRMS.toFixed(4)} post tailRMS=${builtinPost.tailRMS.toFixed(4)}`
    );
    if (voiceFP(builtinPost) !== voiceFP(builtinPre)) {
      failures.push(
        `builtin delta did not round-trip at the render level on ${builtinTarget}: ` +
          `post rms=${builtinPost.rms} tailRMS=${builtinPost.tailRMS} vs pre rms=${builtinPre.rms} tailRMS=${builtinPre.tailRMS}`
      );
    }
    await page.evaluate((id) => window.resetInstrumentParams(id), builtinTarget);

    // A document REPLACED at import must reach the renderer: the drum then renders the sustained modular voice.
    const drumTarget = await page.evaluate(() => {
      const proj = JSON.parse(window.exportJSON());
      for (const inst of proj.instruments || []) {
        const owned = window.getInstrumentParams(inst.id) || {};
        if (window.getInstrumentDoc(inst.id) && !window.isMelodicInstrumentJS(inst.id) && "post_decay" in owned) return inst.id;
      }
      return "";
    });
    if (!drumTarget) throw new Error("no percussive builtin instrument in default project");
    const drumBaseline = await page.evaluate((id) => window.__testCaptureSynthRender(id), drumTarget);
    console.log(
      `[test] migration target=${drumTarget}: baseline peak=${drumBaseline.peak.toFixed(4)} tailRMS=${drumBaseline.tailRMS.toFixed(4)}`
    );
    const migErr = await page.evaluate((id) => {
      const proj = JSON.parse(window.exportJSON());
      const modular = JSON.parse(window.getInstrumentDoc("modular"));
      modular.id = id;
      proj.instrument_docs[id] = modular;
      return window.importJSON(JSON.stringify(proj)) || "";
    }, drumTarget);
    if (migErr) failures.push(`importJSON (document replace) returned error: ${migErr}`);
    const migOrigin = await page.evaluate((id) => JSON.parse(window.getInstrumentDoc(id) || "{}").origin, drumTarget);
    if (migOrigin !== "modular") {
      failures.push(`replaced document origin: got ${migOrigin}, want modular`);
    }
    const migRender = await page.evaluate((id) => window.__testCaptureSynthRender(id), drumTarget);
    console.log(
      `[test] replaced render: peak=${migRender.peak.toFixed(4)} tailRMS=${migRender.tailRMS.toFixed(4)} (drum baseline tailRMS=${drumBaseline.tailRMS.toFixed(4)})`
    );
    // Tail energy tells the sustained voice from the decaying drum.
    if (!(migRender.tailRMS > Math.max(0.05, 5 * drumBaseline.tailRMS))) {
      failures.push(
        `replaced instrument still renders its old drum voice: tailRMS=${migRender.tailRMS.toFixed(4)} ` +
          `(want > ${Math.max(0.05, 5 * drumBaseline.tailRMS).toFixed(4)} — the sustained modular voice)`
      );
    }

    // A saved library entry must reach the render: a reset returns the copy to that SAVED entry and the block follows.
    // Runs LAST: it saves into the session's library.
    const saveTarget = await page.evaluate((skip) => {
      const proj = JSON.parse(window.exportJSON());
      for (const inst of proj.instruments || []) {
        if (inst.id === skip) continue;
        if (window.getInstrumentDoc(inst.id) && !window.isMelodicInstrumentJS(inst.id)) return inst.id;
      }
      return "";
    }, drumTarget);
    if (!saveTarget) throw new Error("no second percussive builtin instrument for the Save case");
    const saveBaseline = await page.evaluate((id) => window.__testCaptureSynthRender(id), saveTarget);
    // The capture is peak-normalised, so a shorter voice reads as lower RMS.
    await page.evaluate((id) => window.setInstrumentParam(id, "post_decay", 0.15), saveTarget);
    const savedTone = await page.evaluate((id) => window.__testCaptureSynthRender(id), saveTarget);
    console.log(
      `[test] save target=${saveTarget}: baseline rms=${saveBaseline.rms.toFixed(4)} post_decay=0.15 rms=${savedTone.rms.toFixed(4)}`
    );
    if (!(savedTone.rms < 0.7 * saveBaseline.rms)) {
      throw new Error(
        `precondition: post_decay=0.15 did not shorten ${saveTarget} (rms ${savedTone.rms} vs ${saveBaseline.rms})`
      );
    }
    const savedRecipe = await page.evaluate((id) => window.saveActiveRecipe(id), saveTarget);
    if (!savedRecipe) failures.push(`saveActiveRecipe(${saveTarget}) returned an empty library id`);
    await page.evaluate((id) => window.resetInstrumentParams(id), saveTarget);
    const afterReset = await page.evaluate((id) => window.__testCaptureSynthRender(id), saveTarget);
    console.log(`[test] after Save + Reset: rms=${afterReset.rms.toFixed(4)} (saved=${savedTone.rms.toFixed(4)}, shipped=${saveBaseline.rms.toFixed(4)})`);
    if (Math.abs(afterReset.rms - savedTone.rms) > 0.15 * savedTone.rms) {
      failures.push(
        `the Saved library entry did not reach the JS render: after Reset rms=${afterReset.rms.toFixed(4)}, ` +
          `want ≈${savedTone.rms.toFixed(4)} (the saved tone, like the desktop dispatch) — shipped is ${saveBaseline.rms.toFixed(4)}`
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
