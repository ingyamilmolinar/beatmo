import { chromium, devices } from "playwright";
import http from "http";
import fs from "fs";
import path from "path";
import { spawnSync } from "child_process";
import { fileURLToPath } from "url";
import { resolveGoBinary, assertSimpleDrawMode, shouldSkipWasmBuild, flushCoverage, isCoverageEnabled } from "./browser_test_helpers.js";
import { cdpTap } from "./touch_cdp_helpers.js";

// The instrument-menu search field: BPM is a value wheel, so text fields are the only native-keyboard path.
const TARGET_ID = "inst-search";

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
  // In the parallel batch this context is not the OS-foreground window, so a fresh input's focus() did not stick and a
  // spurious blur tore it down within a frame; focus emulation makes hasFocus() true. A harness-only fix.
  const focusCdp = await context.newCDPSession(page);
  await focusCdp.send("Emulation.setFocusEmulationEnabled", { enabled: true });
  await page.goto(`http://localhost:${port}/`);
  await page.waitForFunction(() =>
    typeof openInstMenu === "function" &&
    typeof instMenuSearchRect === "function" &&
    typeof instMenuSearchState === "function" &&
    typeof forceDraw === "function" &&
    typeof _mobileInputRegister === "function" &&
    typeof _mobileInputActive === "function"
  );
  await assertSimpleDrawMode(page, false, "mobile native input");
  await page.evaluate(() => forceDraw?.());
  await page.waitForTimeout(200);
  await openSearchTarget(page);
  return { context, page };
}

// Until its search field is laid out.
async function openSearchTarget(page) {
  await page.evaluate(() => openInstMenu(0));
  await page.waitForFunction(() => { const r = instMenuSearchRect(); return r && r.w > 0; },
    null, { timeout: 5000, polling: 50 });
}

// targetRect returns the search field's rect.
async function targetRect(page) {
  return await page.evaluate(() => instMenuSearchRect?.());
}

// z-index:10000.
async function nativeInputPresent(page) {
  return await page.evaluate(() => {
    const inputs = document.querySelectorAll('input[style*="z-index"]');
    for (const inp of inputs) {
      if (inp.style.zIndex === '10000') return true;
    }
    return false;
  });
}

// One function, so the detection condition cannot drift between the helper and the inline loops.
function nativeInputPoll() {
  const inputs = document.querySelectorAll('input[style*="z-index"]');
  for (const inp of inputs) {
    if (inp.style.zIndex === '10000') return true;
  }
  return false;
}

// The flake is DETECTION: throttled rAF starves a rAF-polled waitForFunction, so it polls every 50 ms instead. Creation
// runs on the throttled WASM loop and can take > 600 ms in the batch, hence 1500 ms per attempt; no forceGameTick,
// which can stall the run loop.
const DETECT_TIMEOUT_MS = 1500;
async function tapAwaitInput(page, rect) {
  const cx = rect.x + rect.w / 2;
  const cy = rect.y + rect.h / 2;
  for (let attempt = 0; attempt < 12; attempt++) {
    await cdpTap(page, cx, cy);
    try {
      await page.waitForFunction(nativeInputPoll, null, { timeout: DETECT_TIMEOUT_MS, polling: 50 });
      return true;
    } catch (_) {
      // Not detected yet — let the natural loop tick, then re-tap.
      await page.waitForTimeout(120);
    }
  }
  return false;
}

