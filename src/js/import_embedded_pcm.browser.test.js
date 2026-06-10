// import_embedded_pcm.browser.test.js
//
// Reproduces the reported bug: a freshly-exported project file fails to import
// into a fresh browser instance ("nothing happens", no error toast).
//
// Root cause is NOT g.Import (the Go fast-path test
// internal/ui/import_embedded_pcm_test.go and the direct importJSON() bridge call
// below both succeed). It is the file-PICKER delivery layer:
//   - audio.js openJSONFile() arms a blind `setTimeout(() => settle(''), 3000)`
//     right after input.click(). If the user takes >3s to navigate the native
//     dialog and pick the file (normal), the timer resolves the promise with ''
//     BEFORE the real selection; the later onchange settle(text) is dropped.
//   - The empty string flows to drumview_update.go's `len(res.data)==0` branch,
//     which silently no-ops — no toast, nothing loaded.
//
// This test drives the real picker via Playwright's filechooser with a delayed
// selection (>3s) to deterministically reproduce the silent drop, and verifies
// the imported project actually took effect via exportJSON().
//
// JS Test Charter: this owns the Go↔browser file-picker boundary Go can't reach.

import fs from "fs";
import path from "path";
import { fileURLToPath } from "url";
import { setupFullWasm } from "./real_input_test_helpers.js";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const FIXTURE = path.resolve(
  __dirname,
  "../go/internal/ui/testdata/import_embedded_pcm.json"
);
// Longer than the buggy 3s openJSONFile fallback, to prove a normal-speed file
// pick is not pre-empted.
const SLOW_PICK_MS = 3500;
const EXPECT_IDS = ["kick-1", "snare", "hihat", "clap", "cowbell", "fm-epiano-1"];
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

    // --- REPRO: slow file pick through the real openJSONFile() must NOT drop. ---
    // Playwright intercepts the programmatic input.click() and emits filechooser.
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

    // --- Selecting the file then feeding it through the real bridge import must
    // produce the full project (53 nodes, all 6 instruments). This proves the
    // slow-picked contents are intact end to end (picker text -> g.Import). The
    // Go delivery plumbing (importCh -> onImport -> pendingImportData -> Import)
    // is covered by internal/ui/import_button_flow_test.go; here we assert the
    // bytes the picker now delivers reconstruct the project faithfully. ---
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
