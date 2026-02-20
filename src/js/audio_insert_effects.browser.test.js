// audio_insert_effects.browser.test.js
//
// Tests insert effects lifecycle via Go WASM JS exports.
// Verifies:
//   1. Add an insert effect, verify it appears in the chain
//   2. Toggle effect on/off
//   3. Set effect parameter, verify round-trip
//   4. Add multiple effects, verify ordering
//   5. Remove an effect
//
// Usage: GO=/path/to/.tools/go/bin/go node src/js/audio_insert_effects.browser.test.js

import { chromium } from "playwright";
import http from "http";
import fs from "fs";
import path from "path";
import { spawnSync } from "child_process";
import { fileURLToPath } from "url";
import { resolveGoBinary, shouldSkipWasmBuild, flushCoverage, isCoverageEnabled } from "./browser_test_helpers.js";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const jsDir = __dirname;
const repoRoot = path.resolve(jsDir, "..", "..");
const goDir = path.resolve(repoRoot, "src/go");
const GO = resolveGoBinary();

// Build full WASM app.
if (!shouldSkipWasmBuild("main.wasm")) {
  const build = spawnSync(
    GO,
    ["build", "-ldflags", "-X main.defaultLog=INFO", "-o", path.join(jsDir, "main.wasm"), "./cmd/..."],
    { cwd: goDir, env: { ...process.env, GOOS: "js", GOARCH: "wasm" }, stdio: "inherit" }
  );
  if (build.status !== 0) throw new Error("go build main wasm failed");
}

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

