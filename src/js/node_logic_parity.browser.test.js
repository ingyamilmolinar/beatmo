/**
 * Node-Logic Cross-Platform Parity (WASM ↔ native Go)
 *
 * Imports a circuit that exercises EVERY built-in node-logic kind
 * (every_n_triggers, skip_every_n, probability, trigger_if_prev_*) and asserts
 * the WASM predictor reproduces the native Go schedule BYTE-FOR-BYTE over a deep
 * horizon (many loop iterations). This proves logic-kind evaluation AND the
 * uint64 probability roll are identical across the native↔WASM compile boundary,
 * with the bounded dirty-rebuild fix (core/engine) compiled into the WASM binary
 * — i.e. the choppiness fix did not perturb the schedule on the platform the
 * user said was worst.
 *
 * Golden: src/go/internal/ui/testdata/parity_golden_node_rules.json, generated
 * by `go test -run TestNodeRuleParityGolden ./internal/ui/` (native).
 *
 * COVERED-BY-GO (the rebuild byte-identity at large windowStart, which the UI
 * window anchor makes unreachable in a static import):
 *   core/engine TestDirtyRebuildEquivalentToFullWalk — exhaustive across logic
 *   profiles × windowStart values; pure integer logic, architecture-independent.
 */

import { setupFullWasm } from "./real_input_test_helpers.js";
import { flushCoverage, isCoverageEnabled } from "./coverage_helpers.js";
import fs from "fs";
import path from "path";
import { fileURLToPath } from "url";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const fixturePath = path.resolve(__dirname, "../go/internal/assets/parity_fixture_node_rules.json");
const goldenPath = path.resolve(__dirname, "../go/internal/ui/testdata/parity_golden_node_rules.json");

let cleanup;
let page;

try {
  console.log("node_logic_parity: Setting up WASM...");
  ({ page, cleanup } = await setupFullWasm());
  await page.evaluate(() => forceDraw?.());
  await page.waitForTimeout(200);

  const fixtureJSON = fs.readFileSync(fixturePath, "utf-8");
  const golden = JSON.parse(fs.readFileSync(goldenPath, "utf-8"));
  const horizon = golden.horizon;

  await page.evaluate((json) => importJSON?.(json), fixtureJSON);
  await page.waitForTimeout(300);
  await page.evaluate(() => { updateBeatInfos?.(); forceDraw?.(); });
  await page.waitForTimeout(200);

  let totalMismatches = 0;
  for (let row = 0; row < golden.rows; row++) {
    const goldenRow = golden.small[row];
    if (!goldenRow) { console.log(`  WARNING: golden missing row ${row}`); continue; }

    // Batch-query the predictor in one round-trip.
    const wasm = await page.evaluate(({ row, horizon }) => {
      const visible = [], audible = [], triggered = [];
      for (let idx = 0; idx < horizon; idx++) {
        visible.push(visibleAt?.(row, idx) ?? false);
        audible.push(audibleAt?.(row, idx) ?? false);
        triggered.push(triggeredAt?.(row, idx) ?? false);
      }
      return { visible, audible, triggered };
    }, { row, horizon });

    let rowMismatches = 0;
    const check = (kind, w, g) => {
      for (let idx = 0; idx < horizon; idx++) {
        if (w[idx] !== g[idx]) {
          if (rowMismatches < 5) console.log(`  MISMATCH row=${row} idx=${idx} ${kind}: wasm=${w[idx]} golden=${g[idx]}`);
          rowMismatches++;
        }
      }
    };
    check("visible", wasm.visible, goldenRow.visible);
    check("audible", wasm.audible, goldenRow.audible);
    check("triggered", wasm.triggered, goldenRow.triggered);

    const aud = wasm.audible.filter(Boolean).length;
    console.log(`  row ${row}: audible=${aud}/${horizon} mismatches=${rowMismatches}`);
    totalMismatches += rowMismatches;
  }

  if (totalMismatches > 0) {
    throw new Error(`node-logic parity failed: ${totalMismatches} mismatches`);
  }
  console.log("\nnode_logic_parity: PASS — WASM predictor matches native golden across all logic kinds");
} catch (error) {
  console.error("node_logic_parity: FAIL -", error.message);
  process.exitCode = 1;
} finally {
  if (isCoverageEnabled()) await flushCoverage(page, new URL("../../coverage/browser-raw", import.meta.url).pathname, "node_logic_parity");
  if (cleanup) await cleanup();
}
