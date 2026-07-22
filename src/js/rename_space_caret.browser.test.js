/**
 * Instrument-rename caret editing via REAL keyboard events — Space at the start.
 *
 * Reproduces the reported bug: open the instrument rename box, move the caret to
 * the START with ArrowLeft, press Space — the space MUST be inserted at the
 * beginning of the name (" kick"), not swallowed. Space is also the transport
 * play/pause shortcut, so this also verifies the focused rename box owns it.
 *
 * ROOT CAUSE this pins: the soft-keyboard proxy is kept EMPTY (the app draws its
 * own text), and on some browsers the proxy's 'input'/'beforeinput' event does
 * NOT fire for a key typed into an empty/off-screen input — the exact quirk the
 * code documents for Backspace (index.html). Space was captured ONLY via that
 * 'input' event (unlike Enter/Backspace/arrows, captured from 'keydown'), so on
 * such a browser the space is dropped while keydown-forwarded arrows still work.
 * The second scenario below simulates that browser by neutralizing the proxy's
 * 'input' event and asserts Space still lands (captured from 'keydown').
 *
 * Charter: real input dispatch through the canvas (item 6).
 * Go-side coverage with stubbed input: internal/ui/rename_space_caret_test.go
 */
import { setupFullWasm, clickAndHold } from "./real_input_test_helpers.js";

let cleanup;
let page;

function must(cond, msg) {
  if (!cond) throw new Error(msg);
}

async function openRenameFocused() {
  await page.evaluate(() => { openRenameBox?.(0); forceDraw?.(); });
  await page.waitForTimeout(300);
  const rr = await page.evaluate(() => renameBoxRect?.());
  must(rr, "rename box rect not available after openRenameBox");
  await clickAndHold(page, rr.x + rr.w / 2, rr.y + rr.h / 2, 50);
  await page.waitForTimeout(200);
  const st = await page.evaluate(() => renameBoxState?.());
  must(st && st.focused, `rename box not focused: ${JSON.stringify(st)}`);
  return st;
}

async function caretToStartThenSpace(initial) {
  for (let i = 0; i < initial.length; i++) {
    await page.keyboard.press("ArrowLeft");
    await page.waitForTimeout(60);
  }
  let st = await page.evaluate(() => renameBoxState?.());
  must(st.cursor === 0, `caret not at start (got ${st.cursor}, text=${JSON.stringify(st.text)})`);
  const wasPlaying = await page.evaluate(() => isPlaying?.());
  await page.keyboard.press("Space");
  await page.waitForTimeout(250);
  st = await page.evaluate(() => renameBoxState?.());
  must(
    st.text === " " + initial && st.cursor === 1,
    `SPACE-AT-START BUG: want ${JSON.stringify(" " + initial)}/caret 1, got ${JSON.stringify(st)}`,
  );
  const nowPlaying = await page.evaluate(() => isPlaying?.());
  must(nowPlaying === wasPlaying, "Space in the focused rename box must not toggle playback");
}

try {
  console.log("rename_space_caret: Setting up WASM...");
  ({ page, cleanup } = await setupFullWasm());
  await page.evaluate(() => forceDraw?.());
  await page.waitForTimeout(200);

  // --- Scenario 1: normal browser (proxy input event fires). ---
  let st = await openRenameFocused();
  await caretToStartThenSpace(st.text);
  console.log("rename_space_caret: scenario 1 (normal) OK");

  // Close and reopen for scenario 2.
  await page.keyboard.press("Escape");
  await page.waitForTimeout(150);

  // --- Scenario 2: quirky browser — the proxy's 'input' event never reaches
  // our handler (simulates the empty-proxy event-drop quirk). Space MUST still
  // be captured from 'keydown'. Before the fix this fails (space is dropped). ---
  await page.evaluate(() => {
    const p = document.getElementById("beatmo-kb-proxy");
    // Swallow input/beforeinput in the capture phase so the app's own listeners
    // never see them — as on a browser that doesn't emit them for the empty proxy.
    p.addEventListener("beforeinput", (e) => e.stopImmediatePropagation(), true);
    p.addEventListener("input", (e) => e.stopImmediatePropagation(), true);
  });
  st = await openRenameFocused();
  await caretToStartThenSpace(st.text);
  console.log("rename_space_caret: scenario 2 (input-event dropped) OK");

  console.log("rename_space_caret: PASS - Space inserts at the caret even when the proxy input event is unreliable");
} catch (err) {
  console.error("rename_space_caret: FAIL -", err.message);
  process.exitCode = 1;
} finally {
  if (cleanup) await cleanup();
}
