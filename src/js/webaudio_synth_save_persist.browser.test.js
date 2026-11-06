/**
 * Synth-tab Save / Reset — WASM↔bridge boundary.
 *
 * Charter (CLAUDE.md § JS Test Charter, tier 4 "WASM↔JS bridge plumbing"):
 * this verifies that the real Go Save / Reset logic, running in WASM, leaves
 * the param block audio.js renders from (window.setInstrumentBlock, spec
 * 2026-10-04 §8) in the right state. Logic correctness is owned by Go:
 *   - internal/ui/synth_panel_save_test.go, synth_panel_save_persist_test.go
 *       (Save writes the library and leaves the project copy untouched;
 *        Reset returns the copy to its library entry; Restore shipped + Reset
 *        brings back the shipped sound)
 *
 * The bug this guards: clicking Save used to push an empty param map to JS,
 * dropping the liked tone from the browser's voice cache. Save now copies the
 * project document into the library and pushes nothing — the block audio.js
 * holds must be bit-identical before and after Save.
 */

import { chromium } from "playwright";
import { spawnSync } from "child_process";
import http from "http";
import fs from "fs";
import path from "path";
import { fileURLToPath } from "url";
import {
  resolveGoBinary,
  shouldSkipWasmBuild,
} from "./browser_test_helpers.js";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const jsDir = __dirname;

const chromiumPath = path.join(jsDir, "node_modules", ".cache", "ms-playwright", "chromium");
if (!fs.existsSync(chromiumPath)) {
  spawnSync("npx", ["playwright", "install", "chromium"], { cwd: jsDir, stdio: "inherit" });
}

const goDir = path.resolve(jsDir, "../go");
const GO = resolveGoBinary();

if (!shouldSkipWasmBuild("play_ui.wasm")) {
  const build = spawnSync(
    GO,
    ["build", "-o", path.join(jsDir, "play_ui.wasm"), "./internal/ui/playtest"],
    { cwd: goDir, env: { ...process.env, GOOS: "js", GOARCH: "wasm" }, stdio: "inherit" },
  );
  if (build.status !== 0) throw new Error("go build play_ui failed");
}

const server = http.createServer((req, res) => {
  const file = req.url === "/" ? "/play_ui.html" : req.url;
  const fp = path.join(jsDir, file.replace(/^\//, ""));
  fs.readFile(fp, (err, data) => {
    if (err) {
      res.writeHead(404);
      res.end();
      return;
    }
    let ct = "text/plain";
    if (fp.endsWith(".html")) ct = "text/html";
    else if (fp.endsWith(".js")) ct = "application/javascript";
    else if (fp.endsWith(".wasm")) ct = "application/wasm";
    res.writeHead(200, { "Content-Type": ct });
    res.end(data);
  });
});
await new Promise((r) => server.listen(0, r));
const port = server.address().port;

const browser = await chromium.launch({ args: ["--autoplay-policy=no-user-gesture-required"] });
const page = await browser.newPage();
page.on("console", (msg) => {
  try {
    if (msg.type() === "error") console.log("[PAGE]", msg.type(), msg.text());
  } catch (_) {}
});

const failures = [];
function check(cond, msg) {
  if (!cond) failures.push(msg);
}
const approx = (a, b, eps = 1e-6) => Math.abs(a - b) <= eps;

try {
  await page.goto(`http://localhost:${port}/`);
  await page.waitForFunction(() => typeof ensureDefaultPath === "function");
  await page.evaluate(() => ensureDefaultPath());
  await page.waitForFunction(() => typeof saveActiveRecipe === "function");

  const INST = "kick";
  const K = "post_decay";
  // Drive the whole scenario inside one page.evaluate so the actual numeric
  // values cross the bridge (callExport only returns a shape descriptor).
  const r = await page.evaluate(({ inst, k }) => {
    const blockOf = () => Array.from((__instrumentBlockForTest(inst) || {}).block || []);
    resetActiveRecipe(inst); // clean baseline
    const orig = getInstrumentParams(inst)[k];
    const liked = orig > 0.5 ? 0.3 : 0.8; // a real change inside post_decay's [0,1]

    setInstrumentParam(inst, k, liked); // the knob drag
    const beforeSave = getInstrumentParams(inst)[k];
    const blockBeforeSave = blockOf();

    const savedID = saveActiveRecipe(inst); // the Save button
    const afterSave = getInstrumentParams(inst)[k];
    const blockAfterSave = blockOf();

    setInstrumentParam(inst, k, orig); // move it again, unsaved
    const resetID = resetActiveRecipe(inst); // the Reset button
    const afterReset = getInstrumentParams(inst)[k];
    const doc = JSON.parse(getInstrumentDoc(inst) || "{}");

    return {
      orig, liked, beforeSave, afterSave, afterReset, savedID, resetID,
      origin: doc.origin || "",
      blockLen: blockBeforeSave.length,
      blockSame: blockBeforeSave.length > 0 && blockBeforeSave.every((v, i) => Object.is(v, blockAfterSave[i])),
    };
  }, { inst: INST, k: K });

  check(typeof r.orig === "number", `${INST} has no ${K} value`);
  check(approx(r.beforeSave, r.liked), `pre-save ${K}=${r.beforeSave}, want ${r.liked}`);
  check(r.savedID === INST, `saveActiveRecipe returned ${r.savedID}, want ${INST}`);

  // BUG GUARD (Save must not change the sound): the document and the block
  // audio.js renders from are unchanged by Save.
  check(approx(r.afterSave, r.liked), `post-save ${K}=${r.afterSave}, want ${r.liked}`);
  check(r.blockLen > 0, "no pushed block for the instrument (setInstrumentBlock never ran?)");
  check(r.blockSame, "Save changed the pushed param block — the browser voice would move");

  // Reset returns to the library entry, which after Save is the liked value.
  check(r.resetID === INST, `resetActiveRecipe returned ${r.resetID}, want ${INST}`);
  check(approx(r.afterReset, r.liked), `after Reset ${K}=${r.afterReset}, want the saved ${r.liked}`);
  check(r.origin === INST, `the project copy's origin is ${r.origin}, want ${INST}`);

  if (failures.length) {
    console.error("[synth-save-persist] FAILURES:\n  " + failures.join("\n  "));
    process.exitCode = 1;
  } else {
    console.log("[synth-save-persist] OK — Save leaves the pushed block untouched and Reset returns to the saved entry through the WASM bridge");
  }
} finally {
  await browser.close();
  server.close();
}
