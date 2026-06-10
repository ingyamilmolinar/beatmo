// Observation harness: reproduce "row-rack controls dead after Sampler->Pads"
// in a DESKTOP browser at a MOBILE (narrow) viewport with PLAIN MOUSE
// (no touch emulation) — matching the user's report.
//
// Run: GO=$(pwd)/.tools/go/bin/go WASM_PREBUILT=1 node src/js/zz_rowrack_sampler_pads.browser.test.js
import { chromium } from "playwright";
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
  const build = spawnSync(GO, ["build", "-o", path.join(jsDir, "main.wasm"), "./cmd/..."],
    { cwd: goDir, env: { ...process.env, GOOS: "js", GOARCH: "wasm" }, stdio: "inherit" });
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

const W = 390, H = 780;
const browser = await chromium.launch({ args: ["--autoplay-policy=no-user-gesture-required"] });
const context = await browser.newContext({ viewport: { width: W, height: H }, hasTouch: false, isMobile: false });
const page = await context.newPage();
page.on("console", (m) => { const t = m.text(); if (t.includes("[diag]")) console.log("  page:", t); });
await page.goto(`http://localhost:${port}/`);
await page.waitForFunction(() => typeof forceDraw === "function" && typeof rowMuteBtnRect === "function" && typeof getBPM === "function");
await page.evaluate(() => forceDraw());
await page.waitForTimeout(300);

const activeTab = () => page.evaluate(() => (typeof dumpLevelsLatch === "function" ? dumpLevelsLatch().activeTab : -1));
const viewMode = () => page.evaluate(() => (typeof __viewMode === "function" ? __viewMode() : -1));
console.log("viewport:", W + "x" + H, "initial activeTab:", await activeTab(), "viewMode:", await viewMode());

// Use the REAL bottom-nav segment rect from the Go side.
async function tapSegment(i, label) {
  const r = await page.evaluate((seg) => (typeof __navSegRect === "function" ? __navSegRect(seg) : null), i);
  if (!r) { console.log(`tapSegment(${i} ${label}): no __navSegRect`); return null; }
  const x = Math.round(r.x + r.w / 2), y = Math.round(r.y + r.h / 2);
  await page.mouse.move(x, y);
  await page.mouse.down();
  await page.waitForTimeout(40);
  await page.mouse.up();
  await page.waitForTimeout(160);
  const vm = await viewMode();
  console.log(`tapSegment(${i} ${label}) rect=${JSON.stringify(r)} click=(${x},${y}) -> viewMode=${vm} activeTab=${await activeTab()}`);
  return vm;
}

async function clickMuteCheckToggle(stage) {
  const r = await page.evaluate(() => (window.rowMuteBtnRect ? rowMuteBtnRect(0) : null));
  if (!r || !r.w) { console.log(`[${stage}] no mute rect`); return null; }
  const before = await page.evaluate(() => rowMuted(0));
  const x = Math.round(r.x + r.w / 2), y = Math.round(r.y + r.h / 2);
  await page.mouse.move(x, y);
  await page.mouse.down();
  await page.waitForTimeout(40);
  await page.mouse.up();
  await page.waitForTimeout(160);
  const after = await page.evaluate(() => rowMuted(0));
  const ok = before !== after;
  console.log(`[${stage}] MUTE CLICK at (${x},${y}) vm=${await viewMode()}: ${before}->${after} => ${ok ? "TOGGLES (works)" : ">>> DEAD"}`);
  return ok;
}

async function probeRowControls(stage) {
  const out = await page.evaluate(() => {
    const res = { rects: {}, top: {}, domInputs: [], muteBefore: null };
    const fns = { label: "rowLabelRect", mute: "rowMuteBtnRect", solo: "rowSoloBtnRect", fx: "rowFXBtnRect" };
    for (const [k, fn] of Object.entries(fns)) {
      try {
        const r = window[fn] && window[fn](0);
        if (r && r.w) {
          res.rects[k] = r;
          const el = document.elementFromPoint(r.x + r.w / 2, r.y + r.h / 2);
          res.top[k] = el ? (el.tagName + (el.id ? "#" + el.id : "")) : "<none>";
        }
      } catch (e) { res.top[k] = "err:" + e.message; }
    }
    res.domInputs = [...document.querySelectorAll("input,textarea")].map((el) => ({
      tag: el.tagName, id: el.id, z: getComputedStyle(el).zIndex,
      r: el.getBoundingClientRect(), display: getComputedStyle(el).display,
    }));
    res.anyNativeInput = window.mobileInputAnyActive ? mobileInputAnyActive() : "n/a";
    res.muteBefore = window.rowMuted ? rowMuted(0) : null;
    return res;
  });
  console.log(`\n[${stage}] anyNativeInput=${out.anyNativeInput} muted(0)=${out.muteBefore}`);
  console.log(`  domInputs(${out.domInputs.length}):`, JSON.stringify(out.domInputs));
  for (const k of Object.keys(out.rects)) {
    console.log(`  ${k.padEnd(6)} rect=${JSON.stringify(out.rects[k])} elementOnTop=${out.top[k]}`);
  }
  return out;
}

// 0. CONTROL: mute click on fresh Pads (default view) must toggle.
const ctrlOk = await clickMuteCheckToggle("CONTROL fresh-Pads");
// reset mute back
await page.evaluate(() => { if (rowMuted(0)) toggleMute(0); forceDraw(); });
await page.waitForTimeout(100);

const vmFreshPads = await viewMode();

// 1. Sampler tab.
const vmSampler = await tapSegment(7, "Sampler");

const sampHA = await page.evaluate(() => (typeof __samplerHitAreas === "function" ? __samplerHitAreas() : []));
console.log("sampler hit areas:", JSON.stringify(sampHA.map((h) => h.tag)));
const find = (t) => sampHA.find((h) => h.tag === t);

