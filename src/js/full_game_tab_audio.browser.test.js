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

// Pre-warm EVERY analyzer the measurement loop will read BEFORE measuring
// the first tab, by driving one real priming tab switch through the
// production dispatcher (setEQTab → EnsureAnalyzersForTab).
//
// The dispatcher lazily creates, on the FIRST switch into a master-reading
// tab, the master ("main") preEQ + synth taps AND one analyzer tap per row
// instrument (audio_panel_dispatcher.go AnalyzerInstrumentTapsForTab —
// the desktop↔WASM per-instrument parity wiring). Each creation calls
// rewireChannel(); that first-switch rewire storm momentarily tears down and
// rebuilds the master analyzer's connection. Under the headless software-GL
// fallback the Go Update loop / WebAudio render that re-fills it is starved,
// so the FIRST measured tab (Wave) could read a still-cold analyzer (~0) for
// the whole window even while destination output is loud, while every later
// tab — by then fully idempotent (no rewire) — reads warm. Bare
// enableChannelAnalyzer("main") did NOT prevent this: it creates only the
// master CHANNEL tap, leaving the preEQ/synth + per-instrument creation (and
// its rewire storm) for the first measured tab to absorb.
//
// Driving a real priming switch into a master tab creates the entire tap set
// the exact production way and lets the rewire storm settle; we then poll
// until "main" reports signal. Every tab in the loop below is therefore an
// idempotent switch measuring an already-warm tap. This removes the
// cold-first-tab race without weakening any per-tab floor — each tab still
// independently asserts analyzer signal, and the post-Synth recovery check
// still guards a genuine permanent kill.
await page.evaluate(() => { try { setEQTab("spectrum"); if (typeof forceDraw === "function") forceDraw(); } catch (_) {} });
await page.evaluate(async () => {
  const deadline = performance.now() + 3000;
  while (performance.now() < deadline) {
    try {
      const s = channelAnalyzerSnapshot("main");
      if (s && typeof s.peak === "number" && s.peak > 1e-3) return;
    } catch (_) {}
    await new Promise((res) => setTimeout(res, 30));
  }
});

// `arg` is the string setEQTab() switches on (js_exports_eq_widgets.go).
// readsMaster mirrors tabReadsMasterAnalyzer() in audio_panel_dispatcher.go:
// Wave/Spectrum/Levels(meters)/EQ/Chain(scope) tap the master ("main")
// analyzer; Synth reads its own per-voice preview and disables "main".
//
// softwareGLStarves: the Synth tab is the heaviest draw in the app. Under the
// software WebGL fallback (headless CI Chromium / SwiftShader, "GPU stall due to
// ReadPixels"), its per-frame draw cost starves the WebAudio rendering pipeline
// GLOBALLY — master output collapses ~100x while the tab renders and recovers
// the instant the tab closes (verified by probe bisect: NO AudioNode connect/
// disconnect, NO bridge mutation; in-graph AnalyserNodes see the same collapse,
// so it is the real rendered stream, not the ScriptProcessor tap). This is an
// environment artifact of software GL, NOT the user-reported bug. So for the
// Synth tab we do NOT apply the on-tab RMS floor — instead we (a) confirm the
// audio path is structurally alive while the tab renders (the real bug zeroes
// ALL output: nonZero ~= 0) and (b) confirm visiting Synth doesn't PERMANENTLY
// kill audio via a recovery measurement after switching back to a non-starving
// tab (see the post-loop block + synth_generator_solo_realinput.browser.test.js,
// which guards the same starvation the same way).
const TABS = [
  { arg: "wave", name: "Wave", readsMaster: true },
  { arg: "spectrum", name: "Spectrum", readsMaster: true },
  { arg: "meters", name: "Levels", readsMaster: true },
  { arg: "eq", name: "EQ", readsMaster: true },
  { arg: "scope", name: "Chain", readsMaster: true },
  { arg: "synth", name: "Synth", readsMaster: false, softwareGLStarves: true },
];

