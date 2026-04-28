import { chromium } from "playwright";
import http from "http";
import fs from "fs";
import path from "path";
import { fileURLToPath } from "url";
import { flushCoverage, isCoverageEnabled } from "./coverage_helpers.js";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const jsDir = __dirname;

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
await page.evaluate(async () => await window.audioReady);

const result = await page.evaluate(async () => { window.resetRenderCache?.();
  window.__audioMetrics = { renders: {}, cacheHits: {} };
  window.__captureSamples = false;
  const ensured = await window.ensureSynthSample?.('snare');
  await window.playSound('snare', 0.8);
  await new Promise((r) => setTimeout(r, 30));
  await window.playSoundParams('snare', 0.6, 2, 1.0);
  await new Promise((r) => setTimeout(r, 30));
  return { ensured,
    metrics: window.__audioMetrics,
    stats: typeof window.getRenderCacheStats === 'function' ? window.getRenderCacheStats() : null,
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

console.log('audio cache verified', { metrics, stats });
