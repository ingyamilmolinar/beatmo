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

function buildPlaytest() {
  const build = spawnSync(
    GO,
    ["build", "-o", path.join(jsDir, "play_ui.wasm"), "./internal/ui/playtest"],
    { cwd: goDir, env: { ...process.env, GOOS: "js", GOARCH: "wasm" }, stdio: "inherit" }
  );
  if (build.status !== 0) throw new Error("go build play_ui failed");
}

function serve() {
  const port = 8510 + Math.floor(Math.random() * 1000);
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
  return new Promise((resolve) => server.listen(port, () => resolve({ port, server })));
}

async function measureNodeHighlightDuration(page) {
  // Pick first node among the 1x1 rectangle coords
  const coords = [[0,0],[1,0],[1,1],[0,1]];
  let pick = null;
  for (const [i,j] of coords) {
    const id = await page.evaluate(([i,j]) => (typeof nodeIdAt === 'function' ? nodeIdAt(i,j) : -1), [i,j]);
    if (id >= 0) { pick = [i,j]; break; }
  }
  if (!pick) throw new Error('no node found');
  const [ni, nj] = pick;

  // Wait for rising edge
  let start = 0;
  for (let t = 0; t < 200; t++) {
    await page.waitForTimeout(10);
    await page.evaluate(() => { if (typeof forceDraw === 'function') forceDraw(); });
    const on = await page.evaluate(([i,j]) => nodeHighlightedAt && nodeHighlightedAt(i,j), [ni,nj]);
    if (on) { start = performance.now(); break; }
  }
  if (!start) throw new Error('highlight did not start');
  // Wait for falling edge
  for (let t = 0; t < 400; t++) {
    await page.waitForTimeout(10);
    await page.evaluate(() => { if (typeof forceDraw === 'function') forceDraw(); });
    const on = await page.evaluate(([i,j]) => nodeHighlightedAt && nodeHighlightedAt(i,j), [ni,nj]);
    if (!on) { return performance.now() - start; }
  }
  throw new Error('highlight did not end');
}

function expectedAudioMs(inst, pitch, dur) {
  const base = window.sampleDurationSec ? window.sampleDurationSec(inst) : 0;
  const r = Math.pow(2, (pitch||0)/12) / (dur > 0 ? dur : 1);
  return (base > 0 ? (base / r) * 1000 : 0);
}

buildPlaytest();
const { port, server } = await serve();

const browser = await chromium.launch({ args: ["--autoplay-policy=no-user-gesture-required"] });
const page = await browser.newPage();
await page.goto(`http://localhost:${port}/`);
await page.waitForFunction(() => window.__ready);
await page.evaluate(() => resumeAudio && resumeAudio());

// Regular draw mode
await page.evaluate(() => { if (typeof setSimpleDraw === 'function') setSimpleDraw(false); buildPerfRect(1, 1); setBPM(120); });
// Trigger a single event explicitly to isolate duration
const params = { pitch: 0, duration: 1 };
await page.evaluate(() => triggerOnce && triggerOnce(0,0,0,1));
const durMs = await measureNodeHighlightDuration(page);
const expMs = await page.evaluate(({pitch, duration}) => {
  const base = (typeof sampleDurationSec === 'function') ? sampleDurationSec('snare') : 0;
  const r = Math.pow(2, (pitch||0)/12) / (duration > 0 ? duration : 1);
  return base > 0 ? (base / r) * 1000 : 0;
}, params);
if (!expMs || Math.abs(durMs - expMs) > Math.max(80, expMs * 0.2)) {
  throw new Error(`regular: highlight ${durMs.toFixed(1)}ms != audio ${expMs.toFixed(1)}ms`);
}

// Simple draw mode
await page.reload();
await page.waitForFunction(() => window.__ready);
await page.evaluate(() => resumeAudio && resumeAudio());
await page.evaluate(() => { if (typeof setSimpleDraw === 'function') setSimpleDraw(true); buildPerfRect(1, 1); setBPM(120); });
await page.evaluate(() => triggerOnce && triggerOnce(0,0,0,1));
const durMs2 = await measureNodeHighlightDuration(page);
const expMs2 = await page.evaluate(({pitch, duration}) => {
  const base = (typeof sampleDurationSec === 'function') ? sampleDurationSec('snare') : 0;
  const r = Math.pow(2, (pitch||0)/12) / (duration > 0 ? duration : 1);
  return base > 0 ? (base / r) * 1000 : 0;
}, { pitch: 0, duration: 1 });
if (!expMs2 || Math.abs(durMs2 - expMs2) > Math.max(80, expMs2 * 0.2)) {
  throw new Error(`simple: highlight ${durMs2.toFixed(1)}ms != audio ${expMs2.toFixed(1)}ms`);
}

await browser.close();
server.close();
