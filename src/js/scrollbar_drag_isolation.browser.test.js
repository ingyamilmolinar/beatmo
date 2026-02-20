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

const browser = await chromium.launch({ args: ["--autoplay-policy=no-user-gesture-required"] });

let allPassed = true;
function assert(cond, msg) {
  if (!cond) { allPassed = false; throw new Error(msg); }
}

async function setupDesktopPageWithRows(rowCount = 10) {
  const context = await browser.newContext({ viewport: { width: 1280, height: 720 } });
  const page = await context.newPage();
  await page.goto(`http://localhost:${port}/`);
  await page.waitForFunction(() =>
    typeof addDrumRow === "function" &&
    typeof forceDraw === "function" &&
    typeof totalRows === "function" &&
    typeof scrollBarRect === "function" &&
    typeof scrollThumbRect === "function" &&
    typeof rowMuted === "function" &&
    typeof rowMuteBtnRect === "function"
  );
  await assertSimpleDrawMode(page, false, "scrollbar drag isolation");

  // Add rows to ensure scrollbar is needed
  const currentRows = await page.evaluate(() => totalRows?.());
  for (let i = currentRows; i < rowCount; i++) {
    await page.evaluate(() => addDrumRow?.());
  }
  await page.evaluate(() => forceDraw?.());
  await page.waitForTimeout(300);

  return { context, page };
}

// Test 1: Scrollbar drag past mute button does not activate mute
console.log("Test 1: Scrollbar drag past mute button does not activate mute");
{
  const { context, page } = await setupDesktopPageWithRows(15);
  try {
    // Get scrollbar thumb rect
    const thumb = await page.evaluate(() => scrollThumbRect?.());
    assert(thumb && thumb.h > 0, `scrollThumbRect returned empty: ${JSON.stringify(thumb)}`);

    // Get mute button rect for row 0
    const muteRect = await page.evaluate(() => rowMuteBtnRect?.(0));
    assert(muteRect && muteRect.w > 0, `rowMuteBtnRect(0) returned empty: ${JSON.stringify(muteRect)}`);

    // Verify row 0 starts unmuted
    const mutedBefore = await page.evaluate(() => rowMuted?.(0));
    assert(mutedBefore === false, `Expected row 0 unmuted initially, got ${mutedBefore}`);

    // Drag from scrollbar thumb center downward, passing through mute button Y
    const startX = thumb.x + thumb.w / 2;
    const startY = thumb.y + thumb.h / 2;
    // End well below the mute button
    const endY = muteRect.y + muteRect.h + 50;

    await page.mouse.move(startX, startY);
    await page.mouse.down();
    // Move gradually through mute button area
    const muteX = muteRect.x + muteRect.w / 2;
    for (let y = startY; y <= endY; y += 5) {
      // Drift toward mute button X area to simulate real drag
      const progress = (y - startY) / (endY - startY);
      const x = startX + (muteX - startX) * progress;
      await page.mouse.move(x, y);
    }
    await page.mouse.up();

    await page.evaluate(() => forceDraw?.());
    await page.waitForTimeout(200);

    const mutedAfter = await page.evaluate(() => rowMuted?.(0));
    assert(mutedAfter === false, `Mute button should NOT have fired during scrollbar drag. Row 0 muted=${mutedAfter}`);

    console.log("  PASS");
  } catch (e) {
    console.log(`  FAIL: ${e.message}`);
  } finally {
    await context.close();
  }
}

// Test 2: Touch scroll past buttons (mobile emulation)
console.log("Test 2: Touch scroll past buttons (mobile) does not toggle mute");
{
  const iPhone = devices["iPhone 12"];
  const context = await browser.newContext({ ...iPhone });
  const page = await context.newPage();
  try {
    await page.goto(`http://localhost:${port}/`);
    await page.waitForFunction(() =>
      typeof addDrumRow === "function" &&
      typeof forceDraw === "function" &&
      typeof totalRows === "function" &&
      typeof rowMuted === "function"
    );
    await assertSimpleDrawMode(page, false, "mobile scrollbar drag isolation");

    // Add enough rows for scrolling
    const currentRows = await page.evaluate(() => totalRows?.());
    for (let i = currentRows; i < 10; i++) {
      await page.evaluate(() => addDrumRow?.());
    }
    await page.evaluate(() => forceDraw?.());
    await page.waitForTimeout(300);

    // Verify row 0 starts unmuted
    const mutedBefore = await page.evaluate(() => rowMuted?.(0));
    assert(mutedBefore === false, `Expected row 0 unmuted initially, got ${mutedBefore}`);

    // Touch-drag vertically through the row area
    const muteRect = await page.evaluate(() => rowMuteBtnRect?.(0));
    if (muteRect && muteRect.w > 0) {
      const cx = muteRect.x + muteRect.w / 2;
      const startY = muteRect.y - 40;
      const endY = muteRect.y + muteRect.h + 80;
      await cdpDrag(page, cx, startY, cx, endY, 8);
      await page.evaluate(() => forceDraw?.());
      await page.waitForTimeout(200);

      const mutedAfter = await page.evaluate(() => rowMuted?.(0));
      assert(mutedAfter === false, `Mute should NOT have fired during touch scroll. Row 0 muted=${mutedAfter}`);
    }

    console.log("  PASS");
  } catch (e) {
    console.log(`  FAIL: ${e.message}`);
  } finally {
    if (isCoverageEnabled()) await flushCoverage(page, new URL("../../coverage/browser-raw", import.meta.url).pathname, "scrollbar_drag_isolation");
    await context.close();
  }
}

await browser.close();
server.close();

if (!allPassed) {
  process.exit(1);
}
console.log("\nAll scrollbar drag isolation tests passed!");
