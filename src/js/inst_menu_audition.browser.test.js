/**
 * Instrument-menu audition (cross-platform parity)
 *
 * Pins the WASM-compiled audition flow: clicking an instrument item must
 * not close the menu, so the user can rapidly try multiple sounds without
 * reopening it. The menu only closes via explicit dismissal (X / Escape /
 * click-outside / row switch).
 *
 * COVERED-BY-GO:
 *   - Stay-open after one selection, multi-select session, repeated clicks,
 *     during-playback selection, X/Escape/click-outside close paths,
 *     favorites star non-selecting, row-switch closes prior:
 *     internal/ui/inst_menu_audition_test.go
 *   - Component-level OnSelect callback contract:
 *     internal/ui/drumview_overlay_inst_comp_test.go
 *   - Full DrumView.Update() flow:
 *     internal/ui/inst_menu_interaction_test.go
 *
 * What this test owns (per JS Test Charter §6 — "real input dispatch
 * through the canvas" and §5 — "cross-platform parity"):
 *   1. The WASM-compiled menu component honors the same stay-open contract
 *      as the native build when driven by real Playwright mouse events.
 *   2. The Go→JS export surface (`openInstMenu`, `instMenuItemRects`,
 *      `instMenuOpenState`, `rowInstrument`) routes a real canvas click
 *      to a row-instrument mutation without dismissing the menu.
 */

import { setupFullWasm, clickAndHold } from "./real_input_test_helpers.js";

// Real Ebiten/WASM input loop needs a non-trivial press-hold to register
// both press and release edges. Use clickAndHold(50ms+) instead of
// page.mouse.click(), which often coalesces the down/up before the WASM
// game loop can sample either edge.
async function realClickAt(page, x, y) {
  await clickAndHold(page, Math.floor(x), Math.floor(y), 80);
  await page.waitForTimeout(150);
}

let cleanup;
let page;

