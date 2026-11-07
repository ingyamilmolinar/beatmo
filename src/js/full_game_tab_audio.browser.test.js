/** Play, then walk every audio-panel tab in the production page: output must stay audible and the master-reading tabs
 * must report analyzer data, with a real gesture building the master chain lazily.
 * COVERED-BY-GO: internal/ui/audio_panel_dispatcher_test.go (which analyzers each tab selects) */
import { chromium } from "playwright";
import { buildMainWasm, createServer } from "./real_input_test_helpers.js";
import { startPlaybackWhenReady } from "./audio_probe_bench_helpers.js";
import { waitForAudioReady } from "./browser_test_helpers.js";

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

// Before the page creates the context, with a resettable per-window accumulator.
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
await page.waitForFunction(() => typeof startPlay === "function", null, { timeout: 30000 });
await page.waitForFunction(() => typeof setEQTab === "function", null, { timeout: 30000 });
await page.waitForFunction(() => window.audioReady !== undefined, null, { timeout: 30000 });

// Unlock audio with a real gesture (the actual user flow), then start playback.
await page.mouse.click(200, 200);
await waitForAudioReady(page, { allowReject: true });
await page.evaluate(() => { try { document.dispatchEvent(new Event("mousedown")); } catch (_) {} });
// Play is ignored while the warm-up gate is closed.
await startPlaybackWhenReady(page);
await page.waitForTimeout(1200); // let the demo circuit warm up

// The first switch into a master tab creates every tap and rewires the master, and under software GL the first measured
// tab could read a still-cold analyzer. One real priming switch lets that settle, so each measured tab is an idempotent
// switch on a warm tap; no per-tab floor is weakened.
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

// readsMaster mirrors tabReadsMasterAnalyzer. Under software GL the Synth tab's draw starves WebAudio globally
// (output collapses and recovers when it closes), an environment artifact: Synth gets a structural-alive check plus a
// recovery check after leaving it, not the RMS floor.
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

  // A snapshot is a ~10 ms window that can land between sparse hits, so the max over the window is kept. A cold master
  // tap keeps polling up to a warm-up deadline (software GL settles slowly); a permanently flat tap, the real bug, never
  // crosses the floor and fails.
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
      // Past the base window, stop once warm (or not needed) or once the budget is spent.
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

// The real guard for Synth: a regression kills audio PERMANENTLY across tab switches, while the software-GL starvation
// lifts the moment the tab closes.
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

const RMS_MIN = 1e-3;
const ANALYZER_MIN = 1e-3;
// A dropped audio path emits all-zero buffers; the software-GL starvation emits many small non-zero samples. This
// floor separates the two.
const PATH_ALIVE_NONZERO_MIN = 8192;
const failures = [];
if (pageErrors.length > 0) failures.push(`page raised ${pageErrors.length} error(s): ${pageErrors[0]}`);
for (const r of results) {
  if (r.isPlaying !== true) failures.push(`[${r.tab}] not playing (isPlaying=${r.isPlaying})`);
  if (r.softwareGLStarves) {
    // No RMS floor here (software-GL starvation); a structurally alive path is the requirement.
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
// Recovery proves the drop was reversible starvation, not a permanent kill.
if (recovery.isPlaying !== true) failures.push(`[recovery] not playing after Synth → EQ (isPlaying=${recovery.isPlaying})`);
if (recovery.rms < RMS_MIN) failures.push(`[recovery] AUDIO DID NOT RECOVER after leaving Synth (rms=${recovery.rms.toFixed(6)}, expected >= ${RMS_MIN}) — visiting Synth permanently killed audio`);

if (failures.length > 0) {
  console.error("full_game_tab_audio browser test FAILED:");
  for (const f of failures) console.error("  - " + f);
  process.exit(1);
}
console.log(`full_game_tab_audio browser test PASSED across ${results.length} tabs`);
