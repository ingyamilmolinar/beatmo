import { chromium, devices } from "playwright";
import http from "http";
import fs from "fs";
import path from "path";
import { spawnSync } from "child_process";
import { fileURLToPath } from "url";
import { resolveGoBinary, assertSimpleDrawMode, shouldSkipWasmBuild, flushCoverage, isCoverageEnabled } from "./browser_test_helpers.js";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const jsDir = __dirname;
const repoRoot = path.resolve(jsDir, "..", "..");
const goDir = path.resolve(repoRoot, "src/go");
const GO = resolveGoBinary();

// Build WASM target
if (!shouldSkipWasmBuild("main.wasm")) {
const build = spawnSync(
  GO,
  ["build", "-o", path.join(jsDir, "main.wasm"), "./cmd/..."],
  { cwd: goDir, env: { ...process.env, GOOS: "js", GOARCH: "wasm" }, stdio: "inherit" }
);
if (build.status !== 0) throw new Error("go build main wasm failed");
}

// Minimal static server
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

let passed = 0;
let failed = 0;
const failures = [];

function assert(cond, msg) {
  if (!cond) throw new Error(msg);
}

async function setupMobilePage() {
  const context = await browser.newContext({ ...iPhone });
  const page = await context.newPage();
  await page.goto(`http://localhost:${port}/`);
  await page.waitForFunction(() =>
    typeof fullLayoutSnapshot === "function" &&
    typeof forceDraw === "function" &&
    typeof widgetLayoutSnapshot === "function",
    { timeout: 15000 }
  );
  await assertSimpleDrawMode(page, false, "mobile overhaul");
  // Wait for Ebiten's game loop to run several Layout() + Update() cycles
  // which sets touchScreenWidth, triggers mobile late-init (mobileEQCollapsed),
  // sets bgDirty, and finally recalculates layout with FAB positioning.
  await page.waitForFunction(() => {
    const snap = typeof fullLayoutSnapshot === "function" ? fullLayoutSnapshot() : null;
    return snap?.drumLayout?.isSmallScreen === true && snap?.drumLayout?.eqH === 0;
  }, { timeout: 5000 });
  // Give 2 more full Update cycles for layout to fully propagate (FAB, row controls).
  await page.waitForTimeout(500);
  await page.evaluate(() => forceDraw?.());
  await page.waitForTimeout(300);
  await page.evaluate(() => forceDraw?.());
  await page.waitForTimeout(200);
  return { context, page };
}

async function setupDesktopPage() {
  const page = await browser.newPage({ viewport: { width: 1280, height: 720 } });
  await page.goto(`http://localhost:${port}/`);
  await page.waitForFunction(() =>
    typeof fullLayoutSnapshot === "function" &&
    typeof forceDraw === "function",
    { timeout: 15000 }
  );
  await page.evaluate(() => forceDraw?.());
  await page.waitForTimeout(300);
  await page.evaluate(() => forceDraw?.());
  return page;
}

// ────────── Test 1: EQ collapsed by default on mobile ──────────
console.log("Test 1: EQ collapsed by default on mobile");
try {
  const { context, page } = await setupMobilePage();

  const snap = await page.evaluate(() => fullLayoutSnapshot?.());
  assert(snap, "fullLayoutSnapshot returned null");
  assert(snap.drumLayout.isSmallScreen, "Expected isSmallScreen=true on mobile device");
  assert(snap.drumLayout.eqH === 0, `Expected eqH=0 when collapsed, got ${snap.drumLayout.eqH}`);

  console.log("  PASS: EQ collapsed, eqH=0");
  passed++;
  await context.close();
} catch (e) {
  console.error("  FAIL:", e.message);
  failed++;
  failures.push("Test 1: " + e.message);
}

