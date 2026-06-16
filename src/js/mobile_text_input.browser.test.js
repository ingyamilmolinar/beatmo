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
    typeof kbProxyFocused === "function" &&
    typeof getBPM === "function"
  );
  await assertSimpleDrawMode(page, false, "mobile text input");
  await page.evaluate(() => forceDraw?.());
  await page.waitForTimeout(200);
  return { context, page };
}

// tapBPMAwaitInput taps the BPM box and waits for the mobile native <input>
// (z-index:10000) to appear, re-tapping a few times if needed. The "bpm" rect is
// (re)registered every Update() frame (recalcButtons), but under headless
// software-GL — especially in the parallel test batch — the rAF Update loop is
// throttled, so the first tap's touchend can fire before any Update has
// registered "bpm", and the JS gesture handler then creates no input. Re-tapping
// after a short window lets the natural loop tick and register "bpm"; once
// registered it stays registered. We deliberately do NOT force manual Update
// ticks (forceGameTick), which can stall the WASM run loop. Returns true if the
// input appeared. Mirrors the retry discipline in mobile_native_input.browser.test.js.
async function tapBPMAwaitInput(page, rect) {
  const cx = rect.x + rect.w / 2;
  const cy = rect.y + rect.h / 2;
  for (let attempt = 0; attempt < 8; attempt++) {
    await cdpTap(page, cx, cy);
    try {
      await page.waitForFunction(() => {
        const inputs = document.querySelectorAll('input[style*="z-index"]');
        for (const inp of inputs) {
          if (inp.style.zIndex === '10000') return true;
        }
        return false;
      }, { timeout: 500 });
      return true;
    } catch (_) {
      // Registration not ready yet — let the rAF loop tick, then re-tap.
      await page.waitForTimeout(150);
    }
  }
  return false;
}

// Test 1: BPM soft keyboard focus — native input appears with inputmode=numeric
console.log("Test 1: BPM soft keyboard — native input focused with inputmode=numeric");
{
  const { context, page } = await setupMobilePage();
  try {
    const rect = await page.evaluate(() => bpmBoxRect?.());
    assert(rect && rect.w > 0, "bpmBoxRect returned empty");

    // Tap the BPM box (retry-aware: native input creation races the throttled
    // rAF Update loop that registers the "bpm" rect).
    assert(await tapBPMAwaitInput(page, rect), "native input did not appear after tap");

    // On mobile, the native input system creates a real <input> overlay
    // instead of focusing the hidden proxy. Check that the native input
    // is focused and has the correct inputmode.
    const nativeInputInfo = await page.evaluate(() => {
      const active = document.activeElement;
      if (active && active.tagName === 'INPUT' && active.style.zIndex === '10000') {
        return { focused: true, inputmode: active.getAttribute('inputmode') };
      }
      // Fallback: check proxy (desktop path)
      return { focused: kbProxyFocused?.() === true, inputmode: kbProxyInputMode?.() };
    });
    assert(nativeInputInfo.focused === true, `Expected input focused, got ${nativeInputInfo.focused}`);
    assert(nativeInputInfo.inputmode === "numeric", `Expected inputmode=numeric, got ${nativeInputInfo.inputmode}`);

    console.log("  PASS");
  } catch (e) {
    console.log(`  FAIL: ${e.message}`);
  } finally {
    await context.close();
  }
}

// Test 2: Keyboard dismiss on blur
console.log("Test 2: Keyboard dismiss on blur");
{
  const { context, page } = await setupMobilePage();
  try {
    const rect = await page.evaluate(() => bpmBoxRect?.());
    assert(rect && rect.w > 0, "bpmBoxRect returned empty");

    // Focus BPM box (retry-aware native input creation).
    assert(await tapBPMAwaitInput(page, rect), "native input did not appear after tap");

    // Check that either native input or proxy is focused
    let focused = await page.evaluate(() => {
      const active = document.activeElement;
      if (active && active.tagName === 'INPUT' && active.style.zIndex === '10000') return true;
      return kbProxyFocused?.() === true;
    });
    assert(focused === true, "Expected focused after tap");

    // Tap outside the BPM box to blur (native input commits on blur)
    await cdpTap(page, 10, 10);
    await page.waitForTimeout(300);
    await page.evaluate(() => forceDraw?.());
    await page.waitForTimeout(100);

    // After blur, the native input element should be removed
    const nativeGone = await page.evaluate(() => {
      const inputs = document.querySelectorAll('input[style*="z-index"]');
      for (const inp of inputs) {
        if (inp.style.zIndex === '10000') return false;
      }
      return true;
    });
    assert(nativeGone, "Expected native input removed after blur");

    console.log("  PASS");
  } catch (e) {
    console.log(`  FAIL: ${e.message}`);
  } finally {
    await context.close();
  }
}

