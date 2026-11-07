/** Through REAL keyboard events in the logs panel: a character typed after ArrowLeft lands AT the caret, and Home, End
 * and Delete work. COVERED-BY-GO: logs_search_caret_repro_test.go, textinput_test.go */
import { setupFullWasm, clickAndHold } from "./real_input_test_helpers.js";

let cleanup;
let page;

function must(cond, msg) {
  if (!cond) throw new Error(msg);
}

try {
  console.log("logs_search_caret: Setting up WASM...");
  ({ page, cleanup } = await setupFullWasm());

  await page.evaluate(() => forceDraw?.());
  await page.waitForTimeout(200);

  // Open the logs panel.
  const open = await page.evaluate(() => {
    const ok = openLogsPanel?.();
    forceDraw?.();
    return ok;
  });
  must(open, "logs panel did not open");
  await page.waitForTimeout(200);

  let sr = await page.evaluate(() => logsSearchRect?.());
  must(sr, "logs search rect not available after opening the panel");

  // Click the search field with the real mouse to focus it.
  await clickAndHold(page, sr.x + sr.w / 2, sr.y + sr.h / 2, 50);
  await page.waitForTimeout(200);
  let st = await page.evaluate(() => logsSearchState?.());
  must(st && st.focused, `logs search box not focused after click: ${JSON.stringify(st)}`);

  // Type the seed query with real key events.
  await page.keyboard.type("kick", { delay: 40 });
  await page.waitForTimeout(300);
  st = await page.evaluate(() => logsSearchState?.());
  must(st.text === "kick", `typed query: want "kick", got ${JSON.stringify(st)}`);
  must(st.cursor === 4, `caret after typing: want 4, got ${st.cursor}`);

  // Move the caret back to the start with ArrowLeft.
  for (let i = 0; i < 4; i++) {
    await page.keyboard.press("ArrowLeft");
    await page.waitForTimeout(80);
  }
  st = await page.evaluate(() => logsSearchState?.());
  must(
    st.cursor === 0,
    `ARROW BUG: caret after 4xArrowLeft: want 0, got ${st.cursor} (text=${JSON.stringify(st.text)})`,
  );

  // Insert a character at the beginning.
  await page.keyboard.type("x", { delay: 40 });
  await page.waitForTimeout(300);
  st = await page.evaluate(() => logsSearchState?.());
  must(
    st.text === "xkick" && st.cursor === 1,
    `INSERT-AT-START BUG: want "xkick"/caret 1, got ${JSON.stringify(st)}`,
  );

  // ── End jumps to the end of the string.
  await page.keyboard.press("End");
  await page.waitForTimeout(120);
  st = await page.evaluate(() => logsSearchState?.());
  must(st.cursor === 5, `END: caret should jump to end (5), got ${st.cursor}`);

  // ── Home jumps back to the start.
  await page.keyboard.press("Home");
  await page.waitForTimeout(120);
  st = await page.evaluate(() => logsSearchState?.());
  must(st.cursor === 0, `HOME: caret should jump to start (0), got ${st.cursor}`);

  // ── Delete (forward delete) removes the char AT the caret, caret stays.
  await page.keyboard.press("Delete");
  await page.waitForTimeout(120);
  st = await page.evaluate(() => logsSearchState?.());
  must(
    st.text === "kick" && st.cursor === 0,
    `DELETE: forward-delete at start should give "kick"/caret 0, got ${JSON.stringify(st)}`,
  );

  console.log("logs_search_caret: PASS - caret editing + Home/End/Delete work in the logs search");
} catch (err) {
  console.error("logs_search_caret: FAIL -", err.message);
  process.exitCode = 1;
} finally {
  if (cleanup) await cleanup();
}
