// landscape_block.browser.test.js
//
// Mobile landscape is unsupported. A DOM/CSS overlay (#landscape-block in
// index.html), driven by the real (orientation: landscape) media query and
// anchored to the visible viewport (position:fixed; inset:0), covers the canvas
// with a rotate-to-portrait notice. This is robust against the Ebiten
// canvas/DPR sizing race that mis-positioned the in-engine notice on real
// devices ("top half black, message in the bottom half").
//
// Verifies (browser-only platform feature + Go→JS bridge plumbing, per the JS
// Test Charter):
//   1. Landscape phone viewport => overlay visible, covers the full viewport.
//   2. Overlay is the topmost element at center (blocks input to the canvas).
//   3. Overlay text is localized text pushed from Go (non-empty title/body).
//   4. Portrait => overlay hidden.
//   5. Live rotation round-trip toggles visibility (seamless transition).
//
// Usage: GO=/path/to/.tools/go/bin/go node src/js/landscape_block.browser.test.js
//   (WASM_PREBUILT=1 reuses an existing src/js/main.wasm.)

import { chromium, devices } from "playwright";
import http from "http";
import fs from "fs";
import path from "path";
import { spawnSync } from "child_process";
import { fileURLToPath } from "url";
import { resolveGoBinary, shouldSkipWasmBuild } from "./browser_test_helpers.js";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const jsDir = __dirname;
const repoRoot = path.resolve(jsDir, "..", "..");
const goDir = path.resolve(repoRoot, "src/go");
const GO = resolveGoBinary();

if (!shouldSkipWasmBuild("main.wasm")) {
  const build = spawnSync(
    GO,
    ["build", "-ldflags", "-X main.defaultLog=INFO", "-o", path.join(jsDir, "main.wasm"), "./cmd"],
    { cwd: goDir, env: { ...process.env, GOOS: "js", GOARCH: "wasm" }, stdio: "inherit" }
  );
  if (build.status !== 0) throw new Error("go build main wasm failed");
}