// Test 3: Type into BPM box via page.keyboard
console.log("Test 3: Type BPM value via keyboard");
{
  const { context, page } = await setupMobilePage();
  try {
    const rect = await page.evaluate(() => bpmBoxRect?.());
    assert(rect && rect.w > 0, "bpmBoxRect returned empty");

    // Focus BPM box
    await cdpTap(page, rect.x + rect.w / 2, rect.y + rect.h / 2);
    await page.waitForTimeout(200);
    await page.evaluate(() => forceDraw?.());

    // Type "150" into the proxy input and press Enter
    await page.keyboard.type("150");
    await page.waitForTimeout(100);
    await page.keyboard.press("Enter");
    await page.waitForTimeout(200);
    // Let a few frames process
    for (let i = 0; i < 5; i++) {
      await page.evaluate(() => forceDraw?.());
      await page.waitForTimeout(50);
    }

    const bpm = await page.evaluate(() => getBPM?.());
    assert(bpm === 150, `Expected BPM=150 after typing, got ${bpm}`);
    console.log("  PASS");
  } catch (e) {
    console.log(`  FAIL: ${e.message}`);
  } finally {
    await context.close();
  }
}

// Test 4: Backspace works in proxy
console.log("Test 4: Backspace via proxy input");
{
  const { context, page } = await setupMobilePage();
  try {
    // Focus the proxy input directly and type into it
    await page.evaluate(() => window._kbProxyFocus("numeric"));
    await page.waitForTimeout(100);

    // Type then backspace
    await page.keyboard.type("12345");
    await page.keyboard.press("Backspace");
    await page.keyboard.press("Backspace");
    await page.waitForTimeout(100);

    // Drain chars from proxy
    const chars = await page.evaluate(() => {
      const raw = window._kbProxyDrain();
      return raw ? raw.join("") : "";
    });

    // Should have: 1,2,3,4,5,\b,\b
    assert(chars.includes("1"), "Expected '1' in chars");
    assert(chars.includes("5"), "Expected '5' in chars");
    // Backspace produces '\b' (U+0008)
    const bsCount = (chars.match(/\u0008/g) || []).length; // eslint-disable-line no-control-regex
    assert(bsCount === 2, `Expected 2 backspace chars, got ${bsCount}`);

    console.log("  PASS");
  } catch (e) {
    console.log(`  FAIL: ${e.message}`);
  } finally {
    await context.close();
  }
}

// Test 5: Enter in proxy produces newline char
console.log("Test 5: Enter key via proxy");
{
  const { context, page } = await setupMobilePage();
  try {
    await page.evaluate(() => window._kbProxyFocus("text"));
    await page.waitForTimeout(100);

    await page.keyboard.type("90");
    await page.keyboard.press("Enter");
    await page.waitForTimeout(100);

    const chars = await page.evaluate(() => {
      const raw = window._kbProxyDrain();
      return raw ? raw.join("") : "";
    });

    assert(chars.includes("9"), "Expected '9'");
    assert(chars.includes("0"), "Expected '0'");
    assert(chars.includes("\n"), "Expected newline from Enter");

    console.log("  PASS");
  } catch (e) {
    console.log(`  FAIL: ${e.message}`);
  } finally {
    await context.close();
  }
}

// Test 6: Proxy input element exists and has correct attributes
console.log("Test 6: Proxy input element attributes");
{
  const { context, page } = await setupMobilePage();
  try {
    const attrs = await page.evaluate(() => {
      const el = document.getElementById("beatmo-kb-proxy");
      if (!el) return null;
      return {
        autocomplete: el.getAttribute("autocomplete"),
        autocorrect: el.getAttribute("autocorrect"),
        spellcheck: el.getAttribute("spellcheck"),
        inputmode: el.getAttribute("inputmode"),
      };
    });
    assert(attrs !== null, "beatmo-kb-proxy element not found");
    assert(attrs.autocomplete === "off", `Expected autocomplete=off, got ${attrs.autocomplete}`);
    assert(attrs.autocorrect === "off", `Expected autocorrect=off, got ${attrs.autocorrect}`);
    assert(attrs.spellcheck === "false", `Expected spellcheck=false, got ${attrs.spellcheck}`);
    assert(attrs.inputmode === "text", `Expected default inputmode=text, got ${attrs.inputmode}`);

    console.log("  PASS");
  } catch (e) {
    console.log(`  FAIL: ${e.message}`);
  } finally {
    await context.close();
  }
}