// Test 1: Tap the search field → native <input> element appears
console.log("Test 1: Tap search field → native input appears");
{
  const { context, page } = await setupMobilePage();
  try {
    const rect = await targetRect(page);
    assert(rect && rect.w > 0, "instMenuSearchRect returned empty");

    // Tap the search field → native input element should appear in the DOM.
    const inputExists = await tapAwaitInput(page, rect);
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
    const rect = await targetRect(page);
    assert(rect && rect.w > 0, "instMenuSearchRect returned empty");

    assert(await tapAwaitInput(page, rect), "native input did not appear after tap");

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

// Test 3: typing into the native input reaches Go — the search query mirrors it
console.log("Test 3: Type into native input → search query updated");
{
  const { context, page } = await setupMobilePage();
  try {
    const rect = await targetRect(page);
    assert(rect && rect.w > 0, "instMenuSearchRect returned empty");

    assert(await tapAwaitInput(page, rect), "native input did not appear after tap");

    await page.keyboard.press("Control+a");
    await page.keyboard.type("kick");

    // A fixed interval, not rAF, which the throttled loop starves.
    await page.waitForFunction(() => instMenuSearchState?.()?.text === "kick", null,
      { timeout: 4000, polling: 50 });

    const text = await page.evaluate(() => instMenuSearchState?.()?.text);
    assert(text === "kick", `Expected search text "kick", got ${JSON.stringify(text)}`);

    console.log("  PASS");
  } catch (e) {
    allPassed = false;
    console.log(`  FAIL: ${e.message}`);
  } finally {
    await context.close();
  }
}

// Test 4: a text field's native input has inputmode=text
console.log("Test 4: Search native input has inputmode=text");
{
  const { context, page } = await setupMobilePage();
  try {
    const rect = await targetRect(page);
    assert(rect && rect.w > 0, "instMenuSearchRect returned empty");

    assert(await tapAwaitInput(page, rect), "native input did not appear after tap");

    const mode = await page.evaluate(() => {
      const active = document.activeElement;
      return active ? active.getAttribute('inputmode') : null;
    });
    assert(mode === "text", `Expected inputmode=text, got ${mode}`);

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
      typeof openInstMenu === "function" &&
      typeof instMenuSearchRect === "function" &&
      typeof forceDraw === "function"
    );
    await assertSimpleDrawMode(page, false, "desktop native input");
    await page.evaluate(() => forceDraw?.());
    await page.waitForTimeout(200);
    await openSearchTarget(page);

    const rect = await targetRect(page);
    assert(rect && rect.w > 0, "instMenuSearchRect empty");

    // Click the search field with the mouse (desktop — not touch)
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

// Test 6: Drag across the search rect → does NOT trigger native input (movement threshold)
console.log("Test 6: Drag across rect does not trigger native input");
{
  const { context, page } = await setupMobilePage();
  try {
    const rect = await targetRect(page);
    assert(rect && rect.w > 0, "instMenuSearchRect returned empty");

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
    const rect = await targetRect(page);
    assert(rect && rect.w > 0, "instMenuSearchRect returned empty");

    // Verify input exists (retry-aware)
    assert(await tapAwaitInput(page, rect), "Expected native input before Escape");

    // Press Escape
    await page.keyboard.press("Escape");
    await page.waitForFunction(() => {
      const inputs = document.querySelectorAll('input[style*="z-index"]');
      for (const inp of inputs) {
        if (inp.style.zIndex === '10000') return false;
      }
      return true;
    }, null, { timeout: 2000, polling: 50 });

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
    // Before tapping, mobileInputActive(TARGET_ID) should be false
    const activeBefore = await page.evaluate((id) => mobileInputActive?.(id), TARGET_ID);
    assert(activeBefore === false, `Expected mobileInputActive('${TARGET_ID}')=false before tap, got ${activeBefore}`);

    // Tap the search field to create the native input
    const rect = await targetRect(page);
    assert(rect && rect.w > 0, "instMenuSearchRect returned empty");
    assert(await tapAwaitInput(page, rect), "native input did not appear after tap");

    // Now mobileInputActive should be true
    const activeAfter = await page.evaluate((id) => _mobileInputActive?.(id), TARGET_ID);
    assert(activeAfter === true, `Expected _mobileInputActive('${TARGET_ID}')=true after tap, got ${activeAfter}`);

    console.log("  PASS");
  } catch (e) {
    allPassed = false;
    console.log(`  FAIL: ${e.message}`);
  } finally {
    if (isCoverageEnabled()) await flushCoverage(page, new URL("../../coverage/browser-raw", import.meta.url).pathname, "mobile_native_input");
    await context.close();
  }
}

// Go accepts a tap in a SQUARE envelope (each axis ≤ 10 px), so the JS gesture handler must too: an (8,8) jitter is a
// tap in Go, and a circular test turned it into "drag" with no keyboard. CDP taps have zero movement.
console.log("Test 10: Finger-jitter tap (within Go's tap envelope) still creates native input");
{
  const { context, page } = await setupMobilePage();
  try {
    const rect = await targetRect(page);
    assert(rect && rect.w > 0, "instMenuSearchRect returned empty");

    const cx = Math.round(rect.x + rect.w / 2);
    const cy = Math.round(rect.y + rect.h / 2);
    // (8,8): both axes <= 10 (a tap for Go) but Euclidean distance ~11.3 > 10.
    const jx = cx + 8;
    const jy = cy + 8;

    const cdp = await page.context().newCDPSession(page);
    // The clean CDP touch shape cdpTap uses: an id and an EMPTY touchEnd (a moved endpoint leaves a dangling point that
    // later steals focus), the (8,8) jitter delivered by touchMove, and a ~150 ms hold for the throttled loop.
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
        await page.waitForFunction(nativeInputPoll, null, { timeout: DETECT_TIMEOUT_MS, polling: 50 });
        inputExists = true;
      } catch (_) {
        await page.waitForTimeout(120);
      }
    }
    assert(inputExists, "Expected native input after a realistic (8,8) finger-jitter tap " +
      "(Go treats it as a tap; the JS native-input handler must too)");

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