const results = [];
for (const tab of TABS) {
  // Switch tab + enable its analyzer (mirrors a user click on the tab pill).
  await page.evaluate((a) => { setEQTab(a); if (typeof forceDraw === "function") forceDraw(); }, tab.arg);
  // Reset the output tap and measure THIS tab's window.
  await page.evaluate(() => window.__resetTap && window.__resetTap());

  // Sample the analyzer peak repeatedly and keep the max.
  // channelAnalyzerSnapshot().peak is an INSTANTANEOUS ~10ms time-domain
  // window (fftSize 512 @ ~48 kHz); the demo circuit is a sparse drum
  // pattern, so any SINGLE snapshot can land in a silent gap between hits and
  // read ~0 even though the tap is correctly wired (the destination RMS —
  // accumulated over the whole window by the ScriptProcessor tap — stays
  // loud). Polling for the max mirrors how __rmsAccum accumulates, so both
  // signals cover the same window.
  //
  // BOUNDED WARM-UP, NOT A FIXED WINDOW: under the headless software-GL
  // fallback the master ("main") analyser — wired IN-LINE on the master bus
  // (audio.js rewireChannel) — can read a near-zero floor (~2e-4) for the
  // first ~1-2s of playback on the EARLY tabs while the WebAudio render /
  // per-instrument tap rewire storm settles, even though destination output
  // is already loud (same artifact class as the Synth-tab starvation below).
  // The single priming spike above is not enough: it confirms one transient
  // warm sample, after which the graph can still be mid-settle when the first
  // measured tab's window opens, and a FIXED 900ms window can land entirely
  // inside that cold gap (observed: Wave + Spectrum both stuck at ~2e-4 for
  // their whole windows, then every later tab warm). So instead of a fixed
  // window we always measure a base window (RMS / non-master guards intact),
  // then for a master-reading tab keep polling ONLY while the analyzer is
  // still cold, up to a generous warm-up deadline, early-exiting the instant
  // it crosses the floor. This absorbs the env settling delay WITHOUT
  // weakening the guard: a structurally-dropped / permanently-flat tap (the
  // real bug — "audio plays but the panel is flat") never crosses the floor
  // and fails after the full warm-up budget, and the post-Synth recovery
  // check still guards a genuine permanent kill.
  const ANALYZER_FLOOR = 1e-3;
  const BASE_WINDOW_MS = 900;   // RMS measurement window (also min analyzer sampling)
  const WARMUP_MAX_MS = 3500;   // extra budget for a cold master analyzer to warm under software GL
  const r = await page.evaluate(async ({ readsMaster, FLOOR, BASE_WINDOW_MS, WARMUP_MAX_MS }) => {
    let analyzerPeak = 0;
    const start = performance.now();
    const minDeadline = start + BASE_WINDOW_MS;
    const maxDeadline = start + WARMUP_MAX_MS;
    while (true) {
      try {
        if (typeof channelAnalyzerSnapshot === "function") {
          const s = channelAnalyzerSnapshot("main");
          if (s && typeof s.peak === "number" && s.peak > analyzerPeak) analyzerPeak = s.peak;
        }
      } catch (_) {}
      const now = performance.now();
      // Always measure the base window; past it, stop once the analyzer is
      // warm (or we don't care about it), or once the warm-up budget is spent.
      if (now >= minDeadline && (!readsMaster || analyzerPeak >= FLOOR || now >= maxDeadline)) break;
      await new Promise((res) => setTimeout(res, 30));
    }
    const frames = window.__frames || 0;
    const rms = frames ? Math.sqrt(window.__rmsAccum / frames) : 0;
    return { frames, rms, nonZero: window.__nonZero || 0, analyzerPeak, isPlaying: typeof isPlaying === "function" ? isPlaying() : null };
  }, { readsMaster: tab.readsMaster, FLOOR: ANALYZER_FLOOR, BASE_WINDOW_MS, WARMUP_MAX_MS });
  results.push({ tab: tab.name, readsMaster: tab.readsMaster, softwareGLStarves: !!tab.softwareGLStarves, ...r });
  console.log(`[TAB ${tab.name}] rms=${r.rms.toFixed(5)} nonZero=${r.nonZero} analyzerPeak=${Number(r.analyzerPeak).toFixed(5)} isPlaying=${r.isPlaying}`);
}

