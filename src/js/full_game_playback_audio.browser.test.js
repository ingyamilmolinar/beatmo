/** The production page and main.wasm, Play through the sequencer, and non-silent output at the destination: the only
 * test that proves the full sequencer → output path the narrower ones skip.
 * COVERED-BY-GO: internal/audio/mixer_signal_test.go (the mixer itself) */
import { chromium } from "playwright";
import { buildMainWasm, createServer } from "./real_input_test_helpers.js";
import { startPlaybackWhenReady } from "./audio_probe_bench_helpers.js";
import { isCoverageEnabled, flushCoverage, waitForAudioReady } from "./browser_test_helpers.js";

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

// Before the page creates the context, so this is what the user would hear.
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
await page.waitForFunction(() => window.audioReady !== undefined, null, { timeout: 30000 });

// A real gesture unlocks audio.
await page.mouse.click(200, 200);
await waitForAudioReady(page, { allowReject: true });
await page.evaluate(() => { try { document.dispatchEvent(new Event("mousedown")); } catch (_) {} });
// Play is ignored while the warm-up gate is closed.
await startPlaybackWhenReady(page);

// Let the demo circuit play several beats.
await page.waitForTimeout(2500);

const result = await page.evaluate(() => ({
  isPlaying: typeof isPlaying === "function" ? isPlaying() : null,
  frames: window.__frames || 0,
  nonZero: window.__nonZero || 0,
  rms: window.__frames ? Math.sqrt(window.__rmsAccum / window.__frames) : 0,
  acState: window.__beatmoAcState || null,
}));

if (isCoverageEnabled()) {
  await flushCoverage(page, new URL("../../coverage/browser-raw", import.meta.url).pathname, "full_game_playback_audio");
}
await browser.close();
server.close();

console.log("playback result:", JSON.stringify(result));
if (pageErrors.length) {
  console.log("page errors:", pageErrors.slice(0, 10));
}

// Assertions — the failures that "completely broken, no audio" would trip.
if (pageErrors.length > 0) {
  throw new Error(`production page raised ${pageErrors.length} error(s) — first: ${pageErrors[0]}`);
}
if (result.isPlaying !== true) {
  throw new Error(`startPlay() did not enter playing state (isPlaying=${result.isPlaying})`);
}
if (result.frames < 10000) {
  throw new Error(`AudioContext produced too few frames (${result.frames}) — audio graph not running`);
}
const RMS_MIN = 1e-4;
const NONZERO_MIN = 1000;
if (result.nonZero < NONZERO_MIN || result.rms < RMS_MIN) {
  throw new Error(
    `NO AUDIO during sequencer playback — nonZero=${result.nonZero} rms=${result.rms.toFixed(6)} ` +
    `(expected nonZero>=${NONZERO_MIN}, rms>=${RMS_MIN}). The full-game WASM↔WebAudio render/schedule wiring is dead.`
  );
}

console.log(`full_game_playback_audio browser test PASSED (rms=${result.rms.toFixed(4)}, nonZero=${result.nonZero})`);
