/**
 * Synth-tab footer hit routing + Save As dialog — end-to-end through the
 * real WASM ↔ WebAudio bridge.
 *
 * Mirrors the Go HitArea dispatch tests in synth_panel_footer_hit_test.go
 * (Reset / Save / Save As route to the correct handler on click) and the
 * dialog flow in synth_panel_save_as_dialog_test.go (Save As opens the
 * inline modal, the user can type a name, OK persists, Cancel discards).
 *
 * The user-visible bug this guards against: clicking the footer buttons
 * triggered other controls because the EQ tab's hit areas stayed live on
 * the Synth tab and the synth footer registration order made OUT-column
 * launchers win on overlap. Fixed by (a) gating EQ-tab hit areas behind
 * activeTab==TabEQ in rebuildHitAreas, (b) bumping synth-footer ZIndex
 * to z+1, (c) clamping OUT-column link rects to their column bounds,
 * (d) migrating all synth-tab clickables to shared *Button chrome.
 *
 * COVERED-BY-GO:
 *   - internal/ui/synth_panel_footer_hit_test.go
 *   - internal/ui/synth_panel_save_as_dialog_test.go
 *   - internal/ui/synth_panel_shared_widgets_test.go
 *   - internal/ui/synth_panel_dirty_indicator_test.go
 * This browser test focuses on the JS↔WASM bridge surface only — that
 * the new exports (synthFooterButtonRects, synthSaveAsDialogState,
 * synthSaveAsDialogSetValue, synthSaveAsDialogConfirm, …) wire up to
 * the same DrumView entry points the Go tests exercise.
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

const chromiumPath = path.join(jsDir, "node_modules", ".cache", "ms-playwright", "chromium");
if (!fs.existsSync(chromiumPath)) {
  spawnSync("npx", ["playwright", "install", "chromium"], { cwd: jsDir, stdio: "inherit" });
}

const server = http.createServer((req, res) => {
  const file = req.url === "/" ? "/index.html" : req.url;
  const fp = path.join(jsDir, file.replace(/^\//, ""));
  fs.readFile(fp, (err, data) => {
    if (err) { res.writeHead(404); res.end(); return; }
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

const pageErrors = [];
let exitCode = 0;
let browser;

const clickCanvasAt = async (page, canvasBox, x, y) => {
  const sx = canvasBox.x + x;
  const sy = canvasBox.y + y;
  await page.mouse.move(sx, sy);
  await page.mouse.down();
  await page.waitForTimeout(30);
  await page.mouse.up();
};

try {
  browser = await chromium.launch({ args: ["--autoplay-policy=no-user-gesture-required"] });
  const page = await browser.newPage();
  page.on("pageerror", (err) => {
    pageErrors.push(String(err));
    console.log("[PAGE-ERROR]", String(err));
  });
  page.on("console", (msg) => {
    if (msg.type() === "error") {
      console.log("[PAGE]", msg.type(), msg.text());
    }
  });

  await page.goto(`http://localhost:${port}/`);
  await page.waitForFunction(() =>
    typeof setEQTab === "function" &&
    typeof forceDraw === "function" &&
    typeof setInstrumentParam === "function" &&
    typeof synthFooterButtonRects === "function" &&
    typeof synthSaveAsDialogState === "function" &&
    typeof synthSaveAsDialogSetValue === "function" &&
    typeof synthSaveAsDialogConfirm === "function" &&
    typeof synthSaveAsDialogCancel === "function" &&
    typeof getInstrumentParams === "function" &&
    typeof fullLayoutSnapshot === "function" &&
    typeof totalRows === "function" &&
    typeof rowInstrument === "function"
  );

  // Switch to Synth tab + force layout so footer rects are populated.
  await page.evaluate(() => {
    setEQTab("synth");
    forceDraw();
  });
  await page.waitForTimeout(300);

  const canvasBox = await page.evaluate(() => {
    const c = document.querySelector("canvas");
    if (!c) return null;
    const r = c.getBoundingClientRect();
    return { x: r.x, y: r.y, width: r.width, height: r.height };
  });
  if (!canvasBox) throw new Error("no canvas in DOM");

  // Edit a knob on whatever instrument the synth tab currently
  // shows (the startup demo's first row). We can't assume "snare" —
  // the active row depends on startup_demo.json content.
  // The footer Save/Reset buttons operate on the synth tab's ACTIVE instrument
  // (Go: DrumView.synthTabActiveInstrument = EQ ActiveChannel, else the first
  // non-empty drum row). We MUST target that exact instrument — Reset clears the
  // ACTIVE instrument's overlay, so editing any other id leaves the overlay
  // uncleared and the assertion fails. Mirror the Go resolver instead of
  // guessing from a hardcoded id list (which picked the wrong row while the
  // real active row was dnb-kick, so Reset never cleared our edit).
  const activeInst = await page.evaluate(() => {
    let ch = "";
    try {
      const snap = (typeof fullLayoutSnapshot === "function") ? fullLayoutSnapshot() : null;
      ch = (snap && snap.state && snap.state.channel) || "";
    } catch (_) {}
    if (ch && ch !== "main") return ch;
    const n = (typeof totalRows === "function") ? totalRows() : 0;
    for (let i = 0; i < n; i++) {
      const id = (typeof rowInstrument === "function") ? rowInstrument(i) : "";
      if (id) return id;
    }
    return "";
  });
  if (!activeInst) {
    throw new Error("no active synth instrument — can't drive the synth tab");
  }
  console.log("[TEST] active synth instrument:", activeInst);
  // post_decay lives in the FINISH stage every instrument document carries.
  // Reset's target is the value the open CIRCUIT carried (read it, never
  // assume the hand-edited startup demo's); the edits must differ from it.
  const circuitDecay = await page.evaluate((inst) => getInstrumentParams(inst).post_decay, activeInst);
  const [SAVED, EDITED] = [0.4, 0.7].map((v) => (v === circuitDecay ? v + 0.05 : v));
  await page.evaluate(([inst, v]) => {
    setInstrumentParam(inst, "post_decay", v);
    forceDraw();
  }, [activeInst, SAVED]);
  await page.waitForTimeout(120);

  const footerRects = await page.evaluate(() => synthFooterButtonRects());
  console.log("[TEST] footer rects:", JSON.stringify(footerRects));
  for (const key of ["save", "saveAs", "reset"]) {
    const r = footerRects[key];
    if (!r || r.w <= 0 || r.h <= 0) {
      throw new Error(`synthFooterButtonRects().${key} missing or empty: ${JSON.stringify(r)}`);
    }
  }

  // ─── Click Save As — dialog should open ───
  const sa = footerRects.saveAs;
  await clickCanvasAt(page, canvasBox, sa.x + sa.w / 2, sa.y + sa.h / 2);
  await page.waitForTimeout(120);
  let dlg = await page.evaluate(() => synthSaveAsDialogState());
  if (!dlg) {
    throw new Error("Save As click did not open the dialog (synthSaveAsDialogState() == null)");
  }
  console.log("[TEST] dialog opened, suggested name:", dlg.value);
  if (!dlg.value || dlg.value.length === 0) {
    throw new Error(`dialog opened with empty suggested name: ${JSON.stringify(dlg)}`);
  }

  // ─── Type a name, confirm ───
  const typed = "Browser-Test Punchy Snare";
  await page.evaluate((s) => synthSaveAsDialogSetValue(s), typed);
  await page.evaluate(() => synthSaveAsDialogConfirm());
  await page.waitForTimeout(120);
  dlg = await page.evaluate(() => synthSaveAsDialogState());
  if (dlg !== null) {
    throw new Error(`dialog still open after confirm: ${JSON.stringify(dlg)}`);
  }
  // Confirm Save As created a user.* instrument: the active row's document
  // is repointed at it (id pattern user.<slug>.<content hash>).
  const newID = await page.evaluate((inst) => (JSON.parse(getInstrumentDoc(inst) || "{}").origin || ""), activeInst);
  if (!newID.startsWith("user.")) {
    throw new Error(`Save As did not repoint the document at a new user.* instrument (origin ${JSON.stringify(newID)})`);
  }
  console.log("[TEST] Save As created user instrument:", newID);

  // ─── Click Reset — back to what the circuit carried ───
  // After Save As the row's document equals the new user instrument. Edit
  // again (the row keeps its id: only the document's origin moved, so we keep
  // using activeInst); Reset restores the document the open circuit carried,
  // not the Save As (synth_save_trumps_circuit_test.go).
  await page.evaluate(([inst, v]) => {
    setInstrumentParam(inst, "post_decay", v);
    forceDraw();
  }, [activeInst, EDITED]);
  await page.waitForTimeout(60);
  const beforeReset = await page.evaluate((inst) => getInstrumentParams(inst), activeInst);
  if (!beforeReset || beforeReset.post_decay !== EDITED) {
    throw new Error(`edit didn't take: post_decay=${beforeReset && beforeReset.post_decay}`);
  }
  // Re-read footer rects in case dirty-state spec swap moved them.
  const fr2 = await page.evaluate(() => synthFooterButtonRects());
  const rs = fr2.reset;
  await clickCanvasAt(page, canvasBox, rs.x + rs.w / 2, rs.y + rs.h / 2);
  await page.waitForTimeout(120);
  const afterReset = await page.evaluate((inst) => getInstrumentParams(inst), activeInst);
  const decayAfter = afterReset && afterReset.post_decay;
  if (decayAfter !== circuitDecay) {
    throw new Error(`Reset click did not return to the circuit's value: post_decay=${decayAfter}, want ${circuitDecay} (saved-as ${SAVED}, edited ${EDITED})`);
  }
  const originAfter = await page.evaluate((inst) => JSON.parse(getInstrumentDoc(inst) || "{}").origin || "", activeInst);
  if (originAfter === newID) {
    throw new Error(`Reset kept the Save As identity (origin ${originAfter}); it must restore the circuit's document`);
  }
  console.log("[TEST] Reset returned to the circuit's document; post_decay:", decayAfter, "origin:", originAfter);

  // ─── Click Save As again, then Cancel — dialog must close, nothing saved ───
  await page.evaluate((inst) => {
    setInstrumentParam(inst, "post_decay", 0.55);
    forceDraw();
  }, activeInst);
  await page.waitForTimeout(60);
  const fr3 = await page.evaluate(() => synthFooterButtonRects());
  await clickCanvasAt(page, canvasBox, fr3.saveAs.x + fr3.saveAs.w / 2, fr3.saveAs.y + fr3.saveAs.h / 2);
  await page.waitForTimeout(120);
  let dlg2 = await page.evaluate(() => synthSaveAsDialogState());
  if (!dlg2) throw new Error("dialog did not open second time");
  await page.evaluate(() => synthSaveAsDialogCancel());
  await page.waitForTimeout(60);
  dlg2 = await page.evaluate(() => synthSaveAsDialogState());
  if (dlg2 !== null) {
    throw new Error(`dialog still open after cancel: ${JSON.stringify(dlg2)}`);
  }
  console.log("[TEST] Save As → Cancel: dialog dismissed without saving");

  if (pageErrors.length > 0) {
    throw new Error(`page emitted ${pageErrors.length} errors during the test:\n  ${pageErrors.join("\n  ")}`);
  }

  console.log("[TEST] ✓ synth panel footer + Save As dialog routing OK");
} catch (err) {
  console.error("[TEST FAIL]", err);
  exitCode = 1;
} finally {
  if (browser) await browser.close();
  server.close();
  process.exit(exitCode);
}
