// synth_config_roundtrip.browser.test.js
//
// Verifies that synth config (per-instrument params, including a Synth-tab Save
// which keeps the overlay) survives export → import in REAL WASM/WebAudio — not
// just the Go stub. The Go fast-path tests
// (internal/ui/repro_synth_config_roundtrip_test.go) prove the import/export
// logic; this proves the WASM bridge applies imported params to live audio state
// (audio.SetInstrumentParams → platformInstrumentParamsChanged → JS render).
//
// COVERED-BY-GO: import/export param logic. JS-owned: the Go↔WebAudio param
// plumbing across importJSON.

import { setupFullWasm } from "./real_input_test_helpers.js";

async function main() {
  const { page, cleanup } = await setupFullWasm({ logLevel: "INFO" });
  const failures = [];

  try {
    // Pick an instrument that resolves through a synth recipe (so "drive" — a
    // generic drum knob — is a valid param).
    const target = await page.evaluate(() => {
      const proj = JSON.parse(window.exportJSON());
      for (const inst of proj.instruments || []) {
        if (typeof window.recipeForInstrument === "function" && window.recipeForInstrument(inst.id)) {
          return inst.id;
        }
      }
      return (proj.instruments && proj.instruments[0] && proj.instruments[0].id) || "";
    });
    if (!target) throw new Error("no instrument found in default project");
    console.log(`[test] target instrument=${target}`);

    const driveOf = (proj, id) => {
      const inst = (proj.instruments || []).find((i) => i.id === id);
      return inst && inst.synth_params ? inst.synth_params.drive : undefined;
    };

    // 1) Edit a synth knob.
    await page.evaluate((id) => window.setInstrumentParam(id, "drive", 0.66), target);

    // 2) Export must capture it.
    const exported = await page.evaluate(() => window.exportJSON());
    const drExp = driveOf(JSON.parse(exported), target);
    console.log(`[test] exported synth_params.drive=${drExp}`);
    if (drExp === undefined || Math.abs(drExp - 0.66) > 1e-6)
      failures.push(`export did not capture synth param: drive=${drExp}, want 0.66`);

    // 3) Diverge live state, then import the captured project.
    await page.evaluate((id) => window.setInstrumentParam(id, "drive", 0.1), target);
    const errStr = await page.evaluate((txt) => window.importJSON(txt) || "", exported);
    if (errStr) failures.push(`importJSON returned error: ${errStr}`);

    // 4) Live audio state + a re-export must both reflect the imported value.
    const after = await page.evaluate((id) => {
      const live = window.getInstrumentParams(id);
      const proj = JSON.parse(window.exportJSON());
      const inst = (proj.instruments || []).find((i) => i.id === id);
      return { live: live ? live.drive : undefined, reExported: inst && inst.synth_params ? inst.synth_params.drive : undefined };
    }, target);
    console.log(`[test] after import: live.drive=${after.live} reExported.drive=${after.reExported}`);
    if (after.live === undefined || Math.abs(after.live - 0.66) > 1e-6)
      failures.push(`live param not restored on import: drive=${after.live}, want 0.66`);
    if (after.reExported === undefined || Math.abs(after.reExported - 0.66) > 1e-6)
      failures.push(`re-export after import lost param: drive=${after.reExported}, want 0.66`);

    if (failures.length) throw new Error("SYNTH CONFIG ROUNDTRIP FAILED:\n  - " + failures.join("\n  - "));
    console.log("[test] PASS: synth config round-trips through WASM export/import");
  } finally {
    await cleanup();
  }
}

main().catch((e) => {
  console.error(e);
  process.exit(1);
});
