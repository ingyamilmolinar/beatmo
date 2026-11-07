import { chromium, webkit, devices } from "playwright";
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
  // In the parallel batch this context is not the OS-foreground window, so a fresh input's focus() did not stick; focus
  // emulation makes hasFocus() true. A harness-only fix, as in mobile_native_input.
  const focusCdp = await context.newCDPSession(page);
  await focusCdp.send("Emulation.setFocusEmulationEnabled", { enabled: true });
  await page.goto(`http://localhost:${port}/`);
  await page.waitForFunction(() =>
    typeof openInstMenu === "function" &&
    typeof instMenuSearchRect === "function" &&
    typeof instMenuSearchState === "function" &&
    typeof forceDraw === "function" &&
    typeof kbProxyFocused === "function"
  );
  await assertSimpleDrawMode(page, false, "mobile text input");
  await page.evaluate(() => forceDraw?.());
  await page.waitForTimeout(200);
  await openSearchTarget(page);
  return { context, page };
}

// The instrument-menu search box: BPM is a value wheel.
async function openSearchTarget(page) {
  await page.evaluate(() => openInstMenu(0));
  await page.waitForFunction(() => { const r = instMenuSearchRect(); return r && r.w > 0; },
    null, { timeout: 5000, polling: 50 });
}

async function targetRect(page) {
  return await page.evaluate(() => instMenuSearchRect?.());
}

// searchText reads what Go holds in the search box.
async function searchText(page) {
  return await page.evaluate(() => instMenuSearchState?.()?.text);
}

// Throttled rAF can fire the first touchend before any Update registered the field, so it re-taps (no forceGameTick,
// which can stall the loop), and polls every 50 ms, since a rAF-driven poll starves. Keep it in lockstep with
// mobile_native_input's helper, or whichever keeps the rAF default flakes alone.
async function tapAwaitInput(page, rect) {
  const cx = rect.x + rect.w / 2;
  const cy = rect.y + rect.h / 2;
  for (let attempt = 0; attempt < 12; attempt++) {
    await cdpTap(page, cx, cy);
    try {
      await page.waitForFunction(() => {
        const inputs = document.querySelectorAll('input[style*="z-index"]');
        for (const inp of inputs) {
          if (inp.style.zIndex === '10000') return true;
        }
        return false;
      }, null, { timeout: 1500, polling: 50 });
      return true;
    } catch (_) {
      // Registration not ready yet — let the rAF loop tick, then re-tap.
      await page.waitForTimeout(120);
    }
  }
  return false;
}

