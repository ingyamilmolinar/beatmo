/**
 * full_game_tab_audio.browser.test.js
 *
 * REGRESSION GUARD for "switching to an audio-panel tab shows no analyzer data
 * in the full WASM game."
 *
 * full_game_playback_audio.browser.test.js proves audio plays during playback
 * on the DEFAULT (Pads) view. master_analyzer_playback.browser.test.js proves
 * the master analyzer reports data — but it unlocks audio via resumeAudio(),
 * which happens to build the master chain BEFORE the analyzer is enabled, so it
 * never exercised the real-user-gesture ordering. Neither test switches tabs.
 *
 * The user-reported scenario: start playback, then "go to any of the tabs"
 * (Wave / Spectrum / Levels / EQ / Chain / Synth) → no analyzer data on any tab
 * and audio can't be heard, in WASM (desktop fine).
 *
 * STATUS: this scenario did NOT reproduce against the current branch in the
 * Playwright harness — under both forced-autoplay and real trusted-click unlock,
 * audio plays and every tab reports analyzer data. This test is committed as a
 * guard so any future regression of the play→switch-tab path fails loudly.
 *
 * This test runs the real flow against production index.html + main.wasm:
 * tap the real AudioContext destination, unlock with a real mouse click, start
 * the sequencer, then walk every audio-panel tab via setEQTab() and assert,
 * per tab, that (1) the master output stays NON-SILENT and (2) the master-
 * reading tabs report analyzer signal.
 *
 * COVERED-BY-GO: internal/ui/audio_panel_dispatcher_test.go proves the tab
 * dispatcher selects the right analyzers per tab; this owns the browser
 * end-to-end WASM↔WebAudio path (real gesture + lazy master chain) Go can't reach.
 */
import { chromium } from "playwright";
import { buildMainWasm, createServer } from "./real_input_test_helpers.js";

if (!buildMainWasm({ logLevel: "INFO" })) {
  throw new Error("go build main.wasm failed");
}

const server = await createServer();
const port = server.address().port;

const browser = await chromium.launch({
  args: ["--autoplay-policy=no-user-gesture-required"],
});
const page = await browser.newPage();
const pageErrors = [];
page.on("pageerror", (e) => pageErrors.push(String(e.message)));
page.on("console", (m) => {
  if (m.type() === "error") pageErrors.push("console.error: " + m.text());
});

// Tap the AudioContext destination BEFORE the page creates it so we measure the
// real output the user would hear, with a resettable per-window accumulator.
await page.addInitScript(() => {
  const RealAC = window.AudioContext || window.webkitAudioContext;
  class TestAC extends RealAC {
    constructor(opts) {
      super(opts);
      const dest = super.destination;
      const sp = this.createScriptProcessor(1024, 1, 1);
      window.__rmsAccum = 0;
      window.__nonZero = 0;
      window.__frames = 0;
      window.__resetTap = () => { window.__rmsAccum = 0; window.__nonZero = 0; window.__frames = 0; };
      sp.addEventListener("audioprocess", (e) => {
        const d = e.inputBuffer.getChannelData(0);
        for (let i = 0; i < d.length; i++) {
          window.__rmsAccum += d[i] * d[i];
          if (d[i] !== 0) window.__nonZero++;
        }
        window.__frames += d.length;
      });
      sp.connect(dest);
      Object.defineProperty(this, "destination", { value: sp });
    }
  }
  window.AudioContext = TestAC;
  window.webkitAudioContext = TestAC;
});

await page.goto(`http://localhost:${port}/`, { waitUntil: "load" });
await page.waitForFunction(() => typeof startPlay === "function", { timeout: 30000 });
await page.waitForFunction(() => typeof setEQTab === "function", { timeout: 30000 });
await page.waitForFunction(() => window.audioReady !== undefined, { timeout: 30000 });

// Unlock audio with a real gesture (the actual user flow), then start playback.
await page.mouse.click(200, 200);
await page.evaluate(async () => { try { await window.audioReady; } catch (_) {} });
await page.evaluate(() => { try { document.dispatchEvent(new Event("mousedown")); } catch (_) {} });
await page.evaluate(() => startPlay());
await page.waitForTimeout(1200); // let the demo circuit warm up

