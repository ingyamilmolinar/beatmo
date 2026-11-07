/** Open → navigate → select through the WASM bridge on a phone viewport. Buttons are driven by MOUSE hold-and-poll:
 * at headless software-GL frame rates the touch detector misclassifies an identical tap at random. Real touch handling
 * is covered by the Go functional tests. */

import { chromium, devices } from "playwright";
import http from "http";
import fs from "fs";
import path from "path";
import { spawnSync } from "child_process";
import { fileURLToPath } from "url";
import { buildMainWasm } from "./real_input_test_helpers.js";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const jsDir = __dirname;

console.log("Building WASM...");
if (!buildMainWasm({ logLevel: "INFO" })) {
  throw new Error("go build main wasm failed");
}

const server = http.createServer((req, res) => {
  const cleanUrl = req.url.split("?")[0];
  const file = cleanUrl === "/" ? "/index.html" : cleanUrl;
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

let failed = 0;
function assert(cond, msg) {
  if (!cond) { console.error(`  FAIL: ${msg}`); failed++; return false; }
  console.log(`  PASS: ${msg}`); return true;
}

const iPhone = devices["iPhone 12"]; // portrait, hasTouch, DPR 3
const browser = await chromium.launch({ args: ["--autoplay-policy=no-user-gesture-required"] });
const context = await browser.newContext({ ...iPhone, hasTouch: true });
const page = await context.newPage();
page.on("console", (msg) => console.log(`[PAGE] ${msg.type()}: ${msg.text()}`));
page.on("pageerror", (err) => console.log(`[PAGE ERROR] ${err.message}`));

try {
  await page.goto(`http://localhost:${port}/`);
  await page.waitForFunction(() => typeof instMenuOpenState === "function", null, { timeout: 30000 });
  await page.waitForTimeout(400);

  // A tap registers only if the loop ticks between touchStart and touchEnd, and parallel jobs starve it near 0 fps
  // at boot, so this waits for a steady tick.
  const frameCount = async () =>
    await page.evaluate(() =>
      typeof perfStats === "function" ? perfStats().frames || 0 : 0,
    );
  let steady = 0;
  for (let i = 0; i < 80 && steady < 2; i++) {
    const f0 = await frameCount();
    await page.waitForTimeout(400);
    const f1 = await frameCount();
    // ≥2 frames per 400ms (~5fps) twice in a row = the loop has recovered.
    steady = f1 - f0 >= 2 ? steady + 1 : 0;
  }
  console.log(`warm-up: loop steady (frames advancing)=${steady >= 2}`);

  // Mouse, not touch (see the header), held while polling so the press is sampled across frames.
  const lblRect = await page.evaluate(() =>
    typeof rowLabelRect === "function" ? rowLabelRect(0) : null,
  );
  console.log(`rowLabelRect(0)=${JSON.stringify(lblRect)}`);
  let openedByTouch = false;
  if (lblRect && lblRect.w > 0 && lblRect.h > 0) {
    const cx = lblRect.x + lblRect.w / 2;
    const cy = lblRect.y + lblRect.h / 2;
    for (let attempt = 0; attempt < 5 && !openedByTouch; attempt++) {
      await page.mouse.move(cx, cy);
      await page.mouse.down();
      for (let poll = 0; poll < 25 && !openedByTouch; poll++) {
        await page.waitForTimeout(100);
        openedByTouch = await page.evaluate(() => instMenuOpenState());
      }
      await page.mouse.up();
      await page.mouse.move(0, 0);
      if (!openedByTouch) await page.waitForTimeout(150);
      console.log(`row-label open attempt ${attempt + 1}: open=${openedByTouch}`);
    }
  }
  assert(openedByTouch, "tap on row label OPENED the instrument menu");
  if (!openedByTouch) {
    await page.evaluate(() => openInstMenu(0));
    await page.waitForTimeout(200);
  }

  const before = await page.evaluate(() => rowInstrument(0));
  console.log(`row 0 instrument before = ${before}`);

  // Programmatic: multi-step nav taps drop non-deterministically here. The row's own category often holds only its
  // instrument, so the walk looks for one with a different, selectable item.
  await page.evaluate(() => {
    if (typeof instMenuClickBack === "function") instMenuClickBack();
  });
  await page.waitForTimeout(150);

  const cats = await page.evaluate(() =>
    typeof instMenuCategoryRects === "function" ? instMenuCategoryRects() : [],
  );
  console.log(`categories: ${JSON.stringify((cats || []).map((c) => c.name))}`);

  let items = [];
  let pick = null;
  for (let ci = 0; ci < (cats ? cats.length : 0) && !pick; ci++) {
    const ok = await page.evaluate((i) => instMenuSelectCategory(i), ci);
    if (!ok) continue;
    // Poll for the category's items to render (layout can lag under software-GL).
    for (let poll = 0; poll < 8; poll++) {
      await page.waitForTimeout(100);
      items = await page.evaluate(() => instMenuItemRects());
      if (Array.isArray(items) && items.length > 0) break;
    }
    if (Array.isArray(items) && items.length > 0) {
      const cand = items.find((it) => it.id !== before);
      if (cand) {
        pick = cand;
        break;
      }
    }
    // This category has no different instrument — back out and try the next.
    await page.evaluate(() => {
      if (typeof instMenuClickBack === "function") instMenuClickBack();
    });
    await page.waitForTimeout(120);
  }
  if (!pick) {
    console.error("no selectable instrument in any category; debug:",
      JSON.stringify(await page.evaluate(() => (typeof instMenuDebugState === "function" ? instMenuDebugState() : {}))));
    throw new Error("no selectable instrument item");
  }
  console.log(`selecting instrument item id=${pick.id} at (${pick.x + pick.w / 2}, ${pick.y + pick.h / 2})`);
  // Held until rowInstrument reports the picked id.
  let after = before;
  {
    const cx = pick.x + pick.w / 2;
    const cy = pick.y + pick.h / 2;
    for (let attempt = 0; attempt < 5 && after !== pick.id; attempt++) {
      await page.mouse.move(cx, cy);
      await page.mouse.down();
      for (let poll = 0; poll < 25 && after !== pick.id; poll++) {
        await page.waitForTimeout(100);
        after = await page.evaluate(() => rowInstrument(0));
      }
      await page.mouse.up();
      await page.mouse.move(0, 0);
      if (after !== pick.id) await page.waitForTimeout(150);
      console.log(`item-select attempt ${attempt + 1}: after=${after}`);
    }
  }
  console.log(`row 0 instrument after = ${after}`);
  assert(after === pick.id,
    `tap on instrument selected it (got "${after}", want "${pick.id}")`);

  if (failed === 0) console.log("\ninst_menu_mobile_touch: ALL PASS");
  else console.log(`\ninst_menu_mobile_touch: ${failed} FAILURE(S)`);
} finally {
  await context.close();
  await browser.close();
  server.close();
}

if (failed > 0) process.exit(1);