// Test 1: text field soft keyboard — native input appears with inputmode=text
console.log("Test 1: Search soft keyboard — native input focused with inputmode=text");
{
  const { context, page } = await setupMobilePage();
  try {
    const rect = await targetRect(page);
    assert(rect && rect.w > 0, "instMenuSearchRect returned empty");

    // Retry-aware: creation races the throttled loop that registers the rect.
    assert(await tapAwaitInput(page, rect), "native input did not appear after tap");

    // On mobile a real <input> overlay is focused instead of the hidden proxy.
    const nativeInputInfo = await page.evaluate(() => {
      const active = document.activeElement;
      if (active && active.tagName === 'INPUT' && active.style.zIndex === '10000') {
        return { focused: true, inputmode: active.getAttribute('inputmode') };
      }
      // Fallback: check proxy (desktop path)
      return { focused: kbProxyFocused?.() === true, inputmode: kbProxyInputMode?.() };
    });
    assert(nativeInputInfo.focused === true, `Expected input focused, got ${nativeInputInfo.focused}`);
    assert(nativeInputInfo.inputmode === "text", `Expected inputmode=text, got ${nativeInputInfo.inputmode}`);

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
    const rect = await targetRect(page);
    assert(rect && rect.w > 0, "instMenuSearchRect returned empty");

    // Focus the search field (retry-aware native input creation).
    assert(await tapAwaitInput(page, rect), "native input did not appear after tap");

    // Check that either native input or proxy is focused
    let focused = await page.evaluate(() => {
      const active = document.activeElement;
      if (active && active.tagName === 'INPUT' && active.style.zIndex === '10000') return true;
      return kbProxyFocused?.() === true;
    });
    assert(focused === true, "Expected focused after tap");

    // Tap outside the search field to blur (native input commits on blur)
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

// Test 3: Type into the search field via page.keyboard
console.log("Test 3: Type into the search field via keyboard");
{
  const { context, page } = await setupMobilePage();
  try {
    const rect = await targetRect(page);
    assert(rect && rect.w > 0, "instMenuSearchRect returned empty");

    // A single tap races the throttled loop, and the typed keys would go nowhere.
    assert(await tapAwaitInput(page, rect), "native input did not appear after tap");
    await page.evaluate(() => forceDraw?.());

    await page.keyboard.type("snare");
    await page.waitForTimeout(100);
    await page.keyboard.press("Enter");
    await page.waitForFunction(() => instMenuSearchState?.()?.text === "snare", null,
      { timeout: 4000, polling: 50 }).catch(() => {});

    const text = await searchText(page);
    assert(text === "snare", `Expected search text "snare" after typing, got ${JSON.stringify(text)}`);
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

// Text fields capture keys through an EMPTY hidden proxy, and WebKit fires no beforeinput(deleteContentBackward) on
// an empty input, so Backspace is buffered from keydown. WebKit only (Chromium cannot show it), normal short presses as a
// user; skips if WebKit is missing.
console.log("Test 7: Desktop hardware-keyboard edit on WebKit (Safari backspace)");
{
  let wkBrowser = null;
  try {
    try {
      wkBrowser = await webkit.launch();
    } catch (e) {
      console.log(`  SKIP: WebKit unavailable (${e.message.split("\n")[0]})`);
    }
    if (wkBrowser) {
      const context = await wkBrowser.newContext({ viewport: { width: 1280, height: 720 } });
      const page = await context.newPage();
      try {
        await page.goto(`http://localhost:${port}/`);
        await page.waitForFunction(() =>
          typeof openInstMenu === "function" &&
          typeof instMenuSearchRect === "function" &&
          typeof instMenuSearchState === "function" &&
          typeof forceDraw === "function"
        );
        await assertSimpleDrawMode(page, false, "desktop text input");
        await page.evaluate(() => forceDraw?.());
        await page.waitForTimeout(300);
        await openSearchTarget(page);

        const rect = await targetRect(page);
        assert(rect && rect.w > 0, "instMenuSearchRect empty");
        const cx = rect.x + rect.w / 2, cy = rect.y + rect.h / 2;

        // The focusing press is held ~400 ms for the throttled loop; the KEY presses stay short, which is what must work.
        let edited = false;
        for (let attempt = 0; attempt < 6 && !edited; attempt++) {
          await page.mouse.move(cx, cy);
          await page.mouse.down();
          await page.waitForTimeout(420);
          await page.mouse.up();
          await page.waitForTimeout(150);

          // Short Backspaces clear it, a word plus a stray letter is typed, the stray deleted.
          for (let i = 0; i < 8; i++) { await page.keyboard.press("Backspace"); await page.waitForTimeout(90); }
          await page.keyboard.type("kickx");
          await page.waitForTimeout(120);
          await page.keyboard.press("Backspace");
          await page.waitForTimeout(200);
          for (let i = 0; i < 5; i++) { await page.evaluate(() => forceDraw?.()); await page.waitForTimeout(40); }

          if ((await page.evaluate(() => instMenuSearchState?.()?.text)) === "kick") edited = true;
        }

        assert(edited,
          `Safari hardware keyboard could not type + Backspace in a text field — ` +
          `Backspace on the empty proxy was dropped (beforeinput doesn't fire on empty <input> in WebKit)`);
        console.log("  PASS");
      } finally {
        await context.close();
      }
    }
  } catch (e) {
    console.log(`  FAIL: ${e.message}`);
  } finally {
    if (wkBrowser) await wkBrowser.close();
  }
}

// Mobile layout on a touch-capable device driven by a MOUSE: the native input is created only on touchend, which a mouse
// never fires, so the path is chosen by the OPENING gesture's pointerType, not touch capability.
console.log("Test 7b: Mobile-layout + touch-capable device + MOUSE edits a text field");
{
  // Narrow viewport => mobile LAYOUT; hasTouch:true => touch-CAPABLE device.
  const context = await browser.newContext({ viewport: { width: 414, height: 896 }, hasTouch: true });
  const page = await context.newPage();
  try {
    await page.goto(`http://localhost:${port}/`);
    await page.waitForFunction(() =>
      typeof openInstMenu === "function" && typeof instMenuSearchRect === "function" &&
      typeof instMenuSearchState === "function" && typeof forceDraw === "function"
    );
    await assertSimpleDrawMode(page, false, "mobile-layout mouse");
    await page.evaluate(() => forceDraw?.());
    await page.waitForTimeout(300);
    await openSearchTarget(page);

    const rect = await targetRect(page);
    assert(rect && rect.w > 0, "instMenuSearchRect empty");
    const cx = rect.x + rect.w / 2, cy = rect.y + rect.h / 2;

    // Focus with a MOUSE (hold so the throttled rAF loop sees it) + NORMAL keys.
    let edited = false;
    for (let attempt = 0; attempt < 6 && !edited; attempt++) {
      await page.mouse.move(cx, cy);
      await page.mouse.down();
      await page.waitForTimeout(420);
      await page.mouse.up();
      await page.waitForTimeout(150);
      for (let i = 0; i < 8; i++) { await page.keyboard.press("Backspace"); await page.waitForTimeout(90); }
      await page.keyboard.type("hat");
      await page.waitForTimeout(200);
      for (let i = 0; i < 5; i++) { await page.evaluate(() => forceDraw?.()); await page.waitForTimeout(40); }
      if ((await page.evaluate(() => instMenuSearchState?.()?.text)) === "hat") edited = true;
    }
    assert(edited,
      `mobile-layout + touch-capable + MOUSE: hardware keyboard could not edit the search field — ` +
      `the mobile native-<input> path was taken but no touchend created the input (should use the keyboard path)`);
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

    // On mobile the native input system handles text inputs.
    const rect = await targetRect(page);
    assert(rect && rect.w > 0, "instMenuSearchRect returned empty");

    // Retry-aware: creation races the throttled loop.
    assert(await tapAwaitInput(page, rect), "native input did not appear after tap");

    const nativeActive = await page.evaluate(() => {
      const active = document.activeElement;
      return active && active.tagName === 'INPUT' && active.style.zIndex === '10000';
    });
    assert(nativeActive, `Expected native input focused after cdpTap on the search field`);

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

    // Get the search field rect (registered in game coordinates = CSS pixels)
    const rect = await targetRect(page);
    assert(rect && rect.w > 0, "instMenuSearchRect returned empty");

    // Layout returns CSS dims (390×844); DPR-scaled rects would be 3× larger.
    assert(rect.x < 500, `search rect x=${rect.x} looks DPR-scaled (expected < 500 for CSS coords)`);
    assert(rect.y < 900, `search rect y=${rect.y} looks DPR-scaled (expected < 900 for CSS coords)`);

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
    // The touchend listener captures the final tap's coordinates, identical on every attempt.
    assert(await tapAwaitInput(page, rect), `Expected native input after cdpTap at DPR=${dpr}`);

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

// The hidden proxy holds focus, so the canvas never sees Esc; the proxy forwards it to Go.
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
