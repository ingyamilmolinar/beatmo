import { chromium } from "playwright";
import { spawnSync } from "child_process";
import http from "http";
import fs from "fs";
import path from "path";
import { fileURLToPath } from "url";
import { flushCoverage, isCoverageEnabled } from "./coverage_helpers.js";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const jsDir = __dirname;

// Ensure Playwright's Chromium is installed only if missing.
const chromiumPath = path.join(jsDir, "node_modules", ".cache", "ms-playwright", "chromium");
if (!fs.existsSync(chromiumPath)) { spawnSync("npx", ["playwright", "install", "chromium"], { cwd: jsDir, stdio: "inherit" });
}

const server = http.createServer((req, res) => { try { console.log('[SRV]', req.url); } catch(_) {}
  const file = req.url === "/" ? "/consistency.html" : req.url;
  if (req.url === "/" || req.url === "/consistency.html") { const html = `<!DOCTYPE html><html><body>
<script type="module">
  import drumsFactory from './drums.single.js';
  window.__drumsFactory = drumsFactory;
</script>
<script type="module" src="audio.js"></script>
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
page.on('console', (msg) => { try { console.log('[PAGE]', msg.type(), msg.text()); } catch(_) {} });

// Intercept WebAudio to capture the output of playSound.
await page.addInitScript(() => { window.__samples = [];
  const RealAC = window.AudioContext || window.webkitAudioContext;
  const SAMPLE_TARGET = 120000;
  class TestAC extends RealAC { constructor(opts) { super(opts);
      const dest = super.destination;
      const sp = this.createScriptProcessor(256, 1, 1);
      sp.addEventListener("audioprocess", (e) => { const data = e.inputBuffer.getChannelData(0);
        window.__samples.push(...data);
        if (window.__samples.length >= SAMPLE_TARGET) { window.__done = true;
        }
      });
      sp.connect(dest);
      Object.defineProperty(this, "destination", { value: sp });
    }
  }
  window.AudioContext = TestAC;
  window.webkitAudioContext = TestAC;
});

await page.goto(`http://localhost:${port}/`);

// Wait for audio.js to load and initialize synth samples.
await page.waitForFunction(() => typeof window.playSound === 'function', {}, { timeout: 15000 });
await page.evaluate(() => window.audioReady);

// Trigger user gesture to create the AudioContext (audio.js defers creation until a gesture).
await page.evaluate(() => document.dispatchEvent(new Event('pointerdown')));
await page.waitForFunction(() => window.__audioCtx && window.__audioCtx.state === 'running', {}, { timeout: 5000 });

// Now play via the high-level API and capture its buffer.
await page.evaluate(() => { window.__samples = []; window.__done = false; window.__captureSamples = true; });
// EVERY legacy family migrated to the modular engine — kick/tom/snare/cymbal/bass
// through Phase-6, and FM in Phase-7 (the LAST). render_hihat / render_fm_bass etc.
// are all deleted; those WebAudio paths now flow through render_modular (their
// param-block parity is covered by the xplat audio-compare suite). The ONLY
// remaining bespoke C renderer is render_modular itself, so the base `modular`
// instrument takes both direct-render JS↔C consistency slots: a default render
// here and a second modular render below. This still exercises the playSound →
// renderToCache → WebAudio path against a direct C render reference.
await page.evaluate(() => window.playSound('modular', 1.0));
await page.waitForFunction(() => window.__done === true, {}, { timeout: 10000 });
await page.waitForFunction(() => window.__renderMeta && window.__renderMeta['modular'], {}, { timeout: 5000 });
const { out, meta } = await page.evaluate(() => ({
  out: window.__samples.slice(),
  meta: window.__renderMeta?.['modular'] ?? null
}));
if (!meta) throw new Error('missing render meta for modular');
const ref = await page.evaluate(async ({ frames, sr }) => {
  const factory = window.__drumsFactory;
  if (!factory) throw new Error('no drumsFactory');
  const m = await factory();
  const ptr = m._malloc(frames * 4);
  m.ccall('render_modular', null, ['number','number','number'], [ptr, sr, frames]);
  const data = new Float32Array(m.HEAPF32.buffer, ptr, frames).slice();
  m._free(ptr);
  let peak = 0;
  for (let i = 0; i < data.length; i++) {
    const a = Math.abs(data[i]);
    if (a > peak) peak = a;
  }
  if (peak > 0) {
    const inv = 1 / peak;
    for (let i = 0; i < data.length; i++) data[i] *= inv;
  }
  return Array.from(data);
}, { frames: meta.frames, sr: meta.sr });
// Compare by correlation; require near-identical shape.
const corr = (x, y) => { const n = Math.min(x.length, y.length);
  let sx = 0, sy = 0, sxx = 0, syy = 0, sxy = 0;
  for (let i = 0; i < n; i++) { const a = x[i], b = y[i]; sx += a; sy += b; sxx += a*a; syy += b*b; sxy += a*b; }
  const num = n * sxy - sx * sy;
  const den = Math.sqrt((n*sxx - sx*sx) * (n*syy - sy*sy)) || 1;
  return Math.abs(num / den);
};

const trimLeadingSilence = (arr, threshold = 1e-4) => { if (!Array.isArray(arr)) return [];
  let idx = 0;
  while (idx < arr.length && Math.abs(arr[idx]) <= threshold) idx++;
  return arr.slice(idx);
};

const alignOutput = (refArr, outArr, label) => { const trimmed = trimLeadingSilence(outArr);
  if (trimmed.length < refArr.length) { throw new Error(`captured buffer too short (${label}): got ${trimmed.length}, need ${refArr.length}`);
  }
  return trimmed.slice(0, refArr.length);
};

const outAligned = alignOutput(ref, out, 'modular');
const c = corr(ref, outAligned);
if (c < 0.98) { throw new Error(`miniaudio JS render mismatch (modular): corr=${c.toFixed(4)}`);
}
console.log('drums.js consistency verified (modular)', { corr: c.toFixed(4) });

// Repeat with a second modular render to keep two direct-render JS↔C consistency
// checks. EVERY legacy family (incl. FM, Phase-7) migrated to the modular engine;
// their render_X C exports were deleted — those WebAudio paths now flow through
// render_modular, whose seeded param blocks are covered by the xplat audio-compare
// suite. render_modular is the only remaining bespoke C renderer, so it serves
// both consistency slots (deterministic, so the second render correlates too).
await (async () => {
  await page.evaluate(() => { window.__samples = []; window.__done = false; window.__captureSamples = true; });
  await page.evaluate(() => window.playSound('modular', 1.0));
  await page.waitForFunction(() => window.__done === true, {}, { timeout: 10000 });
  await page.waitForFunction(() => window.__renderMeta && window.__renderMeta['modular'], {}, { timeout: 5000 });
  const { out: cowOut, meta: cowMeta } = await page.evaluate(() => ({
    out: window.__samples.slice(),
    meta: window.__renderMeta?.['modular'] ?? null
  }));
  if (!cowMeta) throw new Error('missing render meta for modular (second check)');
  const cowRef = await page.evaluate(async ({ frames, sr }) => {
    const factory = window.__drumsFactory;
    if (typeof factory !== 'function') throw new Error('no drumsFactory');
    const m = await factory();
    const ptr = m._malloc(frames * 4);
    m.ccall('render_modular', null, ['number','number','number'], [ptr, sr, frames]);
    const data = new Float32Array(m.HEAPF32.buffer, ptr, frames).slice();
    m._free(ptr);
    let peak = 0;
    for (let i = 0; i < data.length; i++) {
      const a = Math.abs(data[i]);
      if (a > peak) peak = a;
    }
    if (peak > 0) {
      const inv = 1 / peak;
      for (let i = 0; i < data.length; i++) data[i] *= inv;
    }
    return Array.from(data);
  }, { frames: cowMeta.frames, sr: cowMeta.sr });
  const cowAligned = alignOutput(cowRef, cowOut, 'modular#2');
  const cc = corr(cowRef, cowAligned);
  if (cc < 0.98) throw new Error(`miniaudio JS render mismatch (modular#2): corr=${cc.toFixed(4)}`);
  console.log('drums.js consistency verified (modular#2)', { corr: cc.toFixed(4) });
})();

if (isCoverageEnabled()) await flushCoverage(page, new URL("../../coverage/browser-raw", import.meta.url).pathname, "drums_consistency");
await browser.close();
server.close();
