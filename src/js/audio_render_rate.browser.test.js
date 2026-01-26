import { chromium } from "playwright";
import http from "http";
import fs from "fs";
import path from "path";
import { fileURLToPath } from "url";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const jsDir = __dirname;

// Serve audio.js (and drums.single.js) directly from the repo.
const port = 8375 + Math.floor(Math.random() * 500);
const server = http.createServer((req, res) => { const file = req.url === "/" ? "/rate.html" : req.url;
  if (req.url === "/" || req.url === "/rate.html") { const html = `<!doctype html><html><body>
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
await new Promise((r) => server.listen(port, r));

const browser = await chromium.launch({ args: ["--autoplay-policy=no-user-gesture-required"] });
const page = await browser.newPage();
await page.goto(`http://localhost:${port}/`);
await page.waitForFunction(() => typeof window.ensureSynthSample === 'function' && typeof window.resetRenderCache === 'function');

const result = await page.evaluate(async () => { // Force a fresh render at the current AudioContext sample rate.
  window.resetRenderCache();
  window.__renderMeta = {};
  await window.ensureSynthSample('snare');
  const meta = window.__renderMeta?.snare || null;
  const stats = (typeof window.getRenderCacheStats === 'function') ? window.getRenderCacheStats() : {};
  const cache = stats.snare || null;
  const ctxSR = window.__audioCtxSR || (window.__audioCtx && window.__audioCtx.sampleRate) || null;
  return { meta, cache, ctxSR };
});

await browser.close();
server.close();

if (!result.meta) { throw new Error('missing render meta for snare');
}
if (result.ctxSR !== result.meta.sr) { throw new Error(`sample rate mismatch: ctx=${result.ctxSR}, render=${result.meta.sr}`);
}
const expectedFrames = Math.round(result.meta.sr * 1.0); // snare seconds=1.0
const tolerance = Math.max(16, Math.round(expectedFrames * 0.02)); // ±2%
if (Math.abs(result.meta.frames - expectedFrames) > tolerance) { throw new Error(`rendered frames off: got ${result.meta.frames}, expected ~${expectedFrames} (±${tolerance})`);
}
if (!result.cache || !result.cache.frames) { throw new Error('render cache missing snare entry');
}
