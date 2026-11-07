// An instrument's document must survive export → import into LIVE audio state (SetInstrumentDoc → setInstrumentBlock).
// COVERED-BY-GO: internal/ui/repro_synth_config_roundtrip_test.go

import { setupFullWasm } from "./real_input_test_helpers.js";

async function main() {
  const { page, cleanup } = await setupFullWasm({ logLevel: "INFO" });
  const failures = [];

  try {
    // Pick an instrument with an instrument document (a synth voice).
    const target = await page.evaluate(() => {
      const proj = JSON.parse(window.exportJSON());
      for (const inst of proj.instruments || []) {
        if (typeof window.getInstrumentDoc === "function" && window.getInstrumentDoc(inst.id)) {
          return inst.id;
        }
      }
      return (proj.instruments && proj.instruments[0] && proj.instruments[0].id) || "";
    });
    if (!target) throw new Error("no instrument found in default project");
    console.log(`[test] target instrument=${target}`);

    // A stage's values are keyed by the engine field name ("drive").
    const driveOf = (proj, id) => {
      const doc = (proj.instrument_docs || {})[id];
      for (const st of Object.values((doc && doc.stages) || {})) {
        if (st && st.values && "drive" in st.values) return st.values.drive;
      }
      return undefined;
    };

    // 1) Edit a synth knob.
    await page.evaluate((id) => window.setInstrumentParam(id, "drive", 0.66), target);

    // 2) Export must capture it.
    const exported = await page.evaluate(() => window.exportJSON());
    const drExp = driveOf(JSON.parse(exported), target);
    console.log(`[test] exported instrument_docs drive=${drExp}`);
    if (drExp === undefined || Math.abs(drExp - 0.66) > 1e-6)
      failures.push(`export did not capture the edit: drive=${drExp}, want 0.66`);

    // 3) Diverge live state, then import the captured project.
    await page.evaluate((id) => window.setInstrumentParam(id, "drive", 0.1), target);
    const errStr = await page.evaluate((txt) => window.importJSON(txt) || "", exported);
    if (errStr) failures.push(`importJSON returned error: ${errStr}`);

    // 4) Live audio state + a re-export must both reflect the imported value.
    const after = await page.evaluate((id) => {
      const live = window.getInstrumentParams(id);
      const doc = (JSON.parse(window.exportJSON()).instrument_docs || {})[id];
      let reExported;
      for (const st of Object.values((doc && doc.stages) || {})) {
        if (st && st.values && "drive" in st.values) reExported = st.values.drive;
      }
      return { live: live ? live.drive : undefined, reExported };
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