const realDrag = async (r, dx, dy, label) => {
  const x = Math.round(r.x + r.w / 2), y = Math.round(r.y + r.h / 2);
  await page.mouse.move(x, y);
  await page.mouse.down();
  await page.waitForTimeout(40);
  for (let s = 1; s <= 4; s++) { await page.mouse.move(x + (dx * s) / 4, y + (dy * s) / 4); await page.waitForTimeout(25); }
  const cap = await page.evaluate(() => (typeof __treeCapturing === "function" ? __treeCapturing() : null));
  await page.mouse.up();
  await page.waitForTimeout(120);
  console.log(`  drag ${label}: midDragCapturing=${JSON.stringify(cap)} afterUp=${JSON.stringify(await page.evaluate(() => __treeCapturing()))}`);
};

// 2a. DRAG a knob (or trim handle) — the user's trigger.
const knob = find("sampler-knob-0") || find("sampler-knob-1");
const handle = find("sampler-handle-start") || find("sampler-handle-end");
if (knob) await realDrag(knob, 0, -30, "knob-0");
if (handle) await realDrag(handle, 25, 0, "handle-start");

// 2b. SAVE AS (the other trigger) — fresh rect, frame-stepped press/release.
const freshHA = await page.evaluate(() => __samplerHitAreas());
const saveAsBtn = freshHA.find((h) => h.tag === "sampler-save-as");
if (saveAsBtn) {
  const x = Math.round(saveAsBtn.x + saveAsBtn.w / 2), y = Math.round(saveAsBtn.y + saveAsBtn.h / 2);
  await page.mouse.move(x, y); await page.mouse.down(); await page.waitForTimeout(50); await page.mouse.up();
  let st = null;
  for (let i = 0; i < 10; i++) { await page.waitForTimeout(60); st = await page.evaluate(() => synthSaveAsDialogState()); if (st) break; }
  console.log(`  saveAs click (${x},${y}) -> dialogState:`, JSON.stringify(st));
  if (st) {
    // VARIANT: leave the Save As dialog OPEN and abandon it via the tab switch
    // (do NOT confirm). This mirrors "I started Save As, then tapped Pads".
    await page.evaluate(() => { synthSaveAsDialogSetValue("mychop"); forceDraw(); });
    await page.waitForTimeout(120);
    console.log("  Save As dialog left OPEN (abandoning via tab switch); dialogOpen=", await page.evaluate(() => synthSaveAsDialogState() != null));
  }
} else {
  console.log("  no sampler-save-as in hit areas");
}
console.log("  capturing before leaving Sampler:", JSON.stringify(await page.evaluate(() => __treeCapturing())));

// 3. Back to Pads.
const vmBackToPads = await tapSegment(0, "Pads");
console.log("  capturing on Pads:", JSON.stringify(await page.evaluate(() => __treeCapturing())));
await probeRowControls("after Sampler->Pads");

// 4. The repro check.
const reproOk = await clickMuteCheckToggle("after Sampler->Pads");

// ---- Gate evaluation ------------------------------------------------------
// We need to distinguish three outcomes and fail on two of them:
//   PASS  : controls are positively confirmed live AFTER the Sampler->Pads
//           transition (reproOk === true), AND we actually drove the scenario.
//   FAIL  (bug reproduced): controls were live on fresh Pads (ctrlOk) but DEAD
//           after the transition (reproOk === false).
//   FAIL  (broken/inconclusive harness): we could not toggle mute even on the
//           fresh control Pads (!ctrlOk), or the transition itself never
//           happened (viewMode never reached Sampler / never returned to Pads).
//           An inconclusive run must NOT count as a pass.
//
// viewMode encoding (mobile bottom-nav): segment 0 == Pads, segment 7 ==
// Sampler. We require vmSampler != vmFreshPads (we left Pads for Sampler) and
// vmBackToPads == vmFreshPads (we came back to the same Pads view), so a "PASS"
// genuinely exercised the cross-tab path rather than never leaving Pads.
const drove =
  vmFreshPads !== null && vmFreshPads !== -1 &&
  vmSampler !== null && vmSampler !== -1 &&
  vmBackToPads !== null && vmBackToPads !== -1 &&
  vmSampler !== vmFreshPads &&            // actually switched to Sampler
  vmBackToPads === vmFreshPads;           // actually returned to Pads

console.log(`\n=== RESULT: control=${ctrlOk ? "works" : "BROKEN-HARNESS"} afterSamplerPads=${reproOk ? "works" : "DEAD"} ===`);
console.log(`  viewModes: freshPads=${vmFreshPads} sampler=${vmSampler} backToPads=${vmBackToPads} droveTransition=${drove}`);

await browser.close();
server.close();

let exitCode = 0;
if (!ctrlOk) {
  console.log(">>> INCONCLUSIVE/BROKEN-HARNESS: could not toggle mute even on fresh Pads (clicks not landing). NOT a pass.");
  exitCode = 1;
} else if (!drove) {
  console.log(">>> INCONCLUSIVE/BROKEN-HARNESS: the Sampler->Pads transition was never driven (viewMode did not reach Sampler and return to Pads). NOT a pass.");
  exitCode = 1;
} else if (!reproOk) {
  console.log(">>> BUG REPRODUCED: mute works on fresh Pads but DEAD after Sampler->Pads. Row-rack controls are dead after the transition.");
  exitCode = 1;
} else {
  console.log(">>> PASS: row-rack mute control positively confirmed LIVE after Sampler->Pads (rowMuted toggled across the transition).");
  exitCode = 0;
}

console.log("\ndone");
process.exit(exitCode);
