import { chromium } from "playwright";
import { spawnSync } from "child_process";
import http from "http";
import fs from "fs";
import path from "path";
import { fileURLToPath } from "url";
import { flushCoverage, isCoverageEnabled } from "./coverage_helpers.js";
import { waitForAudioReady } from "./browser_test_helpers.js";
import { setupFullWasm } from "./real_input_test_helpers.js";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const jsDir = __dirname;

// A page without Go has no instruments: the block is read once from the real app and the C reference rendered from
// the SAME block.
let MODULAR;
{
  const app = await setupFullWasm({ logLevel: "ERROR" });
  try {
    MODULAR = await app.page.evaluate(() => {
      const s = window.__instrumentBlockForTest("modular");
      return s && { block: Array.from(s.block), seconds: s.seconds, amp: s.amp, melodic: s.melodic };
    });
  } finally {
    await app.cleanup();
  }
  if (!MODULAR) throw new Error("the app pushed no block for modular");
}

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
await waitForAudioReady(page);
await page.evaluate((s) => window.setInstrumentBlock("modular", Float64Array.from(s.block), s.seconds, s.amp, s.melodic), MODULAR);

// Trigger user gesture to create the AudioContext (audio.js defers creation until a gesture).
await page.evaluate(() => document.dispatchEvent(new Event('pointerdown')));
await page.waitForFunction(() => window.__audioCtx && window.__audioCtx.state === 'running', {}, { timeout: 5000 });

// Now play via the high-level API and capture its buffer.
await page.evaluate(() => { window.__samples = []; window.__done = false; window.__captureSamples = true; });
// Every family renders through render_modular, the only C renderer, so the base modular voice fills both direct-render
// slots; the families' blocks are covered by the xplat audio-compare suite.
await page.evaluate(() => window.playSound('modular', 1.0));
await page.waitForFunction(() => window.__done === true, {}, { timeout: 10000 });
await page.waitForFunction(() => window.__renderMeta && window.__renderMeta['modular'], {}, { timeout: 5000 });
const { out, meta } = await page.evaluate(() => ({
  out: window.__samples.slice(),
  meta: window.__renderMeta?.['modular'] ?? null
}));
if (!meta) throw new Error('missing render meta for modular');
const ref = await page.evaluate(async ({ frames, sr, block }) => {
  const factory = window.__drumsFactory;
  if (!factory) throw new Error('no drumsFactory');
  const m = await factory();
  const ptr = m._malloc(frames * 4);
  // The instrument's own block — exactly what audio.js renders for it.
  const pp = m._malloc(block.length * 8);
  m.HEAPF64.set(block, pp >> 3);
  m.ccall('render_modular_p', null, ['number','number','number','number'], [ptr, sr, frames, pp]);
  m._free(pp);
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
}, { frames: meta.frames, sr: meta.sr, block: MODULAR.block });
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

// A second modular render: deterministic, so it correlates too.
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
  const cowRef = await page.evaluate(async ({ frames, sr, block }) => {
    const factory = window.__drumsFactory;
    if (typeof factory !== 'function') throw new Error('no drumsFactory');
    const m = await factory();
    const ptr = m._malloc(frames * 4);
    // The instrument's own block — exactly what audio.js renders for it.
    const pp = m._malloc(block.length * 8);
    m.HEAPF64.set(block, pp >> 3);
    m.ccall('render_modular_p', null, ['number','number','number','number'], [ptr, sr, frames, pp]);
    m._free(pp);
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
  }, { frames: cowMeta.frames, sr: cowMeta.sr, block: MODULAR.block });
  const cowAligned = alignOutput(cowRef, cowOut, 'modular#2');
  const cc = corr(cowRef, cowAligned);
  if (cc < 0.98) throw new Error(`miniaudio JS render mismatch (modular#2): corr=${cc.toFixed(4)}`);
  console.log('drums.js consistency verified (modular#2)', { corr: cc.toFixed(4) });
})();

if (isCoverageEnabled()) await flushCoverage(page, new URL("../../coverage/browser-raw", import.meta.url).pathname, "drums_consistency");
await browser.close();
server.close();
