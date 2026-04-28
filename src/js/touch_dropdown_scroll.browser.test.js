import { chromium, devices } from "playwright";
import http from "http";
import fs from "fs";
import path from "path";
import { spawnSync } from "child_process";
import { fileURLToPath } from "url";
import { resolveGoBinary, assertSimpleDrawMode, shouldSkipWasmBuild, flushCoverage, isCoverageEnabled } from "./browser_test_helpers.js";
import { cdpDrag } from "./touch_cdp_helpers.js";

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
    typeof openInstMenu === "function" &&
    typeof instMenuOpenState === "function" &&
    typeof instMenuItemRects === "function" &&
    typeof forceDraw === "function"
  );
  await assertSimpleDrawMode(page, false, "mobile dropdown");
  await page.evaluate(() => forceDraw?.());
  await page.waitForTimeout(300);
  return { context, page };
}

// Test 1: Touch scroll instrument list — menu stays open
console.log("Test 1: Touch scroll instrument list");
{
  const { context, page } = await setupMobilePage();
  try {
    // Open instrument menu for row 0
    await page.evaluate(() => openInstMenu?.(0));
    await page.evaluate(() => forceDraw?.());
    await page.waitForTimeout(200);

    const open1 = await page.evaluate(() => instMenuOpenState?.());
    assert(open1, "inst menu should be open after openInstMenu");

    // Get menu item rects to find a drag target
    const items = await page.evaluate(() => {
      const rects = instMenuItemRects?.();
      if (!rects || rects.length === 0) return [];
      const arr = [];
      for (let i = 0; i < rects.length; i++) {
        arr.push({ x: rects[i].x, y: rects[i].y, w: rects[i].w, h: rects[i].h });
      }
      return arr;
    });

    if (items.length > 0) {
      const item = items[0];
      const startX = item.x + item.w / 2;
      const startY = item.y + item.h / 2;

      // Drag vertically on an instrument item
      await cdpDrag(page, startX, startY, startX, startY - 80, 10, 16);
      await page.evaluate(() => forceDraw?.());
      await page.waitForTimeout(100);

      const openAfterDrag = await page.evaluate(() => instMenuOpenState?.());
      assert(openAfterDrag, "inst menu should stay open after touch scroll drag");
    } else {
      console.log("  (skipped — no menu items visible)");
    }
    console.log("  PASS");
  } catch (e) {
    console.log(`  FAIL: ${e.message}`);
  } finally {
    await context.close();
  }
}

// Test 2: Touch drag on instrument list does NOT close menu (scroll, not select)
// This is the core behavior being fixed: dragging on an instrument should
// scroll the list, not select the instrument.
console.log("Test 2: Touch drag on instrument does not select");
{
  const { context, page } = await setupMobilePage();
  try {
    await page.evaluate(() => openInstMenu?.(0));
    await page.evaluate(() => forceDraw?.());
    await page.waitForTimeout(200);

    const items = await page.evaluate(() => {
      const rects = instMenuItemRects?.();
      if (!rects || rects.length === 0) return [];
      const arr = [];
      for (let i = 0; i < rects.length; i++) {
        arr.push({ id: rects[i].id, x: rects[i].x, y: rects[i].y, w: rects[i].w, h: rects[i].h });
      }
      return arr;
    });

    if (items.length > 0) {
      const item = items[0];
      const cx = item.x + item.w / 2;
      const cy = item.y + item.h / 2;

      // CDP drag on an instrument item — should scroll, not select
      await cdpDrag(page, cx, cy, cx, cy - 60, 8, 16);
      await page.evaluate(() => forceDraw?.());
      await page.waitForTimeout(200);

      // Menu should still be open (drag = scroll, not select)
      const openAfterDrag = await page.evaluate(() => instMenuOpenState?.());
      assert(openAfterDrag, "inst menu should stay open after touch drag on item");

      console.log("  PASS");
    } else {
      console.log("  (skipped — no menu items visible)");
    }
  } catch (e) {
    console.log(`  FAIL: ${e.message}`);
  } finally {
    await context.close();
  }
}

// Test 3: openInstMenu uses component path (instMenuComp)
console.log("Test 3: openInstMenu uses component path");
{
  const { context, page } = await setupMobilePage();
  try {
    await page.evaluate(() => openInstMenu?.(0));
    await page.evaluate(() => forceDraw?.());
    await page.waitForTimeout(200);

    const open = await page.evaluate(() => instMenuOpenState?.());
    assert(open, "inst menu should be open");

    // Verify that the component path rendered items (instMenuItemRects non-empty)
    const items = await page.evaluate(() => {
      const rects = instMenuItemRects?.();
      return rects ? rects.length : 0;
    });
    assert(items > 0, "inst menu should have visible items (component path)");

    console.log("  PASS");
  } catch (e) {
    console.log(`  FAIL: ${e.message}`);
  } finally {
    if (isCoverageEnabled()) await flushCoverage(page, new URL("../../coverage/browser-raw", import.meta.url).pathname, "mobile_dropdown_touch_scroll");
    await context.close();
  }
}

// Cleanup
await browser.close();
server.close();

if (allPassed) {
  console.log("\nAll mobile dropdown touch scroll tests passed.");
  process.exit(0);
} else {
  console.log("\nSome mobile dropdown touch scroll tests FAILED.");
  process.exit(1);
}