// RECOVERY measurement: we end the loop ON the Synth tab (the heaviest draw),
// whose per-frame cost starves WebAudio under software GL (see TABS comment).
// Switch back to a non-starving tab and confirm master output RECOVERS above the
// floor. This is the real guard for the Synth tab: the user-reported bug
// PERMANENTLY kills audio across tab switches, so a genuine regression stays
// silent here; the software-GL starvation lifts the moment the Synth tab closes.
const recovery = await (async () => {
  await page.evaluate(() => { setEQTab("eq"); if (typeof forceDraw === "function") forceDraw(); });
  await page.waitForTimeout(400); // let the starved draw pipeline recover
  await page.evaluate(() => window.__resetTap && window.__resetTap());
  return page.evaluate(async () => {
    await new Promise((res) => setTimeout(res, 900));
    const frames = window.__frames || 0;
    const rms = frames ? Math.sqrt(window.__rmsAccum / frames) : 0;
    return { frames, rms, nonZero: window.__nonZero || 0, isPlaying: typeof isPlaying === "function" ? isPlaying() : null };
  });
})();
console.log(`[RECOVERY after Synth → EQ] rms=${recovery.rms.toFixed(5)} nonZero=${recovery.nonZero} isPlaying=${recovery.isPlaying}`);

await page.evaluate(() => { try { stopPlay(); } catch (_) {} });
await browser.close();
server.close();

if (pageErrors.length) console.log("page errors:", pageErrors.slice(0, 10));

// Assertions: on EVERY tab audio must stay audible; the master-reading tabs
// must report analyzer signal (the user-reported bug zeroed all of them).
const RMS_MIN = 1e-3;
const ANALYZER_MIN = 1e-3;
// A structurally-dropped audio path (the user-reported bug — "audio can't be
// heard on any tab") emits all-zero buffers, so the tap sees ~0 non-zero
// samples. The software-GL Synth-tab starvation instead emits MANY small-but-
// non-zero samples (observed ~3e5 nonZero at ~2e-4 RMS). This floor cleanly
// separates the two: it catches a true structural drop while tolerating the
// env-induced amplitude collapse on the Synth tab.
const PATH_ALIVE_NONZERO_MIN = 8192;
const failures = [];
if (pageErrors.length > 0) failures.push(`page raised ${pageErrors.length} error(s): ${pageErrors[0]}`);
for (const r of results) {
  if (r.isPlaying !== true) failures.push(`[${r.tab}] not playing (isPlaying=${r.isPlaying})`);
  if (r.softwareGLStarves) {
    // Don't apply the RMS floor: the Synth tab's draw cost starves WebAudio
    // ~100x under software GL (see TABS comment). Instead assert the audio path
    // is structurally ALIVE while the tab renders — the real bug would zero it.
    if (r.nonZero < PATH_ALIVE_NONZERO_MIN) {
      failures.push(`[${r.tab}] AUDIO PATH DEAD during tab (nonZero=${r.nonZero}, expected >= ${PATH_ALIVE_NONZERO_MIN}) — output structurally silent, not just starved`);
    }
  } else if (r.rms < RMS_MIN) {
    failures.push(`[${r.tab}] AUDIO SILENT during tab (rms=${r.rms.toFixed(6)}, expected >= ${RMS_MIN})`);
  }
  if (r.readsMaster && r.analyzerPeak < ANALYZER_MIN) {
    failures.push(`[${r.tab}] NO ANALYZER DATA (peak=${Number(r.analyzerPeak).toFixed(6)}, expected >= ${ANALYZER_MIN}) — master analyzer tap dropped`);
  }
}
// After leaving the Synth tab, master output MUST recover above the floor —
// proves the Synth-tab drop was reversible software-GL starvation, not the
// user-reported permanent kill that survives tab switches.
if (recovery.isPlaying !== true) failures.push(`[recovery] not playing after Synth → EQ (isPlaying=${recovery.isPlaying})`);
if (recovery.rms < RMS_MIN) failures.push(`[recovery] AUDIO DID NOT RECOVER after leaving Synth (rms=${recovery.rms.toFixed(6)}, expected >= ${RMS_MIN}) — visiting Synth permanently killed audio`);

if (failures.length > 0) {
  console.error("full_game_tab_audio browser test FAILED:");
  for (const f of failures) console.error("  - " + f);
  process.exit(1);
}
console.log(`full_game_tab_audio browser test PASSED across ${results.length} tabs`);