// `arg` is the string setEQTab() switches on (js_exports_eq_widgets.go).
// readsMaster mirrors tabReadsMasterAnalyzer() in audio_panel_dispatcher.go:
// Wave/Spectrum/Levels(meters)/EQ/Chain(scope) tap the master ("main")
// analyzer; Synth reads its own per-voice preview and disables "main".
const TABS = [
  { arg: "wave", name: "Wave", readsMaster: true },
  { arg: "spectrum", name: "Spectrum", readsMaster: true },
  { arg: "meters", name: "Levels", readsMaster: true },
  { arg: "eq", name: "EQ", readsMaster: true },
  { arg: "scope", name: "Chain", readsMaster: true },
  { arg: "synth", name: "Synth", readsMaster: false },
];

const results = [];
for (const tab of TABS) {
  // Switch tab + enable its analyzer (mirrors a user click on the tab pill).
  await page.evaluate((a) => { setEQTab(a); if (typeof forceDraw === "function") forceDraw(); }, tab.arg);
  // Reset the output tap and measure THIS tab's window.
  await page.evaluate(() => window.__resetTap && window.__resetTap());

  // Sample the analyzer peak repeatedly across the measurement window and
  // keep the max. channelAnalyzerSnapshot().peak is an INSTANTANEOUS
  // ~10ms time-domain window (fftSize 512 @ ~48 kHz); the demo circuit is a
  // sparse drum pattern, so any SINGLE snapshot can land in a silent gap
  // between hits and read ~0 even though the tap is correctly wired (the
  // destination RMS — accumulated over the whole 900ms window by the
  // ScriptProcessor tap — stays loud). Polling for the max mirrors how
  // __rmsAccum accumulates, so both signals cover the same window. A
  // structurally-dropped tap still fails hard: peak stays 0 across every
  // poll. Without this, the first tab visited (Wave) flaked at the 1e-3
  // boundary whenever its lone snapshot happened to fall between hits.
  const r = await page.evaluate(async () => {
    let analyzerPeak = 0;
    const deadline = performance.now() + 900;
    while (performance.now() < deadline) {
      try {
        if (typeof channelAnalyzerSnapshot === "function") {
          const s = channelAnalyzerSnapshot("main");
          if (s && typeof s.peak === "number" && s.peak > analyzerPeak) analyzerPeak = s.peak;
        }
      } catch (_) {}
      await new Promise((res) => setTimeout(res, 30));
    }
    const frames = window.__frames || 0;
    const rms = frames ? Math.sqrt(window.__rmsAccum / frames) : 0;
    return { frames, rms, nonZero: window.__nonZero || 0, analyzerPeak, isPlaying: typeof isPlaying === "function" ? isPlaying() : null };
  });
  results.push({ tab: tab.name, readsMaster: tab.readsMaster, ...r });
  console.log(`[TAB ${tab.name}] rms=${r.rms.toFixed(5)} nonZero=${r.nonZero} analyzerPeak=${Number(r.analyzerPeak).toFixed(5)} isPlaying=${r.isPlaying}`);
}

await page.evaluate(() => { try { stopPlay(); } catch (_) {} });
await browser.close();
server.close();

if (pageErrors.length) console.log("page errors:", pageErrors.slice(0, 10));

// Assertions: on EVERY tab audio must stay audible; the master-reading tabs
// must report analyzer signal (the user-reported bug zeroed all of them).
const RMS_MIN = 1e-3;
const ANALYZER_MIN = 1e-3;
const failures = [];
if (pageErrors.length > 0) failures.push(`page raised ${pageErrors.length} error(s): ${pageErrors[0]}`);
for (const r of results) {
  if (r.isPlaying !== true) failures.push(`[${r.tab}] not playing (isPlaying=${r.isPlaying})`);
  if (r.rms < RMS_MIN) failures.push(`[${r.tab}] AUDIO SILENT during tab (rms=${r.rms.toFixed(6)}, expected >= ${RMS_MIN})`);
  if (r.readsMaster && r.analyzerPeak < ANALYZER_MIN) {
    failures.push(`[${r.tab}] NO ANALYZER DATA (peak=${Number(r.analyzerPeak).toFixed(6)}, expected >= ${ANALYZER_MIN}) — master analyzer tap dropped`);
  }
}

if (failures.length > 0) {
  console.error("full_game_tab_audio browser test FAILED:");
  for (const f of failures) console.error("  - " + f);
  process.exit(1);
}
console.log(`full_game_tab_audio browser test PASSED across ${results.length} tabs`);
