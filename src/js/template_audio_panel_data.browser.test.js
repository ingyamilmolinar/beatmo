/**
 * Template-load audio-panel data (WASM regression).
 *
 * Bug: loading a template circuit (which ships WITHOUT per-instrument
 * EQ blocks) and playing it produced audible audio but the audio-panel
 * tabs showed NO per-instrument data on WASM, while desktop was fine.
 *
 * Root cause: on WASM each per-instrument WebAudio AnalyserNode is only
 * wired when that instrument is EQ'd (applyRowEQ) or selected as the EQ
 * channel. The startup demo EQ's every instrument so its analysers are
 * wired; templates don't, so their per-instrument analysers stayed
 * unwired → channelAnalyzerSnapshot(id) returned all-zero even while the
 * master (summed) analyser saw signal. Desktop is unaffected because the
 * mixer's analyzer Service feeds every instrument slot unconditionally.
 *
 * COVERED-BY-GO (dispatcher rule unit): src/go/internal/ui/
 *   audio_panel_dispatcher_test.go TestAnalyzerInstrumentTapsForTab.
 * This file verifies the live WASM↔WebAudio integration.
 *
 * The discriminator is robust to scheduling jitter: an UNWIRED analyser
 * returns EXACTLY 0 forever; a wired analyser on an audible instrument
 * reports a non-zero peak at some point in the polling window.
 */

import { chromium } from "playwright";
import { spawnSync } from "child_process";
import path from "path";
import fs from "fs";
import { fileURLToPath } from "url";
import { startServer, initPage } from "./visual_test_helpers.js";
import { resolveGoBinary } from "./browser_test_helpers.js";

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

// rock.json ships no per-instrument "eq" blocks — the exact shape that
// left the analysers unwired. Its instruments:
const TEMPLATE = "rock.json";
const TEMPLATE_INSTR = ["kick-tight", "snare", "hihat", "crash", "fm-bass"];
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
  await page.evaluate(() => window.audioReady);

  // Load the template, open the Levels tab (shows per-instrument
  // strips), and play via the SEQUENCER — exactly the user flow.
  const importErr = await page.evaluate((json) => {
    try { return importJSON(json); } catch (e) { return String(e); }
  }, templateJSON);
  if (importErr) { console.error("FAIL: importJSON error:", importErr); failures++; }
  await page.evaluate(() => { forceDraw?.(); });
  await page.evaluate(() => runScene?.("crop_eq_tab_levels"));
  await settle(page, 300);

  // Poll over ~4s. Accumulate the max peak per instrument (sequencer
  // hits are intermittent), plus the master to prove audio is flowing.
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

  // Sanity: audio must actually be playing, else the per-instrument
  // check would pass/fail for the wrong reason.
  if (!masterLive) {
    console.error("FAIL: master analyser saw no signal — audio never played (test setup issue)");
    failures++;
  }

  // The fix: every audible template instrument must report a live
  // per-instrument analyser (NON-ZERO peak). Pre-fix every value is EXACTLY 0
  // (the analyser node is never wired into the graph), so the only robust
  // discriminator is "non-zero vs exactly zero". Use a tiny epsilon well above
  // float dust but far below any real signal: quiet/transient instruments are
  // legitimately low (e.g. a hi-hat reads ~1.2e-4, ~20x quieter than the kick
  // at ~2.3e-3) yet are clearly wired — the old 0.0005 cutoff false-failed them
  // even though they are not the unwired-analyser bug this test guards.
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
