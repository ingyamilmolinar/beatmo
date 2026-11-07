/** Real browser keydowns must reach Go's key handlers at all: the Go tests inject through isKeyPressed and never see
 * the Ebiten JS path or canvas focus. COVERED-BY-GO: menu_keynav_test.go, menu_keynav_integration_test.go,
 * menu_keynav_discipline_test.go */
import { setupFullWasm } from "./real_input_test_helpers.js";
import { flushCoverage, isCoverageEnabled } from "./coverage_helpers.js";

let cleanup;
let page;

const NO_SELECTION = -1;

async function navState(menu) {
  return await page.evaluate((m) => menuKeyNavState(m), menu);
}

/** Polls for the frame that CONSUMED the key: a fixed sleep read state before a slow frame processed it. Presses
 * exactly once (a retry would advance twice); on timeout returns quietly so the caller's assertion speaks. */
async function pressKey(key, until) {
  await page.keyboard.press(key);
  const deadline = Date.now() + 5000;
  while (Date.now() < deadline) {
    await page.waitForTimeout(20);
    if (await until()) return;
  }
}

/** Press key and wait until navState(menu).selected moves off `from`. */
async function pressKeyUntilSelectionLeaves(key, menu, from) {
  await pressKey(key, async () => {
    const st = await navState(menu);
    return st !== null && st.selected !== from;
  });
}

function fail(msg) {
  throw new Error(msg);
}

try {
  console.log("menu_keynav: Setting up full WASM environment...");
  ({ page, cleanup } = await setupFullWasm());

  // ---- Templates menu: headers + search + scrolling, the hardest case ----
  await page.evaluate(() => runScene("template_menu_open"));
  await page.waitForTimeout(400);

  let st = await navState("overflow");
  if (!st) fail("template menu did not open (menuKeyNavState returned null)");
  console.log(`  template menu open: ${st.count} rows, selected=${st.selected}`);
  if (st.selected !== NO_SELECTION) {
    fail(`menu opened with row ${st.selected} highlighted; nothing should be ` +
         `highlighted until the user acts`);
  }

  // ---- Test 1: a real ArrowDown highlights a row ----
  await pressKeyUntilSelectionLeaves("ArrowDown", "overflow", NO_SELECTION);
  st = await navState("overflow");
  console.log(`  Test 1: ArrowDown -> selected=${st.selected}`);
  if (st.selected === NO_SELECTION) {
    fail("a real browser ArrowDown did not move the highlight — the key event " +
         "never reached the Go handler (canvas focus / Ebiten key path)");
  }
  const first = st.selected;
  console.log("  PASS");

  // ---- Test 2: repeated ArrowDown advances, ArrowUp returns ----
  await pressKeyUntilSelectionLeaves("ArrowDown", "overflow", first);
  st = await navState("overflow");
  const second = st.selected;
  if (!(second > first)) {
    fail(`second ArrowDown did not advance the highlight (${first} -> ${second})`);
  }
  await pressKeyUntilSelectionLeaves("ArrowUp", "overflow", second);
  st = await navState("overflow");
  console.log(`  Test 2: down,down,up -> ${first},${second},${st.selected}`);
  if (st.selected !== first) {
    fail(`ArrowUp returned to ${st.selected}, want ${first}`);
  }
  console.log("  PASS");

  // ---- Test 3: arrowing past the visible page scrolls the list ----
  const scrollBefore = await page.evaluate(() => overflowMenuScrollState());
  for (let i = 0; i < 12; i++) {
    const before = (await navState("overflow")).selected;
    await pressKeyUntilSelectionLeaves("ArrowDown", "overflow", before);
  }
  const scrollAfter = await page.evaluate(() => overflowMenuScrollState());
  st = await navState("overflow");
  console.log(`  Test 3: after 12 more downs selected=${st.selected}, ` +
              `scroll first ${scrollBefore.first} -> ${scrollAfter.first}`);
  if (scrollAfter.first <= scrollBefore.first) {
    fail("arrowing past the visible page did not scroll the list — the highlight " +
         "walked off-screen and the user is steering something invisible");
  }
  if (st.selected < scrollAfter.first ||
      st.selected >= scrollAfter.first + scrollAfter.visible) {
    fail(`highlight ${st.selected} is outside the visible window ` +
         `[${scrollAfter.first},${scrollAfter.first + scrollAfter.visible})`);
  }
  console.log("  PASS");

  // ---- Test 4: Enter performs the highlighted row's action ----
  const bpmBefore = await page.evaluate(() => getBPM?.() ?? null);
  await pressKey("Enter", async () => (await navState("overflow")) === null);
  await page.waitForTimeout(600);
  const stAfter = await navState("overflow");
  const bpmAfter = await page.evaluate(() => getBPM?.() ?? null);
  console.log(`  Test 4: Enter -> menu ${stAfter === null ? "closed" : "still open"}, ` +
              `bpm ${bpmBefore} -> ${bpmAfter}`);
  if (stAfter !== null) {
    fail("Enter on a highlighted template row left the menu open — the row's " +
         "action (import + close) did not run");
  }
  console.log("  PASS");

  // ---- Test 5: a plain list menu (subdiv) navigates the same way ----
  await page.evaluate(() => runScene("subdiv_menu_open"));
  await page.waitForTimeout(400);
  let sub = await navState("subdiv");
  if (sub) {
    if (sub.selected !== NO_SELECTION) {
      fail(`subdiv menu opened with row ${sub.selected} highlighted`);
    }
    await pressKeyUntilSelectionLeaves("ArrowDown", "subdiv", NO_SELECTION);
    sub = await navState("subdiv");
    console.log(`  Test 5: subdiv ArrowDown -> selected=${sub.selected} of ${sub.count}`);
    if (sub.selected === NO_SELECTION) {
      fail("ArrowDown did not move the subdiv menu highlight");
    }
    console.log("  PASS");
  } else {
    console.log("  Test 5: subdiv scene unavailable, skipped");
  }

  console.log("menu_keynav: PASS - real browser keys drive every list menu");
} catch (error) {
  console.error("menu_keynav: FAIL -", error.message);
  process.exitCode = 1;
} finally {
  if (isCoverageEnabled()) {
    await flushCoverage(page, new URL("../../coverage/browser-raw", import.meta.url).pathname, "menu_keynav");
  }
  if (cleanup) await cleanup();
}
