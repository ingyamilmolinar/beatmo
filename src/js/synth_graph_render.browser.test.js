// The Synth graphs are drawn from renderDocPCM, which must return bit for bit the buffer playback caches, with and
// without a Sampler edit. COVERED-BY-GO: TestGraphRenderIsThePlayedVoice (desktop) and the internal/ui pipeline.

import { setupFullWasm } from "./real_input_test_helpers.js";

const IDS = ["modular-pad", "kick-808"];

async function compareGraphToPlayback(page, id, edit) {
  return page.evaluate(
    async ({ id, edit }) => {
      const info = window.__instrumentBlockForTest(id);
      if (!info) return { skip: `no block pushed for ${id}` };
      window.updateSampleEdit(id, JSON.stringify(edit || { StartFrac: 0, EndFrac: 1 }));
      if (!(await window.ensureSynthSample(id))) return { error: `playback render of ${id} failed` };
      const played = window.captureInstrumentPCM(id, false);
      if (!played) return { error: `no cached playback buffer for ${id}` };
      const graph = await window.renderDocPCM(id, info.block, info.seconds, info.amp, info.gateTail, info.fx);
      if (graph.length !== played.length || graph.sr !== played.sr) {
        return { error: `length/sr differ: graph ${graph.length}@${graph.sr}, played ${played.length}@${played.sr}` };
      }
      const a = new Float32Array(graph.bytes.buffer, graph.bytes.byteOffset, graph.length);
      const b = new Float32Array(played.bytes.buffer, played.bytes.byteOffset, played.length);
      for (let i = 0; i < a.length; i++) {
        if (a[i] !== b[i]) return { error: `sample ${i} differs: graph ${a[i]}, played ${b[i]}` };
      }
      window.updateSampleEdit(id, JSON.stringify({ StartFrac: 0, EndFrac: 1 }));
      return { ok: true, n: a.length };
    },
    { id, edit }
  );
}

async function main() {
  const { page, cleanup } = await setupFullWasm({ logLevel: "INFO" });
  const failures = [];
  let compared = 0;
  try {
    await page.waitForFunction(
      () =>
        typeof window.renderDocPCM === "function" &&
        typeof window.__instrumentBlockForTest === "function" &&
        typeof window.ensureSynthSample === "function"
    );
    const edits = [null, { StartFrac: 0.05, EndFrac: 0.7, Reverse: true, GainDB: -3 }];
    for (const id of IDS) {
      for (const edit of edits) {
        const res = await compareGraphToPlayback(page, id, edit);
        const tag = `${id} edit=${edit ? "on" : "off"}`;
        if (res.skip) {
          console.log(`[test] SKIP ${tag}: ${res.skip}`);
        } else if (res.error) {
          failures.push(`${tag}: ${res.error}`);
        } else {
          compared++;
          console.log(`[test] ${tag}: graph render == played voice (${res.n} samples)`);
        }
      }
    }
    if (compared === 0) failures.push("compared nothing: no instrument had a pushed block");
  } finally {
    await cleanup();
  }
  if (failures.length) {
    console.error("FAIL synth_graph_render:\n  " + failures.join("\n  "));
    process.exit(1);
  }
  console.log(`PASS synth_graph_render (${compared} comparisons)`);
}

main().catch((e) => {
  console.error(e);
  process.exit(1);
});
