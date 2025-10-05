import { chromium } from "playwright";
import { spawnSync } from "child_process";
import http from "http";
import fs from "fs";
import path from "path";
import { fileURLToPath } from "url";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const jsDir = __dirname;
const goDir = path.resolve(jsDir, "../go");
const GO = process.env.GO || "go";

const build = spawnSync(
  GO,
  ["build", "-o", path.join(jsDir, "play_ui.wasm"), "./internal/ui/playtest"],
  { cwd: goDir, env: { ...process.env, GOOS: "js", GOARCH: "wasm" }, stdio: "inherit" }
);
if (build.status !== 0) throw new Error("go build play_ui failed");

const port = 8470 + Math.floor(Math.random() * 1000);
const server = http.createServer((req, res) => {
  const file = req.url === "/" ? "/ui.html" : req.url;
  if (req.url === "/" || req.url === "/ui.html") {
    const html = `<!doctype html><body>
<script type="module" src="audio.js"></script>
<script src="wasm_exec.js"></script>
<script>
  const go = new Go();
  WebAssembly.instantiateStreaming(fetch('play_ui.wasm'), go.importObject)
    .then(r => go.run(r.instance))
    .catch(err => console.error(err));
  window.__ready = new Promise(r => {
    const iv = setInterval(() => { if (typeof startPlay === 'function' && typeof buildPerfRect === 'function') { clearInterval(iv); r(true); } }, 10);
  });
</script>`;
    res.writeHead(200, { "Content-Type": "text/html" });
    res.end(html);
    return;
  }
  const filePath = path.join(jsDir, file.replace(/^\//, ""));
  fs.readFile(filePath, (err, data) => {
    if (err) { res.writeHead(404); res.end(); return; }
    let ct = "text/plain";
    if (filePath.endsWith(".html")) ct = "text/html";
    else if (filePath.endsWith(".js")) ct = "application/javascript";
    else if (filePath.endsWith(".wasm")) ct = "application/wasm";
    res.writeHead(200, { "Content-Type": ct });
    res.end(data);
  });
});
await new Promise((r) => server.listen(port, r));

let browser;
try {
  browser = await chromium.launch({ args: ["--autoplay-policy=no-user-gesture-required"] });
  const page = await browser.newPage();
  await page.goto(`http://localhost:${port}/`);
  await page.waitForFunction(() => window.__ready);
  await page.evaluate(() => { if (typeof resumeAudio === 'function') resumeAudio(); });

  // Ensure grid cache/tile are built even under simpleDraw.
  await page.evaluate(() => { if (typeof forceDraw === 'function') forceDraw(); });
  const grid = await page.evaluate(() => (typeof gridCacheInfo === 'function' ? gridCacheInfo() : null));
  if (!grid || !grid.tileReady) throw new Error('grid tile not built');

  // Build small graph and start playback with simple draw explicitly.
  await page.evaluate(() => { if (typeof setSimpleDraw === 'function') setSimpleDraw(true); buildPerfRect(1, 1); setBPM(120); startPlay(); });
  // Wait up to 2s for highlights to appear (engine-driven).
  let hasAny = false;
  for (let t = 0; t < 20 && !hasAny; t++) {
    await page.waitForTimeout(100);
    hasAny = await page.evaluate(() => {
      if (typeof hasRealtimeHighlight !== 'function') return false;
      for (let i = 0; i < 16; i++) {
        if (hasRealtimeHighlight(0, i)) return true;
      }
      return false;
    });
  }
  if (!hasAny) throw new Error('no highlights detected while playing');
} finally {
  if (browser) await browser.close();
  server.close();
}
