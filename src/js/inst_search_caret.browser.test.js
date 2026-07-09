/**
 * Instrument-menu search caret editing via REAL keyboard events.
 *
 * Reproduces the reported bug: focus the search bar, type a query, move the
 * caret back to the START with the ArrowLeft key, and type a character — the
 * character must insert AT the caret (not at the end, and not be silently
 * dropped). This exercises the browser keyboard path (canvas focus, ebiten
 * key events, soft-keyboard proxy interplay) that Go's stubbed-input tests
 * cannot reach. Charter: real input dispatch through the canvas (item 6).
 *
 * Go-side coverage of the same contract with stubbed input:
 *   internal/ui/inst_menu_search_cursor_test.go
 */
import { setupFullWasm, clickAndHold } from "./real_input_test_helpers.js";

let cleanup;
let page;

function must(cond, msg) {
  if (!cond) throw new Error(msg);
}

try {
  console.log("inst_search_caret: Setting up WASM...");
  ({ page, cleanup } = await setupFullWasm());

  await page.evaluate(() => forceDraw?.());
  await page.waitForTimeout(200);

  // Open the instrument menu for row 0.
  await page.evaluate(() => {
    openInstMenu?.(0);
    forceDraw?.();
  });
  await page.waitForTimeout(300);
  const open = await page.evaluate(() => instMenuOpenState?.());
  must(open, "instrument menu did not open");

  // Drill into instruments mode when the menu opened on categories.
  let sr = await page.evaluate(() => instMenuSearchRect?.());
  if (!sr) {
    const cats = await page.evaluate(() => instMenuCategoryRects?.());
    must(cats && cats.length > 0, "no search rect and no category rects to drill into");
    const c = cats[0];
    await clickAndHold(page, c.x + c.w / 2, c.y + c.h / 2, 50);
    await page.waitForTimeout(300);
    sr = await page.evaluate(() => instMenuSearchRect?.());
  }
  must(sr, "search rect not available after entering instruments mode");

  // Click the search field with the real mouse to focus it.
  await clickAndHold(page, sr.x + sr.w / 2, sr.y + sr.h / 2, 50);
  await page.waitForTimeout(200);
  let st = await page.evaluate(() => instMenuSearchState?.());
  must(st && st.focused, `search box not focused after click: ${JSON.stringify(st)}`);

  // Type the seed query with real key events through the page.
  await page.keyboard.type("kick", { delay: 40 });
  await page.waitForTimeout(300);
  st = await page.evaluate(() => instMenuSearchState?.());
  must(st.text === "kick", `typed query: want "kick", got ${JSON.stringify(st)}`);
  must(st.cursor === 4, `caret after typing: want 4, got ${st.cursor}`);

  // Move the caret back to the start with the arrow keys.
  for (let i = 0; i < 4; i++) {
    await page.keyboard.press("ArrowLeft");
    await page.waitForTimeout(80);
  }
  st = await page.evaluate(() => instMenuSearchState?.());
  must(
    st.cursor === 0,
    `ARROW BUG: caret after 4×ArrowLeft: want 0, got ${st.cursor} (text=${JSON.stringify(st.text)}) — ` +
      "arrow keys must move the search caret in the browser",
  );

  // Insert a character at the beginning.
  await page.keyboard.type("x", { delay: 40 });
  await page.waitForTimeout(300);
  st = await page.evaluate(() => instMenuSearchState?.());
  must(
    st.text === "xkick",
    `INSERT-AT-START BUG: want "xkick", got ${JSON.stringify(st)} — the typed character must ` +
      "insert at the caret, not at the end / nowhere",
  );
  must(st.cursor === 1, `caret after insert: want 1, got ${st.cursor}`);

  // And the live filter must have followed the edit.
  const bc = await page.evaluate(() => instMenuSearchRect?.() && true);
  must(bc, "menu should still be open with the search bar visible");

  // ── Space at the start (the originally-reported character). Space is also
  // the transport play/pause shortcut — it must insert into the focused box,
  // not toggle playback.
  for (let i = 0; i < 1; i++) {
    await page.keyboard.press("ArrowLeft");
    await page.waitForTimeout(80);
  }
  st = await page.evaluate(() => instMenuSearchState?.());
  must(st.cursor === 0, `caret back to 0 before space, got ${st.cursor}`);
  await page.keyboard.press("Space");
  await page.waitForTimeout(300);
  st = await page.evaluate(() => instMenuSearchState?.());
  must(
    st.text === " xkick" && st.cursor === 1,
    `SPACE-AT-START: want " xkick"/caret 1, got ${JSON.stringify(st)}`,
  );
  const playing = await page.evaluate(() => isPlaying?.());
  must(!playing, "Space in the focused search box must not start playback");

  // ── Same caret contract on the CATEGORIES-view global search bar.
  // Clear the pending query with real keys (caret to end, then backspace all
  // 6 chars of " xkick"), then Back returns to the categories view.
  for (let i = 0; i < 5; i++) {
    await page.keyboard.press("ArrowRight");
    await page.waitForTimeout(60);
  }
  for (let i = 0; i < 6; i++) {
    await page.keyboard.press("Backspace");
    await page.waitForTimeout(60);
  }
  st = await page.evaluate(() => instMenuSearchState?.());
  must(
    st && st.text === "",
    `backspace-clear: want empty text, got ${JSON.stringify(st)} (ArrowRight/Backspace editing broken)`,
  );
  const wentBack = await page.evaluate(() => instMenuClickBack?.());
  must(wentBack, "back button must return to the categories view");
  await page.evaluate(() => forceDraw?.());
  await page.waitForTimeout(300);
  const crumbs = await page.evaluate(() => instMenuBreadcrumbPath?.());
  must(
    crumbs && crumbs.length === 1,
    `expected the categories view (breadcrumb depth 1), got ${JSON.stringify(crumbs)}`,
  );
  const gsr = await page.evaluate(() => instMenuSearchRect?.());
  must(gsr, "categories view must expose the global search bar rect");
  await clickAndHold(page, gsr.x + gsr.w / 2, gsr.y + gsr.h / 2, 50);
  await page.waitForTimeout(200);
  st = await page.evaluate(() => instMenuSearchState?.());
  must(st && st.focused, `global search box not focused after click: ${JSON.stringify(st)}`);
  await page.keyboard.type("kick", { delay: 40 });
  await page.waitForTimeout(300);
  st = await page.evaluate(() => instMenuSearchState?.());
  must(st.text === "kick", `global bar typed query: want "kick", got ${JSON.stringify(st)}`);
  for (let i = 0; i < 4; i++) {
    await page.keyboard.press("ArrowLeft");
    await page.waitForTimeout(80);
  }
  st = await page.evaluate(() => instMenuSearchState?.());
  must(st.cursor === 0, `GLOBAL-BAR ARROW BUG: caret after 4×ArrowLeft want 0, got ${st.cursor}`);
  await page.keyboard.type("x", { delay: 40 });
  await page.waitForTimeout(300);
  st = await page.evaluate(() => instMenuSearchState?.());
  must(
    st.text === "xkick" && st.cursor === 1,
    `GLOBAL-BAR INSERT-AT-START BUG: want "xkick"/caret 1, got ${JSON.stringify(st)}`,
  );

  console.log("inst_search_caret: PASS - caret editing via real keyboard works on both search bars");
} catch (error) {
  console.error("inst_search_caret: FAIL -", error.message);
  process.exitCode = 1;
} finally {
  if (cleanup) {
    await cleanup();
  }
}