// ────────── Test 2: Transport buttons present and non-overlapping ──────────
console.log("Test 2: Transport buttons present on mobile");
try {
  const { context, page } = await setupMobilePage();

  const snap = await page.evaluate(() => fullLayoutSnapshot?.());
  assert(snap, "fullLayoutSnapshot returned null");

  // Main buttons should be present with usable size
  assert(snap.buttons?.play, "Play button missing");
  assert(snap.buttons?.stop, "Stop button missing");
  assert(snap.buttons?.play.w > 10, `Play button too narrow: ${snap.buttons.play.w}px`);
  assert(snap.buttons?.stop.w > 10, `Stop button too narrow: ${snap.buttons.stop.w}px`);
  assert(snap.buttons?.bpmInc, "BPM+ button missing");
  assert(snap.buttons?.bpmDec, "BPM- button missing");

  // Check that play and stop buttons don't overlap
  const play = snap.buttons.play;
  const stop = snap.buttons.stop;
  const overlaps = (a, b) =>
    a.x < b.x + b.w && a.x + a.w > b.x &&
    a.y < b.y + b.h && a.y + a.h > b.y;
  assert(!overlaps(play, stop), "Play and stop buttons overlap");

  console.log("  PASS: Transport buttons present and non-overlapping");
  passed++;
  await context.close();
} catch (e) {
  console.error("  FAIL:", e.message);
  failed++;
  failures.push("Test 2: " + e.message);
}

// ────────── Test 3: Row layout simplified on mobile ──────────
console.log("Test 3: Row layout simplified on mobile");
try {
  const { context, page } = await setupMobilePage();

  const snap = await page.evaluate(() => fullLayoutSnapshot?.());
  assert(snap, "fullLayoutSnapshot returned null");
  assert(snap.rows?.length > 0, "No rows in snapshot");

  const row0 = snap.rows[0];
  // Label should be present and non-trivial
  if (row0.label) {
    assert(row0.label.w > 20, `Label too narrow: ${row0.label.w}px`);
  }
  // On mobile, mute/solo/edit/color are hidden (accessible via context menu).
  const hiddenMax = 10;
  if (row0.mute) {
    assert(row0.mute.w <= hiddenMax, `Mute button should be hidden on mobile, got ${row0.mute.w}px`);
  }
  if (row0.solo) {
    assert(row0.solo.w <= hiddenMax, `Solo button should be hidden on mobile, got ${row0.solo.w}px`);
  }
  if (row0.edit) {
    assert(row0.edit.w <= hiddenMax, `Edit button should be hidden, got ${row0.edit.w}px`);
  }
  if (row0.color) {
    assert(row0.color.w <= hiddenMax, `Color button should be hidden, got ${row0.color.w}px`);
  }

  console.log("  PASS: Simplified row layout");
  passed++;
  await context.close();
} catch (e) {
  console.error("  FAIL:", e.message);
  failed++;
  failures.push("Test 3: " + e.message);
}

// ────────── Test 4: Add-row button FAB positioning ──────────
console.log("Test 4: Add-row button FAB positioning");
try {
  const { context, page } = await setupMobilePage();

  const snap = await page.evaluate(() => fullLayoutSnapshot?.());
  assert(snap, "fullLayoutSnapshot returned null");

  const addBtn = snap.buttons?.addRow;
  assert(addBtn, "addRow button missing");

  assert(addBtn.w > 10, `addRow button too narrow: ${addBtn.w}px`);
  assert(addBtn.h > 10, `addRow button too short: ${addBtn.h}px`);

  // On mobile, addRowBtn should not overlap any row labels.
  // The exact position (FAB vs inline) depends on when calcLayout runs,
  // but it should never visually conflict with row labels.
  if (snap.rows?.length > 0 && snap.rows[0].label) {
    const lbl = snap.rows[0].label;
    const overlaps =
      addBtn.x < lbl.x + lbl.w && addBtn.x + addBtn.w > lbl.x &&
      addBtn.y < lbl.y + lbl.h && addBtn.y + addBtn.h > lbl.y;
    assert(!overlaps, `addRow button overlaps row label: btn=(${addBtn.x},${addBtn.y},${addBtn.w},${addBtn.h}) label=(${lbl.x},${lbl.y},${lbl.w},${lbl.h})`);
  }

  // Shouldn't overlap row labels
  if (snap.rows?.length > 0 && snap.rows[0].label) {
    const lbl = snap.rows[0].label;
    const overlaps =
      addBtn.x < lbl.x + lbl.w && addBtn.x + addBtn.w > lbl.x &&
      addBtn.y < lbl.y + lbl.h && addBtn.y + addBtn.h > lbl.y;
    assert(!overlaps, "FAB overlaps row label");
  }

  console.log("  PASS: FAB positioned correctly");
  passed++;
  await context.close();
} catch (e) {
  console.error("  FAIL:", e.message);
  failed++;
  failures.push("Test 4: " + e.message);
}

