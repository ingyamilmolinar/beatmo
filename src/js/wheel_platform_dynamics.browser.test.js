/** The ACTUAL magnitudes the DOM produces: under -tags test ebiten.Wheel() is stubbed, so Go can prove the conversion but
 * never that the constants match reality (one notch once scrolled a whole menu). Also the index.html shim: deltaMode and
 * per-frame accumulation. COVERED-BY-GO: wheel_platform_parity_test.go, wheel_dynamics_discipline_test.go */
import { setupFullWasm, wheelAt } from "./real_input_test_helpers.js";
import { flushCoverage, isCoverageEnabled } from "./coverage_helpers.js";

let cleanup;
let page;

// Must match defaultWheelDynamics().MaxRowsPerFrame in input_dynamics.go.
const MAX_ROWS_PER_FRAME = 3;

async function openTemplateMenu() {
  await page.evaluate(() => runScene("template_menu_open"));
  await page.waitForTimeout(300);
  const st = await page.evaluate(() => overflowMenuScrollState());
  if (!st) throw new Error("template menu did not open / no scroll state");
  if (st.total <= st.visible) {
    throw new Error(`template list does not overflow (total=${st.total} visible=${st.visible}); nothing to scroll`);
  }
  return st;
}

async function scrollState() {
  return await page.evaluate(() => overflowMenuScrollState());
}

/** Playwright's mouse.wheel is always DOM_DELTA_PIXEL; Firefox's line mode needs this. */
async function wheelWithDeltaMode(x, y, deltaY, deltaMode) {
  await page.evaluate(
    ({ x, y, deltaY, deltaMode }) => {
      const el = document.elementFromPoint(x, y) || document.body;
      el.dispatchEvent(
        new WheelEvent("wheel", {
          deltaY, deltaMode, clientX: x, clientY: y,
          bubbles: true, cancelable: true,
        }),
      );
    },
    { x, y, deltaY, deltaMode },
  );
}

function fail(msg) {
  throw new Error(msg);
}

try {
  console.log("wheel_dynamics: Setting up full WASM environment...");
  ({ page, cleanup } = await setupFullWasm());

  // The shim must exist before anything else is meaningful.
  const shim = await page.evaluate(() => {
    const w = globalThis.__beatmoWheel;
    return w ? { present: true, keys: Object.keys(w).sort() } : { present: false };
  });
  if (!shim.present) {
    fail("__beatmoWheel transport shim missing from index.html — Go cannot see deltaMode, " +
         "so wheel notches cannot be sized and one notch will scroll the whole list");
  }
  console.log(`  shim present, fields: ${shim.keys.join(",")}`);

  const menu = await openTemplateMenu();
  const box = await page.evaluate(() => {
    const c = document.querySelector("canvas").getBoundingClientRect();
    return { x: c.x, y: c.y, w: c.width, h: c.height };
  });
  // Park the pointer over the scrolling rows of the popup.
  const px = Math.round(box.x + box.w * 0.85);
  const py = Math.round(box.y + box.h * 0.25);
  console.log(`  menu open: total=${menu.total} visible=${menu.visible}, pointer=(${px},${py})`);

  // ---- Test 1: one REAL notch (Chrome/Safari/Edge: DOM_DELTA_PIXEL ~100px)
  let before = (await scrollState()).first;
  await wheelAt(page, px, py, 100);
  await page.waitForTimeout(200);
  let after = (await scrollState()).first;
  let moved = after - before;
  console.log(`  Test 1: one 100px notch -> ${moved} rows`);
  if (moved < 1) {
    fail(`one real wheel notch scrolled ${moved} rows — the wheel is dead in the browser`);
  }
  if (moved > MAX_ROWS_PER_FRAME) {
    fail(`one real wheel notch scrolled ${moved} rows (max ${MAX_ROWS_PER_FRAME}). ` +
         `The raw DOM deltaY is reaching MenuScroll unnormalized — this is the bug ` +
         `where a single notch runs through the entire list.`);
  }
  console.log("  PASS");

  // ---- Test 2: Firefox's notch (DOM_DELTA_LINE, deltaY 3) must feel the same
  before = (await scrollState()).first;
  await wheelWithDeltaMode(px, py, 3, 1);
  await page.waitForTimeout(200);
  after = (await scrollState()).first;
  const movedLine = after - before;
  console.log(`  Test 2: one DOM_DELTA_LINE notch (deltaY 3) -> ${movedLine} rows`);
  if (movedLine !== moved) {
    fail(`a Firefox line-mode notch scrolled ${movedLine} rows but a pixel-mode notch ` +
         `scrolled ${moved}. deltaMode is being ignored, so one browser's notch is ` +
         `~33x the other's.`);
  }
  console.log("  PASS");

  // ---- Test 3: a violent flick must glide, not teleport to the end
  before = (await scrollState()).first;
  await wheelAt(page, px, py, 3000);
  await page.waitForTimeout(50); // read within a frame or two of the event
  after = (await scrollState()).first;
  const flick = after - before;
  console.log(`  Test 3: 3000px flick -> ${flick} rows in ~1 frame (list total ${menu.total})`);
  if (flick >= menu.total - menu.visible) {
    fail(`a single flick jumped ${flick} rows, reaching the end of a ${menu.total}-row list. ` +
         `Per-frame emission must be capped so the list glides.`);
  }
  console.log("  PASS");

  // Ebiten keeps only a frame's last event, dropping motion during fast scrolls.
  const accum = await page.evaluate(() => {
    const el = document.querySelector("canvas") || document.body;
    const mk = (dy) => new WheelEvent("wheel", { deltaY: dy, deltaMode: 0, bubbles: true });
    // Three events with no frame boundary between them.
    el.dispatchEvent(mk(10));
    el.dispatchEvent(mk(10));
    el.dispatchEvent(mk(10));
    return new Promise((res) =>
      requestAnimationFrame(() => res({ y: globalThis.__beatmoWheel.y, mode: globalThis.__beatmoWheel.mode })),
    );
  });
  console.log(`  Test 4: three 10px events in one frame -> shim reports y=${accum.y} mode=${accum.mode}`);
  if (Math.abs(accum.y) < 30) {
    fail(`shim reported y=${accum.y} for three 10px events; events within a frame must ` +
         `ACCUMULATE (Ebiten's backend assigns instead of adding, dropping motion)`);
  }
  console.log("  PASS");

  console.log("wheel_dynamics: PASS - browser wheel notches match native detents");
} catch (error) {
  console.error("wheel_dynamics: FAIL -", error.message);
  process.exitCode = 1;
} finally {
  if (isCoverageEnabled()) {
    await flushCoverage(page, new URL("../../coverage/browser-raw", import.meta.url).pathname, "wheel_dynamics");
  }
  if (cleanup) await cleanup();
}