const server = http.createServer((req, res) => {
  const file = req.url === "/" ? "/index.html" : req.url.split("?")[0];
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

let exitCode = 0;
const fail = (msg) => { console.error(`  ✗ ${msg}`); exitCode = 1; };
const pass = (msg) => console.log(`  ✓ ${msg}`);

// Reads the overlay's runtime state from the page.
async function overlayState(page) {
  return page.evaluate(() => {
    const el = document.getElementById("landscape-block");
    if (!el) return { exists: false };
    const cs = getComputedStyle(el);
    const r = el.getBoundingClientRect();
    const cx = Math.floor(window.innerWidth / 2);
    const cy = Math.floor(window.innerHeight / 2);
    const top = document.elementFromPoint(cx, cy);
    return {
      exists: true,
      display: cs.display,
      visible: cs.display !== "none",
      coversViewport: r.width >= window.innerWidth - 1 && r.height >= window.innerHeight - 1,
      title: (el.querySelector(".lb-title") || {}).textContent || "",
      body: (el.querySelector(".lb-body") || {}).textContent || "",
      topmostIsOverlay: !!top && (top.id === "landscape-block" || el.contains(top)),
    };
  });
}

async function waitWasm(page) {
  await page.waitForFunction(
    () => typeof window.forceDraw === "function",
    { timeout: 20000 },
  ).catch(() => {});
  await page.waitForTimeout(1000);
}

const browser = await chromium.launch({ args: ["--autoplay-policy=no-user-gesture-required"] });
try {
  // ── 1) Landscape phone: overlay visible + covers viewport + blocks input ──
  {
    console.log("--- landscape phone ---");
    const ctx = await browser.newContext({ ...devices["Galaxy S9+ landscape"] });
    const page = await ctx.newPage();
    await page.goto(`http://localhost:${port}/`);
    await waitWasm(page);
    const s = await overlayState(page);
    if (!s.exists) fail("#landscape-block element missing from index.html");
    if (!s.visible) fail(`overlay not visible in landscape (display=${s.display})`); else pass("overlay visible in landscape");
    if (!s.coversViewport) fail("overlay does not cover the full viewport"); else pass("overlay covers viewport");
    if (!s.topmostIsOverlay) fail("overlay is not topmost at center (input would leak to canvas)"); else pass("overlay is topmost (blocks input)");
    if (!s.title || !s.body) fail(`overlay text not set (title=${JSON.stringify(s.title)} body=${JSON.stringify(s.body)})`); else pass(`overlay text present: "${s.title}"`);
    await ctx.close();
  }

  // ── 2) Portrait phone: overlay hidden ──
  {
    console.log("--- portrait phone ---");
    const ctx = await browser.newContext({ ...devices["Galaxy S9+"] });
    const page = await ctx.newPage();
    await page.goto(`http://localhost:${port}/`);
    await waitWasm(page);
    const s = await overlayState(page);
    if (s.visible) fail(`overlay must be hidden in portrait (display=${s.display})`); else pass("overlay hidden in portrait");
    await ctx.close();
  }

  // ── 3) Live rotation round-trip: portrait -> landscape -> portrait ──
  {
    console.log("--- rotation round-trip ---");
    const ctx = await browser.newContext({ viewport: { width: 390, height: 844 }, isMobile: true, hasTouch: true, deviceScaleFactor: 2 });
    const page = await ctx.newPage();
    await page.goto(`http://localhost:${port}/`);
    await waitWasm(page);

    let s = await overlayState(page);
    if (s.visible) fail("round-trip: overlay should start hidden in portrait"); else pass("round-trip: hidden (portrait)");

    await page.setViewportSize({ width: 844, height: 390 });
    await page.waitForTimeout(400);
    s = await overlayState(page);
    if (!s.visible) fail("round-trip: overlay should be visible after rotating to landscape"); else pass("round-trip: visible (landscape)");

    await page.setViewportSize({ width: 390, height: 844 });
    await page.waitForTimeout(400);
    s = await overlayState(page);
    if (s.visible) fail("round-trip: overlay should hide again after rotating back to portrait"); else pass("round-trip: hidden again (portrait)");
    await ctx.close();
  }

  // ── 4) Viewport height is re-pinned to the VISIBLE viewport on rotation ──
  // Regression guard for the iOS-Safari 100dvh bug: after rotating back to
  // portrait the layout-viewport height can stay stale, so the canvas ends up a
  // different height than the screen — a black band at the top and a wrong
  // (mis-scaled) game resolution. The fix pins --app-vh to the real viewport and
  // re-applies it on orientationchange/resize. Here we deliberately corrupt the
  // pinned height (simulating the stale value), fire the rotation events, and
  // assert the canvas refits the visible viewport exactly. Without the handler
  // the stale height persists and the canvas no longer fills the screen.
  {
    console.log("--- viewport re-pin on rotation ---");
    const ctx = await browser.newContext({ viewport: { width: 390, height: 844 }, isMobile: true, hasTouch: true, deviceScaleFactor: 3 });
    const page = await ctx.newPage();
    await page.goto(`http://localhost:${port}/`);
    await waitWasm(page);

    // The app must size off a pinned CSS var (not raw 100dvh) so a stale value
    // can be corrected. Corrupt it to simulate iOS leaving a wrong viewport.
    const stuck = await page.evaluate(() => {
      document.documentElement.style.setProperty("--app-vh", (window.innerHeight + 90) + "px");
      return Math.round(document.body.getBoundingClientRect().height);
    });
    if (stuck === undefined) fail("could not read body height");
    if (Math.abs(stuck - 844) <= 1) fail("app height does not follow --app-vh (CSS not pinned to the var) — stale viewport can't be corrected");
    else pass(`app height follows --app-vh (stuck body=${stuck})`);

    // Fire the events a real device rotation fires; the handler must re-pin.
    await page.evaluate(() => {
      window.dispatchEvent(new Event("orientationchange"));
      window.dispatchEvent(new Event("resize"));
    });
    await page.waitForTimeout(600);

    const s = await page.evaluate(() => {
      const c = document.querySelector("canvas");
      const r = c ? c.getBoundingClientRect() : null;
      return {
        bodyH: Math.round(document.body.getBoundingClientRect().height),
        innerW: window.innerWidth,
        innerH: window.innerHeight,
        canvasLeft: r ? Math.round(r.left) : null,
        canvasTop: r ? Math.round(r.top) : null,
        canvasW: r ? Math.round(r.width) : null,
        canvasH: r ? Math.round(r.height) : null,
        fb: c ? [c.width, c.height] : null, // framebuffer (attribute) dims
      };
    });
    if (s.bodyH !== s.innerH) fail(`app height NOT re-pinned after rotation (body=${s.bodyH} inner=${s.innerH}) — black band / wrong resolution`); else pass("app height re-pinned to visible viewport after rotation");
    if (s.canvasTop !== 0) fail(`canvas has a top gap after rotation (top=${s.canvasTop})`); else pass("no top black band (canvas top=0)");
    if (Math.abs(s.canvasH - s.innerH) > 1) fail(`canvas does not fill viewport height after rotation (canvas=${s.canvasH} inner=${s.innerH}) — wrong resolution`); else pass("canvas fills viewport height");
    // Width / right-side coverage: a vertical black box on the right means the
    // canvas does not span the full width (or its framebuffer is the wrong shape).
    if (s.canvasLeft !== 0) fail(`canvas has a left gap after rotation (left=${s.canvasLeft})`); else pass("canvas anchored to left edge");
    if (Math.abs(s.canvasW - s.innerW) > 1) fail(`canvas does not fill viewport WIDTH after rotation (canvas=${s.canvasW} inner=${s.innerW}) — black box on the right`); else pass("canvas fills viewport width (no right black box)");
    // Framebuffer aspect must match the display aspect, else Ebiten letterboxes
    // / pillarboxes the game inside the canvas (a black band, e.g. right 50%).
    if (s.fb) {
      const fbAspect = s.fb[0] / s.fb[1];
      const dispAspect = s.canvasW / s.canvasH;
      if (Math.abs(fbAspect - dispAspect) > 0.02) fail(`framebuffer aspect ${fbAspect.toFixed(3)} != display aspect ${dispAspect.toFixed(3)} — game pillar/letter-boxed (black box)`); else pass("framebuffer aspect matches display (no boxing)");
    }
    await ctx.close();
  }
} catch (e) {
  fail(`unexpected error: ${e && e.stack ? e.stack : e}`);
} finally {
  await browser.close();
  server.close();
}

console.log(exitCode === 0 ? "\nPASS landscape_block" : "\nFAIL landscape_block");
process.exit(exitCode);
