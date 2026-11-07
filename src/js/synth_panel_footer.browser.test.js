// Synth-tab Save As, the only save, through the real WASM <-> WebAudio bridge:
// a click opens the prefilled dialog, a new name makes a new user instrument,
// the same name saves over it, and Cancel saves nothing.

// COVERED-BY-GO: internal/ui/save_as_only_test.go,
// synth_panel_footer_hit_test.go, synth_panel_save_as_dialog_test.go

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
    typeof getInstrumentDoc === "function" &&
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

  // Save As acts on the Synth tab's ACTIVE instrument (the EQ channel, else
  // the first non-empty row), so the test must edit that exact id.
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
  const originOf = (inst) => page.evaluate((i) => JSON.parse(getInstrumentDoc(i) || "{}").origin || "", inst);
  const decayOf = (inst) => page.evaluate((i) => getInstrumentParams(i).post_decay, inst);
  const setDecay = async (v) => {
    await page.evaluate(([inst, val]) => {
      setInstrumentParam(inst, "post_decay", val);
      forceDraw();
    }, [activeInst, v]);
    await page.waitForTimeout(60);
  };
  // Click Save As at its visible centre and return the dialog it opened.
  const openSaveAs = async (label) => {
    const rects = await page.evaluate(() => synthFooterButtonRects());
    const sa = rects.saveAs;
    if (!sa || sa.w <= 0 || sa.h <= 0) {
      throw new Error(`${label}: synthFooterButtonRects().saveAs missing or empty: ${JSON.stringify(rects)}`);
    }
    await clickCanvasAt(page, canvasBox, sa.x + sa.w / 2, sa.y + sa.h / 2);
    await page.waitForTimeout(120);
    const dlg = await page.evaluate(() => synthSaveAsDialogState());
    if (!dlg) throw new Error(`${label}: Save As click did not open the dialog`);
    return dlg;
  };

  const circuitDecay = await decayOf(activeInst);
  const [FIRST, SECOND] = [0.4, 0.7].map((v) => (v === circuitDecay ? v + 0.05 : v));
  await setDecay(FIRST);

  // Save As is the only save: the header carries no Save and no Reset.
  const footerRects = await page.evaluate(() => synthFooterButtonRects());
  console.log("[TEST] footer rects:", JSON.stringify(footerRects));
  for (const gone of ["save", "reset"]) {
    if (footerRects[gone]) {
      throw new Error(`synthFooterButtonRects().${gone} is present; Save As is the only save: ${JSON.stringify(footerRects)}`);
    }
  }

  // ─── Save As a new name → a new user.* instrument ───
  let dlg = await openSaveAs("first Save As");
  console.log("[TEST] dialog opened, prefilled name:", dlg.value);
  if (!dlg.value) {
    throw new Error(`dialog opened without the instrument's current name: ${JSON.stringify(dlg)}`);
  }
  const typed = "Browser-Test Punchy Snare";
  await page.evaluate((s) => synthSaveAsDialogSetValue(s), typed);
  await page.evaluate(() => synthSaveAsDialogConfirm());
  await page.waitForTimeout(120);
  dlg = await page.evaluate(() => synthSaveAsDialogState());
  if (dlg !== null) {
    throw new Error(`dialog still open after confirm: ${JSON.stringify(dlg)}`);
  }
  const newID = await originOf(activeInst);
  if (!newID.startsWith("user.")) {
    throw new Error(`Save As did not repoint the document at a new user.* instrument (origin ${JSON.stringify(newID)})`);
  }
  console.log("[TEST] Save As created user instrument:", newID);

  // ─── Save As again keeps the name → saves over that instrument ───
  await setDecay(SECOND);
  dlg = await openSaveAs("second Save As");
  if (dlg.value !== typed) {
    throw new Error(`Save As must be prefilled with the current name ${JSON.stringify(typed)}, got ${JSON.stringify(dlg.value)}`);
  }
  await page.evaluate(() => synthSaveAsDialogConfirm());
  await page.waitForTimeout(120);
  if ((await page.evaluate(() => synthSaveAsDialogState())) !== null) {
    throw new Error("dialog still open after saving over");
  }
  const originOver = await originOf(activeInst);
  if (originOver !== newID) {
    throw new Error(`saving over under the same name made another instrument: origin ${originOver}, want ${newID}`);
  }
  if ((await decayOf(activeInst)) !== SECOND) {
    throw new Error("saving over changed the instrument's sound");
  }
  console.log("[TEST] Save As under the same name saved over", newID);

  // ─── Save As → Cancel: dialog closes, nothing saved ───
  await setDecay(0.55);
  await openSaveAs("third Save As");
  await page.evaluate(() => synthSaveAsDialogCancel());
  await page.waitForTimeout(60);
  dlg = await page.evaluate(() => synthSaveAsDialogState());
  if (dlg !== null) {
    throw new Error(`dialog still open after cancel: ${JSON.stringify(dlg)}`);
  }
  if ((await originOf(activeInst)) !== newID) {
    throw new Error("Cancel changed the instrument the row's document comes from");
  }
  console.log("[TEST] Save As → Cancel: dialog dismissed without saving");

  if (pageErrors.length > 0) {
    throw new Error(`page emitted ${pageErrors.length} errors during the test:\n  ${pageErrors.join("\n  ")}`);
  }

  console.log("[TEST] ✓ synth panel Save As routing OK");
} catch (err) {
  console.error("[TEST FAIL]", err);
  exitCode = 1;
} finally {
  if (browser) await browser.close();
  server.close();
  process.exit(exitCode);
}
