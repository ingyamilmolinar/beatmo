/** A rectangle of 4 nodes: edges are only allowed between nodes sharing an I or J coordinate. */

import { setupFullWasm } from "./real_input_test_helpers.js";
import {
  shiftDragEdge,
  assertNodeExists,
  assertNoNode,
} from "./real_input_actions.js";
import { flushCoverage, isCoverageEnabled } from "./coverage_helpers.js";

let cleanup;
let page;

try {
  console.log("real_input_edge_drag: Setting up WASM...");
  ({ page, cleanup } = await setupFullWasm());

  await page.evaluate(() => {
    ensureDefaultPath?.();
    forceDraw?.();
  });
  await page.waitForTimeout(200);

  console.log("real_input_edge_drag: Creating 4 nodes via API...");
  const nodes = [[50, -4], [54, -4], [54, -8], [50, -8]];
  await page.evaluate((ns) => {
    for (const [i, j] of ns) addNode?.(i, j, "regular");
    forceDraw?.();
  }, nodes);
  await page.waitForTimeout(200);

  // Pan camera to center on test nodes
  const rect0 = await page.evaluate(([i, j]) => nodeRect?.(i, j), nodes[0]);
  if (!rect0) throw new Error("Node 0 has no screen rect");
  await page.evaluate(([r]) => {
    const s = splitY?.();
    panBy?.(640 - r.x - 30, (40 + s) / 2 - r.y);
    forceDraw?.();
  }, [rect0]);
  await page.waitForTimeout(200);

  console.log("real_input_edge_drag: Creating edge (50,-4)->(54,-4) via shift-drag...");
  await shiftDragEdge(page, ...nodes[0], ...nodes[1]);

  const succs0 = await page.evaluate(([i, j]) => nodeSuccessorsGrid?.(i, j), nodes[0]);
  const hasEdge01 = succs0?.some(s => s.i === nodes[1][0] && s.j === nodes[1][1]);
  if (!hasEdge01) throw new Error("Shift-drag failed to create edge 0->1");
  console.log("  Edge 0->1 created via shift-drag");

  console.log("real_input_edge_drag: Completing loop via API...");
  await page.evaluate((ns) => {
    addEdgeGrid?.(ns[1][0], ns[1][1], ns[2][0], ns[2][1]);
    addEdgeGrid?.(ns[2][0], ns[2][1], ns[3][0], ns[3][1]);
    addEdgeGrid?.(ns[3][0], ns[3][1], ns[0][0], ns[0][1]);
    updateBeatInfos?.();
  }, nodes);
  await page.waitForTimeout(200);

  // Verify all 4 edges exist
  for (let k = 0; k < nodes.length; k++) {
    const src = nodes[k], dst = nodes[(k + 1) % nodes.length];
    const s = await page.evaluate(([i, j]) => nodeSuccessorsGrid?.(i, j), src);
    const has = s?.some(x => x.i === dst[0] && x.j === dst[1]);
    if (!has) throw new Error(`Missing edge ${k}->${(k + 1) % nodes.length}`);
  }
  console.log("  All 4 edges verified");

  // Via the API: in WASM the browser context menu interferes with right-click.
  console.log("real_input_edge_drag: Deleting node 1 via API...");
  await page.evaluate(([i, j]) => deleteNodeGrid?.(i, j), nodes[1]);

  await assertNoNode(page, ...nodes[1], "Node 1 should be deleted");
  console.log("  Node 1 deleted");

  await assertNodeExists(page, ...nodes[0], "Node 0 should survive");
  await assertNodeExists(page, ...nodes[2], "Node 2 should survive");
  await assertNodeExists(page, ...nodes[3], "Node 3 should survive");

  // Edge 0->1 should be gone since node 1 is deleted
  const succs0After = await page.evaluate(([i, j]) => nodeSuccessorsGrid?.(i, j), nodes[0]);
  const stillLinked = succs0After?.some(s => s.i === nodes[1][0] && s.j === nodes[1][1]);
  if (stillLinked) throw new Error("Edge to deleted node still exists");

  // Edge 3->0 should still exist
  const succs3 = await page.evaluate(([i, j]) => nodeSuccessorsGrid?.(i, j), nodes[3]);
  const e30 = succs3?.some(s => s.i === nodes[0][0] && s.j === nodes[0][1]);
  console.log(`  Edge 3->0: ${e30 ? "intact" : "removed"}`);

  // The user's real flow: click a node (the sidebar opens over the grid's left strip), then shift+drag from it. No
  // closeNodeMenu: the connect gesture itself must win over the open sidebar.
  console.log("real_input_edge_drag: Connect from a selected node (sidebar open)...");
  const src = [50, -12], dst = [54, -12];
  await page.evaluate((ns) => {
    for (const [i, j] of ns) addNode?.(i, j, "regular");
    forceDraw?.();
  }, [src, dst]);
  await page.waitForTimeout(200);

  // Inside the sidebar's strip, where clicked nodes in a large circuit routinely are.
  const srcRect = await page.evaluate(([i, j]) => nodeRect?.(i, j), src);
  if (!srcRect) throw new Error("source node has no screen rect");
  await page.evaluate(([r]) => {
    const s = splitY?.();
    panBy?.(120 - (r.x + r.w / 2), (40 + s) / 2 - (r.y + r.h / 2));
    forceDraw?.();
  }, [srcRect]);
  await page.waitForTimeout(200);

  const at = async ([i, j]) => {
    const r = await page.evaluate(([gi, gj]) => nodeRect?.(gi, gj), [i, j]);
    if (!r) throw new Error(`node (${i},${j}) has no screen rect`);
    return { x: Math.floor(r.x + r.w / 2), y: Math.floor(r.y + r.h / 2) };
  };

  // Step 6a: plain REAL click selects the source node and opens its sidebar.
  const srcPt = await at(src);
  await page.mouse.click(srcPt.x, srcPt.y);
  await page.waitForTimeout(250);

  // All real gestures, nothing closed programmatically.
  const dstPt = await at(dst);
  await page.mouse.move(srcPt.x, srcPt.y);
  await page.waitForTimeout(50);
  await page.keyboard.down("Shift");
  await page.waitForTimeout(50);
  await page.mouse.down();
  await page.waitForTimeout(80);
  const steps = 10;
  for (let k = 1; k <= steps; k++) {
    const t = k / steps;
    await page.mouse.move(
      Math.floor(srcPt.x + (dstPt.x - srcPt.x) * t),
      Math.floor(srcPt.y + (dstPt.y - srcPt.y) * t),
    );
    await page.waitForTimeout(20);
  }
  await page.waitForTimeout(80);
  await page.mouse.up();
  await page.keyboard.up("Shift");
  await page.waitForTimeout(250);

  const succsSrc = await page.evaluate(([i, j]) => nodeSuccessorsGrid?.(i, j), src);
  const connected = succsSrc?.some((s) => s.i === dst[0] && s.j === dst[1]);
  if (!connected) {
    throw new Error(
      "shift-drag from a SELECTED node (sidebar open) failed to create the edge — the sidebar consumed the connect gesture",
    );
  }
  console.log("  Edge created from selected node with sidebar open");

  console.log("real_input_edge_drag: PASS - Edge creation, deletion, and cleanup via real input");
} catch (error) {
  console.error("real_input_edge_drag: FAIL -", error.message);
  process.exitCode = 1;
} finally {
  if (isCoverageEnabled()) await flushCoverage(page, new URL("../../coverage/browser-raw", import.meta.url).pathname, "real_input_edge_drag");
  if (cleanup) {
    await cleanup();
  }
}
