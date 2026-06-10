/**
 * full_game_playback_audio.browser.test.js
 *
 * REGRESSION GUARD for "the game produces no audio during playback."
 *
 * Every other audio test exercises a NARROWER path: webaudio_smoke drives the
 * `playtest.wasm` auto-player, e2e_transport checks play/stop STATE (not sound),
 * audio_panel_render checks the analyzer tabs paint. NONE of them loaded the
 * production `index.html` + `main.wasm` full game, pressed Play through the
 * sequencer, and verified that real audio leaves the AudioContext destination.
 *
 * This test closes that gap: it is a 100%-production reproduction (createServer
 * serves index.html verbatim) that taps the AudioContext destination, calls
 * startPlay(), and asserts non-silent output. If the WASM↔WebAudio render/
 * schedule wiring ever goes dead while Go unit tests stay green, THIS fails.
 *
 * COVERED-BY-GO: internal/audio/mixer_signal_test.go proves the mixer itself
 * produces non-zero RMS; this owns the browser end-to-end sequencer→output path
 * that Go cannot reach.
 */
import { chromium } from "playwright";
import { buildMainWasm, createServer } from "./real_input_test_helpers.js";
import { isCoverageEnabled, flushCoverage } from "./browser_test_helpers.js";

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

// Tap the AudioContext destination BEFORE the page creates it, so we measure
// the real output the user would hear. Mirrors webaudio_smoke's TestAC tap.
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
await page.waitForFunction(() => typeof startPlay === "function", { timeout: 30000 });
await page.waitForFunction(() => window.audioReady !== undefined, { timeout: 30000 });

// Unlock audio with a real user gesture (WebAudio requires one), let warmup
// settle, then start sequencer playback through the production transport.
await page.mouse.click(200, 200);
await page.evaluate(async () => { try { await window.audioReady; } catch (_) {} });
await page.evaluate(() => { try { document.dispatchEvent(new Event("mousedown")); } catch (_) {} });
await page.evaluate(() => startPlay());

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
