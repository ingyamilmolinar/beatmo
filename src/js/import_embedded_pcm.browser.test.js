// A freshly exported project must import into a fresh browser through the REAL picker, slowly: a blind 3 s timer after
// input.click() once resolved with '' before a normal-speed pick, and the empty result silently no-oped.
// COVERED-BY-GO: internal/ui/import_embedded_pcm_test.go (g.Import)

import fs from "fs";
import path from "path";
import { fileURLToPath } from "url";
import { setupFullWasm } from "./real_input_test_helpers.js";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const FIXTURE = path.resolve(
  __dirname,
  "../go/internal/ui/testdata/import_embedded_pcm.json"
);
// Longer than a 3 s fallback timer would wait, so a normal-speed pick is proven not pre-empted.
const SLOW_PICK_MS = 3500;
// exportJSON() reflects post-import state: every embedded-PCM row must keep its id.
const EXPECT_IDS = ["kick-snap", "snare", "hihat", "clap", "cowbell", "fm-epiano-drive"];
const EXPECT_NODES = 53;

async function main() {
  const json = fs.readFileSync(FIXTURE, "utf8");
  console.log(`[test] fixture ${FIXTURE} (${json.length} bytes)`);

  const { page, cleanup } = await setupFullWasm({ logLevel: "INFO" });
  const failures = [];

  try {
    // --- Sanity: the bridge import works directly (isolates picker from Import). ---
    const sane = await page.evaluate((txt) => {
      try {
        window.importJSON(txt);
        const out = JSON.parse(window.exportJSON());
        return { nodes: (out.nodes || []).length, ids: (out.instruments || []).map((i) => i.id) };
      } catch (e) {
        return { error: String(e && e.message ? e.message : e) };
      }
    }, json);
    console.log("[test] direct importJSON ->", JSON.stringify(sane));
    if (sane.error) failures.push(`direct importJSON threw: ${sane.error}`);
    else if (sane.nodes !== EXPECT_NODES)
      failures.push(`direct importJSON nodes=${sane.nodes}, want ${EXPECT_NODES}`);

    // Playwright turns the programmatic input.click() into a filechooser event.
    const slowChooser = async (chooser) => {
      await page.waitForTimeout(SLOW_PICK_MS);
      await chooser.setFiles(FIXTURE);
    };
    page.on("filechooser", slowChooser);

    const picked = await page.evaluate(() => window.openJSONFile());
    console.log(`[test] openJSONFile (slow pick) resolved ${picked ? picked.length : 0} bytes`);
    if (!picked || picked.length < 1000) {
      failures.push(
        `openJSONFile dropped a slow (${SLOW_PICK_MS}ms) selection: resolved ${picked ? picked.length : 0} bytes (the 3s timeout bug)`
      );
    }
    page.off("filechooser", slowChooser);

    // The picked bytes must rebuild the full project (53 nodes, 6 instruments) through the real import; the Go delivery
    // plumbing is import_button_flow_test.go's.
    const loaded = await page.evaluate((txt) => {
      window.importJSON(txt);
      const out = JSON.parse(window.exportJSON());
      return { nodes: out.nodes.length, ids: out.instruments.map((i) => i.id), rows: window.totalRows() };
    }, picked || "");
    console.log(`[test] picked-bytes import -> ${JSON.stringify(loaded)}`);
    if (loaded.nodes !== EXPECT_NODES)
      failures.push(`picked-bytes import nodes=${loaded.nodes}, want ${EXPECT_NODES}`);
    if (loaded.rows !== 6) failures.push(`picked-bytes import rows=${loaded.rows}, want 6`);
    for (const id of EXPECT_IDS)
      if (!loaded.ids.includes(id)) failures.push(`picked-bytes import missing instrument ${id}`);

    if (failures.length) {
      throw new Error("IMPORT REPRO FAILED:\n  - " + failures.join("\n  - "));
    }
    console.log("[test] PASS: embedded-PCM project imports through the real file picker");
  } finally {
    await cleanup();
  }
}

main().catch((e) => {
  console.error(e);
  process.exit(1);
});
