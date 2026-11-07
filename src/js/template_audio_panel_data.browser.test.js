/** A template carries no per-instrument EQ, and on WASM a channel analyser is wired only when EQ'd or selected, so
 * templates showed no per-instrument data. An unwired analyser reads EXACTLY 0 forever, a wired one non-zero at some
 * point. COVERED-BY-GO: audio_panel_dispatcher_test.go TestAnalyzerInstrumentTapsForTab */

import { chromium } from "playwright";
import { spawnSync } from "child_process";
import path from "path";
import fs from "fs";
import { fileURLToPath } from "url";
import { startServer, initPage } from "./visual_test_helpers.js";
import { resolveGoBinary, waitForAudioReady } from "./browser_test_helpers.js";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const jsDir = __dirname;
const goDir = path.resolve(jsDir, "..", "go");

function buildWasmSafely() {
  if (process.env.WASM_PREBUILT === "1") {
    if (!fs.existsSync(path.join(jsDir, "main.wasm"))) throw new Error(`WASM_PREBUILT=1 but main.wasm missing`);
    return;
  }
  const GO = resolveGoBinary();
  const build = spawnSync(
    GO,
    ["build", "-ldflags", "-X main.defaultLog=INFO", "-o", path.join(jsDir, "main.wasm"), "./cmd"],
    { cwd: goDir, env: { ...process.env, GOOS: "js", GOARCH: "wasm" }, stdio: "inherit" }
  );
  if (build.status !== 0) throw new Error("go build main.wasm failed");
}

// No built-in template carries an "eq" block. Strings carry signal between onsets where percussion is mostly silent.
const TEMPLATE = "bach-brandenburg-3.json";
const TEMPLATE_INSTR = ["violin", "viola", "cello", "harpsichord"];
const templateJSON = fs.readFileSync(
  path.resolve(goDir, "internal/assets/templates", TEMPLATE), "utf8");

async function settle(page, ms) {
  for (let i = 0; i < 6; i++) {
    await page.evaluate(() => forceDraw?.());
    await page.waitForTimeout(40);
  }
  await page.waitForTimeout(ms);
}

console.log("Building WASM...");
buildWasmSafely();

const { server, port } = await startServer();
const browser = await chromium.launch({ args: ["--autoplay-policy=no-user-gesture-required"] });
const page = await initPage(browser, { width: 1280, height: 720 }, port);
page.on("console", (m) => { if (/panic/i.test(m.text())) console.log("  [page]", m.text()); });

let failures = 0;
try {
  // Audio unlock handshake.
  await page.evaluate(() => {
    document.dispatchEvent(new Event("pointerdown"));
    if (typeof resumeAudio === "function") resumeAudio();
  });
  await page.waitForFunction(
    () => window.__audioCtx && window.__audioCtx.state === "running",
    {}, { timeout: 10000 });
  await waitForAudioReady(page);

  // The user's flow: Levels open, played through the sequencer.
  const importErr = await page.evaluate((json) => {
    try { return importJSON(json); } catch (e) { return String(e); }
  }, templateJSON);
  if (importErr) { console.error("FAIL: importJSON error:", importErr); failures++; }
  await page.evaluate(() => { forceDraw?.(); });
  await page.evaluate(() => runScene?.("crop_eq_tab_levels"));
  await settle(page, 300);

  // Hits are intermittent, so the max per instrument is kept.
  const maxPeak = Object.fromEntries(TEMPLATE_INSTR.map((id) => [id, 0]));
  let masterLive = false;
  for (let i = 0; i < 50; i++) {
    await page.evaluate(() => forceDraw?.());
    const sample = await page.evaluate((ids) => {
      const out = { instr: {} };
      for (const id of ids) {
        let p = 0;
        try { const s = window.channelAnalyzerSnapshot(id); if (s) p = s.peak || 0; } catch (_) {}
        out.instr[id] = p;
      }
      try {
        const st = typeof probeAnalyzerState === "function" ? probeAnalyzerState() : null;
        out.masterActive = !!(st && st.masterActive && st.masterWaveformLen > 0);
      } catch (_) { out.masterActive = false; }
      return out;
    }, TEMPLATE_INSTR);
    if (sample.masterActive) masterLive = true;
    for (const id of TEMPLATE_INSTR) {
      if (sample.instr[id] > maxPeak[id]) maxPeak[id] = sample.instr[id];
    }
    await page.waitForTimeout(80);
  }

  console.log("master live (audio flowing):", masterLive);
  console.log("per-instrument max peak:", JSON.stringify(maxPeak));

  // Otherwise the per-instrument check passes or fails for the wrong reason.
  if (!masterLive) {
    console.error("FAIL: master analyser saw no signal — audio never played (test setup issue)");
    failures++;
  }

  // Non-zero vs exactly zero: quiet instruments are legitimately low (a hi-hat ~1.2e-4), so the epsilon sits just above
  // float dust.
  const SIGNAL = 1e-5;
  const silent = TEMPLATE_INSTR.filter((id) => maxPeak[id] <= SIGNAL);
  if (silent.length > 0) {
    console.error(
      `FAIL: ${silent.length}/${TEMPLATE_INSTR.length} template instruments have a SILENT ` +
      `per-instrument analyser while audible: ${silent.join(", ")}`);
    failures++;
  } else {
    console.log("PASS: all template instruments report live per-instrument analyser data");
  }
} finally {
  await page.close();
  await browser.close();
  server.close();
}

if (failures > 0) { console.error(`\n${failures} check(s) failed`); process.exit(1); }
console.log("\nAll checks passed.");