try {
  console.log("inst_menu_audition: setting up WASM...");
  ({ page, cleanup } = await setupFullWasm());

  // Settle and ensure no stray playback / menu state from a prior run.
  await page.evaluate(() => {
    if (typeof stopPlay === "function") stopPlay();
    if (typeof forceDraw === "function") forceDraw();
  });
  await page.waitForTimeout(200);

  // Make sure the row-0 menu is closed before opening (idempotent).
  const initiallyOpen = await page.evaluate(() =>
    typeof instMenuOpenState === "function" ? instMenuOpenState() : false,
  );
  if (initiallyOpen) {
    // Pressing Escape via keyboard handles the close.
    await page.keyboard.press("Escape");
    await page.waitForTimeout(100);
  }

  // Verify the exports we depend on exist before we drive anything.
  const exportsAvailable = await page.evaluate(() => ({
    rowLabelRect: typeof rowLabelRect === "function",
    instMenuItemRects: typeof instMenuItemRects === "function",
    instMenuOpenState: typeof instMenuOpenState === "function",
    rowInstrument: typeof rowInstrument === "function",
  }));
  for (const [name, ok] of Object.entries(exportsAvailable)) {
    if (!ok) throw new Error(`required JS export missing: ${name}`);
  }

  // ─── Step 1: open the instrument menu for row 0 via a REAL canvas click
  // on the row label. This both opens the menu and lets the press→release
  // cycle clear suppressClicksUntilRelease, so subsequent menu-item clicks
  // are not eaten by the suppression guard.
  console.log("inst_menu_audition: opening menu via real canvas click on row 0 label");
  const labelRect = await page.evaluate(() => rowLabelRect(0));
  if (!labelRect || labelRect.w <= 0 || labelRect.h <= 0) {
    throw new Error(`rowLabelRect(0) invalid: ${JSON.stringify(labelRect)}`);
  }
  await realClickAt(
    page,
    labelRect.x + labelRect.w / 2,
    labelRect.y + labelRect.h / 2,
  );

  let isOpen = await page.evaluate(() => instMenuOpenState());
  if (!isOpen) throw new Error("menu did not open after row label click");

  const initialInstrument = await page.evaluate(() => rowInstrument(0));
  console.log(`inst_menu_audition: row 0 starts on "${initialInstrument}"`);

  // ─── Step 2: pick a different instrument with a real canvas click ────
  let items = await page.evaluate(() => instMenuItemRects());
  if (!Array.isArray(items) || items.length < 2) {
    throw new Error(
      `instMenuItemRects returned ${items ? items.length : 0} items; need ≥ 2 to test multi-select`,
    );
  }

  const firstPick = items.find((it) => it.id !== initialInstrument) ?? items[0];
  if (!firstPick) throw new Error("no instrument item to click");
  const cx1 = Math.floor(firstPick.x + firstPick.w / 2);
  const cy1 = Math.floor(firstPick.y + firstPick.h / 2);
  console.log(
    `inst_menu_audition: real click on item id="${firstPick.id}" at (${cx1},${cy1})`,
  );
  await realClickAt(page, cx1, cy1);

  let rowInst = await page.evaluate(() => rowInstrument(0));
  if (rowInst !== firstPick.id) {
    throw new Error(
      `after first click: rowInstrument(0)="${rowInst}", want "${firstPick.id}"`,
    );
  }

  isOpen = await page.evaluate(() => instMenuOpenState());
  if (!isOpen) {
    throw new Error("menu must stay open after first selection (audition contract)");
  }
  console.log(`inst_menu_audition: row instrument=${rowInst}, menu still open`);

  // ─── Step 3: pick another different instrument without reopening ─────
  items = await page.evaluate(() => instMenuItemRects());
  const secondPick =
    items.find((it) => it.id !== rowInst && it.id !== initialInstrument) ??
    items.find((it) => it.id !== rowInst);
  if (!secondPick) throw new Error("no second instrument item to click");
  const cx2 = Math.floor(secondPick.x + secondPick.w / 2);
  const cy2 = Math.floor(secondPick.y + secondPick.h / 2);
  console.log(
    `inst_menu_audition: real click on item id="${secondPick.id}" at (${cx2},${cy2})`,
  );
  await realClickAt(page, cx2, cy2);

  rowInst = await page.evaluate(() => rowInstrument(0));
  if (rowInst !== secondPick.id) {
    throw new Error(
      `after second click: rowInstrument(0)="${rowInst}", want "${secondPick.id}"`,
    );
  }

  isOpen = await page.evaluate(() => instMenuOpenState());
  if (!isOpen) {
    throw new Error("menu must stay open after second selection (audition contract)");
  }
  console.log(
    `inst_menu_audition: row instrument=${rowInst}, menu still open after 2nd pick`,
  );

  // ─── Step 4: explicit dismissal via Escape closes the menu ───────────
  // Hold Escape long enough for the WASM game loop's rAF to sample the
  // key as pressed. Under CPU contention (parallel browser tests), rAF
  // can be delayed past a fast keydown→keyup pair, leaving Ebiten's
  // polled IsKeyPressed(KeyEscape) false on every frame. Mirrors the
  // hold-then-poll pattern documented for clickAndHold().
  await page.keyboard.down("Escape");
  await page.waitForTimeout(150);
  await page.keyboard.up("Escape");

  try {
    await page.waitForFunction(
      () => typeof instMenuOpenState === "function" && instMenuOpenState() === false,
      { timeout: 3000 },
    );
  } catch (_) {
    throw new Error("menu must close after Escape press");
  }
  isOpen = await page.evaluate(() => instMenuOpenState());
  if (isOpen) {
    throw new Error("menu must close after Escape press");
  }

  console.log(
    "inst_menu_audition: OK — menu stays open across selections; Escape closes it",
  );
} catch (err) {
  console.error("inst_menu_audition FAILED:", err);
  if (cleanup) await cleanup();
  process.exit(1);
}

if (cleanup) await cleanup();
process.exit(0);