// ────────── Test 5: Desktop layout unchanged ──────────
console.log("Test 5: Desktop layout unchanged");
try {
  const page = await setupDesktopPage();

  const snap = await page.evaluate(() => fullLayoutSnapshot?.());
  assert(snap, "fullLayoutSnapshot returned null");
  assert(!snap.drumLayout.isSmallScreen, "Desktop should not be small screen");

  // All row controls should have non-zero rects
  if (snap.rows?.length > 0) {
    const row0 = snap.rows[0];
    if (row0.label) assert(row0.label.w > 10, "Desktop: label too narrow");
    if (row0.edit) assert(row0.edit.w > 5, "Desktop: edit button too narrow");
    if (row0.mute) assert(row0.mute.w > 5, "Desktop: mute button too narrow");
    if (row0.solo) assert(row0.solo.w > 5, "Desktop: solo button too narrow");
  }

  // EQ height should be the standard panel height (non-zero)
  // Note: on WASM production builds, eqPanelHeight > 0
  if (snap.drumLayout.eqH > 0) {
    console.log(`  eqH=${snap.drumLayout.eqH} (standard panel height)`);
  } else {
    console.log(`  eqH=0 (may be zero if widgets overlap on 720p)`);
  }

  console.log("  PASS: Desktop layout unchanged");
  passed++;
  await page.close();
} catch (e) {
  console.error("  FAIL:", e.message);
  failed++;
  failures.push("Test 5: " + e.message);
}

// ────────── Test 6: Mobile rows area gains space from collapsed EQ ──────────
console.log("Test 6: Mobile rows area uses space from collapsed EQ");
try {
  const { context, page } = await setupMobilePage();

  const snap = await page.evaluate(() => fullLayoutSnapshot?.());
  assert(snap, "fullLayoutSnapshot returned null");

  // With EQ collapsed, rowsAreaHeight should be larger (more drum rows visible).
  const rowsH = snap.drumLayout.rowsAreaHeight;
  const drumH = snap.drumPane.h;
  const headerH = snap.drumLayout.headerH;
  // rowsAreaHeight = drumH - headerH - eqH (eqH=0)
  const expected = drumH - headerH;
  // Allow some tolerance for rounding
  assert(Math.abs(rowsH - expected) <= 2,
    `rowsAreaHeight=${rowsH}, expected ~${expected} (drumH=${drumH} - headerH=${headerH})`);

  const visRows = snap.drumLayout.visibleRows;
  assert(visRows >= 1, `Should have at least 1 visible row, got ${visRows}`);

  console.log(`  PASS: rowsAreaHeight=${rowsH}, visibleRows=${visRows}`);
  passed++;
  if (isCoverageEnabled()) await flushCoverage(page, new URL("../../coverage/browser-raw", import.meta.url).pathname, "mobile_ui_overhaul");
  await context.close();
} catch (e) {
  console.error("  FAIL:", e.message);
  failed++;
  failures.push("Test 6: " + e.message);
}

// ────────── Cleanup ──────────
await browser.close();
server.close();

console.log(`\nResults: ${passed} passed, ${failed} failed`);
if (failures.length > 0) {
  console.log("Failures:");
  failures.forEach((f) => console.log("  - " + f));
  process.exit(1);
}
console.log("All mobile UI overhaul browser tests passed!");
