// synth_knob_viz.browser.test.js
//
// Verifies the Synth-tab right-pane "mirror" through the REAL WASM bridge:
//   1. it produces a non-empty render (synthMirrorPCMLen > 0), and
//   2. it REACTS to a knob change (synthMirrorPCMChecksum differs after a param
//      edit). A length-only check would pass even for a frozen render that
//      ignores the knobs — which is exactly the bug this guards against.
//
// The mirror's signal source (RenderInstrumentPreview) is a pure-Go, no-cgo
// approximate synth compiled INTO main.wasm, so this exercises the genuine
// WASM render path (not a deterministic stub). DSP correctness and the
// reactivity of the checksum/render math are covered in Go.
//
// COVERED-BY-GO: internal/audio/preview_render_responsiveness_test.go (preview
// reacts to every stage param), internal/ui/synth_mirror_test.go (mirror
// render/cache/ghost + checksum reacts to content),
// internal/ui/synth_ghost_test.go (per-knob ghost capture/fade),
// internal/ui/synth_concept_viz_test.go (concept renderer ink + ghosts).

import { setupFullWasm } from "./real_input_test_helpers.js";

async function main() {
  const { page, cleanup } = await setupFullWasm({ logLevel: "INFO" });
  const failures = [];

  try {
    // Open the Synth tab so the right-pane mirror is the active surface.
    const opened = await page.evaluate(() => {
      if (typeof window.setActiveEQTab !== "function") return false;
      return !!window.setActiveEQTab("synth");
    });
    if (!opened) failures.push("setActiveEQTab('synth') failed or unavailable");

    // Pick an instrument that resolves through a synth recipe.
    const target = await page.evaluate(() => {
      const proj = JSON.parse(window.exportJSON());
      for (const inst of proj.instruments || []) {
        if (typeof window.recipeForInstrument === "function" && window.recipeForInstrument(inst.id)) {
          return inst.id;
        }
      }
      return (proj.instruments && proj.instruments[0] && proj.instruments[0].id) || "";
    });
    if (!target) throw new Error("no synth instrument found in default project");
    console.log(`[test] target instrument=${target}`);

    // Baseline render: must be non-empty, with a stable content fingerprint.
    const baseline = await page.evaluate(() => ({
      len: typeof window.synthMirrorPCMLen === "function" ? window.synthMirrorPCMLen() : -1,
      sum: typeof window.synthMirrorPCMChecksum === "function" ? window.synthMirrorPCMChecksum() : null,
    }));
    console.log(`[test] baseline len=${baseline.len} checksum=${baseline.sum}`);

    if (baseline.len === -1) failures.push("synthMirrorPCMLen export missing");
    else if (!(baseline.len > 0)) failures.push(`synthMirrorPCMLen = ${baseline.len}, want > 0 (empty render)`);
    if (baseline.sum === null) failures.push("synthMirrorPCMChecksum export missing");
    else if (baseline.sum === 0) failures.push("synthMirrorPCMChecksum = 0 (empty render)");

    // Change a wave-SHAPE param (oscillator type) and re-render. The "Your
    // sound" trace is cycle-normalized so it tracks timbre (osc / filter / drive
    // / FM), so flipping the oscillator shape MUST change the fingerprint —
    // proving the WASM mirror reflects the knobs, not a frozen wave.
    const changed = await page.evaluate((id) => {
      // Default oscillator is sine (0); switch to square (2) for a clear shape
      // change. setInstrumentParam clamps to the param's range.
      window.setInstrumentParam(id, "osc_type", 2);
      return typeof window.synthMirrorPCMChecksum === "function" ? window.synthMirrorPCMChecksum() : null;
    }, target);
    console.log(`[test] post-change checksum=${changed}`);

    if (changed !== null && baseline.sum !== null && changed === baseline.sum) {
      failures.push(`mirror did not react to a param change: checksum stayed ${changed} (frozen render)`);
    }

    if (failures.length) throw new Error("SYNTH KNOB VIZ FAILED:\n  - " + failures.join("\n  - "));
    console.log("[test] PASS: synth mirror renders non-empty PCM AND reacts to knob changes through the WASM bridge");
  } finally {
    await cleanup();
  }
}

main().catch((e) => {
  console.error(e);
  process.exit(1);
});
