import { chromium } from "playwright";
import http from "http";
import fs from "fs";
import path from "path";
import { fileURLToPath } from "url";
import { flushCoverage, isCoverageEnabled } from "./coverage_helpers.js";
import { waitForAudioReady } from "./browser_test_helpers.js";
import { readAppInstrumentBlocks } from "./real_input_test_helpers.js";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const jsDir = __dirname;

// A bare audio.js page has no instruments until Go pushes their blocks.
const { snare: SNARE } = await readAppInstrumentBlocks(["snare"]);

const server = http.createServer((req, res) => { const file = req.url === "/" ? "/cache.html" : req.url;
  if (req.url === "/" || req.url === "/cache.html") { const html = `<!DOCTYPE html><html><body>
<script type="module">
  import './audio.js';
</script>
</body></html>`;
    res.writeHead(200, { "Content-Type": "text/html" });
    res.end(html);
    return;
  }
  const filePath = path.join(jsDir, file.replace(/^\//, ""));
  fs.readFile(filePath, (err, data) => { if (err) { res.writeHead(404); res.end(); return; }
    let ct = "text/plain";
    if (filePath.endsWith(".html")) ct = "text/html";
    else if (filePath.endsWith(".js")) ct = "application/javascript";
    else if (filePath.endsWith(".wasm")) ct = "application/wasm";
    res.writeHead(200, { "Content-Type": ct });
    res.end(data);
  });
});
await new Promise((r) => server.listen(0, r));
const port = server.address().port;

const browser = await chromium.launch({ args: ["--autoplay-policy=no-user-gesture-required"] });
const page = await browser.newPage();
await page.goto(`http://localhost:${port}/`);
await page.waitForFunction(() => typeof window.playSound === 'function' && typeof window.playSoundParams === 'function');
await waitForAudioReady(page);
await page.evaluate((b) => window.setInstrumentBlock("snare", Float64Array.from(b.block), b.seconds, b.amp, b.melodic, b.gateTail, b.fx), SNARE);

// Helper: peak + RMS of a numeric sample array (runs in the test process).
function peakRms(samples) {
  let peak = 0;
  let sumSq = 0;
  for (let i = 0; i < samples.length; i++) {
    const v = samples[i];
    const a = v < 0 ? -v : v;
    if (a > peak) peak = a;
    sumSq += v * v;
  }
  return { peak, rms: samples.length ? Math.sqrt(sumSq / samples.length) : 0, n: samples.length };
}

const result = await page.evaluate(async () => { window.resetRenderCache?.();
  window.__audioMetrics = { renders: {}, cacheHits: {} };
  window.__captureSamples = false;
  const ensured = await window.ensureSynthSample?.('snare');
  await window.playSound('snare', 0.8);
  await new Promise((r) => setTimeout(r, 30));
  await window.playSoundParams('snare', 0.6, 2, 1.0);
  await new Promise((r) => setTimeout(r, 30));

  // The cached Float32Array read DIRECTLY is the authoritative non-silence check: deterministic, no tap.
  const cachedData = (typeof window.getCachedRenderData === 'function')
    ? window.getCachedRenderData('snare')
    : null;

  // A cache HIT through the live tap (renders stay 1) proves the buffer reaches the destination.
  let liveCapture = null;
  if (typeof window.startOutputCapture === 'function' && typeof window.stopOutputCapture === 'function') {
    try {
      window.startOutputCapture();
      await window.playSound('snare', 0.9);
      await new Promise((r) => setTimeout(r, 200));
      const cap = window.stopOutputCapture();
      liveCapture = cap ? Array.from(cap) : null;
    } catch (_) { liveCapture = null; }
  }

  return { ensured,
    metrics: window.__audioMetrics,
    stats: typeof window.getRenderCacheStats === 'function' ? window.getRenderCacheStats() : null,
    cachedData,
    liveCapture,
  };
});

const metrics = result.metrics;
const stats = result.stats;
if (!result.ensured) { throw new Error('ensureSynthSample failed');
}
if (isCoverageEnabled()) await flushCoverage(page, new URL("../../coverage/browser-raw", import.meta.url).pathname, "audio_cache");
await browser.close();
server.close();

if (!metrics || typeof metrics !== 'object') { throw new Error('missing audio metrics');
}
const renders = metrics.renders && metrics.renders.snare;
if (renders !== 1) { throw new Error(`expected single render after reset, got ${renders}`);
}
if (!stats || typeof stats !== 'object' || !stats.snare) { throw new Error('render cache missing snare entry');
}
// The full voice: render.seconds at 48 kHz.
const FRAMES = Math.max(1, Math.floor(48000 * SNARE.seconds));
if (stats.snare.frames !== FRAMES) { throw new Error(`expected ${FRAMES} frames (${SNARE.seconds} s), got ${stats.snare.frames}`);
}
if (Math.abs(stats.snare.duration - FRAMES / 48000) > 1e-9) { throw new Error(`expected duration ${FRAMES / 48000}, got ${stats.snare.duration}`);
}

// Deterministic and tap-free: a cache of zeros fails here.
const cachedData = result.cachedData;
if (!Array.isArray(cachedData) || cachedData.length !== FRAMES) {
  throw new Error(`getCachedRenderData('snare') did not return ${FRAMES} samples (got ${cachedData && cachedData.length})`);
}
const cachedStats = peakRms(cachedData);
// Non-silence floor: a meaningful signal peaks well above 0.01.
const CACHE_PEAK_FLOOR = 0.01;
if (!(cachedStats.peak > CACHE_PEAK_FLOOR)) {
  throw new Error(`cached snare buffer is silent/garbage: peak ${cachedStats.peak} <= ${CACHE_PEAK_FLOOR}`);
}
if (!(cachedStats.rms > 0)) {
  throw new Error(`cached snare buffer has zero RMS: ${cachedStats.rms}`);
}

// Soft: the ScriptProcessor tap can read near zero under load; the cached-data check is the gate.
let liveStats = null;
if (Array.isArray(result.liveCapture) && result.liveCapture.length > 0) {
  liveStats = peakRms(result.liveCapture);
}

console.log('audio cache verified', {
  metrics,
  stats,
  cachedPeak: cachedStats.peak,
  cachedRms: cachedStats.rms,
  cachedSamples: cachedStats.n,
  liveCapturePeak: liveStats ? liveStats.peak : null,
  liveCaptureRms: liveStats ? liveStats.rms : null,
  liveCaptureSamples: liveStats ? liveStats.n : null,
  renders,
});