// Test 7: Desktop keyboard still works (regression)
console.log("Test 7: Desktop keyboard regression");
{
  // Use desktop viewport (no mobile emulation)
  const context = await browser.newContext({ viewport: { width: 1280, height: 720 } });
  const page = await context.newPage();
  try {
    await page.goto(`http://localhost:${port}/`);
    await page.waitForFunction(() =>
      typeof bpmBoxRect === "function" &&
      typeof forceDraw === "function" &&
      typeof getBPM === "function"
    );
    await assertSimpleDrawMode(page, false, "desktop text input");
    await page.evaluate(() => forceDraw?.());
    await page.waitForTimeout(200);

    const rect = await page.evaluate(() => bpmBoxRect?.());
    assert(rect && rect.w > 0, "bpmBoxRect empty");

    // Click BPM box with mouse
    await page.mouse.click(rect.x + rect.w / 2, rect.y + rect.h / 2);
    await page.waitForTimeout(200);
    await page.evaluate(() => forceDraw?.());

    // On desktop, the proxy should NOT be focused (isSmallScreen returns false)
    const focused = await page.evaluate(() => kbProxyFocused?.());
    assert(focused === false, `Expected proxy NOT focused on desktop, got ${focused}`);

    console.log("  PASS");
  } catch (e) {
    console.log(`  FAIL: ${e.message}`);
  } finally {
    await context.close();
  }
}

// Test 8: Text input gesture system is present and functional
console.log("Test 8: Text input gesture system present");
{
  const { context, page } = await setupMobilePage();
  try {
    // Both the focus-rect system and the mobile native input system should exist
    const hasFuncs = await page.evaluate(() =>
      typeof window._kbRegisterFocusRect === "function" &&
      typeof window._kbClearFocusRects === "function" &&
      typeof window._mobileInputRegister === "function" &&
      typeof window._mobileInputActive === "function"
    );
    assert(hasFuncs, "Expected focus-rect and mobile native input functions");

    // After WASM starts and forceDraw, tapping the BPM box area should create
    // a native input (on mobile, the native input system handles text inputs).
    const rect = await page.evaluate(() => bpmBoxRect?.());
    assert(rect && rect.w > 0, "bpmBoxRect returned empty");

    // Tap the BPM box — the native input system creates a real <input>
    // (retry-aware: creation races the throttled rAF Update loop).
    assert(await tapBPMAwaitInput(page, rect), "native input did not appear after tap");

    const nativeActive = await page.evaluate(() => {
      const active = document.activeElement;
      return active && active.tagName === 'INPUT' && active.style.zIndex === '10000';
    });
    assert(nativeActive, `Expected native input focused after cdpTap on BPM box`);

    console.log("  PASS");
  } catch (e) {
    console.log(`  FAIL: ${e.message}`);
  } finally {
    await context.close();
  }
}

