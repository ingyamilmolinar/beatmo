// COVERED-BY-GO: internal/ui/node_clipboard_test.go TestCtrlRRedoesViaKeyboard
// Browser-only half: in a real WASM build Ctrl-R redoes AND the tab does not reload (Ebiten's canvas keydown handler
// preventDefaults every key).
import { setupFullWasm } from "./real_input_test_helpers.js";
import assert from "node:assert";

let page, cleanup;

async function run() {
  ({ page, cleanup } = await setupFullWasm());

  // A reload wipes the JS heap, so a surviving sentinel proves there was none.
  await page.evaluate(() => {
    ensureDefaultPath?.();
    forceDraw?.();
    window.__loadedSentinel = "alive";
  });
  await page.waitForTimeout(300);

  // Focus the canvas so ebiten receives keydowns; ensure the kb-proxy isn't focused.
  await page.evaluate(() => {
    if (typeof _kbProxyBlur === "function") _kbProxyBlur();
    document.querySelector("canvas")?.focus();
  });
  // In the transport strip, not the grid: a tap on an empty cell adds a node.
  await page.mouse.click(20, 20);
  await page.waitForTimeout(100);

  const base = await page.evaluate(() => totalNodes());
  await page.evaluate(() => { addNode?.(70, 70, "regular"); forceDraw?.(); });
  await page.waitForTimeout(200);
  const afterAdd = await page.evaluate(() => totalNodes());
  assert.strictEqual(afterAdd, base + 1, "addNode should add one node");

  // Undo via Ctrl-Z.
  await page.keyboard.down("Control");
  await page.waitForTimeout(60);
  await page.keyboard.press("KeyZ");
  await page.keyboard.up("Control");
  await page.waitForTimeout(200);
  const afterUndo = await page.evaluate(() => totalNodes());
  assert.strictEqual(afterUndo, base, "Ctrl-Z should undo the add");

  // If Ebiten ever stops preventDefaulting, the page reloads and both assertions break.
  await page.keyboard.down("Control");
  await page.waitForTimeout(60);
  await page.keyboard.press("KeyR");
  await page.keyboard.up("Control");
  await page.waitForTimeout(300);

  const sentinel = await page.evaluate(() => window.__loadedSentinel);
  assert.strictEqual(sentinel, "alive", "page reloaded on Ctrl-R (preventDefault failed)");

  const afterRedo = await page.evaluate(() => totalNodes());
  assert.strictEqual(afterRedo, base + 1, "Ctrl-R should redo the add");

  console.log("PASS ctrl_r_redo: redo applied, no reload");
}

run()
  .then(async () => { await cleanup?.(); process.exit(0); })
  .catch(async (err) => { console.error(err); await cleanup?.(); process.exit(1); });
