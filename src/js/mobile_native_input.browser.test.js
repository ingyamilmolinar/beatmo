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
  ["build", "-o", path.join(jsDir, "main.wasm"), "./cmd/..."],
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

// Test 1: Tap BPM box → native <input> element appears
console.log("Test 1: Tap BPM box → native input appears");
{
  const { context, page } = await setupMobilePage();
  try {
    const rect = await page.evaluate(() => bpmBoxRect?.());
    assert(rect && rect.w > 0, "bpmBoxRect returned empty");

    // Tap the BPM box
    const cx = rect.x + rect.w / 2;
    const cy = rect.y + rect.h / 2;
    await cdpTap(page, cx, cy);
    await page.waitForTimeout(300);

    // Check for native input element in DOM (created by mobile native input system)
    const inputExists = await page.evaluate(() => {
      const inputs = document.querySelectorAll('input[style*="z-index"]');
      for (const inp of inputs) {
        if (inp.style.zIndex === '10000') return true;
      }
      return false;
    });
    assert(inputExists, "Expected native input element with z-index:10000 after tap");

    console.log("  PASS");
  } catch (e) {
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

    await cdpTap(page, rect.x + rect.w / 2, rect.y + rect.h / 2);
    await page.waitForTimeout(300);

    const isFocused = await page.evaluate(() => {
      const active = document.activeElement;
      return active && active.tagName === 'INPUT' && active.style.zIndex === '10000';
    });
    assert(isFocused, "Expected native input to be document.activeElement");

    console.log("  PASS");
  } catch (e) {
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

    await cdpTap(page, rect.x + rect.w / 2, rect.y + rect.h / 2);
    await page.waitForTimeout(300);

    // Clear the native input and type new BPM
    await page.keyboard.press("Control+a");
    await page.keyboard.type("140");
    await page.keyboard.press("Enter");
    await page.waitForTimeout(300);

    // Let frames process the result
    for (let i = 0; i < 10; i++) {
      await page.evaluate(() => forceDraw?.());
      await page.waitForTimeout(50);
    }

    const bpm = await page.evaluate(() => getBPM?.());
    assert(bpm === 140, `Expected BPM=140, got ${bpm}`);

    console.log("  PASS");
  } catch (e) {
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

    await cdpTap(page, rect.x + rect.w / 2, rect.y + rect.h / 2);
    await page.waitForTimeout(300);

    const mode = await page.evaluate(() => {
      const active = document.activeElement;
      return active ? active.getAttribute('inputmode') : null;
    });
    assert(mode === "numeric", `Expected inputmode=numeric, got ${mode}`);

    console.log("  PASS");
  } catch (e) {
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

    await cdpTap(page, rect.x + rect.w / 2, rect.y + rect.h / 2);
    await page.waitForTimeout(300);

    // Verify input exists
    let inputExists = await page.evaluate(() => {
      const inputs = document.querySelectorAll('input[style*="z-index"]');
      for (const inp of inputs) {
        if (inp.style.zIndex === '10000') return true;
      }
      return false;
    });
    assert(inputExists, "Expected native input before Escape");

    // Press Escape
    await page.keyboard.press("Escape");
    await page.waitForTimeout(200);

    // Input should be removed
    inputExists = await page.evaluate(() => {
      const inputs = document.querySelectorAll('input[style*="z-index"]');
      for (const inp of inputs) {
        if (inp.style.zIndex === '10000') return true;
      }
      return false;
    });
    assert(!inputExists, "Expected native input removed after Escape");

    console.log("  PASS");
  } catch (e) {
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
    await cdpTap(page, rect.x + rect.w / 2, rect.y + rect.h / 2);
    await page.waitForTimeout(300);

    // Now mobileInputActive should be true
    const activeAfter = await page.evaluate(() => _mobileInputActive?.("bpm"));
    assert(activeAfter === true, `Expected _mobileInputActive('bpm')=true after tap, got ${activeAfter}`);

    console.log("  PASS");
  } catch (e) {
    console.log(`  FAIL: ${e.message}`);
  } finally {
    if (isCoverageEnabled()) await flushCoverage(page, new URL("../../coverage/browser-raw", import.meta.url).pathname, "mobile_native_input");
    await context.close();
  }
}

await browser.close();
server.close();

if (!allPassed) {
  process.exit(1);
}
console.log("\nAll mobile native input tests passed!");