// Test 9: DPR coordinate correctness — verify touch coords are CSS pixels, not DPR-scaled
console.log("Test 9: DPR coordinate correctness (DPR=3)");
{
  // Use iPhone 12 (DPR=3) to verify the touchend handler uses CSS coordinates
  const { context, page } = await setupMobilePage();
  try {
    // Verify DPR is actually > 1 in this emulation
    const dpr = await page.evaluate(() => window.devicePixelRatio);
    assert(dpr >= 2, `Expected DPR >= 2 for iPhone emulation, got ${dpr}`);

    // Get BPM box rect (registered in game coordinates = CSS pixels)
    const rect = await page.evaluate(() => bpmBoxRect?.());
    assert(rect && rect.w > 0, "bpmBoxRect returned empty");

    // Verify rect coords are in game coordinate range (not DPR-scaled).
    // Game Layout returns CSS viewport dims (390x844 for iPhone 12).
    // If rects were DPR-scaled, they'd be 3x larger (>1170).
    assert(rect.x < 500, `BPM rect x=${rect.x} looks DPR-scaled (expected < 500 for CSS coords)`);
    assert(rect.y < 900, `BPM rect y=${rect.y} looks DPR-scaled (expected < 900 for CSS coords)`);

    // Instrument the touchend handler to capture computed touch coordinates.
    await page.evaluate(() => {
      window._testLastTouchCoords = null;
      const canvas = document.querySelector('canvas');
      canvas.addEventListener('touchend', (e) => {
        if (!e.changedTouches || e.changedTouches.length === 0) return;
        const touch = e.changedTouches[0];
        const cr = canvas.getBoundingClientRect();
        window._testLastTouchCoords = {
          rawClientX: touch.clientX,
          rawClientY: touch.clientY,
          canvasLeft: cr.left,
          canvasTop: cr.top,
          cx: touch.clientX - cr.left,
          cy: touch.clientY - cr.top,
          dpr: window.devicePixelRatio,
        };
      }, { passive: true });
    });

    const tapX = rect.x + rect.w / 2;
    const tapY = rect.y + rect.h / 2;
    // Retry-aware tap: native input creation races the throttled rAF Update loop
    // that registers the "bpm" rect. The touchend listener above captures the
    // coordinates of the (final) tap, which are identical on every attempt.
    assert(await tapBPMAwaitInput(page, rect), `Expected native input after cdpTap at DPR=${dpr}`);

    // Verify the native input was created (hit test worked with CSS coords)
    const nativeActive = await page.evaluate(() => {
      const active = document.activeElement;
      return active && active.tagName === 'INPUT' && active.style.zIndex === '10000';
    });
    assert(nativeActive, `Expected native input focused after cdpTap at DPR=${dpr}`);

    // Verify the computed coords are CSS pixels (not DPR-multiplied)
    const coords = await page.evaluate(() => window._testLastTouchCoords);
    if (coords) {
      const expectedCx = tapX;
      const tolerance = 5;
      assert(
        Math.abs(coords.cx - expectedCx) < tolerance,
        `Touch cx=${coords.cx} should be ~${expectedCx} (CSS px), not ${expectedCx * dpr} (DPR-scaled)`
      );
    }

    console.log("  PASS");
  } catch (e) {
    console.log(`  FAIL: ${e.message}`);
  } finally {
    if (isCoverageEnabled()) await flushCoverage(page, new URL("../../coverage/browser-raw", import.meta.url).pathname, "mobile_text_input");
    await context.close();
  }
}

// Test: Escape via the keyboard proxy forwards to Go and dismisses the keyboard.
// Regression for the browser bug where Esc did nothing on text inputs: the
// hidden proxy holds keyboard focus, so the canvas never saw Esc. The proxy now
// forwards Escape to Go via window._kbProxyEscapeGo (registered in softKeyboardInit).
console.log("Test: Escape via proxy forwards to Go and blurs the proxy");
{
  const { context, page } = await setupMobilePage();
  try {
    const registered = await page.evaluate(() => typeof window._kbProxyEscapeGo === "function");
    assert(registered, "Go should register window._kbProxyEscapeGo in softKeyboardInit");

    await page.evaluate(() => {
      window.__escFwd = 0;
      const orig = window._kbProxyEscapeGo;
      window._kbProxyEscapeGo = function () {
        window.__escFwd++;
        if (typeof orig === "function") orig.apply(this, arguments);
      };
    });

    // Focus the proxy (simulates a text input gaining focus on WASM).
    await page.evaluate(() => window._kbProxyFocus("text"));
    await page.waitForTimeout(80);
    const focusedBefore = await page.evaluate(
      () => document.activeElement && document.activeElement.id === "beatmo-kb-proxy"
    );
    assert(focusedBefore, "proxy should be focused before Escape");

    await page.keyboard.press("Escape");
    await page.waitForTimeout(80);

    const r = await page.evaluate(() => ({
      fwd: window.__escFwd,
      stillFocused: document.activeElement && document.activeElement.id === "beatmo-kb-proxy",
    }));
    assert(r.fwd >= 1, "Escape should forward to Go via _kbProxyEscapeGo");
    assert(!r.stillFocused, "Escape should blur the proxy (dismiss soft keyboard)");

    console.log("  PASS");
  } catch (e) {
    console.log(`  FAIL: ${e.message}`);
  } finally {
    if (isCoverageEnabled()) await flushCoverage(page, new URL("../../coverage/browser-raw", import.meta.url).pathname, "mobile_text_input");
    await context.close();
  }
}

await browser.close();
server.close();

if (!allPassed) {
  process.exit(1);
}
console.log("\nAll mobile text input tests passed!");
