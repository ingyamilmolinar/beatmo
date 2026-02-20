import { chromium } from "playwright";
import http from "http";
import fs from "fs";
import path from "path";
import { fileURLToPath } from "url";
import { flushCoverage, isCoverageEnabled } from "./coverage_helpers.js";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const jsDir = __dirname;

const server = http.createServer((req, res) => { const file = req.url === "/" ? "/bus.html" : req.url;
  if (req.url === "/" || req.url === "/bus.html") { const html = `<!DOCTYPE html><html><body>
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
await page.goto(`http://localhost:${port}/`);
await page.waitForFunction(() => typeof window.playSound === 'function' && typeof window.__audioBusCountById === 'function');

// Fill buses with many distinct volumes to create > MAX entries for an instrument (snare).
await page.evaluate(async () => { const vols = [];
  for (let i = 0; i <= 20; i++) vols.push(i / 20); // > VOL_Q+ cap
  for (const v of vols) { await window.playSound('snare', v);
  }
});

// Let buses age
await page.waitForTimeout(2200);

// Trigger eviction by requesting a new bus
await page.evaluate(async () => { await window.playSound('snare', 0.33);
});

const perId = await page.evaluate(() => window.__audioBusCountById('snare'));
if (isCoverageEnabled()) await flushCoverage(page, new URL("../../coverage/browser-raw", import.meta.url).pathname, "bus_lru");
await browser.close();
server.close();

if (perId > 16) { throw new Error(`expected per-id bus count <= 16, got ${perId}`);
}
console.log('bus LRU ok (snare):', perId);