let exitCode = 0;
let browser;
try {
  browser = await chromium.launch({ args: ["--autoplay-policy=no-user-gesture-required"] });
  const page = await browser.newPage();
  await page.goto(`http://localhost:${port}/`);
  // Wait for both Go WASM exports and audio.js exports.
  await page.waitForFunction(() =>
    typeof addInsertEffectJS === "function" &&
    typeof removeInsertEffectJS === "function" &&
    typeof toggleInsertEffectJS === "function" &&
    typeof setInsertEffectParamJS === "function" &&
    typeof getInsertEffectsJS === "function" &&
    typeof moveInsertEffectJS === "function" &&
    typeof insertEffectCatalogJS === "function"
  );

  // Unlock audio context.
  await page.evaluate(() => {
    document.dispatchEvent(new Event("pointerdown"));
    resumeAudio?.();
  });
  await page.waitForTimeout(100);
  await page.waitForFunction(
    () => window.__audioCtx && window.__audioCtx.state === "running",
    {},
    { timeout: 10000 }
  );
  await page.evaluate(() => window.audioReady);

  // Settle: let the game loop finish buildDemo / initial import.
  await page.waitForTimeout(500);

  // Clean slate: remove any insert effects the demo build may have left.
  await page.evaluate(() => {
    const json = getInsertEffectsJS("kick");
    const slots = (JSON.parse(json || "[]")) || [];
    for (let i = slots.length - 1; i >= 0; i--) {
      removeInsertEffectJS("kick", i);
    }
  });

  // ========================================================================
  // Scenario 1: Add an insert effect, verify it appears in the chain
  // ========================================================================
  console.log("--- Scenario 1: Add insert effect ---");

  const s1 = await page.evaluate(() => {
    const beforeJSON = getInsertEffectsJS("kick");
    const before = JSON.parse(beforeJSON || "[]") || [];

    const slotIdx = addInsertEffectJS("kick", "distortion");

    const afterJSON = getInsertEffectsJS("kick");
    const after = JSON.parse(afterJSON);

    return { beforeLen: before.length, slotIdx, afterLen: after.length, afterType: after.length > 0 ? after[after.length - 1].type : null };
  });

  if (s1.beforeLen !== 0) throw new Error(`Scenario 1 FAIL: expected 0 initial effects, got ${s1.beforeLen}`);
  if (s1.slotIdx < 0) throw new Error(`Scenario 1 FAIL: addInsertEffectJS returned negative index ${s1.slotIdx}`);
  if (s1.afterLen !== 1) throw new Error(`Scenario 1 FAIL: expected 1 effect after add, got ${s1.afterLen}`);
  if (s1.afterType !== "distortion") throw new Error(`Scenario 1 FAIL: expected type distortion, got ${s1.afterType}`);
  console.log("  PASS");

  // ========================================================================
  // Scenario 2: Toggle effect on/off
  // ========================================================================
  console.log("--- Scenario 2: Toggle effect on/off ---");

  const s2 = await page.evaluate(() => {
    // Effect should be enabled by default.
    const effectsBeforeJSON = getInsertEffectsJS("kick");
    const effectsBefore = JSON.parse(effectsBeforeJSON);
    const enabledBefore = effectsBefore.length > 0 ? effectsBefore[0].enabled : null;

    // Toggle off
    toggleInsertEffectJS("kick", 0, false);
    const afterOffJSON = getInsertEffectsJS("kick");
    const afterOff = JSON.parse(afterOffJSON);
    const enabledOff = afterOff.length > 0 ? afterOff[0].enabled : null;

    // Toggle back on
    toggleInsertEffectJS("kick", 0, true);
    const afterOnJSON = getInsertEffectsJS("kick");
    const afterOn = JSON.parse(afterOnJSON);
    const enabledOn = afterOn.length > 0 ? afterOn[0].enabled : null;

    return { enabledBefore, enabledOff, enabledOn };
  });

  if (s2.enabledBefore !== true) throw new Error(`Scenario 2 FAIL: expected enabled=true initially, got ${s2.enabledBefore}`);
  if (s2.enabledOff !== false) throw new Error(`Scenario 2 FAIL: expected enabled=false after toggle off, got ${s2.enabledOff}`);
  if (s2.enabledOn !== true) throw new Error(`Scenario 2 FAIL: expected enabled=true after toggle on, got ${s2.enabledOn}`);
  console.log("  PASS");

  // ========================================================================
  // Scenario 3: Set effect parameter, verify round-trip
  // ========================================================================
  console.log("--- Scenario 3: Set effect parameter ---");

  const s3 = await page.evaluate(() => {
    // Read current params
    const beforeJSON = getInsertEffectsJS("kick");
    const before = JSON.parse(beforeJSON);
    const driveBefore = before.length > 0 && before[0].params ? before[0].params.drive : undefined;

    // Set drive param
    setInsertEffectParamJS("kick", 0, "drive", 8.5);

    const afterJSON = getInsertEffectsJS("kick");
    const after = JSON.parse(afterJSON);
    const driveAfter = after.length > 0 && after[0].params ? after[0].params.drive : undefined;

    return { driveBefore, driveAfter };
  });

  // Drive should have changed to the new value
  if (s3.driveAfter === undefined) throw new Error(`Scenario 3 FAIL: drive param not found after set`);
  if (Math.abs(s3.driveAfter - 8.5) > 0.01) throw new Error(`Scenario 3 FAIL: expected drive=8.5, got ${s3.driveAfter}`);
  console.log("  PASS");

  // ========================================================================
  // Scenario 4: Add multiple effects, verify ordering
  // ========================================================================
  console.log("--- Scenario 4: Multiple effects ordering ---");

  const s4 = await page.evaluate(() => {
    // Already have distortion at slot 0 from scenario 1.
    const idx2 = addInsertEffectJS("kick", "delay");
    const idx3 = addInsertEffectJS("kick", "filter");

    const afterJSON = getInsertEffectsJS("kick");
    const after = JSON.parse(afterJSON);

    const types = after.map(e => e.type);
    const count = after.length;

    // Move filter (slot 2) to slot 0
    moveInsertEffectJS("kick", 2, 0);

    const movedJSON = getInsertEffectsJS("kick");
    const moved = JSON.parse(movedJSON);
    const movedTypes = moved.map(e => e.type);

    return { count, types, idx2, idx3, movedTypes };
  });

  if (s4.count !== 3) throw new Error(`Scenario 4 FAIL: expected 3 effects, got ${s4.count}`);
  // Check initial ordering
  if (s4.types[0] !== "distortion") throw new Error(`Scenario 4 FAIL: expected slot 0 = distortion, got ${s4.types[0]}`);
  if (s4.types[1] !== "delay") throw new Error(`Scenario 4 FAIL: expected slot 1 = delay, got ${s4.types[1]}`);
  if (s4.types[2] !== "filter") throw new Error(`Scenario 4 FAIL: expected slot 2 = filter, got ${s4.types[2]}`);
  // Check after move: filter should be at slot 0
  if (s4.movedTypes[0] !== "filter") throw new Error(`Scenario 4 FAIL: expected slot 0 = filter after move, got ${s4.movedTypes[0]}`);
  console.log("  PASS");

  // ========================================================================
  // Scenario 5: Remove an effect
  // ========================================================================
  console.log("--- Scenario 5: Remove an effect ---");

  const s5 = await page.evaluate(() => {
    const beforeJSON = getInsertEffectsJS("kick");
    const before = JSON.parse(beforeJSON);
    const countBefore = before.length;

    // Remove the first effect (filter, after the move in scenario 4)
    removeInsertEffectJS("kick", 0);

    const afterJSON = getInsertEffectsJS("kick");
    const after = JSON.parse(afterJSON);
    const countAfter = after.length;
    const remainingTypes = after.map(e => e.type);

    // Remove all remaining
    while (true) {
      const json = getInsertEffectsJS("kick");
      const arr = JSON.parse(json);
      if (arr.length === 0) break;
      removeInsertEffectJS("kick", 0);
    }

    const finalJSON = getInsertEffectsJS("kick");
    const final = JSON.parse(finalJSON);

    return { countBefore, countAfter, remainingTypes, finalCount: final.length };
  });

  if (s5.countBefore !== 3) throw new Error(`Scenario 5 FAIL: expected 3 effects before remove, got ${s5.countBefore}`);
  if (s5.countAfter !== 2) throw new Error(`Scenario 5 FAIL: expected 2 effects after remove, got ${s5.countAfter}`);
  // filter was at slot 0; after removing it, distortion and delay should remain
  if (!s5.remainingTypes.includes("distortion")) throw new Error(`Scenario 5 FAIL: distortion should remain after removing filter`);
  if (!s5.remainingTypes.includes("delay")) throw new Error(`Scenario 5 FAIL: delay should remain after removing filter`);
  if (s5.finalCount !== 0) throw new Error(`Scenario 5 FAIL: expected 0 effects after removing all, got ${s5.finalCount}`);
  console.log("  PASS");

  // ========================================================================
  // Bonus: Verify effect catalog is available
  // ========================================================================
  console.log("--- Bonus: Effect catalog ---");
  const catalog = await page.evaluate(() => {
    const json = insertEffectCatalogJS();
    return JSON.parse(json);
  });
  const catalogTypes = Object.keys(catalog);
  if (catalogTypes.length < 4) throw new Error(`Catalog FAIL: expected at least 4 effect types, got ${catalogTypes.length}`);
  console.log(`  Effect catalog has ${catalogTypes.length} types: ${catalogTypes.join(", ")}`);
  console.log("  PASS");

  console.log("\nAll insert effects tests passed.");
  if (isCoverageEnabled()) await flushCoverage(page, new URL("../../coverage/browser-raw", import.meta.url).pathname, "audio_insert_effects");
} catch (err) {
  console.error("FAIL:", err.message);
  exitCode = 1;
} finally {
  if (browser) await browser.close();
  server.close();
}

process.exit(exitCode);
