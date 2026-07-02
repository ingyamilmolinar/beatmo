import { chromium, devices } from "playwright";
import http from "http";
import fs from "fs";
import path from "path";
import { spawnSync } from "child_process";
import { fileURLToPath } from "url";
import { resolveGoBinary, assertSimpleDrawMode, shouldSkipWasmBuild, flushCoverage, isCoverageEnabled } from "./browser_test_helpers.js";
import { cdpTap } from "./touch_cdp_helpers.js";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const jsDir = __dirname;
const repoRoot = path.resolve(jsDir, "..", "..");
const goDir = path.resolve(repoRoot, "src/go");
const GO = resolveGoBinary();

// Build WASM
if (!shouldSkipWasmBuild("main.wasm")) {
const build = spawnSync(
  GO,
  ["build", "-o", path.join(jsDir, "main.wasm"), "./cmd"],
  { cwd: goDir, env: { ...process.env, GOOS: "js", GOARCH: "wasm" }, stdio: "inherit" }
);
if (build.status !== 0) throw new Error("go build main wasm failed");
}

// Static server
const server = http.createServer((req, res) => {
  const file = req.url === "/" ? "/index.html" : req.url;
  const filePath = path.join(jsDir, file.replace(/^\//, ""));
  fs.readFile(filePath, (err, data) => {
    if (err) { res.writeHead(404); res.end(); return; }
    let ct = "text/plain";
    if (filePath.endsWith(".html")) ct = "text/html";
    else if (filePath.endsWith(".js")) ct = "application/javascript";
    else if (filePath.endsWith(".wasm")) ct = "application/wasm";
    res.writeHead(200, { "Content-Type": ct });
    res.end(data);
  });
});
await new Promise((r) => server.listen(0, r));
const port = server.address().port;

const iPhone = devices["iPhone 12"];
const browser = await chromium.launch({ args: ["--autoplay-policy=no-user-gesture-required"] });

let allPassed = true;
function assert(cond, msg) {
  if (!cond) { allPassed = false; throw new Error(msg); }
}

async function setupMobilePage() {
  const context = await browser.newContext({ ...iPhone });
  const page = await context.newPage();
  // Force the page to be treated as focused. In the 4-job parallel batch this
  // context is usually NOT the OS-foreground window, so a freshly-created native
  // <input>'s .focus() does not stick — a spurious `blur` fires within ~1 frame,
  // our blur handler commits + tears the <input> down, and the detection poll
  // (even at 50 ms) never sees it. createCount==blurCount on every retry proved
  // the input is created then instantly blurred. setFocusEmulationEnabled makes
  // document.hasFocus()==true so focus() holds and the spurious blur never fires.
  // (Pure test-harness fix — production runs in a real focused tab.)
  const focusCdp = await context.newCDPSession(page);
  await focusCdp.send("Emulation.setFocusEmulationEnabled", { enabled: true });
  await page.goto(`http://localhost:${port}/`);
  await page.waitForFunction(() =>
    typeof bpmBoxRect === "function" &&
    typeof forceDraw === "function" &&
    typeof getBPM === "function" &&
    typeof rowLabelText === "function" &&
    typeof _mobileInputRegister === "function" &&
    typeof _mobileInputActive === "function"
  );
  await assertSimpleDrawMode(page, false, "mobile native input");
  await page.evaluate(() => forceDraw?.());
  await page.waitForTimeout(200);
  return { context, page };
}

// nativeInputPresent reports whether the mobile native <input> (z-index:10000)
// is currently in the DOM.
async function nativeInputPresent(page) {
  return await page.evaluate(() => {
    const inputs = document.querySelectorAll('input[style*="z-index"]');
    for (const inp of inputs) {
      if (inp.style.zIndex === '10000') return true;
    }
    return false;
  });
}

// nativeInputPoll is the page-function shared by every "did the mobile native
// <input> (z-index:10000) appear?" poll. Kept as a single function so the
// detection condition can't drift between the helper and the inline loops.
function nativeInputPoll() {
  const inputs = document.querySelectorAll('input[style*="z-index"]');
  for (const inp of inputs) {
    if (inp.style.zIndex === '10000') return true;
  }
  return false;
}

// tapBPMAwaitInput taps the BPM box and waits for the native input to appear,
// re-tapping a few times if needed. The "bpm" rect is (re)registered every
// Update() frame (recalcButtons → mobileInputRegister) and stays registered, so a
// tap that lands creates the input synchronously inside the touchend gesture.
//
// The headless flake is a DETECTION race, not a creation race: under the 4-job
// parallel batch software-GL throttles the rAF-driven game loop, and
// page.waitForFunction defaults to `polling: 'raf'` — so that same throttled rAF
// starves the detection poll, and the input can appear yet the poll never
// evaluates within the timeout (the later, more-degraded sub-tests 7/9/10 lost
// this race while the early ones won it). We poll on a fixed 50 ms interval
// instead, which fires independently of the page's rAF, and allow a few extra
// re-tap attempts for margin. We deliberately do NOT force manual Update ticks
// (forceGameTick), which can stall the WASM run loop. Returns true if the input
// appeared. Mirrors the retry discipline used for other headless tap tests.
//
// Per-attempt budget = DETECT_TIMEOUT_MS. The detection poll itself is timer-
// driven (immune to rAF starvation), but input *creation* runs on the WASM game
// loop: under the 4-job parallel batch the loop is throttled hard enough that
// the touchend->gesture->mobileInputRegister pipeline can take >600 ms to emit
// the DOM <input>. 600 ms left no margin for that creation latency on the later,
// most-degraded sub-tests, so we widen the per-attempt window to 1500 ms while
// keeping the 12 re-tap attempts (cost is paid only on the failing path).
const DETECT_TIMEOUT_MS = 1500;
async function tapBPMAwaitInput(page, rect) {
  const cx = rect.x + rect.w / 2;
  const cy = rect.y + rect.h / 2;
  for (let attempt = 0; attempt < 12; attempt++) {
    await cdpTap(page, cx, cy);
    try {
      await page.waitForFunction(nativeInputPoll, { timeout: DETECT_TIMEOUT_MS, polling: 50 });
      return true;
    } catch (_) {
      // Not detected yet — let the natural loop tick, then re-tap.
      await page.waitForTimeout(120);
    }
  }
  return false;
}

// Test 1: Tap BPM box → native <input> element appears
console.log("Test 1: Tap BPM box → native input appears");
{
  const { context, page } = await setupMobilePage();
  try {
    const rect = await page.evaluate(() => bpmBoxRect?.());
    assert(rect && rect.w > 0, "bpmBoxRect returned empty");

    // Tap the BPM box → native input element should appear in the DOM.
    const inputExists = await tapBPMAwaitInput(page, rect);
    assert(inputExists, "Expected native input element with z-index:10000 after tap");

    console.log("  PASS");
  } catch (e) {
    allPassed = false;
    console.log(`  FAIL: ${e.message}`);
  } finally {
    await context.close();
  }
}

// Test 2: Native input is auto-focused
console.log("Test 2: Native input is auto-focused");
{
  const { context, page } = await setupMobilePage();
  try {
    const rect = await page.evaluate(() => bpmBoxRect?.());
    assert(rect && rect.w > 0, "bpmBoxRect returned empty");

    assert(await tapBPMAwaitInput(page, rect), "native input did not appear after tap");

    const isFocused = await page.evaluate(() => {
      const active = document.activeElement;
      return active && active.tagName === 'INPUT' && active.style.zIndex === '10000';
    });
    assert(isFocused, "Expected native input to be document.activeElement");

    console.log("  PASS");
  } catch (e) {
    allPassed = false;
    console.log(`  FAIL: ${e.message}`);
  } finally {
    await context.close();
  }
}

// Test 3: BPM native input — type + Enter → BPM updated
console.log("Test 3: Type BPM + Enter → BPM updated");
{
  const { context, page } = await setupMobilePage();
  try {
    const rect = await page.evaluate(() => bpmBoxRect?.());
    assert(rect && rect.w > 0, "bpmBoxRect returned empty");

    assert(await tapBPMAwaitInput(page, rect), "native input did not appear after tap");

    // Clear the native input and type new BPM
    await page.keyboard.press("Control+a");
    await page.keyboard.type("140");
    await page.keyboard.press("Enter");

    // Enter commits the JS native input synchronously, but Go applies the result
    // on its next Update poll (mobileInputPollResult → SetBPM). Poll getBPM on a
    // fixed interval (rather than the default rAF, which the throttled game loop
    // starves) until the natural Update loop has processed it.
    await page.waitForFunction(() => typeof getBPM === "function" && getBPM() === 140,
      { timeout: 4000, polling: 50 });

    const bpm = await page.evaluate(() => getBPM?.());
    assert(bpm === 140, `Expected BPM=140, got ${bpm}`);

    console.log("  PASS");
  } catch (e) {
    allPassed = false;
    console.log(`  FAIL: ${e.message}`);
  } finally {
    await context.close();
  }
}

// Test 4: BPM native input has inputmode=numeric
console.log("Test 4: BPM native input has inputmode=numeric");
{
  const { context, page } = await setupMobilePage();
  try {
    const rect = await page.evaluate(() => bpmBoxRect?.());
    assert(rect && rect.w > 0, "bpmBoxRect returned empty");

    assert(await tapBPMAwaitInput(page, rect), "native input did not appear after tap");

    const mode = await page.evaluate(() => {
      const active = document.activeElement;
      return active ? active.getAttribute('inputmode') : null;
    });
    assert(mode === "numeric", `Expected inputmode=numeric, got ${mode}`);

    console.log("  PASS");
  } catch (e) {
    allPassed = false;
    console.log(`  FAIL: ${e.message}`);
  } finally {
    await context.close();
  }
}

// Test 5: Desktop viewport → native inputs NOT shown
console.log("Test 5: Desktop viewport → no native inputs");
{
  const context = await browser.newContext({ viewport: { width: 1280, height: 720 } });
  const page = await context.newPage();
  try {
    await page.goto(`http://localhost:${port}/`);
    await page.waitForFunction(() =>
      typeof bpmBoxRect === "function" &&
      typeof forceDraw === "function"
    );
    await assertSimpleDrawMode(page, false, "desktop native input");
    await page.evaluate(() => forceDraw?.());
    await page.waitForTimeout(200);

    const rect = await page.evaluate(() => bpmBoxRect?.());
    assert(rect && rect.w > 0, "bpmBoxRect empty");

    // Click BPM box with mouse (desktop — not touch)
    await page.mouse.click(rect.x + rect.w / 2, rect.y + rect.h / 2);
    await page.waitForTimeout(200);
    await page.evaluate(() => forceDraw?.());

    // No native input should appear
    const inputExists = await page.evaluate(() => {
      const inputs = document.querySelectorAll('input[style*="z-index"]');
      for (const inp of inputs) {
        if (inp.style.zIndex === '10000') return true;
      }
      return false;
    });
    assert(!inputExists, "Expected NO native input on desktop");

    console.log("  PASS");
  } catch (e) {
    allPassed = false;
    console.log(`  FAIL: ${e.message}`);
  } finally {
    await context.close();
  }
}

// Test 6: Drag across BPM rect → does NOT trigger native input (movement threshold)
console.log("Test 6: Drag across rect does not trigger native input");
{
  const { context, page } = await setupMobilePage();
  try {
    const rect = await page.evaluate(() => bpmBoxRect?.());
    assert(rect && rect.w > 0, "bpmBoxRect returned empty");

    // Simulate a drag (touchstart at one point, touchend far away)
    const startX = rect.x + 2;
    const startY = rect.y + rect.h / 2;
    const endX = startX + 50; // > 10px threshold
    const endY = startY;

    const cdp = await page.context().newCDPSession(page);
    await cdp.send("Input.dispatchTouchEvent", {
      type: "touchStart",
      touchPoints: [{ x: startX, y: startY }],
    });
    await page.waitForTimeout(50);
    await cdp.send("Input.dispatchTouchEvent", {
      type: "touchMove",
      touchPoints: [{ x: endX, y: endY }],
    });
    await page.waitForTimeout(50);
    await cdp.send("Input.dispatchTouchEvent", {
      type: "touchEnd",
      touchPoints: [],
    });
    await page.waitForTimeout(300);

    // No native input should appear (drag, not tap)
    const inputExists = await page.evaluate(() => {
      const inputs = document.querySelectorAll('input[style*="z-index"]');
      for (const inp of inputs) {
        if (inp.style.zIndex === '10000') return true;
      }
      return false;
    });
    assert(!inputExists, "Expected no native input after drag (movement > threshold)");

    console.log("  PASS");
  } catch (e) {
    allPassed = false;
    console.log(`  FAIL: ${e.message}`);
  } finally {
    await context.close();
  }
}

// Test 7: Escape key cancels native input
console.log("Test 7: Escape cancels native input");
{
  const { context, page } = await setupMobilePage();
  try {
    const rect = await page.evaluate(() => bpmBoxRect?.());
    assert(rect && rect.w > 0, "bpmBoxRect returned empty");

    // Verify input exists (retry-aware)
    assert(await tapBPMAwaitInput(page, rect), "Expected native input before Escape");

    // Press Escape
    await page.keyboard.press("Escape");
    await page.waitForFunction(() => {
      const inputs = document.querySelectorAll('input[style*="z-index"]');
      for (const inp of inputs) {
        if (inp.style.zIndex === '10000') return false;
      }
      return true;
    }, { timeout: 2000, polling: 50 });

    // Input should be removed
    const inputExists = await nativeInputPresent(page);
    assert(!inputExists, "Expected native input removed after Escape");

    console.log("  PASS");
  } catch (e) {
    allPassed = false;
    console.log(`  FAIL: ${e.message}`);
  } finally {
    await context.close();
  }
}

// Test 8: JS mobile input functions exist
console.log("Test 8: JS mobile input system functions exist");
{
  const { context, page } = await setupMobilePage();
  try {
    const funcs = await page.evaluate(() => ({
      register: typeof _mobileInputRegister,
      registerTrigger: typeof _mobileInputRegisterTrigger,
      clear: typeof _mobileInputClear,
      active: typeof _mobileInputActive,
      anyActive: typeof _mobileInputAnyActive,
      pollResult: typeof _mobileInputPollResult,
      getValue: typeof _mobileInputGetValue,
      close: typeof _mobileInputClose,
      closeAll: typeof _mobileInputCloseAll,
    }));

    for (const [name, type] of Object.entries(funcs)) {
      assert(type === "function", `Expected _mobileInput${name} to be function, got ${type}`);
    }

    console.log("  PASS");
  } catch (e) {
    allPassed = false;
    console.log(`  FAIL: ${e.message}`);
  } finally {
    await context.close();
  }
}

// Test 9: Go-side mobileInputActive export works
console.log("Test 9: Go-side mobileInputActive JS export");
{
  const { context, page } = await setupMobilePage();
  try {
    // Before tapping, mobileInputActive("bpm") should be false
    const activeBefore = await page.evaluate(() => mobileInputActive?.("bpm"));
    assert(activeBefore === false, `Expected mobileInputActive('bpm')=false before tap, got ${activeBefore}`);

    // Tap BPM box to create native input
    const rect = await page.evaluate(() => bpmBoxRect?.());
    assert(rect && rect.w > 0, "bpmBoxRect returned empty");
    assert(await tapBPMAwaitInput(page, rect), "native input did not appear after tap");

    // Now mobileInputActive should be true
    const activeAfter = await page.evaluate(() => _mobileInputActive?.("bpm"));
    assert(activeAfter === true, `Expected _mobileInputActive('bpm')=true after tap, got ${activeAfter}`);

    console.log("  PASS");
  } catch (e) {
    allPassed = false;
    console.log(`  FAIL: ${e.message}`);
  } finally {
    if (isCoverageEnabled()) await flushCoverage(page, new URL("../../coverage/browser-raw", import.meta.url).pathname, "mobile_native_input");
    await context.close();
  }
}

// Test 10: Realistic finger-jitter tap still creates the native input.
//
// Regression: real iOS Safari taps carry a few px of jitter between touchstart
// and touchend. Go's tap detector (gesture.go) accepts a tap when BOTH axes move
// <= tapMaxMovePx (10px) — a *square* envelope (~14px diagonally). The JS native-
// input gesture handler rejected the gesture with a *circular* Euclidean test
// (dx*dx+dy*dy > 100, i.e. distance > 10px). A (8,8) jitter (distance ~11.3px)
// therefore opened the in-canvas editor (Go: tap) but created NO native input
// (JS: "drag") → the soft keyboard never appeared. Playwright/CDP taps have zero
// movement, so this gap was invisible until now. The two detectors must agree.
console.log("Test 10: Finger-jitter tap (within Go's tap envelope) still creates native input");
{
  const { context, page } = await setupMobilePage();
  try {
    const rect = await page.evaluate(() => bpmBoxRect?.());
    assert(rect && rect.w > 0, "bpmBoxRect returned empty");

    const cx = Math.round(rect.x + rect.w / 2);
    const cy = Math.round(rect.y + rect.h / 2);
    // (8,8): both axes <= 10 (a tap for Go) but Euclidean distance ~11.3 > 10.
    const jx = cx + 8;
    const jy = cy + 8;

    const cdp = await page.context().newCDPSession(page);
    // Retry like tapBPMAwaitInput, with the same fixed-interval detection poll so a
    // throttled rAF can't starve it. The "bpm" rect is (re)registered every Update
    // frame and stays registered; re-dispatching the jitter tap covers the case
    // where the first attempt raced an Update that had not yet run.
    //
    // The jitter MUST be delivered with the same clean CDP touch shape that
    // cdpTap (which powers the 8 passing gesture tests) uses, or this test alone
    // becomes flaky under the 4-job parallel software-GL batch. Two requirements:
    //   1. Carry a touch `id` and RELEASE with an EMPTY touchEnd. Ending a touch
    //      with a non-empty, moved touchpoint leaves a dangling active point in
    //      Chromium's input pipeline; that stray point later cancels and steals
    //      focus from the freshly-focused <input>, blurring it away within ~1
    //      frame (before the detection poll catches it). The canonical
    //      touchStart(id) -> touchMove(jitter,id) -> touchEnd([]) sequence
    //      releases cleanly, so no dangling point can blur the input.
    //   2. Deliver the (8,8) jitter via a touchMove (not by moving the touchEnd
    //      point). The page's touchend handler reports the released touch at its
    //      last known position (the jitter point), so touchStartPos->end delta is
    //      still (8,8) — both axes <= 10, the square envelope Go treats as a tap.
    //   3. Hold ~150ms like cdpTap so the throttled WASM loop reliably has the
    //      "bpm" rect registered when the gesture lands.
    let inputExists = false;
    for (let attempt = 0; attempt < 12 && !inputExists; attempt++) {
      const touchId = 200 + attempt;
      await cdp.send("Input.dispatchTouchEvent", {
        type: "touchStart",
        touchPoints: [{ x: cx, y: cy, id: touchId }],
      });
      await page.waitForTimeout(80);
      // Move 8px on each axis — the realistic finger jitter under test.
      await cdp.send("Input.dispatchTouchEvent", {
        type: "touchMove",
        touchPoints: [{ x: jx, y: jy, id: touchId }],
      });
      await page.waitForTimeout(80);
      // Clean release (empty touchPoints) — no dangling point to steal focus.
      await cdp.send("Input.dispatchTouchEvent", {
        type: "touchEnd",
        touchPoints: [],
      });
      try {
        await page.waitForFunction(nativeInputPoll, { timeout: DETECT_TIMEOUT_MS, polling: 50 });
        inputExists = true;
      } catch (_) {
        await page.waitForTimeout(120);
      }
    }
    assert(inputExists, "Expected native input after a realistic (8,8) finger-jitter tap " +
      "(Go treats it as a tap and opens the editor; the JS native-input handler must too)");

    console.log("  PASS");
  } catch (e) {
    allPassed = false;
    console.log(`  FAIL: ${e.message}`);
  } finally {
    await context.close();
  }
}

await browser.close();
server.close();

if (!allPassed) {
  process.exit(1);
}
console.log("\nAll mobile native input tests passed!");
