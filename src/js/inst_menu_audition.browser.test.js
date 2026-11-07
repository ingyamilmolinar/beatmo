/** Through the WASM build and real mouse events, clicking an item changes the row without closing the menu, so the user
 * can try sounds rapidly. COVERED-BY-GO: inst_menu_audition_test.go, drumview_overlay_inst_comp_test.go,
 * inst_menu_interaction_test.go */

import { setupFullWasm, clickAndHold } from "./real_input_test_helpers.js";

// page.mouse.click() can coalesce down and up before the WASM loop samples either edge.
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

  // A real click also lets the press→release clear the click suppression, so later item clicks are not eaten.
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

  // Held until rAF samples it: under contention a fast keydown→keyup can fall between frames.
  await page.keyboard.down("Escape");
  await page.waitForTimeout(150);
  await page.keyboard.up("Escape");

  try {
    await page.waitForFunction(
      () => typeof instMenuOpenState === "function" && instMenuOpenState() === false, null,
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
