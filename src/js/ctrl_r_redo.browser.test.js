// COVERED-BY-GO: Go covers the redo wiring (internal/ui/node_clipboard_test.go
// TestCtrlRRedoesViaKeyboard). This JS test owns the browser-only, end-to-end
// concern: in a real WASM build, Ctrl-R drives Go's redo AND the browser does
// NOT reload the tab.
//
// Reload-suppression is provided by Ebiten itself, not custom JS: Ebiten's
// canvas keydown handler (ebiten/v2 internal/ui/ui_js.go setCanvasEventHandlers)
// calls e.preventDefault() UNCONDITIONALLY on every keydown once the canvas is
// focused — which is exactly the state in which Ctrl-R can reach Go and redo.
// A previously-added index.html Ctrl-R handler was therefore redundant and was
// removed (verified: this test passes identically with or without it). This test
// still guards the real regression surface: the ungated Ctrl-R redo binding and
// that the WASM canvas path suppresses the native reload.
import { setupFullWasm } from "./real_input_test_helpers.js";
import assert from "node:assert";

let page, cleanup;

async function run() {
  ({ page, cleanup } = await setupFullWasm());

  // Boot settle + a per-load sentinel: a reload wipes the JS heap, so if this
  // survives Ctrl-R the page did NOT reload.
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
  // NOTE: click lands in the desktop transport-bar strip (y < gridTopOffset,
  // 40px), NOT the grid pane — a click on an empty grid cell adds a node
  // (tap-to-add), which would corrupt the node-count assertions below.
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

  // Redo via Ctrl-R (the ungated Go binding). Ebiten's canvas keydown handler
  // preventDefaults the native reload; if that ever regresses the page reloads
  // and both assertions below break.
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
